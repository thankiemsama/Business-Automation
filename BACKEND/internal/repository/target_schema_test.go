package repository

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

//go:embed migrations/0002_rls_partition.sql
var targetSchemaDDL string

//go:embed migrations/0003_vector.sql
var targetVectorDDL string

func TestTargetSchemaRLSAndPartitions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL database with CREATEROLE")
	}
	checkTargetSchema(t, dsn, false)
}

func checkTargetSchema(t *testing.T, dsn string, requireVector bool) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	schema := fmt.Sprintf("target_w1_%d", time.Now().UnixNano())
	role := pq.QuoteIdentifier(schema + "_app")
	namespace := pq.QuoteIdentifier(schema)
	exec := func(statement string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE ROLE " + role + " NOLOGIN NOBYPASSRLS")
	defer func() {
		for _, statement := range []string{"DROP SCHEMA IF EXISTS " + namespace + " CASCADE", "DROP ROLE " + role} {
			if _, err := db.Exec(statement); err != nil {
				t.Error(err)
			}
		}
	}()
	migration, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer migration.Rollback()
	if _, err := migration.ExecContext(ctx, strings.ReplaceAll(targetSchemaDDL, "platform_v2", schema)); err != nil {
		t.Fatal(err)
	}
	if err := migration.Commit(); err != nil {
		t.Fatal(err)
	}
	var tables, partitioned int
	err = db.QueryRowContext(ctx, "SELECT count(*) FILTER (WHERE c.relkind IN ('r','p') AND NOT c.relispartition AND c.relrowsecurity AND c.relforcerowsecurity), count(*) FILTER (WHERE c.relkind = 'p') FROM pg_class c JOIN pg_namespace n ON c.relnamespace = n.oid WHERE n.nspname = $1", schema).Scan(&tables, &partitioned)
	if err != nil || tables != 17 || partitioned != 3 {
		t.Fatalf("tables=%d partitioned=%d err=%v", tables, partitioned, err)
	}
	exec("GRANT USAGE ON SCHEMA " + namespace + " TO " + role)
	for _, table := range []string{"tenants", "customers", "conversations", "messages"} {
		exec("GRANT SELECT, INSERT ON " + namespace + "." + table + " TO " + role)
	}

	run := func(tenant string, fn func(*sql.Tx) error) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE "+role); err != nil {
			return err
		}
		if tenant != "" {
			if _, err := tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
				return err
			}
		}
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	for _, tenant := range []string{"a", "b"} {
		if err := run(tenant, func(tx *sql.Tx) error {
			for _, statement := range []string{
				"INSERT INTO " + namespace + ".tenants (id,name) VALUES ($1,'Test')",
				"INSERT INTO " + namespace + ".customers (tenant_id,id,name) VALUES ($1,'customer','Test')",
				"INSERT INTO " + namespace + ".conversations (tenant_id,id,customer_id,channel,status) VALUES ($1,'thread','customer','web','open')",
				"INSERT INTO " + namespace + ".messages (tenant_id,id,conversation_id,sender,content,sequence) VALUES ($1,'current','thread','note','Test',1)",
				"INSERT INTO " + namespace + ".messages (tenant_id,id,conversation_id,sender,content,sequence,created_at) VALUES ($1,'historical','thread','agent','Test',2,'2000-01-01 00:00:00+00')",
			} {
				if _, err := tx.ExecContext(ctx, statement, tenant); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for scope, want := range map[string]int{"a": 2, "b": 2, "": 0} {
		if err := run(scope, func(tx *sql.Tx) error {
			var count int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+namespace+".messages").Scan(&count); err != nil {
				return err
			}
			if count != want {
				return fmt.Errorf("scope %q sees %d messages, expected %d", scope, count, want)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	err = run("a", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO "+namespace+".customers (tenant_id,id,name) VALUES ('b','blocked','Test')")
		return err
	})
	var pgErr *pq.Error
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("expected RLS to reject cross-tenant insert, got %v", err)
	}

	t.Run("pgvector", func(t *testing.T) {
		var available bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector')").Scan(&available); err != nil {
			t.Fatal(err)
		}
		if !available {
			if requireVector {
				t.Fatal("Testcontainers PostgreSQL image must provide pgvector")
			}
			t.Skip("PostgreSQL instance has no pgvector")
		}
		if _, err := db.ExecContext(ctx, strings.ReplaceAll(targetVectorDDL, "platform_v2", schema)); err != nil {
			t.Fatal(err)
		}
	})
}
