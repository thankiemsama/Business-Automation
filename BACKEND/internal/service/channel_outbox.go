package service

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"businessautomation/internal/core"
)

const channelOutbox = "channel_outbox"

func isLiveChannel(channel string) bool {
	return channel == core.ChannelZalo || channel == core.ChannelWhatsApp
}

func validChannelText(channel, content string) bool {
	limit := 2000
	if channel == core.ChannelWhatsApp {
		limit = 4096
	}
	count := utf8.RuneCountInString(content)
	return count > 0 && count <= limit
}

func (m *ChannelManager) enqueueReply(store core.Writer, conversation, message core.Record) error {
	thread, ok := store.Record(conversation.TenantID, tokenHash("channel-thread:"+conversation.ID), "channel_threads")
	if !ok || thread.Channel != conversation.Channel {
		return core.Fail(409, "Hội thoại chưa có địa chỉ nhận trên kênh")
	}
	return m.enqueue(store, conversation.TenantID, conversation.Channel, thread.Content, thread.Name, message.Content, tokenHash("channel-send:"+message.ID), conversation.ID, message.ID)
}

func (m *ChannelManager) enqueue(store core.Writer, tenant, channel, expectedAccount, to, content, id, conversation, messageID string) error {
	_, account, _, err := m.providerAccount(store, tenant, channel)
	if err != nil {
		return err
	}
	if expectedAccount != "" && account.AccountID != expectedAccount {
		return core.Fail(409, "Tài khoản kênh đã thay đổi")
	}
	if !channelID.MatchString(to) || !validChannelText(channel, content) {
		return core.Fail(400, "ID người nhận hoặc độ dài tin nhắn không hợp lệ")
	}
	now := m.conversations.clock.Now().UTC()
	store.PutRecord(core.Record{ID: id, TenantID: tenant, Kind: channelOutbox, Channel: channel, Name: to, Subject: account.AccountID,
		Content: content, ConversationID: conversation, MessageID: messageID, Status: "pending", CreatedAt: now, UpdatedAt: now, EmailRetryAt: &now})
	return nil
}

func (m *ChannelManager) TestDelivery(user core.User, channel, to, content string) (core.Record, error) {
	if user.Role != core.RoleOwner {
		return core.Record{}, core.Fail(403, "Chỉ owner gửi tin kiểm thử")
	}
	if m.conversations == nil {
		return core.Record{}, core.Fail(503, "Kênh chưa sẵn sàng")
	}
	id := m.conversations.ids.NewID()
	var job core.Record
	err := updateTenant(m.repo, user.TenantID, func(store core.Writer) error {
		if err := m.enqueue(store, user.TenantID, channel, "", strings.TrimSpace(to), strings.TrimSpace(content), id, "", ""); err != nil {
			return err
		}
		job, _ = store.Record(user.TenantID, id, channelOutbox)
		return nil
	})
	return publicChannelDelivery(job), err
}

func publicChannelDelivery(job core.Record) core.Record {
	job.Content, job.Subject, job.Name = "", "", ""
	return job
}

func (m *ChannelManager) Deliveries(user core.User, channel string) ([]core.Record, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner xem hàng đợi kênh")
	}
	items := []core.Record{}
	err := viewTenant(m.repo, user.TenantID, func(store core.Reader) error {
		for _, job := range store.Records(user.TenantID, channelOutbox) {
			if job.Channel == channel && (job.Status != "sent" || job.ConversationID == "") {
				items = append(items, publicChannelDelivery(job))
			}
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool {
		if (items[i].Status == "failed") != (items[j].Status == "failed") {
			return items[i].Status == "failed"
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items[:min(len(items), 100)], err
}

func (m *ChannelManager) Retry(user core.User, channel, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner thử gửi lại")
	}
	return updateTenant(m.repo, user.TenantID, func(store core.Writer) error {
		job, ok := store.Record(user.TenantID, id, channelOutbox)
		if !ok || job.Channel != channel {
			return core.Fail(404, "Không tìm thấy lượt gửi")
		}
		if job.Status != "failed" {
			return core.Fail(409, "Chỉ có thể thử lại lượt gửi đã vào DLQ")
		}
		_, account, _, err := m.providerAccount(store, user.TenantID, channel)
		if err != nil {
			return err
		}
		if account.AccountID != job.Subject {
			return core.Fail(409, "Tài khoản kênh đã thay đổi")
		}
		now := m.conversations.clock.Now().UTC()
		job.Status, job.EmailAttempts, job.EmailLastError, job.EmailRetryAt, job.UpdatedAt = "pending", 0, "", &now, now
		m.saveDelivery(store, job, now)
		return nil
	})
}

func (m *ChannelManager) Run(ctx context.Context) {
	runAIWork(ctx, func() { m.processChannelOutbox(ctx) })
}

func (m *ChannelManager) processChannelOutbox(ctx context.Context) {
	if m.conversations == nil {
		return
	}
	now := m.conversations.clock.Now().UTC()
	var due []core.Record
	if err := m.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, job := range store.Records(tenant, channelOutbox) {
				if job.EmailRetryAt != nil && !job.EmailRetryAt.After(now) && (job.Status == "sending" || job.Status == "pending") {
					due = append(due, job)
				}
			}
		}
		return nil
	}); err != nil {
		return
	}
	sort.Slice(due, func(i, j int) bool { return due[i].EmailRetryAt.Before(*due[j].EmailRetryAt) })
	for _, job := range due[:min(12, len(due))] {
		if ctx.Err() != nil {
			return
		}
		m.processDelivery(ctx, job)
	}
}

