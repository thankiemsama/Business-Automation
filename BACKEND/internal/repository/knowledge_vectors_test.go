package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestKnowledgeVectorsPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires an isolated PostgreSQL database with pgvector")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	var available bool
	if err := admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector')").Scan(&available); err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("pgvector is unavailable")
	}
	schema := fmt.Sprintf("platform_vector_test_%d", time.Now().UnixNano())
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
	repo, err := OpenPostgres(ctx, scoped.String())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.EnableKnowledgeVectors(ctx); err != nil {
		t.Fatal(err)
	}
	parallel, orthogonal := make([]float64, 256), make([]float64, 256)
	parallel[0], orthogonal[1] = 1, 1
	_, err = repo.RankKnowledgeVectors("other", []string{"private"}, map[string][]float64{"private": parallel}, parallel, 5)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"private", "good", "irrelevant"}
	matches, err := repo.RankKnowledgeVectors("tenant", keys, map[string][]float64{"good": parallel, "irrelevant": orthogonal}, parallel, 5)
	if err != nil || len(matches) != 2 || matches[0].Key != "good" || matches[0].Score != 1 || matches[1].Score != 0 {
		t.Fatal(matches, err)
	}
	found, err := repo.HasKnowledgeVectors("tenant", keys)
	if err != nil || len(found) != 2 || found["private"] || !found["good"] || !found["irrelevant"] {
		t.Fatal(found, err)
	}
}
