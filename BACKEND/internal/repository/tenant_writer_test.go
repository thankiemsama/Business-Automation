package repository

import (
	"errors"
	"path/filepath"
	"testing"

	"businessautomation/internal/core"
)

func TestFileTenantTransaction(t *testing.T) {
	file, err := Open(filepath.Join(t.TempDir(), "tenant.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkTenantTransaction(t, file, file)
	checkTenantRead(t, file)
}

func checkTenantTransaction(t *testing.T, repo core.Repository, scoped core.TenantUpdater) {
	t.Helper()
	if err := repo.Update(func(w core.Writer) error {
		w.PutUser(core.User{ID: "scope-user-b", TenantID: "scope-b"})
		w.PutRecord(core.Record{ID: "scope-record-b", TenantID: "scope-b", Kind: core.KindTickets, Subject: "original"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := scoped.UpdateTenant("scope-a", func(w core.Writer) error {
		if _, ok := w.UserByID("scope-user-b"); ok {
			t.Error("foreign user visible")
		}
		w.PutRecord(core.Record{ID: "discard-me", TenantID: "scope-a", Kind: core.KindTickets})
		w.PutRecord(core.Record{ID: "scope-record-b", TenantID: "scope-b", Kind: core.KindTickets, Subject: "overwritten"})
		return nil
	})
	if !errors.Is(err, errTenantScope) {
		t.Fatalf("cross-tenant write should fail, got %v", err)
	}
	if err := scoped.UpdateTenant("scope-a", func(w core.Writer) error {
		if err := w.Savepoint(func(inner core.Writer) error {
			inner.Record("scope-b", "scope-record-b", core.KindTickets)
			return nil
		}); !errors.Is(err, errTenantScope) {
			t.Errorf("savepoint must enforce tenant scope, got %v", err)
		}
		w.PutRecord(core.Record{ID: "keep-me", TenantID: "scope-a", Kind: core.KindTickets})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.View(func(r core.Reader) error {
		if _, ok := r.Record("scope-a", "discard-me", core.KindTickets); ok {
			t.Error("cross-tenant error let a partial write commit")
		}
		if _, ok := r.Record("scope-a", "keep-me", core.KindTickets); !ok {
			t.Error("valid write missing after savepoint")
		}
		if foreign, _ := r.Record("scope-b", "scope-record-b", core.KindTickets); foreign.Subject != "original" {
			t.Error("foreign record changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func checkTenantLocks(t *testing.T, first, second *Postgres) {
	t.Helper()
	if err := first.UpdateTenant("scope-a", func(w core.Writer) error {
		store := w.(*tenantWriter).store.(*sqlStore)
		tenant, _ := scanOne[string](store, "SELECT current_setting('app.tenant_id', true)")
		if tenant != "scope-a" {
			t.Errorf("transaction has scope %q", tenant)
		}
		tx, err := second.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var same, independent, global bool
		err = tx.QueryRow("SELECT pg_try_advisory_xact_lock($1::integer, hashtext('scope-a')), pg_try_advisory_xact_lock($1::integer, hashtext('scope-b')), pg_try_advisory_xact_lock($1::bigint)", compatibilityWriteLock).Scan(&same, &independent, &global)
		if err == nil && (same || !independent || global) {
			t.Errorf("unexpected lock behavior: same=%t independent=%t global=%t", same, independent, global)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func checkTenantRead(t *testing.T, repo core.TenantViewer) {
	t.Helper()
	if err := repo.ViewTenant("scope-b", func(r core.Reader) error {
		if _, writer := r.(core.Writer); writer {
			t.Error("read callback exposes a writer")
		}
		if row, ok := r.Record("scope-b", "scope-record-b", core.KindTickets); !ok || row.Subject != "original" {
			t.Errorf("own record not found: %+v", row)
		}
		if db, ok := r.(*tenantReader).store.(*sqlStore); ok {
			tenant, _ := scanOne[string](db, "SELECT current_setting('app.tenant_id', true)")
			if tenant != "scope-b" {
				t.Errorf("SQL read scope = %q", tenant)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ViewTenant("scope-a", func(r core.Reader) error {
		if _, found := r.Record("scope-b", "scope-record-b", core.KindTickets); found {
			t.Error("foreign ticket exposed")
		}
		return nil
	}); !errors.Is(err, errTenantScope) {
		t.Fatalf("expected cross-tenant read failure, got %v", err)
	}
	if db, ok := repo.(*Postgres); ok {
		if err := db.View(func(r core.Reader) error {
			scope, _ := scanOne[string](r.(*sqlStore), "SELECT COALESCE(current_setting('app.tenant_id', true), '')")
			if scope != "" {
				t.Errorf("scope leaked beyond transaction: %q", scope)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
