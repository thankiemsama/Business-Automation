package service

import (
	"encoding/base64"
	"net/http"
	"strings"

	"businessautomation/internal/core"
)

const maximumMessageBytes = 16000

type ConversationService struct {
	repo          core.Repository
	clock         Clock
	ids           IDGenerator
	replies       ReplyProvider
	draftProvider ReplyProvider
	automations   AutomationRunner
	mailer        *EmailMailer
	channels      *ChannelManager
}

func NewConversations(repo core.Repository, clock Clock, ids IDGenerator, replies ReplyProvider, automations AutomationRunner) *ConversationService {
	return &ConversationService{repo: repo, clock: clock, ids: ids, replies: replies, draftProvider: replies, automations: automations}
}

// Message atomically records an inbound message and executes enabled keyword rules.
func (s *ConversationService) Message(user core.User, conversationID, content, sender string) (core.Record, error) {
	return s.MessageWithAttachment(user, conversationID, content, sender, "", "")
}

func (s *ConversationService) MessageWithAttachment(user core.User, conversationID, content, sender, name, attachment string) (core.Record, error) {
	return s.SendMessage(user, conversationID, content, sender, name, attachment, "")
}

func (s *ConversationService) SendMessage(user core.User, conversationID, content, sender, name, attachment, sourceMessageID string) (core.Record, error) {
	var result core.Record
	var recipient string
	if err := validateAttachment(name, attachment); err != nil {
		return result, err
	}
	content = strings.TrimSpace(content)
	if (content == "" && attachment == "") || len(content) > maximumMessageBytes {
		return result, core.Fail(400, "Tin nhắn phải có 1–16000 byte")
	}
	if sender != core.SenderCustomer && sender != core.SenderAgent && sender != "note" {
		return result, core.Fail(400, "Vai trò gửi không hợp lệ")
	}
	if sourceMessageID != "" && (sender != core.SenderAgent || len(sourceMessageID) > 512) {
		return result, core.Fail(400, "Tin nguồn không hợp lệ")
	}
	err := updateAsTenant(s.repo, user, func(store core.Writer) error {
		conversation, err := findOwnedRecord(store, user, conversationID, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		if sourceMessageID != "" {
			if _, err := checkDraftContext(store, user, conversationID, sourceMessageID); err != nil {
				return err
			}
		}
		if conversation.Channel == core.ChannelEmail && conversation.Email != "" && sender == core.SenderAgent {
			if s.mailer == nil {
				return core.Fail(503, "Chưa cấu hình gửi email")
			}
			recipient = conversation.Email
		}
		external := sender == core.SenderAgent && isLiveChannel(conversation.Channel)
		if external && (s.channels == nil || attachment != "") {
			return core.Fail(400, "Kênh này hiện hỗ trợ gửi tin văn bản")
		}
		result = s.storeMessage(store, user, conversation, content, sender)
		if external {
			if err := s.channels.enqueueReply(store, conversation, result); err != nil {
				return err
			}
			result.Status = "pending"
		}
		if attachment != "" {
			result.AttachmentName, result.AttachmentData = name, attachment
		}
		if recipient != "" {
			result.Status = "pending"
			result.Email = recipient
			result.Subject = conversation.Subject
			inbound := latestInboundBefore(store, conversation, result)
			result.EmailInReplyTo, result.EmailReferences = inbound.EmailInReplyTo, inbound.EmailReferences
		}
		if attachment != "" || recipient != "" || external {
			store.PutRecord(result)
		}
		if sender == core.SenderCustomer {
			conversation.UpdatedAt = result.UpdatedAt
			return s.automations.Run(store, user, conversation, result)
		}

		return nil
	})
	return result, err
}

type Reply struct {
	Message         core.Record `json:"message"`
	SourceID        string      `json:"source_id,omitempty"`
	SourceMessageID string      `json:"source_message_id,omitempty"`
	Provider        string      `json:"provider"`
	NeedsHuman      bool        `json:"needs_human"`
}

func (s *ConversationService) Reply(user core.User, conversationID string) (Reply, error) {
	var result Reply
	err := s.repo.Update(func(store core.Writer) error {
		conversation, err := findOwnedRecord(store, user, conversationID, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		if isLiveChannel(conversation.Channel) {
			return core.Fail(409, "Hãy dùng Soạn nháp hoặc gửi tin từ hộp thư của kênh")
		}
		var latest core.Record
		for _, message := range store.ConversationMessages(user.TenantID, conversationID) {
			if laterMessage(message, latest) {
				latest = message
			}
		}
		if latest.ID == "" || latest.Sender != core.SenderCustomer {
			return core.Fail(409, "Cần tin nhắn mới từ khách hàng")
		}
		answer, err := s.replies.Answer(latest.Content, store.Records(user.TenantID, core.KindKnowledge))
		if err != nil {
			return err
		}
		result = Reply{
			SourceID:        answer.SourceID,
			SourceMessageID: latest.ID,
			NeedsHuman:      answer.NeedsHuman,
			Provider:        answer.Provider,
			Message:         s.storeMessage(store, user, conversation, answer.Content, core.SenderAssistant),
		}
		return nil
	})
	return result, err
}

// Sequence makes message ordering deterministic even when a clock returns equal timestamps.
func nextSequence(store core.Reader, user core.User, conversationID string) uint64 {
	var highestSequence uint64
	for _, message := range store.ConversationMessages(user.TenantID, conversationID) {
		highestSequence = max(highestSequence, message.Sequence)
	}
	return highestSequence + 1
}

func laterMessage(a, b core.Record) bool {
	if a.Sequence != b.Sequence {
		return a.Sequence > b.Sequence
	}
	if a.CreatedAt.Equal(b.CreatedAt) {
		return a.ID > b.ID
	}
	return a.CreatedAt.After(b.CreatedAt)
}

func (s *ConversationService) storeMessage(store core.Writer, user core.User, conversation core.Record, content, sender string) core.Record {
	now := s.clock.Now().UTC()
	message := core.Record{
		ID:             s.ids.NewID(),
		TenantID:       user.TenantID,
		Kind:           core.KindMessages,
		ConversationID: conversation.ID,
		Content:        content,
		Sender:         sender,
		AuthorID:       user.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
		Sequence:       nextSequence(store, user, conversation.ID),
	}
	store.PutRecord(message)
	if sender == core.SenderAgent && !isLiveChannel(conversation.Channel) && (conversation.Channel != core.ChannelEmail || conversation.Email == "") {
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			if ticket.ConversationID == conversation.ID && ticket.Status != core.StatusResolved && ticket.FirstResponseAt == nil {
				recordTicketResponse(store, &ticket, user.ID, now)
			}
		}
	}
	conversation.UpdatedAt = now
	store.PutRecord(conversation)
	return message
}

func validateAttachment(name, attachment string) error {
	if name != "" || attachment != "" {
		data, err := base64.StdEncoding.DecodeString(attachment)
		mime := http.DetectContentType(data)
		if err != nil || len(data) == 0 || len(data) > 32*1024 || len(name) == 0 || len(name) > 200 ||
			strings.ContainsAny(name, "/\\\r\n") || (mime != "image/png" && mime != "image/jpeg" && mime != "application/pdf") {
			return core.Fail(400, "Tệp phải là PNG, JPEG hoặc PDF, tối đa 32 KiB")
		}
	}
	return nil
}
