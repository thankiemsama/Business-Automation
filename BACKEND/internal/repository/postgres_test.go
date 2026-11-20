package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"businessautomation/internal/core"

	"github.com/lib/pq"
)

func TestPostgresImportAndConcurrentTransactions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a PostgreSQL test database URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("platform_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	scoped, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	options := scoped.Query()
	options.Set("search_path", schema)
	scoped.RawQuery = options.Encode()

	source := filepath.Join(t.TempDir(), "legacy.json")
	const legacy = `{"users":{"u":{"id":"u","tenant_id":"t","email":"u@example.test","password_hash":"hash"}},
		"sessions":{"s":{"UserID":"u","ExpiresAt":"2030-01-01T00:00:00Z"}},
		"records":{"ticket":{"id":"ticket","tenant_id":"t","kind":"tickets","subject":"restored","tags":["urgent"]},
		"message":{"id":"message","tenant_id":"t","kind":"messages","conversation_id":"conversation",
		"sender":"agent","status":"pending","email":"customer@example.test","created_at":"2026-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(source, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := OpenPostgres(ctx, scoped.String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	broken := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(broken, []byte(strings.Replace(legacy, `"id":"ticket"`, `"id":"mismatched"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := first.ImportFile(ctx, broken); err == nil {
		t.Fatal("import should roll back when a record key does not match")
	}
	if counts, err := first.ImportFile(ctx, source); err != nil || counts != (ImportCounts{1, 1, 2}) {
		t.Fatalf("import counts = %+v, err = %v", counts, err)
	}
	second, err := OpenPostgres(ctx, scoped.String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.ImportFile(ctx, source); err == nil {
		t.Fatal("repeated import should fail")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := second.View(func(store core.Reader) error {
		ticket, ok := store.Record("t", "ticket", core.KindTickets)
		if !ok || ticket.Version != 1 || ticket.Subject != "restored" || len(ticket.Tags) != 1 {
			t.Errorf("unexpected restored ticket: %+v", ticket)
		}
		if _, exposed := store.Record("other", "ticket", core.KindTickets); exposed {
			t.Error("cross-tenant read")
		}
		if user, ok := store.UserByEmail("u@example.test"); !ok || user.PasswordHash != "hash" {
			t.Error("user not restored")
		}
		if _, ok := store.Sessions("u")["s"]; !ok {
			t.Error("session not restored")
		}
		if len(store.ConversationMessages("t", "conversation")) != 1 || len(store.DueEmailReplies(now, 8)) != 1 {
			t.Error("pending email not restored")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	checkIdleTickets(t, second)
	checkIndexedIdleWorkflowCandidates(t, second)
	checkTenantRecordKey(t, first)
	checkWorkflowQueue(t, first)
	checkLockedWorkflowJob(t, first, second)
	checkSavepoint(t, first)
	checkSQLSavepointError(t, first)
	checkTenantTransaction(t, first, first)
	checkTenantRead(t, first)
	checkTenantLocks(t, first, second)

	rollback := errors.New("rollback")
	if err := first.Update(func(store core.Writer) error {
		store.DeleteSession("s")
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("expected rollback, got %v", err)
	}
	var group sync.WaitGroup
	for i := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := []*Postgres{first, second}[i%2].Update(func(store core.Writer) error {
				ticket, _ := store.Record("t", "ticket", core.KindTickets)
				ticket.Sequence++
				store.PutRecord(ticket)
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if err := first.View(func(store core.Reader) error {
		ticket, _ := store.Record("t", "ticket", core.KindTickets)
		if ticket.Sequence != 8 || ticket.Version != 9 {
			t.Errorf("lost update: sequence = %d, version = %d", ticket.Sequence, ticket.Version)
		}
		if _, ok := store.Session("s"); !ok {
			t.Error("transaction rollback lost session")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
