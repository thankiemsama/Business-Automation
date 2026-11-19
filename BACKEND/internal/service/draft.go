package service

import "businessautomation/internal/core"

func (s *ConversationService) ConfigureDraftProvider(provider ReplyProvider) {
	s.draftProvider = provider
}

func (s *ConversationService) Draft(user core.User, id string) (Reply, error) {
	var latest core.Record
	var articles []core.Record
	modelEnabled := true
	err := viewTenant(s.repo, user.TenantID, func(store core.Reader) error {
		conversation, err := findOwnedRecord(store, user, id, core.KindConversations)
		if err != nil {
			return err
		}
		if conversation.Status == core.StatusClosed {
			return core.Fail(409, "Hội thoại đã đóng")
		}
		latest = latestPublicMessage(store, user.TenantID, id)
		if latest.ID == "" || latest.Sender != core.SenderCustomer {
			return core.Fail(409, "Cần tin nhắn mới từ khách hàng")
		}
		modelEnabled = featureEnabled(store, user.TenantID, "ai")
		articles = store.Records(user.TenantID, core.KindKnowledge)
		return nil
	})
	if err != nil {
		return Reply{}, err
	}
	// The draft is anchored to the source customer message it was generated
	// from. If that message is no longer the newest one when the agent sends
	// the draft, the reply is rejected and the agent must regenerate.
	provider := s.draftProvider
	if !modelEnabled {
		provider = LocalKnowledgeProvider{}
	}
	var answer Answer
	if model, ok := provider.(*OpenAIKnowledge); ok {
		answer, err = model.answer(user.TenantID, latest.Content, articles)
	} else {
		answer, err = provider.Answer(latest.Content, articles)
	}
	if err != nil {
		answer, err = (LocalKnowledgeProvider{}).Answer(latest.Content, articles)
		if err != nil {
			return Reply{}, err
		}
	}
	result := Reply{
		Message:         core.Record{Content: answer.Content},
		SourceID:        answer.SourceID,
		SourceMessageID: latest.ID,
		Provider:        answer.Provider,
		NeedsHuman:      answer.NeedsHuman,
	}
	if !answer.NeedsHuman {
		err := viewTenant(s.repo, user.TenantID, func(store core.Reader) error {
			if _, err := checkDraftContext(store, user, id, latest.ID); err != nil {
				return err
			}
			if answer.SourceID == "" {
				return nil
			}
			current, ok := store.Record(user.TenantID, answer.SourceID, core.KindKnowledge)
			for _, article := range articles {
				if article.ID == answer.SourceID && ok && current.Enabled && current.Subject == article.Subject && current.Content == article.Content {
					return nil
				}
			}
			return core.Fail(409, "Nguồn kiến thức đã thay đổi; hãy soạn lại bản nháp")
		})
		if err != nil {
			return Reply{}, err
		}
		return result, nil
	}
	err = updateTenant(s.repo, user.TenantID, func(store core.Writer) error {
		conversation, err := checkDraftContext(store, user, id, latest.ID)
		if err != nil {
			return err
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

func latestPublicMessage(store core.Reader, tenant, id string) core.Record {
	var latest core.Record
	for _, message := range store.ConversationMessages(tenant, id) {
		if message.Sender != "note" && laterMessage(message, latest) {
			latest = message
		}
	}
	return latest
}

func checkDraftContext(store core.Reader, user core.User, id, sourceID string) (core.Record, error) {
	conversation, err := findOwnedRecord(store, user, id, core.KindConversations)
	if err != nil {
		return conversation, err
	}
	if conversation.Status == core.StatusClosed {
		return conversation, core.Fail(409, "Hội thoại đã đóng")
	}
	latest := latestPublicMessage(store, user.TenantID, id)
	if latest.ID != sourceID || latest.Sender != core.SenderCustomer {
		return conversation, core.Fail(409, "Ngữ cảnh hội thoại đã thay đổi; hãy soạn lại bản nháp")
	}
	return conversation, nil
}
