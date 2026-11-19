package repository

import (
	"errors"
	"path/filepath"
	"testing"

	"businessautomation/internal/core"
)

func TestFileSavepoint(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkSavepoint(t, repo)
}

func checkSavepoint(t *testing.T, repo core.Repository) {
	t.Helper()
	cancel := errors.New("cancel sub-operation")
	record := func(id string) core.Record {
		return core.Record{ID: id, TenantID: "savepoint-test", Kind: core.KindTickets}
	}
	if err := repo.Update(func(w core.Writer) error {
		w.PutRecord(record("before"))
		err := w.Savepoint(func(inner core.Writer) error {
			inner.PutRecord(record("discarded"))
			return cancel
		})
		if !errors.Is(err, cancel) {
			t.Errorf("want rollback reason, got %v", err)
		}
		if err := w.Savepoint(func(inner core.Writer) error {
			inner.PutRecord(record("kept"))
			return nil
		}); err != nil {
			return err
		}
		w.PutRecord(record("after"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.View(func(r core.Reader) error {
		for _, id := range []string{"before", "after", "kept"} {
			if _, ok := r.Record("savepoint-test", id, core.KindTickets); !ok {
				t.Errorf("record %s disappeared after sub-operation", id)
			}
		}
		if _, ok := r.Record("savepoint-test", "discarded", core.KindTickets); ok {
			t.Error("failed sub-operation escaped rollback")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func checkSQLSavepointError(t *testing.T, repo *Postgres) {
	t.Helper()
	if err := repo.Update(func(w core.Writer) error {
		err := w.Savepoint(func(inner core.Writer) error {
			inner.(*sqlStore).exec("SELECT 1 / 0")
			return nil
		})
		if err == nil {
			t.Error("SQL failure was ignored")
		}
		w.PutRecord(core.Record{ID: "recovered", TenantID: "savepoint-test", Kind: core.KindTickets})
		return nil
	}); err != nil {
		t.Fatal("write after recovering a failed SQL statement:", err)
	}
}
