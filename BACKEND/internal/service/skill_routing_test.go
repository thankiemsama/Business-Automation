package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestSkillRoutingMatchesVIPAndOverflowsWithinTenant(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	primary := core.User{ID: "primary", TenantID: owner.TenantID, Role: core.RoleAgent}
	overflow := core.User{ID: "overflow", TenantID: owner.TenantID, Role: core.RoleAgent}
	foreign := core.User{ID: "foreign", TenantID: "different", Role: core.RoleAgent}
	if err := ops.repo.Update(func(store core.Writer) error {
		for _, member := range []core.User{primary, overflow, foreign} {
			store.PutUser(member)
		}
		store.PutRecord(core.Record{ID: "customer", TenantID: owner.TenantID, Kind: core.KindCustomers, Tags: []string{"vip"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	config := SkillRouting{Enabled: true,
		Groups: []SkillGroup{
			{Key: "main", Name: "Main", Members: []string{primary.ID}, Capacity: 1},
			{Key: "backup", Name: "Backup", Members: []string{overflow.ID}, Capacity: 1},
		},
		Rules: []SkillRoute{
			{Channel: core.ChannelWeb, Tag: " VIP ", Priority: core.PriorityHigh, Primary: "main", Overflow: "backup"},
		},
	}
	if _, err := ops.SaveSkillRouting(primary, config); err == nil {
		t.Fatal("agent modified routing")
	}
	config.Groups[1].Members = []string{foreign.ID}
	if _, err := ops.SaveSkillRouting(owner, config); err == nil {
		t.Fatal("cross-tenant member accepted")
	}
	config.Groups[1].Members = []string{overflow.ID}
	if _, err := ops.SaveSkillRouting(owner, config); err != nil {
		t.Fatal(err)
	}
	create := func(id, channel, manual, expected string) core.Record {
		t.Helper()
		var stored core.Record
		if err := ops.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
			store.PutRecord(core.Record{ID: id, TenantID: owner.TenantID, Kind: core.KindConversations,
				Status: core.StatusOpen, Channel: channel, CustomerID: "customer", Priority: core.PriorityHigh,
				AssigneeID: manual, CreatedAt: clock.Now()})
			stored, _ = store.Record(owner.TenantID, id, core.KindConversations)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if stored.AssigneeID != expected {
			t.Fatalf("conversation %s assigned to %q, expected %q", id, stored.AssigneeID, expected)
		}
		return stored
	}
	create("other-channel", core.ChannelEmail, "", "")
	first := create("main-thread", core.ChannelWeb, "", primary.ID)
	create("overflow-thread", core.ChannelWeb, "", overflow.ID)
	create("at-capacity", core.ChannelWeb, "", "")
	create("manual", core.ChannelWeb, owner.ID, owner.ID)
	if err := ops.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		first.Status = core.StatusClosed
		store.PutRecord(first)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	create("new-capacity", core.ChannelWeb, "", primary.ID)
}
