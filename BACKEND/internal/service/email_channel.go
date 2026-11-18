package service

import (
	"net/mail"
	"strings"
	"time"

	"businessautomation/internal/core"
)

type InboundEmail struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Text     string `json:"text"`
}

func (s *ConversationService) ConfigureEmail(mailer *EmailMailer) { s.mailer = mailer }

func (s *ConversationService) IngestEmail(tenant string, in InboundEmail) (core.Record, error) {
	var result core.Record
	address, err := mail.ParseAddress(in.From)
	in.Text = strings.TrimSpace(in.Text)
	if err != nil || len(address.Address) > 254 || len(in.ID) == 0 || len(in.ID) > 512 || len(in.ThreadID) > 512 || len(in.Subject) > 400 || len(in.Text) == 0 || len(in.Text) > maximumMessageBytes {
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
		if marker, ok := store.Record(tenant, eventID, "email_events"); ok {
			result, _ = store.Record(tenant, marker.Name, core.KindMessages)
			return nil
		}
		now := s.clock.Now().UTC()
		user := core.User{TenantID: tenant}
		threadID := tokenHash("email-thread:" + tenant + ":" + email + ":" + in.ThreadID)
		thread, _ := store.Record(tenant, threadID, "email_threads")
		conversation, ok := store.Record(tenant, thread.ConversationID, core.KindConversations)
		if !ok || conversation.Status == core.StatusClosed {
			name := address.Name
			if name == "" || len(name) > 200 {
				name = email
			}
			subject := strings.TrimSpace(in.Subject)
			if subject == "" {
				subject = "Email hỗ trợ"
			}
			customer := core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindCustomers, Name: name, Email: email, CreatedAt: now, UpdatedAt: now}
			conversation = core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindConversations, CustomerID: customer.ID, Email: email, Channel: core.ChannelEmail, Subject: subject, Status: core.StatusOpen, CreatedAt: now, UpdatedAt: now}
			store.PutRecord(customer)
			store.PutRecord(conversation)
			store.PutRecord(core.Record{ID: threadID, TenantID: tenant, Kind: "email_threads", ConversationID: conversation.ID})
		}
		result = s.storeMessage(store, user, conversation, in.Text, core.SenderCustomer)
		store.PutRecord(core.Record{ID: eventID, TenantID: tenant, Kind: "email_events", Name: result.ID})
		return s.automations.Run(store, user, conversation, result)
	})
	return result, err
}

func (s *ConversationService) DeliverEmail(user core.User, conversationID, messageID string) (core.Record, error) {
	var message core.Record
	err := s.repo.View(func(store core.Reader) error {
		var err error
		message, err = findOwnedRecord(store, user, messageID, core.KindMessages)
		if err != nil || message.ConversationID != conversationID || message.Email == "" || message.Sender != core.SenderAgent {
			return core.Fail(404, "Không tìm thấy email")
		}
		if message.Status == "pending" && s.clock.Now().Sub(message.CreatedAt) >= 23*time.Hour {
			return core.Fail(409, "Đã hết thời hạn gửi lại an toàn")
		}
		return nil
	})
	if err != nil || message.Status == "sent" {
		return message, err
	}
	if err := s.mailer.Send(message.Email, "Re: "+message.Subject, message.Content, message.ID, message.AttachmentName, message.AttachmentData); err != nil {
		return message, err
	}
	err = s.repo.Update(func(store core.Writer) error {
		current, err := findOwnedRecord(store, user, messageID, core.KindMessages)
		if err != nil {
			return err
		}
		if current.Status == "sent" {
			message = current
			return nil
		}
		current.Status = "sent"
		now := s.clock.Now().UTC()
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			if ticket.ConversationID == conversationID && ticket.Status != core.StatusResolved && ticket.FirstResponseAt == nil {
				recordTicketResponse(store, &ticket, current.AuthorID, now)
			}
		}
		store.PutRecord(current)
		message = current
		return nil
	})
	return message, err
}
