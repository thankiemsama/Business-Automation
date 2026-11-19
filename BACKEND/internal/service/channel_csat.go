package service

import (
	"strings"
	"time"

	"businessautomation/internal/core"
)

func (m *ChannelManager) queueChannelSurvey(store core.Writer, conversation core.Record) {
	now := m.conversations.clock.Now().UTC()
	message := core.Record{ID: m.conversations.ids.NewID(), TenantID: conversation.TenantID, Kind: core.KindMessages, ConversationID: conversation.ID,
		Sender: core.SenderAssistant, Content: "Bạn hài lòng với hỗ trợ ở mức nào? Hãy trả lời CSAT 1 đến CSAT 5, với 5 là rất hài lòng.",
		Status: "pending", Sequence: nextSequence(store, core.User{TenantID: conversation.TenantID}, conversation.ID), CreatedAt: now, UpdatedAt: now}
	if err := m.enqueueReply(store, conversation, message); err != nil {
		return
	}
	store.PutRecord(message)
	store.PutRecord(core.Record{ID: tokenHash("channel-survey:" + conversation.ID), TenantID: conversation.TenantID, Kind: "channel_surveys",
		MessageID: message.ID, ConversationID: conversation.ID, CreatedAt: now})
}

func (m *ChannelManager) acceptChannelRating(store core.Writer, conversation core.Record, inbound inboundChannelMessage, now time.Time) bool {
	content := strings.ToUpper(inbound.text)
	if inbound.attachment != "" || len(content) != len("CSAT 5") || !strings.HasPrefix(content, "CSAT ") {
		return false
	}
	rating := int(content[5]) - '0'
	if rating < 1 || rating > 5 {
		return false
	}
	survey, ok := store.Record(conversation.TenantID, tokenHash("channel-survey:"+conversation.ID), "channel_surveys")
	if !ok {
		return false
	}
	sent, ok := store.Record(conversation.TenantID, survey.MessageID, core.KindMessages)
	if !ok || sent.Status != "sent" || now.After(survey.CreatedAt.Add(48*time.Hour)) {
		return false
	}
	if conversation.Rating == 0 {
		conversation.Rating, conversation.RatedAt = rating, &now
		store.PutRecord(conversation)
	}
	return true
}
