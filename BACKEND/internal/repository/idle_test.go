package repository

import (
	"path/filepath"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestFileIdleTickets(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkIdleTickets(t, store)
}

func checkIdleTickets(t *testing.T, repo core.Repository) {
	t.Helper()
	cutoff := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.Update(func(w core.Writer) error {
		for _, record := range []core.Record{
			{ID: "idle-boundary", TenantID: "idle-test", Kind: core.KindTickets, CreatedAt: cutoff},
			{ID: "idle-active-ticket", TenantID: "idle-test", Kind: core.KindTickets, CreatedAt: cutoff, UpdatedAt: cutoff.Add(time.Hour)},
			{ID: "idle-link", TenantID: "idle-test", Kind: core.KindTickets, CreatedAt: cutoff, ConversationID: "idle-conversation"},
			{ID: "idle-conversation", TenantID: "idle-test", Kind: core.KindConversations, UpdatedAt: cutoff.Add(time.Hour)},
			{ID: "idle-resolved", TenantID: "idle-test", Kind: core.KindTickets, CreatedAt: cutoff, Status: core.StatusResolved},
			{ID: "idle-other-tenant", TenantID: "other", Kind: core.KindTickets, CreatedAt: cutoff},
			{ID: "idle-foreign-link", TenantID: "idle-test", Kind: core.KindTickets, CreatedAt: cutoff, ConversationID: "idle-foreign-conversation"},
			{ID: "idle-foreign-conversation", TenantID: "other", Kind: core.KindConversations, UpdatedAt: cutoff.Add(time.Hour)},
		} {
			w.PutRecord(record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.View(func(r core.Reader) error {
		items := r.IdleTickets("idle-test", cutoff)
		if len(items) != 2 || items[0].ID != "idle-boundary" || items[1].ID != "idle-foreign-link" {
			t.Errorf("unexpected idle candidates: %+v", items)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
