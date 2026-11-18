package service

import (
	"strings"
	"time"

	"businessautomation/internal/core"
)

func generalPublicIntent(value string) bool {
	value = normalizeDiacritics(strings.Trim(strings.TrimSpace(value), "!.?"))
	switch value {
	case "xin chao", "chao", "hello", "hi", "cam on", "thanks", "toi can ho tro", "can ho tro":
		return true
	}
	return false
}

func (s *AIService) runChatbot() {
	var conversation, message core.Record
	var articles []core.Record
	recent := s.clock.Now().Add(-20 * time.Minute)
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			if !featureEnabled(store, tenant, "ai") {
				continue
			}
			messages := store.Records(tenant, core.KindMessages)
			latest := map[string]core.Record{}
			for _, msg := range messages {
				if msg.Sender != "note" && laterMessage(msg, latest[msg.ConversationID]) {
					latest[msg.ConversationID] = msg
				}
			}
			activeTickets := map[string]bool{}
			for _, ticket := range store.Records(tenant, core.KindTickets) {
				if ticket.Status != core.StatusResolved {
					activeTickets[ticket.ConversationID] = true
				}
			}
			for _, session := range store.Records(tenant, "chat_sessions") {
				candidate, ok := store.Record(tenant, session.ConversationID, core.KindConversations)
				current := latest[candidate.ID]
				if !ok || candidate.Channel != core.ChannelWeb || candidate.Status != core.StatusOpen || candidate.AssigneeID != "" || activeTickets[candidate.ID] || current.Sender != core.SenderCustomer || !current.CreatedAt.After(recent) || current.Content == "" {
					continue
				}
				if _, done := store.Record(tenant, tokenHash("chatbot:"+current.ID), "ai_bot_markers"); done {
					continue
				}
				conversation, message = candidate, current
				for _, article := range store.Records(tenant, core.KindKnowledge) {
					if article.Public {
						articles = append(articles, article)
					}
				}
				return nil
			}
		}
		return nil
	})
	if message.ID == "" {
		return
	}
	answer := Answer{NeedsHuman: true}
	if message.Sentiment != "angry" && message.AttachmentData == "" {
		generated, err := s.model.Answer(message.Content, articles)
		if err == nil && (generated.SourceID != "" || generalPublicIntent(message.Content)) {
			answer = generated
		}
	}
	_ = s.repo.Update(func(store core.Writer) error {
		tenant := conversation.TenantID
		fresh, ok := store.Record(tenant, conversation.ID, core.KindConversations)
		if !ok || fresh.Status != core.StatusOpen || fresh.AssigneeID != "" || fresh.Channel != core.ChannelWeb {
			return nil
		}
		for _, candidate := range store.Records(tenant, core.KindMessages) {
			if candidate.ConversationID == fresh.ID && candidate.Sender != "note" && laterMessage(candidate, message) {
				return nil
			}
		}
		for _, ticket := range store.Records(tenant, core.KindTickets) {
			if ticket.ConversationID == fresh.ID && ticket.Status != core.StatusResolved {
				return nil
			}
		}
		key := tokenHash("chatbot:" + message.ID)
		if _, exists := store.Record(tenant, key, "ai_bot_markers"); exists {
			return nil
		}
		now := s.clock.Now().UTC()
		store.PutRecord(core.Record{ID: key, TenantID: tenant, Kind: "ai_bot_markers", CreatedAt: now})
		if answer.SourceID != "" {
			current, available := store.Record(tenant, answer.SourceID, core.KindKnowledge)
			if !available || !current.Public || !current.Enabled {
				answer.NeedsHuman = true
			}
			for _, article := range articles {
				if article.ID == current.ID && (article.Content != current.Content || article.Subject != current.Subject) {
					answer.NeedsHuman = true
				}
			}
		}
		if answer.NeedsHuman {
			store.PutRecord(core.Record{ID: s.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindTickets, ConversationID: fresh.ID, CustomerID: fresh.CustomerID, Subject: "Cần nhân viên hỗ trợ: " + fresh.Subject, Content: message.Content, Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: now, UpdatedAt: now})
			store.(*workspaceWriter).notify(fresh, "ai_handoff", "Hội thoại cần nhân viên hỗ trợ")
		} else {
			actor := core.User{TenantID: tenant}
			store.PutRecord(core.Record{ID: s.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindMessages, ConversationID: fresh.ID, Sender: core.SenderAssistant, Content: answer.Content, Sequence: nextSequence(store, actor, fresh.ID), CreatedAt: now, UpdatedAt: now})
			fresh.UpdatedAt = now
			store.PutRecord(fresh)
		}
		return nil
	})
}
