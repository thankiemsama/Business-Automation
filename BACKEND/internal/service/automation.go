package service

import "businessautomation/internal/core"

// AutomationRunner participates in the caller's Unit of Work; it must not perform external side effects.
type AutomationRunner interface {
	Run(core.Writer, core.User, core.Record, core.Record) error
}

type KeywordAutomation struct {
	ids   IDGenerator
	clock Clock
}

func NewKeywordAutomation(ids IDGenerator, clock Clock) *KeywordAutomation {
	return &KeywordAutomation{ids: ids, clock: clock}
}

func (automation *KeywordAutomation) Run(store core.Writer, user core.User, conversation, message core.Record) error {
	rules := store.Records(user.TenantID, core.KindAutomations)
	tickets := store.Records(user.TenantID, core.KindTickets)

	for _, rule := range rules {
		if !rule.Enabled || !matchesAnyKeyword(message.Content, rule.Keywords) {
			continue
		}
		if hasOpenAutomationTicket(tickets, conversation.ID, rule.ID) {
			continue
		}

		ticket := automation.newTicket(user, conversation, message, rule)
		store.PutRecord(ticket)
		tickets = append(tickets, ticket)
	}
	return nil
}

func hasOpenAutomationTicket(tickets []core.Record, conversationID, ruleID string) bool {
	for _, ticket := range tickets {
		if ticket.ConversationID != conversationID || ticket.Status == core.StatusResolved {
			continue
		}
		// Earlier snapshots stored the originating rule ID in the ticket name.
		matchesRule := ticket.AutomationID == ruleID
		matchesLegacyRule := ticket.AutomationID == "" && ticket.Name == ruleID
		if matchesRule || matchesLegacyRule {
			return true
		}
	}
	return false
}

func (automation *KeywordAutomation) newTicket(user core.User, conversation, message, rule core.Record) core.Record {
	now := automation.clock.Now().UTC()
	return core.Record{
		ID:             automation.ids.NewID(),
		TenantID:       user.TenantID,
		Kind:           core.KindTickets,
		AutomationID:   rule.ID,
		Subject:        rule.Name,
		Content:        message.Content,
		CustomerID:     conversation.CustomerID,
		ConversationID: conversation.ID,
		Status:         core.StatusOpen,
		Priority:       core.PriorityNormal,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

var _ AutomationRunner = (*KeywordAutomation)(nil)
