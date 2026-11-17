package service

import (
	"strings"

	"businessautomation/internal/core"
)

const maximumMessageBytes = 16000

type ConversationService struct {
	repo        core.Repository
	clock       Clock
	ids         IDGenerator
	replies     ReplyProvider
	automations AutomationRunner
}

func NewConversations(repo core.Repository, clock Clock, ids IDGenerator, replies ReplyProvider, automations AutomationRunner) *ConversationService {
	return &ConversationService{repo: repo, clock: clock, ids: ids, replies: replies, automations: automations}
}

// Message atomically records an inbound message and executes enabled keyword rules.
func (s *ConversationService) Message(user core.User, conversationID, content, sender string) (core.Record, error) {
	var result core.Record
	content = strings.TrimSpace(content)
	if content == "" || len(content) > maximumMessageBytes {
		return result, core.Fail(400, "Tin nhắn phải có 1–16000 byte")
	}
	if sender != core.SenderCustomer && sender != core.SenderAgent {
		return result, core.Fail(400, "Sender phải là customer hoặc agent")
	}
	err := s.repo.Update(func(store core.Writer) error {
		conversation, err := findOwnedRecord(store, user, conversationID, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		result = s.storeMessage(store, user, conversation, content, sender)
		if sender == core.SenderCustomer {
			conversation.UpdatedAt = result.UpdatedAt
			return s.automations.Run(store, user, conversation, result)
		}

		return nil
	})
	return result, err
}

type Reply struct {
	Message    core.Record `json:"message"`
	SourceID   string      `json:"source_id,omitempty"`
	Provider   string      `json:"provider"`
	NeedsHuman bool        `json:"needs_human"`
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
		var latest core.Record
		for _, message := range store.Records(user.TenantID, core.KindMessages) {
			if message.TenantID == user.TenantID && message.Kind == core.KindMessages && message.ConversationID == conversationID && laterMessage(message, latest) {
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
			SourceID:   answer.SourceID,
			NeedsHuman: answer.NeedsHuman,
			Provider:   answer.Provider,
			Message:    s.storeMessage(store, user, conversation, answer.Content, core.SenderAssistant),
		}
		return nil
	})
	return result, err
}

// Sequence makes message ordering deterministic even when a clock returns equal timestamps.
func nextSequence(store core.Reader, user core.User, conversationID string) uint64 {
	var highestSequence uint64
	for _, message := range store.Records(user.TenantID, core.KindMessages) {
		if message.ConversationID == conversationID && message.Sequence > highestSequence {
			highestSequence = message.Sequence
		}
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
		CreatedAt:      now,
		UpdatedAt:      now,
		Sequence:       nextSequence(store, user, conversation.ID),
	}
	store.PutRecord(message)
	conversation.UpdatedAt = now
	store.PutRecord(conversation)
	return message
}
