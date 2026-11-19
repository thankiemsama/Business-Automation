package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"businessautomation/internal/core"
)

type InboundEmail struct {
	MessageID  string `json:"-"`
	DeliveryID string `json:"-"`
	ID         string `json:"id"`
	ThreadID   string `json:"thread_id"`
	From       string `json:"from"`
	Subject    string `json:"subject"`
	Text       string `json:"text"`
}

func (s *ConversationService) ConfigureEmail(mailer *EmailMailer) { s.mailer = mailer }

func (s *ConversationService) IngestEmail(tenant string, in InboundEmail) (core.Record, error) {
	var result core.Record
	address, err := mail.ParseAddress(in.From)
	in.Text = strings.TrimSpace(in.Text)
	if err != nil || len(address.Address) > 254 || len(in.ID) == 0 || len(in.ID) > 512 || len(in.DeliveryID) > 512 || len(in.ThreadID) > 512 || len(in.Subject) > 400 || len(in.Text) == 0 || len(in.Text) > maximumMessageBytes {
		return result, core.Fail(400, "Email không hợp lệ")
	}
	email := strings.ToLower(address.Address)
	if in.ThreadID == "" {
		in.ThreadID = in.ID
	}
	err = s.repo.Update(func(store core.Writer) error {
		if len(store.Users(tenant)) == 0 {
			return core.Fail(404, "Không tìm thấy hộp hỗ trợ")
		}
		eventID := tokenHash("email-event:" + tenant + ":" + in.ID)
		delivery := ""
		if in.DeliveryID != "" {
			delivery = tokenHash("email-delivery:" + tenant + ":" + in.DeliveryID)
			if marker, ok := store.Record(tenant, delivery, "email_events"); ok {
				result, _ = store.Record(tenant, marker.Name, core.KindMessages)
				return nil
			}
		}
		if marker, ok := store.Record(tenant, eventID, "email_events"); ok {
			if delivery != "" {
				store.PutRecord(core.Record{ID: delivery, TenantID: tenant, Kind: "email_events", Name: marker.Name})
			}
			result, _ = store.Record(tenant, marker.Name, core.KindMessages)
			return nil
		}
		now := s.clock.Now().UTC()
		user := core.User{TenantID: tenant}
		threadID := tokenHash("email-thread:" + tenant + ":" + email + ":" + in.ThreadID)
		thread, _ := store.Record(tenant, threadID, "email_threads")
		conversation, ok := store.Record(tenant, thread.ConversationID, core.KindConversations)
		if !ok || conversation.Status == core.StatusClosed {
			// Identify the customer by email so replies from the same sender
			// across new threads attach to one profile instead of fragments.
			customer, found := findCustomerByEmail(store, tenant, email)
			if !found {
				name := address.Name
				if name == "" || len(name) > 200 {
					name = email
				}
				customer = core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindCustomers, Name: name, Email: email, CreatedAt: now, UpdatedAt: now}
				store.PutRecord(customer)
			} else if address.Name != "" && len(address.Name) <= 200 && strings.EqualFold(customer.Name, email) {
				customer.Name = address.Name
				customer.UpdatedAt = now
				store.PutRecord(customer)
			}
			subject := strings.TrimSpace(in.Subject)
			if subject == "" {
				subject = "Email hỗ trợ"
			}
			conversation = core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindConversations, CustomerID: customer.ID, Email: email, Channel: core.ChannelEmail, Subject: subject, Status: core.StatusOpen, CreatedAt: now, UpdatedAt: now}
			store.PutRecord(conversation)
			store.PutRecord(core.Record{ID: threadID, TenantID: tenant, Kind: "email_threads", ConversationID: conversation.ID})
		}
		result = s.storeMessage(store, user, conversation, in.Text, core.SenderCustomer)
		if in.MessageID != "" {
			result.EmailInReplyTo, result.EmailReferences = in.MessageID, emailMessageID(in.ThreadID)
			store.PutRecord(result)
		}
		store.PutRecord(core.Record{ID: eventID, TenantID: tenant, Kind: "email_events", Name: result.ID})
		if delivery != "" {
			store.PutRecord(core.Record{ID: delivery, TenantID: tenant, Kind: "email_events", Name: result.ID})
		}
		return s.automations.Run(store, user, conversation, result)
	})
	return result, err
}

const (
	emailRetryWindow = 23 * time.Hour
	emailClaimLease  = time.Minute
)

