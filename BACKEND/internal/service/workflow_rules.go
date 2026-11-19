package service

import (
	"encoding/json"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const workflowRulesKind = "workflow_rules"
const workflowJobsKind = core.KindWorkflowJobs

type WorkflowConditions struct {
	Channel  string   `json:"channel"`
	Priority string   `json:"priority"`
	Status   string   `json:"status"`
	Contains string   `json:"contains"`
	Keywords []string `json:"keywords_any,omitempty"`
}
type WorkflowAction struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}
type WorkflowBranch struct {
	Conditions WorkflowConditions `json:"conditions"`
	Then       []WorkflowAction   `json:"then"`
	Otherwise  []WorkflowAction   `json:"otherwise"`
}
type WorkflowRule struct {
	MigratedFrom string             `json:"migrated_from,omitempty"`
	ID           string             `json:"id,omitempty"`
	Name         string             `json:"name"`
	Enabled      bool               `json:"enabled"`
	Trigger      string             `json:"trigger"`
	IdleDays     int                `json:"idle_days,omitempty"`
	Conditions   WorkflowConditions `json:"conditions"`
	Actions      []WorkflowAction   `json:"actions"`
	Branch       *WorkflowBranch    `json:"branch,omitempty"`
}

func workflowConfig[T any](store core.Reader, tenant, kind string) []T {
	items := []T{}
	for _, record := range store.Records(tenant, kind) {
		if record.Status == "removed" {
			continue
		}
		var item T
		if json.Unmarshal([]byte(record.Content), &item) == nil {
			items = append(items, item)
		}
	}
	return items
}

func (o *Operations) Rules(user core.User) ([]WorkflowRule, error) {
	var items []WorkflowRule
	err := o.repo.View(func(store core.Reader) error {
		items = workflowConfig[WorkflowRule](store, user.TenantID, workflowRulesKind)
		return nil
	})
	return items, err
}

func (o *Operations) MigrateKeywordRule(user core.User, id string) (WorkflowRule, error) {
	var result WorkflowRule
	err := o.repo.Update(func(store core.Writer) error {
		legacy, ok := store.Record(user.TenantID, id, core.KindAutomations)
		if !ok {
			return core.Fail(404, "Không tìm thấy quy tắc từ khóa")
		}
		key := tokenHash("workflow-migration:" + user.TenantID + ":" + id)
		if existing, ok := store.Record(user.TenantID, key, workflowRulesKind); ok && existing.Status != "removed" {
			if err := json.Unmarshal([]byte(existing.Content), &result); err != nil {
				return err
			}
		} else {
			if len(legacy.Keywords) == 0 {
				return core.Fail(400, "Quy tắc chưa có từ khóa")
			}
			result = WorkflowRule{ID: key, Name: legacy.Name, Enabled: legacy.Enabled, Trigger: "message_received", MigratedFrom: legacy.ID, Conditions: WorkflowConditions{Keywords: legacy.Keywords}, Actions: []WorkflowAction{{Type: "create_ticket"}}}
			if err := o.saveConfig(store, user.TenantID, workflowRulesKind, key, result.Enabled, result); err != nil {
				return err
			}
		}
		legacy.Enabled = false
		legacy.UpdatedAt = o.clock.Now().UTC()
		store.PutRecord(legacy)
		return nil
	})
	return result, err
}