func (m *ChannelManager) processDelivery(ctx context.Context, candidate core.Record) {
	var provider *liveChannel
	var account channelAccount
	var credentials channelCredentials
	var job core.Record
	var claimed bool
	err := updateTenant(m.repo, candidate.TenantID, func(store core.Writer) error {
		job, _ = store.Record(candidate.TenantID, candidate.ID, channelOutbox)
		now := m.conversations.clock.Now().UTC()
		if job.EmailRetryAt == nil || job.EmailRetryAt.After(now) {
			return nil
		}
		if job.Status == "sending" {
			// A provider may have accepted the request before the worker stopped.
			job.Status, job.EmailLastError, job.EmailRetryAt = "failed", "delivery_unknown", nil
			m.saveDelivery(store, job, now)
			return nil
		}
		if job.Status != "pending" {
			return nil
		}
		var err error
		provider, account, credentials, err = m.providerAccount(store, job.TenantID, job.Channel)
		if err != nil || account.AccountID != job.Subject {
			job.Status, job.EmailLastError, job.EmailRetryAt = "failed", "channel_unavailable", nil
			m.saveDelivery(store, job, now)
			return nil
		}
		lease := now.Add(time.Minute)
		job.Status, job.EmailRetryAt, job.UpdatedAt = "sending", &lease, now
		job.EmailAttempts++
		store.PutRecord(job)
		claimed = true
		return nil
	})
	if err != nil || !claimed {
		return
	}
	sent := provider.send(ctx, account, credentials, job.Name, job.Content, job.ID)
	_ = updateTenant(m.repo, job.TenantID, func(store core.Writer) error {
		current, ok := store.Record(job.TenantID, job.ID, channelOutbox)
		if !ok || current.Status != "sending" || !sameNullableTime(current.EmailRetryAt, job.EmailRetryAt) {
			return nil
		}
		now := m.conversations.clock.Now().UTC()
		current.EmailRetryAt, current.EmailLastError, current.EmailProviderID = nil, sent.failure, sent.receipt
		switch {
		case sent.receipt != "":
			current.Status = "sent"
		case sent.retry && current.EmailAttempts < 5:
			retryAt := now.Add(time.Minute * (1 << (current.EmailAttempts - 1)))
			current.Status, current.EmailRetryAt = "pending", &retryAt
		default:
			current.Status = "failed"
		}
		m.saveDelivery(store, current, now)
		return nil
	})
}

func (m *ChannelManager) saveDelivery(store core.Writer, job core.Record, now time.Time) {
	job.UpdatedAt = now
	store.PutRecord(job)
	if message, ok := store.Record(job.TenantID, job.MessageID, core.KindMessages); ok && message.ConversationID == job.ConversationID {
		previousStatus := message.Status
		message.Status, message.EmailLastError = job.Status, job.EmailLastError
		message.EmailProviderID = job.EmailProviderID
		store.PutRecord(message)
		if job.Status == "failed" && previousStatus != "failed" {
			if conversation, ok := store.Record(job.TenantID, message.ConversationID, core.KindConversations); ok {
				if workspace, ok := store.(*workspaceWriter); ok {
					workspace.notify(conversation, "channel_delivery_failed", "Tin nhắn kênh cần đối soát")
				}
				if message.Sender == core.SenderAssistant && conversation.Status != core.StatusClosed {
					inbound := latestInboundBefore(store, conversation, message)
					if inbound.ID != "" {
						botHandoff(store, m.conversations.ids, conversation, inbound, now)
					}
				}
			}
		}
		if job.Status == "sent" && message.Sender == core.SenderAgent {
			for _, ticket := range store.Records(job.TenantID, core.KindTickets) {
				if ticket.ConversationID == message.ConversationID && ticket.Status != core.StatusResolved && ticket.FirstResponseAt == nil {
					recordTicketResponse(store, &ticket, message.AuthorID, now)
				}
			}
		}
	}
}
