package service

import (
	"encoding/json"
	"sort"

	"businessautomation/internal/core"
)

// governanceKind stores the per-tenant agent control plane (kill switches,
// risk tiers, approval requirements). It is audited like other operational
// kinds so governance changes leave a structured trail.
const governanceKind = "ai_governance"

// riskTiers is the closed set of agent risk classifications, aligned with the
// NIST AI RMF Govern/Manage functions and EU AI Act high-risk tiers.
var riskTiers = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}

// AgentGovernance is the owner-configurable control plane for AI agents.
// PlatformKill is a break-glass switch that stops every agent for the tenant;
// Disabled is a per-agent kill switch; RiskTiers classifies each agent; and
// Approval marks agents whose actions a human must confirm before execution.
type AgentGovernance struct {
	PlatformKill bool              `json:"platform_kill"`
	Disabled     map[string]bool   `json:"disabled"`
	RiskTiers    map[string]string `json:"risk_tiers"`
	Approval     map[string]bool   `json:"approval"`
}

func newAgentGovernance() AgentGovernance {
	return AgentGovernance{Disabled: map[string]bool{}, RiskTiers: map[string]string{}, Approval: map[string]bool{}}
}

func governanceID(tenant string) string { return tokenHash("governance:" + tenant) }

func normalizeGovernance(config *AgentGovernance) {
	if config.Disabled == nil {
		config.Disabled = map[string]bool{}
	}
	if config.RiskTiers == nil {
		config.RiskTiers = map[string]string{}
	}
	if config.Approval == nil {
		config.Approval = map[string]bool{}
	}
}

func loadGovernance(store core.Reader, tenant string) AgentGovernance {
	config := newAgentGovernance()
	if record, found := store.Record(tenant, governanceID(tenant), governanceKind); found {
		_ = json.Unmarshal([]byte(record.Content), &config)
	}
	normalizeGovernance(&config)
	return config
}

// GovernanceAllows is the kill-switch gate consulted by every agent runtime
// before it acts: a platform or per-agent kill switch denies access.
func GovernanceAllows(store core.Reader, tenant, agent string) bool {
	config := loadGovernance(store, tenant)
	return !config.PlatformKill && !config.Disabled[agent]
}

// AgentRisk returns the configured risk tier for an agent, defaulting to
// "medium" when unset or invalid.
func AgentRisk(store core.Reader, tenant, agent string) string {
	if tier := loadGovernance(store, tenant).RiskTiers[agent]; riskTiers[tier] {
		return tier
	}
	return "medium"
}

// RequiresApproval reports whether a human must approve the agent's actions.
func RequiresApproval(store core.Reader, tenant, agent string) bool {
	return loadGovernance(store, tenant).Approval[agent]
}

// AgentCanAutoAct reports whether the agent may take actions automatically,
// without a human confirming each one. An agent may auto-act only when it has
// not been killed, does not require approval, and is not a high/critical risk
// tier. This is the gate auto-execution paths (auto-resolve, chatbot reply)
// must consult; human-confirmed paths only need GovernanceAllows.
func AgentCanAutoAct(store core.Reader, tenant, agent string) bool {
	if !GovernanceAllows(store, tenant, agent) {
		return false
	}
	if RequiresApproval(store, tenant, agent) {
		return false
	}
	switch AgentRisk(store, tenant, agent) {
	case "low", "medium":
		return true
	default:
		return false
	}
}

// Governance is the owner-facing control service for AI agent governance.
type Governance struct {
	repo  *Workspace
	clock Clock
}

func NewGovernance(repo *Workspace, clock Clock) *Governance {
	return &Governance{repo: repo, clock: clock}
}

// AgentInventoryEntry summarizes one registered agent for the inventory view.
type AgentInventoryEntry struct {
	ID       string `json:"id"`
	Risk     string `json:"risk"`
	Disabled bool   `json:"disabled"`
	Approval bool   `json:"approval"`
	Active   bool   `json:"active"`
}

// Inventory lists the registered agents with their governance state.
func (g *Governance) Inventory(user core.User) (AgentGovernance, []AgentInventoryEntry, error) {
	if user.Role != core.RoleOwner {
		return AgentGovernance{}, nil, core.Fail(403, "Chỉ owner quản lý governance")
	}
	config, entries := AgentGovernance{}, []AgentInventoryEntry{}
	err := g.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		var err error
		config = loadGovernance(store, user.TenantID)
		ids := []string{}
		for _, preset := range agentRegistry {
			ids = append(ids, preset.ID)
		}
		ids = append(ids, "ticket_agent", "chatbot", "voicebot")
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			entries = append(entries, AgentInventoryEntry{
				ID:       id,
				Risk:     AgentRisk(store, user.TenantID, id),
				Disabled: config.Disabled[id],
				Approval: config.Approval[id],
				Active:   GovernanceAllows(store, user.TenantID, id) && !config.PlatformKill,
			})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		return err
	})
	return config, entries, err
}

// Save updates the tenant's governance configuration and platform kill switch.
func (g *Governance) Save(user core.User, config AgentGovernance, platformKill bool) (AgentGovernance, error) {
	if user.Role != core.RoleOwner {
		return config, core.Fail(403, "Chỉ owner quản lý governance")
	}
	normalizeGovernance(&config)
	if len(config.Disabled) > 20 || len(config.RiskTiers) > 20 || len(config.Approval) > 20 {
		return config, core.Fail(400, "Mỗi nhóm cấu hình governance tối đa 20 mục")
	}
	for _, tier := range config.RiskTiers {
		if !riskTiers[tier] {
			return config, core.Fail(400, "Mức rủi ro phải là low, medium, high hoặc critical")
		}
	}
	config.PlatformKill = platformKill
	err := g.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		payload, err := json.Marshal(config)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: governanceID(user.TenantID), TenantID: user.TenantID, Kind: governanceKind,
			Content: string(payload), Enabled: !platformKill, UpdatedAt: g.clock.Now().UTC()})
		return nil
	})
	return config, err
}