func (o *Operations) SaveRule(user core.User, rule WorkflowRule) (WorkflowRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" || len([]rune(rule.Name)) > 100 {
		return rule, core.Fail(400, "Tên workflow không hợp lệ")
	}
	switch rule.Trigger {
	case "ticket_idle":
		if rule.IdleDays < 1 || rule.IdleDays > 365 {
			return rule, core.Fail(400, "Số ngày chờ phải từ 1 đến 365")
		}
	case "message_received", "ticket_created", "sla_breached":
		rule.IdleDays = 0
	default:
		return rule, core.Fail(400, "Trigger không hợp lệ")
	}
	actions := append([]WorkflowAction(nil), rule.Actions...)
	if rule.Branch != nil {
		actions = append(actions, rule.Branch.Then...)
		actions = append(actions, rule.Branch.Otherwise...)
	}
	if len(actions) == 0 || len(actions) > 12 {
		return rule, core.Fail(400, "Mỗi workflow cần từ 1 đến 12 hành động")
	}
	if err := validateConditions(rule.Conditions, rule.Trigger); err != nil {
		return rule, err
	}
	if rule.Branch != nil {
		if err := validateConditions(rule.Branch.Conditions, rule.Trigger); err != nil {
			return rule, err
		}
	}
	err := o.repo.Update(func(store core.Writer) error {
		for _, action := range actions {
			switch action.Type {
			case "notify_owner", "email_owner", "create_ticket", "assign_owner", "resolve_ticket":
				if action.Type == "resolve_ticket" && rule.Trigger == "message_received" {
					return core.Fail(400, "Trigger tin nhắn không chỉ định phiếu để đóng")
				}
			case "assign":
				target, ok := store.UserByID(action.Value)
				if !ok || target.TenantID != user.TenantID || target.Disabled {
					return core.Fail(400, "Người phụ trách không hợp lệ")
				}
			default:
				return core.Fail(400, "Hành động không hợp lệ")
			}
			if len([]rune(action.Value)) > 200 {
				return core.Fail(400, "Giá trị hành động quá dài")
			}
		}
		rule.MigratedFrom = ""
		if rule.ID == "" {
			rule.ID = o.repo.IDs.NewID()
		} else {
			old, ok := store.Record(user.TenantID, rule.ID, workflowRulesKind)
			if !ok || old.Status == "removed" {
				return core.Fail(404, "Không tìm thấy workflow")
			}
			var previous WorkflowRule
			if err := json.Unmarshal([]byte(old.Content), &previous); err != nil {
				return err
			}
			rule.MigratedFrom = previous.MigratedFrom
		}
		return o.saveConfig(store, user.TenantID, workflowRulesKind, rule.ID, rule.Enabled, rule)
	})
	return rule, err
}

func validateConditions(value WorkflowConditions, trigger string) error {
	if len(value.Keywords) > 30 {
		return core.Fail(400, "Tối đa 30 từ khóa")
	}
	for _, keyword := range value.Keywords {
		if strings.TrimSpace(keyword) == "" || len([]rune(keyword)) > 120 {
			return core.Fail(400, "Từ khóa không hợp lệ")
		}
	}
	if len([]rune(value.Contains)) > 120 {
		return core.Fail(400, "Từ khóa điều kiện quá dài")
	}
	if value.Channel != "" && value.Channel != "web" && value.Channel != "email" && value.Channel != "phone" && !contains(core.PublicChannels, value.Channel) {
		return core.Fail(400, "Kênh không hợp lệ")
	}
	if value.Priority != "" && !contains([]string{"low", "normal", "high", "urgent"}, value.Priority) {
		return core.Fail(400, "Ưu tiên không hợp lệ")
	}
	statuses := []string{"open", "in_progress", "resolved"}
	if trigger == "message_received" {
		statuses = []string{"open", "closed"}
	}
	if value.Status != "" && !contains(statuses, value.Status) {
		return core.Fail(400, "Trạng thái không hợp lệ")
	}
	return nil
}

func conditionsMatch(c WorkflowConditions, channel, priority, status, content string) bool {
	return (len(c.Keywords) == 0 || matchesAnyKeyword(content, c.Keywords)) && (c.Channel == "" || c.Channel == channel) &&
		(c.Priority == "" || c.Priority == priority) &&
		(c.Status == "" || c.Status == status) &&
		(strings.TrimSpace(c.Contains) == "" || strings.Contains(strings.ToLower(content), strings.ToLower(strings.TrimSpace(c.Contains))))
}

func (w *workspaceWriter) queueWorkflow(trigger string, record core.Record) {
	if !featureEnabled(w.Writer, record.TenantID, "workflows") {
		return
	}
	conversation, _ := w.Record(record.TenantID, record.ConversationID, core.KindConversations)
	for _, rule := range workflowConfig[WorkflowRule](w.Writer, record.TenantID, workflowRulesKind) {
		if rule.Enabled && rule.Trigger == trigger && conditionsMatch(WorkflowConditions{Channel: rule.Conditions.Channel, Contains: rule.Conditions.Contains, Keywords: rule.Conditions.Keywords}, conversation.Channel, "", "", record.Content) {
			w.queueRule(rule, record, w.workspace.Clock.Now().UTC())
		}
	}
}

func (w *workspaceWriter) queueRule(rule WorkflowRule, record core.Record, now time.Time) {
	id := tokenHash("workflow:" + rule.ID + ":" + record.ID)
	if existing, exists := w.Record(record.TenantID, id, workflowJobsKind); exists && existing.Status != "postponed" {
		return
	}
	w.Writer.PutRecord(core.Record{ID: id, TenantID: record.TenantID, Kind: workflowJobsKind, AutomationID: rule.ID, ConversationID: record.ID, Name: record.Kind, Subject: rule.Trigger, Enabled: true, CreatedAt: now, UpdatedAt: now})
}
