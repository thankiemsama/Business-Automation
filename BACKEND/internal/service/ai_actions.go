package service

import (
	"encoding/json"
	"strings"

	"businessautomation/internal/core"
)

// aiActionsKind stores the per-tenant AI action registry: a bounded catalog of
// concrete, tenant-owned actions an AI agent may propose and (after a human
// approval gate for high-risk items) execute. This mirrors the Decagon/Gorgias
// "agent takes backend actions" pattern on top of Nexa's existing
// ticket-agent plan/apply flow, but generalizes it beyond resolve/send_csat.
const aiActionsKind = "ai_actions"

// AIAction is one executable capability the agent may request. Risk gates the
// approval requirement: low/medium actions auto-run when the governance
// allows auto-act; high/critical always need a human to confirm the plan.
type AIAction struct {
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	Risk       string             `json:"risk"`
	Enabled    bool               `json:"enabled"`
	Channels   []string           `json:"channels,omitempty"`
	Conditions AIActionConditions `json:"conditions,omitempty"`
}

// AIActionConditions mirrors the closed workflow-condition vocabulary so the
// same rule engine can filter which conversations an action applies to.
type AIActionConditions struct {
	Channel  string   `json:"channel,omitempty"`
	Priority string   `json:"priority,omitempty"`
	Contains string   `json:"contains,omitempty"`
	Keywords []string `json:"keywords_any,omitempty"`
}

func validActionRisk(risk string) bool {
	return riskTiers[risk]
}

// NewAIActions builds the owner-facing action catalog service.
func NewAIActions(repo *Workspace, clock Clock, model *OpenAIKnowledge) *AIActionService {
	return &AIActionService{repo: repo, clock: clock, model: model}
}

// AIActionService manages the tenant's action catalog and the approval gate.
type AIActionService struct {
	repo  *Workspace
	clock Clock
	model *OpenAIKnowledge
}

// List returns the tenant's enabled and disabled actions for the owner to manage.
func (s *AIActionService) List(user core.User) ([]AIAction, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner quản lý hành động AI")
	}
	result := []AIAction{}
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, aiActionsKind) {
			if record.Status == "removed" {
				continue
			}
			var item AIAction
			if json.Unmarshal([]byte(record.Content), &item) == nil {
				result = append(result, item)
			}
		}
		return nil
	})
	return result, err
}

// Save creates or updates a single action in the catalog.
func (s *AIActionService) Save(user core.User, item AIAction) (AIAction, error) {
	if user.Role != core.RoleOwner {
		return item, core.Fail(403, "Chỉ owner quản lý hành động AI")
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || len([]rune(item.Name)) > 100 {
		return item, core.Fail(400, "Tên hành động không hợp lệ")
	}
	if !validActionRisk(item.Risk) {
		return item, core.Fail(400, "Mức rủi ro phải là low, medium, high hoặc critical")
	}
	for _, channel := range item.Channels {
		if channel != "" && !contains(core.PublicChannels, channel) &&
			!contains([]string{core.ChannelWeb, core.ChannelEmail, core.ChannelPhone}, channel) {
			return item, core.Fail(400, "Kênh trong hành động không hợp lệ")
		}
	}
	if len(item.Channels) > 12 || len(item.Conditions.Keywords) > 30 ||
		len([]rune(item.Conditions.Contains)) > 120 {
		return item, core.Fail(400, "Điều kiện hành động vượt giới hạn")
	}
	err := s.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		if item.ID == "" {
			item.ID = s.repo.IDs.NewID()
			count := 0
			for _, record := range store.Records(user.TenantID, aiActionsKind) {
				if record.Status != "removed" {
					count++
				}
			}
			if count >= 40 {
				return core.Fail(409, "Tối đa 40 hành động AI đang lưu")
			}
		} else {
			old, ok := store.Record(user.TenantID, item.ID, aiActionsKind)
			if !ok || old.Status == "removed" {
				return core.Fail(404, "Không tìm thấy hành động")
			}
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: item.ID, TenantID: user.TenantID, Kind: aiActionsKind,
			Name: item.Name, Content: string(payload), Enabled: item.Enabled, UpdatedAt: s.clock.Now().UTC()})
		return nil
	})
	return item, err
}

// Remove disables an action in the catalog.
func (s *AIActionService) Remove(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner quản lý hành động AI")
	}
	return s.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, aiActionsKind)
		if !ok || record.Status == "removed" {
			return core.Fail(404, "Không tìm thấy hành động")
		}
		record.Enabled, record.Status = false, "removed"
		record.UpdatedAt = s.clock.Now().UTC()
		store.PutRecord(record)
		return nil
	})
}

// ActionAvailable reports whether a named action is enabled for the tenant and
// not blocked by governance. Agent runtimes consult this before proposing.
func ActionAvailable(store core.Reader, tenant, name string) bool {
	for _, record := range store.Records(tenant, aiActionsKind) {
		if !record.Enabled || record.Status == "removed" {
			continue
		}
		var item AIAction
		if json.Unmarshal([]byte(record.Content), &item) != nil || item.Name != name {
			continue
		}
		if !GovernanceAllows(store, tenant, "ticket_agent") {
			return false
		}
		return item.Risk == "low" || item.Risk == "medium" || AgentCanAutoAct(store, tenant, "ticket_agent")
	}
	return false
}
