package service

import (
	"strings"

	"businessautomation/internal/core"
)

// ResourcePolicy is the Strategy port for resource validation and write permissions.
type ResourcePolicy interface {
	Validate(core.Reader, core.User, *core.Record) error
	CanWrite(core.User) bool
}

func DefaultPolicies() map[string]ResourcePolicy {
	return map[string]ResourcePolicy{
		core.KindCustomers:     CustomerPolicy{},
		core.KindConversations: ConversationPolicy{},
		core.KindTickets:       TicketPolicy{},
		core.KindKnowledge:     KnowledgePolicy{},
		core.KindAutomations:   AutomationPolicy{},
	}
}

type CustomerPolicy struct{}

func (CustomerPolicy) CanWrite(user core.User) bool {
	return user.Role == core.RoleOwner || user.Role == core.RoleAgent
}

func (CustomerPolicy) Validate(store core.Reader, user core.User, record *core.Record) error {
	if record.Name == "" {
		return core.Fail(400, "Tên khách hàng là bắt buộc")
	}

	return nil
}

type ConversationPolicy struct{}

func (ConversationPolicy) CanWrite(user core.User) bool {
	return user.Role == core.RoleOwner || user.Role == core.RoleAgent
}

func (ConversationPolicy) Validate(store core.Reader, user core.User, record *core.Record) error {
	if _, err := findOwnedRecord(store, user, record.CustomerID, core.KindCustomers); err != nil {
		return err
	}
	if record.Channel == "" {
		record.Channel = core.ChannelWeb
	}
	if record.Channel != core.ChannelWeb && record.Channel != core.ChannelEmail && record.Channel != core.ChannelPhone {
		return core.Fail(400, "Channel phải là web, email hoặc phone")
	}
	if record.Status == "" {
		record.Status = core.StatusOpen
	}
	if record.Status != core.StatusOpen && record.Status != core.StatusClosed {
		return core.Fail(400, "Trạng thái hội thoại không hợp lệ")
	}

	return nil
}

type TicketPolicy struct{}

func (TicketPolicy) CanWrite(user core.User) bool {
	return user.Role == core.RoleOwner || user.Role == core.RoleAgent
}

func (TicketPolicy) Validate(store core.Reader, user core.User, record *core.Record) error {
	if record.Subject == "" {
		return core.Fail(400, "Tiêu đề ticket là bắt buộc")
	}
	if record.Status == "" {
		record.Status = core.StatusOpen
	}
	if record.Status != core.StatusOpen && record.Status != core.StatusInProgress && record.Status != core.StatusResolved {
		return core.Fail(400, "Trạng thái ticket không hợp lệ")
	}
	if record.Priority == "" {
		record.Priority = core.PriorityNormal
	}
	if record.Priority != core.PriorityLow && record.Priority != core.PriorityNormal && record.Priority != core.PriorityHigh && record.Priority != core.PriorityUrgent {
		return core.Fail(400, "Độ ưu tiên không hợp lệ")
	}
	if record.CustomerID != "" {
		if _, err := findOwnedRecord(store, user, record.CustomerID, core.KindCustomers); err != nil {
			return err
		}
	}
	if record.ConversationID != "" {
		conversation, err := findOwnedRecord(store, user, record.ConversationID, core.KindConversations)
		if err != nil {
			return err
		}
		if record.CustomerID != "" && record.CustomerID != conversation.CustomerID {
			return core.Fail(400, "Khách hàng không khớp hội thoại")
		}
		record.CustomerID = conversation.CustomerID
	}

	return nil
}

type KnowledgePolicy struct{}

func (KnowledgePolicy) CanWrite(user core.User) bool { return user.Role == core.RoleOwner }
func (KnowledgePolicy) Validate(store core.Reader, user core.User, record *core.Record) error {
	if record.Subject == "" || record.Content == "" {
		return core.Fail(400, "Tiêu đề và nội dung kiến thức là bắt buộc")
	}

	return nil
}

type AutomationPolicy struct{}

func (AutomationPolicy) CanWrite(user core.User) bool { return user.Role == core.RoleOwner }
func (AutomationPolicy) Validate(store core.Reader, user core.User, record *core.Record) error {
	if record.Name == "" || len(record.Keywords) == 0 {
		return core.Fail(400, "Tên và từ khóa automation là bắt buộc")
	}
	for index, keyword := range record.Keywords {
		record.Keywords[index] = strings.TrimSpace(strings.ToLower(keyword))
		if record.Keywords[index] == "" {
			return core.Fail(400, "Từ khóa không được rỗng")
		}
	}
	return nil
}
