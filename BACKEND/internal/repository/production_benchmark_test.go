package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"github.com/lib/pq"
)

func checkTenantRecordKey(t *testing.T, repo core.Repository) {
	t.Helper()
	if err := repo.Update(func(w core.Writer) error {
		w.PutRecord(core.Record{TenantID: "key-a", ID: "same-id", Kind: core.KindTickets, Subject: "changed"})
		w.PutRecord(core.Record{TenantID: "key-b", ID: "same-id", Kind: core.KindTickets, Subject: "independent"})
		w.PutRecord(core.Record{TenantID: "key-a", ID: "same-id", Kind: core.KindTickets, Subject: "updated"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.View(func(r core.Reader) error {
		a, okA := r.Record("key-a", "same-id", core.KindTickets)
		b, okB := r.Record("key-b", "same-id", core.KindTickets)
		if !okA || !okB || a.Subject != "updated" || b.Subject != "independent" || a.Version != 2 || b.Version != 1 {
			return fmt.Errorf("tenant-scoped update or version mismatch: A=%+v B=%+v", a, b)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSingleTicketWrite100k(b *testing.B) {
	fixture, err := Open(filepath.Join(b.TempDir(), "state.json"))
	if err != nil {
		b.Fatal(err)
	}
	content := strings.Repeat("sample ticket content ", 8)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := fixture.Update(func(w core.Writer) error {
		for i := range 100_000 {
			w.PutRecord(core.Record{
				ID: fmt.Sprintf("ticket-%06d", i), TenantID: fmt.Sprintf("tenant-%03d", i%100),
				Kind: core.KindTickets, Subject: "Support request", Content: content,
				Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: at, UpdatedAt: at,
			})
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}

	b.Run("File", func(b *testing.B) { benchmarkSingleTicketWrite(b, fixture, at) })
	b.Run("Postgres", func(b *testing.B) {
		dsn := os.Getenv("TEST_DATABASE_URL")
		if dsn == "" {
			b.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL database URL")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		admin, err := sql.Open("postgres", dsn)
		if err != nil {
			b.Fatal(err)
		}
		defer admin.Close()
		schema := fmt.Sprintf("platform_bench_%d", time.Now().UnixNano())
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
			b.Fatal(err)
		}
		defer func() {
			if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
				b.Error(err)
			}
		}()
		scoped, err := url.Parse(dsn)
		if err != nil {
			b.Fatal(err)
		}
		options := scoped.Query()
		options.Set("search_path", schema)
		scoped.RawQuery = options.Encode()
		repo, err := OpenPostgres(ctx, scoped.String())
		if err != nil {
			b.Fatal(err)
		}
		defer repo.Close()
		counts, err := repo.ImportFile(ctx, fixture.path)
		if err != nil || counts.Records != 100_000 {
			b.Fatalf("load benchmark fixture: counts=%+v err=%v", counts, err)
		}
		benchmarkSingleTicketWrite(b, repo, at)
	})
}

func benchmarkSingleTicketWrite(b *testing.B, repo core.Repository, at time.Time) {
	b.Helper()
	samples := make([]time.Duration, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		start := time.Now()
		if err := repo.Update(func(w core.Writer) error {
			record, ok := w.Record("tenant-000", "ticket-000000", core.KindTickets)
			if !ok {
				return fmt.Errorf("benchmark ticket missing")
			}
			record.UpdatedAt = at.Add(time.Duration(i+1) * time.Second)
			w.PutRecord(record)
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		samples[i] = time.Since(start)
	}
	b.StopTimer()
	slices.Sort(samples)
	index := len(samples) - 1 - len(samples)/20
	b.ReportMetric(float64(samples[index])/float64(time.Millisecond), "p95-ms")
}
