package service

import "businessautomation/internal/core"

func (s *ConversationService) ConfigureDraftProvider(provider ReplyProvider) {
	s.draftProvider = provider
}

func (s *ConversationService) Draft(user core.User, id string) (Reply, error) {
	var latest core.Record
	var articles []core.Record
	modelEnabled := true
	err := s.repo.View(func(store core.Reader) error {
		conversation, err := findOwnedRecord(store, user, id, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		for _, message := range store.Records(user.TenantID, core.KindMessages) {
			if message.ConversationID == id && message.Sender != "note" && laterMessage(message, latest) {
				latest = message
			}
		}
		if latest.Sender != core.SenderCustomer {
			return core.Fail(409, "Cần tin nhắn mới từ khách hàng")
		}
		modelEnabled = featureEnabled(store, user.TenantID, "ai")
		articles = store.Records(user.TenantID, core.KindKnowledge)
		return nil
	})
	if err != nil {
		return Reply{}, err
	}
	provider := s.draftProvider
	if !modelEnabled {
		provider = LocalKnowledgeProvider{}
	}
	answer, err := provider.Answer(latest.Content, articles)
	if err != nil {
		answer, err = (LocalKnowledgeProvider{}).Answer(latest.Content, articles)
		if err != nil {
			return Reply{}, err
		}
	}
	result := Reply{Message: core.Record{Content: answer.Content}, SourceID: answer.SourceID, Provider: answer.Provider, NeedsHuman: answer.NeedsHuman}
	if !answer.NeedsHuman {
		return result, nil
	}
	err = s.repo.Update(func(store core.Writer) error {
		conversation, err := findOwnedRecord(store, user, id, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		for _, message := range store.Records(user.TenantID, core.KindMessages) {
			if message.ConversationID == id && message.Sender != "note" && laterMessage(message, latest) {
				return core.Fail(409, "Hội thoại có tin nhắn mới; hãy thử lại")
			}
		}
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			if ticket.ConversationID == id && ticket.Status != core.StatusResolved {
				return nil
			}
		}
		now := s.clock.Now().UTC()
		store.PutRecord(core.Record{ID: s.ids.NewID(), TenantID: user.TenantID, Kind: core.KindTickets, ConversationID: id, CustomerID: conversation.CustomerID, AssigneeID: conversation.AssigneeID, Subject: "Cần nhân viên hỗ trợ: " + conversation.Subject, Content: latest.Content, Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: now, UpdatedAt: now})
		return nil
	})
	return result, err
}
