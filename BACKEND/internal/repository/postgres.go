package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/telemetry"

	_ "github.com/lib/pq"
)

// Postgres stores one row per user, session or business record.
type Postgres struct{ db *sql.DB }

//go:embed migrations/001_initial.sql
var initialSchema string

//go:embed migrations/002_workflow_activity.sql
var workflowActivitySchema string

//go:embed migrations/003_tenant_record_key.sql
var tenantRecordKeySchema string

//go:embed migrations/004_pending_workflow_jobs.sql
var pendingWorkflowSchema string

// Write callbacks can enforce invariants across several rows. Serialize those callbacks
// across processes until the repository contract supports narrower locks.
const compatibilityWriteLock = 771144931

func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	p := &Postgres{db: db}
	if err := p.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	return p, nil
}

func (p *Postgres) Close() error { return p.db.Close() }

func (p *Postgres) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

func (p *Postgres) migrate(ctx context.Context) error {
	return p.transaction(ctx, false, "", func(s *sqlStore) error {
		s.exec("CREATE TABLE IF NOT EXISTS platform_schema_migrations (version integer PRIMARY KEY)")
		for index, migration := range []string{initialSchema, workflowActivitySchema, tenantRecordKeySchema, pendingWorkflowSchema} {
			version := index + 1
			exists, _ := scanOne[bool](s, "SELECT EXISTS (SELECT 1 FROM platform_schema_migrations WHERE version = $1)", version)
			if !exists {
				s.exec(migration)
				s.exec("INSERT INTO platform_schema_migrations VALUES ($1)", version)
			}
		}
		return nil
	})
}

func (p *Postgres) View(fn func(core.Reader) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.transaction(ctx, true, "", func(s *sqlStore) error { return fn(s) })
}

func (p *Postgres) Update(fn func(core.Writer) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.transaction(ctx, false, "", func(s *sqlStore) error { return fn(s) })
}

func (p *Postgres) ViewTenant(tenantID string, run func(core.Reader) error) error {
	if tenantID == "" {
		return errTenantScope
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.transaction(ctx, true, tenantID, func(s *sqlStore) error {
		return withTenantReader(s, tenantID, run)
	})
}

func (p *Postgres) UpdateTenant(tenantID string, run func(core.Writer) error) error {
	if tenantID == "" {
		return errTenantScope
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.transaction(ctx, false, tenantID, func(s *sqlStore) error {
		return withTenantWriter(s, tenantID, run)
	})
}

func (p *Postgres) transaction(ctx context.Context, readOnly bool, tenantID string, fn func(*sqlStore) error) error {
	options := sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: readOnly}
	if readOnly {
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, err := p.db.BeginTx(ctx, &options)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	store := &sqlStore{tx: tx, ctx: ctx}
	if !readOnly {
		started := time.Now()
		if tenantID == "" {
			store.exec("SELECT pg_advisory_xact_lock($1::bigint)", compatibilityWriteLock)
		} else {
			// Scoped writers share the compatibility gate while legacy writes still take it exclusively.
			store.exec("SELECT pg_advisory_xact_lock_shared($1::bigint)", compatibilityWriteLock)
			store.exec("SELECT pg_advisory_xact_lock($1::integer, hashtext($2))", compatibilityWriteLock, tenantID)
		}
		telemetry.ObserveWriteLock(time.Since(started), store.err == nil)
		if store.err != nil {
			return store.err
		}
	}
	if tenantID != "" {
		store.exec("SELECT set_config('app.tenant_id', $1, true)", tenantID)
		if store.err != nil {
			return store.err
		}
	}
	callbackErr := fn(store)
	if store.err != nil {
		return store.err
	}
	if callbackErr != nil {
		return callbackErr
	}
	return tx.Commit()
}

type ImportCounts struct{ Users, Sessions, Records int }

// ImportFile restores a legacy snapshot into an empty database in one transaction.
func (p *Postgres) ImportFile(ctx context.Context, path string) (ImportCounts, error) {
	source, err := openFile(path, false)
	if err != nil {
		return ImportCounts{}, err
	}
	state := source.state
	counts := ImportCounts{len(state.Users), len(state.Sessions), len(state.Records)}
	err = p.transaction(ctx, false, "", func(s *sqlStore) error {
		occupied, _ := scanOne[bool](s, `SELECT EXISTS (SELECT 1 FROM platform_imports)
			OR EXISTS (SELECT 1 FROM platform_users) OR EXISTS (SELECT 1 FROM platform_sessions)
			OR EXISTS (SELECT 1 FROM platform_records)`)
		if occupied {
			return errors.New("JSON import requires an empty PostgreSQL database that has not been imported into")
		}
		for id, user := range state.Users {
			if id != user.ID {
				return errors.New("user key does not match JSON snapshot")
			}
			s.PutUser(user)
		}
		for id, session := range state.Sessions {
			s.PutSession(id, session)
		}
		for id, record := range state.Records {
			if id != record.ID {
				return errors.New("record key does not match JSON snapshot")
			}
			s.exec("INSERT INTO platform_records (id, tenant_id, kind, pending_email_at, data, workflow_activity_at, pending_workflow_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
				id, record.TenantID, record.Kind, pendingEmailAt(record), encodeJSON(s, record), workflowActivityAt(record), pendingWorkflowAt(record))
		}
		s.exec("INSERT INTO platform_imports (source) VALUES ('legacy-json')")
		return nil
	})
	if err != nil {
		return ImportCounts{}, err
	}
	return counts, nil
}

var _ core.Repository = (*Postgres)(nil)
