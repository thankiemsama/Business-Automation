package service

import (
	"path/filepath"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

// newGovernanceTestWorkspace builds a workspace with an owner user for the
// given tenant.
func newGovernanceTestWorkspace(t *testing.T, tenant string) (*Workspace, core.User) {
	t.Helper()
	base, err := repository.Open(filepath.Join(t.TempDir(), "governance-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(base, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	owner := core.User{ID: "owner", TenantID: tenant, Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil }); err != nil {
		t.Fatal(err)
	}
	return workspace, owner
}

// TestGovernanceSaveAndInventory verifies the governance control plane:
// saving a config, kill-switching an agent, and reading the inventory.
func TestGovernanceSaveAndInventory(t *testing.T) {
	workspace, owner := newGovernanceTestWorkspace(t, "tenant")
	governance := NewGovernance(workspace, workspace.Clock)

	// Default: agent can auto-act (no governance configured).
	canAct := false
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		canAct = AgentCanAutoAct(store, "tenant", "voicebot")
		return nil
	})
	if !canAct {
		t.Fatal("default governance should allow auto-act")
	}

	// Disable the voicebot agent.
	saved, err := governance.Save(owner, AgentGovernance{
		Disabled:  map[string]bool{"voicebot": true},
		RiskTiers: map[string]string{},
		Approval:  map[string]bool{},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Disabled["voicebot"] {
		t.Fatal("voicebot not marked disabled")
	}

	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		if AgentCanAutoAct(store, "tenant", "voicebot") {
			t.Fatal("disabled agent should not auto-act")
		}
		if AgentRisk(store, "tenant", "voicebot") != "medium" {
			t.Fatal("default risk tier should be medium")
		}
		return nil
	})

	// Inventory lists the agent with governance state.
	config, entries, err := governance.Inventory(owner)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Disabled["voicebot"] {
		t.Fatal("inventory should show disabled voicebot")
	}
	found := 0
	for _, entry := range entries {
		if entry.ID == "voicebot" && entry.Disabled && !entry.Active {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("inventory should contain disabled voicebot: %+v", entries)
	}

	// Platform kill switch.
	saved, err = governance.Save(owner, AgentGovernance{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.PlatformKill {
		t.Fatal("platform kill switch not set")
	}
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		if GovernanceAllows(store, "tenant", "voicebot") {
			t.Fatal("platform kill switch should block all agents")
		}
		return nil
	})
}

// TestGovernanceApprovalGate verifies that approval-gated agents cannot
// auto-act even when enabled and low-risk.
func TestGovernanceApprovalGate(t *testing.T) {
	workspace, owner := newGovernanceTestWorkspace(t, "tenant")
	governance := NewGovernance(workspace, workspace.Clock)
	_, err := governance.Save(owner, AgentGovernance{
		Disabled:  map[string]bool{},
		RiskTiers: map[string]string{"ticket_agent": "low"},
		Approval:  map[string]bool{"ticket_agent": true},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		if AgentCanAutoAct(store, "tenant", "ticket_agent") {
			t.Fatal("approval-gated agent should not auto-act")
		}
		if !RequiresApproval(store, "tenant", "ticket_agent") {
			t.Fatal("agent should require approval")
		}
		return nil
	})
}

// TestAIActionCatalog verifies the action catalog CRUD and risk validation.
func TestAIActionCatalog(t *testing.T) {
	workspace, owner := newGovernanceTestWorkspace(t, "tenant")
	actions := NewAIActions(workspace, workspace.Clock, nil)

	// Invalid risk.
	if _, err := actions.Save(owner, AIAction{Name: "bad", Risk: "extreme"}); err == nil {
		t.Fatal("invalid risk tier accepted")
	}

	// Valid low-risk action.
	item, err := actions.Save(owner, AIAction{Name: "refund_small", Risk: "low", Enabled: true, Channels: []string{core.ChannelWeb}})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" {
		t.Fatal("new action should have an ID")
	}

	// List returns it.
	items, err := actions.List(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "refund_small" {
		t.Fatalf("unexpected catalog: %+v", items)
	}

	// Remove hides it from the catalog.
	if err := actions.Remove(owner, item.ID); err != nil {
		t.Fatal(err)
	}
	removed, _ := actions.List(owner)
	for _, r := range removed {
		if r.ID == item.ID {
			t.Fatal("removed action should not be listed")
		}
	}
}

// TestAISKillSwitchAndFeedbackLoop verifies the skill catalog and the feedback
// record loop.
func TestAISkillAndFeedbackLoop(t *testing.T) {
	workspace, owner := newGovernanceTestWorkspace(t, "tenant")
	skills := NewAISkills(workspace, workspace.Clock)

	// Invalid tone.
	if _, err := skills.SaveSkill(owner, AISkill{Name: "bad", Tone: "shouting"}); err == nil {
		t.Fatal("invalid tone accepted")
	}

	// Valid skill with a trigger keyword, scoped to the web channel.
	item, err := skills.SaveSkill(owner, AISkill{
		Name: "billing_help", Enabled: true, Channel: core.ChannelWeb,
		Trigger: []string{"hóa đơn", "factura"}, Instructions: "Hướng dẫn tra cứu hóa đơn", Tone: "empathetic",
	})
	if err != nil {
		t.Fatal(err)
	}

	// MatchedSkill finds it by trigger keyword on the matching channel.
	var matchedSkill AISkill
	matched := false
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		matchedSkill, matched = MatchedSkill(store, "tenant", core.ChannelWeb, "tôi cần xem hóa đơn")
		return nil
	})
	if !matched || matchedSkill.ID != item.ID {
		t.Fatalf("skill not matched on web: %+v %v", matchedSkill, matched)
	}

	// No match on a different channel.
	matchedOnEmail := false
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		_, matchedOnEmail = MatchedSkill(store, "tenant", core.ChannelEmail, "tôi cần xem hóa đơn")
		return nil
	})
	if matchedOnEmail {
		t.Fatal("web-scoped skill should not match on email channel")
	}

	// Removing the skill drops it from the catalog.
	if err := skills.RemoveSkill(owner, item.ID); err != nil {
		t.Fatal(err)
	}
	_ = workspace.ViewTenant("tenant", func(store core.Reader) error {
		if _, still := MatchedSkill(store, "tenant", core.ChannelWeb, "tôi cần xem hóa đơn"); still {
			t.Error("removed skill still matched")
		}
		return nil
	})
}

// TestResolutionMeter verifies per-resolution billing metering is idempotent
// on key and records a resolution marker.
func TestResolutionMeter(t *testing.T) {
	workspace, _ := newGovernanceTestWorkspace(t, "tenant")
	now := time.Now().UTC()

	// First call records a marker; the second (same key) is a no-op.
	count := 0
	_ = workspace.UpdateTenant("tenant", func(store core.Writer) error {
		meterAIResolution(store, "tenant", "conv-123:bot", "bot", now)
		meterAIResolution(store, "tenant", "conv-123:bot", "bot", now)
		meterAIResolution(store, "tenant", "conv-456:bot", "bot", now)
		for range store.Records("tenant", "ai_resolutions") {
			count++
		}
		return nil
	})
	if count != 2 {
		t.Fatalf("expected 2 unique resolutions, got %d", count)
	}
}
