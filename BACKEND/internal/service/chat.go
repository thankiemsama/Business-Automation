package service

import (
	"sort"
	"strings"

	"businessautomation/internal/core"
)

type ChatMessage struct {
	AttachmentName string `json:"attachment_name,omitempty"`
	AttachmentData string `json:"attachment_data,omitempty"`
	ID             string `json:"id"`
	Content        string `json:"content"`
	Sender         string `json:"sender"`
}

func (s *ConversationService) StartChat(tenant, name, content, fileName, fileData string) (string, error) {
	content, name = strings.TrimSpace(content), strings.TrimSpace(name)
	if err := validateAttachment(fileName, fileData); err != nil {
		return "", err
	}
	if len(name) == 0 || len(name) > 200 || (len(content) == 0 && fileData == "") || len(content) > maximumMessageBytes {
		return "", core.Fail(400, "Tên hoặc tin nhắn không hợp lệ")
	}
	token := s.ids.NewID()
	err := s.repo.Update(func(store core.Writer) error {
		if len(store.Users(tenant)) == 0 {
			return core.Fail(404, "Không tìm thấy hộp hỗ trợ")
		}
		now := s.clock.Now().UTC()
		user := core.User{TenantID: tenant}
		customer := core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindCustomers, Name: name, CreatedAt: now, UpdatedAt: now}
		conversation := core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindConversations, Subject: "Chat website · " + name, CustomerID: customer.ID, Channel: core.ChannelWeb, Status: core.StatusOpen, CreatedAt: now, UpdatedAt: now}
		store.PutRecord(customer)
		store.PutRecord(conversation)
		store.PutRecord(core.Record{ID: tokenHash(token), TenantID: tenant, Kind: "chat_sessions", ConversationID: conversation.ID})
		message := s.storeMessage(store, user, conversation, content, core.SenderCustomer)
		if fileData != "" {
			message.AttachmentName, message.AttachmentData = fileName, fileData
			store.PutRecord(message)
		}
		return s.automations.Run(store, user, conversation, message)
	})
	return token, err
}

func (s *ConversationService) PublicChat(token, content, fileName, fileData string, rating int) ([]ChatMessage, error) {
	result := []ChatMessage{}
	// The public token embeds a tenant identifier plus an independently generated secret.
	tenant, secret, ok := strings.Cut(token, ".")
	if !ok || secret == "" {
		return nil, core.Fail(401, "Phiên chat không hợp lệ")
	}
	content = strings.TrimSpace(content)
	if err := validateAttachment(fileName, fileData); err != nil {
		return nil, err
	}
	if len(content) > maximumMessageBytes || rating < 0 || rating > 5 || ((content != "" || fileData != "") && rating > 0) {
		return nil, core.Fail(400, "Dữ liệu không hợp lệ")
	}
	run := func(store core.Reader, writer core.Writer) error {
		session, ok := store.Record(tenant, tokenHash(secret), "chat_sessions")
		if !ok {
			return core.Fail(401, "Phiên chat không hợp lệ")
		}
		user := core.User{TenantID: tenant}
		conversation, err := findOwnedRecord(store, user, session.ConversationID, core.KindConversations)
		if err != nil {
			return err
		}
		if content != "" || fileData != "" {
			if conversation.Status != core.StatusOpen {
				return core.Fail(409, "Hội thoại đã đóng")
			}
			previous := latestPublicMessage(store, tenant, conversation.ID)
			message := s.storeMessage(writer, user, conversation, content, core.SenderCustomer)
			if fileData != "" {
				message.AttachmentName, message.AttachmentData = fileName, fileData
				writer.PutRecord(message)
			}
			if fileData == "" && confirmBotResolution(writer, conversation, previous, message) {
				conversation.Status = core.StatusClosed
			} else if err := s.automations.Run(writer, user, conversation, message); err != nil {
				return err
			}
		}
		if rating > 0 {
			if conversation.Status != core.StatusClosed || conversation.Rating != 0 {
				return core.Fail(409, "Chỉ đánh giá một lần sau khi hội thoại đóng")
			}
			now := s.clock.Now().UTC()
			conversation.Rating, conversation.RatedAt = rating, &now
			writer.PutRecord(conversation)
		}
		var messages []core.Record
		for _, message := range store.Records(tenant, core.KindMessages) {
			if message.ConversationID == conversation.ID && message.Sender != "note" {
				messages = append(messages, message)
			}
		}
		sort.Slice(messages, func(i, j int) bool { return laterMessage(messages[j], messages[i]) })
		for _, message := range messages[max(0, len(messages)-30):] {
			result = append(result, ChatMessage{ID: message.ID, Content: message.Content, Sender: message.Sender, AttachmentName: message.AttachmentName, AttachmentData: message.AttachmentData})
		}
		return nil
	}
	if content == "" && fileData == "" && rating == 0 {
		err := s.repo.View(func(store core.Reader) error { return run(store, nil) })
		return result, err
	}
	err := s.repo.Update(func(store core.Writer) error { return run(store, store) })
	return result, err
}
