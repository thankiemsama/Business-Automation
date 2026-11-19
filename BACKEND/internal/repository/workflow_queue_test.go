package repository

import (
	"path/filepath"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestFileWorkflowQueue(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkWorkflowQueue(t, repo)
}

func checkWorkflowQueue(t *testing.T, repo core.Repository) {
	t.Helper()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.Update(func(w core.Writer) error {
		w.PutUser(core.User{ID: "queue-owner", TenantID: "workflow-test"})
		for _, job := range []core.Record{
			{TenantID: "workflow-test", ID: "workflow-a", CreatedAt: at, Enabled: true},
			{TenantID: "workflow-test", ID: "workflow-b", CreatedAt: at, Enabled: true},
			{TenantID: "workflow-test", ID: "workflow-history", CreatedAt: at.Add(-time.Hour)},
			{TenantID: "unknown-tenant", ID: "workflow-orphan", CreatedAt: at.Add(-time.Hour), Enabled: true},
		} {
			job.Kind = core.KindWorkflowJobs
			w.PutRecord(job)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	verify := func(limit int, want string) {
		t.Helper()
		if err := repo.View(func(r core.Reader) error {
			jobs := r.PendingWorkflowJobs(limit)
			if len(jobs) != 1 || jobs[0].ID != want {
				t.Errorf("want one job %q, got %+v", want, jobs)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	verify(1, "workflow-a")
	if err := repo.Update(func(w core.Writer) error {
		job, ok := w.LockWorkflowJob("workflow-test", "workflow-a")
		if !ok {
			t.Error("pending job not available")
		}
		job.Enabled = false
		w.PutRecord(job)
		if _, ok := w.LockWorkflowJob("workflow-test", "workflow-a"); ok {
			t.Error("finished job was offered again")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	verify(25, "workflow-b")
}

func checkLockedWorkflowJob(t *testing.T, first, second *Postgres) {
	t.Helper()
	blocker, err := first.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec("SELECT 1 FROM platform_records WHERE tenant_id = 'workflow-test' AND id = 'workflow-b' FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	if err := second.Update(func(w core.Writer) error {
		if _, ok := w.LockWorkflowJob("workflow-test", "workflow-b"); ok {
			t.Error("locked job was offered by the second connection")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