func (s *ConversationService) RunEmailReplies(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		s.processEmailReplies(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *ConversationService) processEmailReplies(ctx context.Context) {
	var due []core.Record
	if err := s.repo.View(func(store core.Reader) error {
		due = store.DueEmailReplies(s.clock.Now(), 8)
		return nil
	}); err != nil {
		slog.Error("email reply queue read failed", "error", err)
		return
	}
	for _, message := range due {
		if ctx.Err() != nil {
			return
		}
		user := core.User{TenantID: message.TenantID}
		if _, err := s.DeliverEmail(ctx, user, message.ConversationID, message.ID); err != nil {
			slog.Error("email reply processing failed", "tenant", user.TenantID, "message", message.ID, "error", err)
		}
	}
}

// A claim is committed before sending. Both the worker and the manual endpoint use
// the same lease and provider key, including after a process dies mid-request.
func (s *ConversationService) DeliverEmail(ctx context.Context, user core.User, conversationID, messageID string) (core.Record, error) {
	var message core.Record
	var claimed, expired bool
	err := s.repo.Update(func(store core.Writer) error {
		var err error
		message, err = findOwnedRecord(store, user, messageID, core.KindMessages)
		if err != nil || message.ConversationID != conversationID || message.Email == "" || message.Sender != core.SenderAgent {
			return core.Fail(404, "Không tìm thấy email")
		}
		now := s.clock.Now().UTC()
		if message.Status != "pending" || (message.EmailRetryAt != nil && message.EmailRetryAt.After(now)) {
			return nil
		}
		if !now.Before(message.CreatedAt.Add(emailRetryWindow)) {
			expired = true
			message.Status = "failed"
			message.EmailRetryAt = nil
			if message.EmailLastError == "" {
				message.EmailLastError = "retry_window_expired"
			}
		} else {
			claimed = true
			message.EmailAttempts++
			lease := now.Add(emailClaimLease)
			message.EmailRetryAt = &lease
		}
		store.PutRecord(message)
		return nil
	})
	if err != nil {
		return message, err
	}
	if expired {
		logEmailReplyFailure(message, "retry_window_expired")
	}
	if !claimed {
		return message, nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	receipt, status, sendErr := s.mailer.send(sendCtx, message.Email, "Re: "+message.Subject, message.Content,
		message.ID, message.AttachmentName, message.AttachmentData, message.EmailInReplyTo, message.EmailReferences)
	cancel()

	failure := ""
	switch {
	case s.mailer == nil:
		failure = "mailer_unconfigured"
	case sendErr != nil && status == 0:
		failure = "provider_unreachable"
	case sendErr != nil:
		failure = fmt.Sprintf("provider_http_%d", status)
	case receipt == "":
		failure = "provider_receipt_missing"
	}

	finalized := false
	err = s.repo.Update(func(store core.Writer) error {
		current, err := findOwnedRecord(store, user, messageID, core.KindMessages)
		if err != nil {
			return err
		}
		if current.Status != "pending" || current.EmailAttempts != message.EmailAttempts {
			message = current
			return nil
		}
		now := s.clock.Now().UTC()
		current.EmailRetryAt = nil
		current.EmailLastError = failure
		if failure == "" {
			current.Status = "sent"
			current.EmailProviderID = receipt
			for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
				if ticket.ConversationID == conversationID && ticket.Status != core.StatusResolved && ticket.FirstResponseAt == nil {
					recordTicketResponse(store, &ticket, current.AuthorID, now)
				}
			}
		} else {
			deadline := current.CreatedAt.Add(emailRetryWindow)
			if !now.Before(deadline) {
				current.Status = "failed"
			} else {
				delay := 30 * time.Second * (1 << min(current.EmailAttempts-1, 6))
				retryAt := now.Add(delay)
				if deadline.Before(retryAt) {
					retryAt = deadline
				}
				current.EmailRetryAt = &retryAt
			}
		}
		store.PutRecord(current)
		message = current
		finalized = true
		return nil
	})
	if err == nil && finalized && failure != "" {
		if message.Status == "failed" {
			logEmailReplyFailure(message, failure)
		} else {
			slog.Warn("email reply retry scheduled", "tenant", message.TenantID, "message", message.ID,
				"attempts", message.EmailAttempts, "reason", failure, "retry_at", message.EmailRetryAt)
		}
	}
	return message, err
}

func logEmailReplyFailure(message core.Record, reason string) {
	slog.Error("email reply requires reconciliation", "tenant", message.TenantID, "message", message.ID,
		"attempts", message.EmailAttempts, "reason", reason, "last_error", message.EmailLastError)
}
