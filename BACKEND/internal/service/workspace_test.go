package service

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

type workspaceClock struct{ now time.Time }

func (c *workspaceClock) Now() time.Time { return c.now }

func TestSLAWarningAndNotification(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "sla.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: "ticket", TenantID: user.TenantID, Kind: core.KindTickets, Priority: core.PriorityUrgent, Status: core.StatusOpen, Subject: "SLA test", CreatedAt: clock.now})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, events, stop := workspace.Watch(user.TenantID)
	defer stop()
	clock.now = clock.now.Add(30 * time.Minute)
	workspace.sweepSLA()
	select {
	case <-events:
	default:
		t.Fatal("SLA warning did not refresh the workspace")
	}
	if items, err := workspace.Notifications(user); err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	clock.now = clock.now.Add(30 * time.Minute)
	workspace.sweepSLA()
	workspace.sweepSLA()
	select {
	case <-events:
	default:
		t.Fatal("SLA breach did not refresh the workspace")
	}
	items, err := workspace.Notifications(user)
	if err != nil || len(items) != 1 || items[0].Status != "sla" {
		t.Fatal(items, err)
	}
}

func TestWorkspaceSavepointDiscardsFailedHooks(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "workspace.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(repo, SystemClock{}, RandomIDs{})
	const tenant = "savepoint-tenant"
	ticket := core.Record{ID: "ticket", TenantID: tenant, Kind: core.KindTickets, Status: core.StatusOpen}
	if err := workspace.Update(func(w core.Writer) error {
		w.PutRecord(ticket)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	workspace.SetOnResolved(func(core.Record) { calls++ })
	_, events, release := workspace.Watch(tenant)
	defer release()
	cancel := errors.New("cancel")
	ticket.Status = core.StatusResolved
	if err := workspace.Update(func(w core.Writer) error {
		err := w.Savepoint(func(inner core.Writer) error {
			inner.PutRecord(ticket)
			return cancel
		})
		if !errors.Is(err, cancel) {
			t.Errorf("want sub-operation error, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(events) != 0 {
		t.Fatal("failed savepoint published workspace effects")
	}
	if err := workspace.Update(func(w core.Writer) error {
		return w.Savepoint(func(inner core.Writer) error {
			inner.PutRecord(ticket)
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(events) != 1 {
		t.Fatal("committed savepoint did not publish workspace effects")
	}
}
