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
			if !featureEnabled(store, tenant, "ai") || !s.autoChat && !agentPresetEnabled(store, tenant, "inbox_handoff") {
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
			sessions := store.Records(tenant, "chat_sessions")
			if s.channels != nil {
				sessions = append(sessions, store.Records(tenant, "channel_threads")...)
			}
			for _, session := range sessions {
				candidate, ok := store.Record(tenant, session.ConversationID, core.KindConversations)
				current := latest[candidate.ID]
				if !ok || !botChannel(candidate.Channel) || candidate.Status != core.StatusOpen || candidate.AssigneeID != "" || activeTickets[candidate.ID] || current.Sender != core.SenderCustomer || current.AutomationID == "lead_campaign_command" || !current.CreatedAt.After(recent) || (current.Content == "" && current.AttachmentName == "" && current.AttachmentData == "") {
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
	knowledgeGap := false
	if message.Sentiment != "angry" && message.AttachmentData == "" && message.AttachmentName == "" {
		generated, err := s.model.answer(conversation.TenantID, message.Content, articles)
		knowledgeGap = err == nil && generated.knowledgeGap && !generalPublicIntent(message.Content)
		_, lowerBound := s.chatCalibration.probability(generated.Confidence)
		if err == nil && validSentiment(generated.Sentiment) && generated.Sentiment != "angry" &&
			s.chatThreshold >= .7 && lowerBound >= s.chatThreshold &&
			(generated.SourceID != "" || generalPublicIntent(message.Content)) {
			answer = generated
		}
	}
	_ = s.repo.Update(func(store core.Writer) error {
		tenant := conversation.TenantID
		if !featureEnabled(store, tenant, "ai") || !s.autoChat && !agentPresetEnabled(store, tenant, "inbox_handoff") {
			return nil
		}
		fresh, ok := store.Record(tenant, conversation.ID, core.KindConversations)
		if !ok || fresh.Status != core.StatusOpen || fresh.AssigneeID != "" || !botChannel(fresh.Channel) {
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
		currentMessage, found := store.Record(tenant, message.ID, core.KindMessages)
		if !found || currentMessage.Content != message.Content {
			return nil
		}
		if currentMessage.Sentiment == "angry" || currentMessage.AttachmentData != "" || currentMessage.AttachmentName != "" {
			answer.NeedsHuman = true
		}
		now := s.clock.Now().UTC()
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
		marker := core.Record{ID: key, TenantID: tenant, Kind: "ai_bot_markers", ConversationID: fresh.ID, CreatedAt: now}
		actor := core.User{TenantID: tenant}
		content := answer.Content
		if answer.SourceID != "" {
			content += "\n\nNếu vấn đề đã được xử lý, hãy trả lời ‘Đã giải quyết’."
		}
		reply := core.Record{ID: s.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindMessages, ConversationID: fresh.ID,
			Sender: core.SenderAssistant, Content: content, Sequence: nextSequence(store, actor, fresh.ID), CreatedAt: now, UpdatedAt: now}
		if !answer.NeedsHuman && isLiveChannel(fresh.Channel) {
			if s.channels == nil || s.channels.enqueueReply(store, fresh, reply) != nil {
				answer.NeedsHuman = true
			} else {
				reply.Status = "pending"
			}
		}
		if answer.NeedsHuman {
			marker.Status = "handoff"
			if knowledgeGap && knowledgeDraftEnabled(store, tenant) {
				marker.Name, marker.MessageID = "knowledge_gap", message.ID
			}
			botHandoff(store, s.repo.IDs, fresh, message, now)
		} else {
			marker.Status = "answered"
			store.PutRecord(reply)
			if answer.SourceID != "" {
				store.PutRecord(core.Record{ID: tokenHash("bot-answer:" + reply.ID), TenantID: tenant, Kind: "ai_bot_answers",
					CreatedAt: now, ConversationID: fresh.ID, MessageID: message.ID})
			}
			fresh.UpdatedAt = now
			store.PutRecord(fresh)
		}
		store.PutRecord(marker)
		return nil
	})
}

func latestInboundBefore(store core.Reader, conversation, reply core.Record) core.Record {
	var latest core.Record
	for _, message := range store.ConversationMessages(conversation.TenantID, conversation.ID) {
		if message.Sender == core.SenderCustomer && laterMessage(reply, message) && laterMessage(message, latest) {
			latest = message
		}
	}
	return latest
}

func botHandoff(store core.Writer, ids IDGenerator, conversation, inbound core.Record, now time.Time) {
	for _, ticket := range store.Records(conversation.TenantID, core.KindTickets) {
		if ticket.ConversationID == conversation.ID && ticket.Status != core.StatusResolved {
			return
		}
	}
	ticket := core.Record{ID: ids.NewID(), TenantID: conversation.TenantID, Kind: core.KindTickets, ConversationID: conversation.ID,
		CustomerID: conversation.CustomerID, AssigneeID: conversation.AssigneeID, Subject: "Cần nhân viên hỗ trợ: " + conversation.Subject,
		Content: inbound.Content, Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: now, UpdatedAt: now}
	response, resolution := ticketDeadlines(ticket)
	ticket.ResponseDueAt, ticket.ResolutionDueAt = &response, &resolution
	store.PutRecord(ticket)
	if workspace, ok := store.(*workspaceWriter); ok {
		workspace.notify(conversation, "ai_handoff", "Hội thoại cần nhân viên hỗ trợ")
	}
}

func botChannel(channel string) bool {
	return channel == core.ChannelWeb || isLiveChannel(channel)
}

func confirmBotResolution(store core.Writer, conversation, previous, customer core.Record) bool {
	if normalizeDiacritics(strings.Trim(strings.TrimSpace(customer.Content), ".!")) != "da giai quyet" ||
		previous.Sender != core.SenderAssistant || conversation.AssigneeID != "" ||
		(isLiveChannel(conversation.Channel) && previous.Status != "sent") {
		return false
	}
	if _, ok := store.Record(conversation.TenantID, tokenHash("bot-answer:"+previous.ID), "ai_bot_answers"); !ok {
		return false
	}
	avoidedTicket := true
	for _, ticket := range store.Records(conversation.TenantID, core.KindTickets) {
		if ticket.ConversationID == conversation.ID {
			if ticket.Status != core.StatusResolved {
				return false
			}
			avoidedTicket = false
		}
	}
	conversation.Status, conversation.UpdatedAt = core.StatusClosed, customer.CreatedAt
	store.PutRecord(conversation)
	store.PutRecord(core.Record{ID: tokenHash("bot-resolved:" + conversation.ID), TenantID: conversation.TenantID, Kind: "ai_bot_resolutions",
		ConversationID: conversation.ID, Channel: conversation.Channel, Enabled: avoidedTicket, CreatedAt: customer.CreatedAt})
	return true
}
