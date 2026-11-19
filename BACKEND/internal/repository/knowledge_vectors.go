package repository

import (
	"context"
	"encoding/json"
	"time"

	"businessautomation/internal/core"

	"github.com/lib/pq"
)

// EnableKnowledgeVectors provisions the index only when semantic AI is configured.
func (p *Postgres) EnableKnowledgeVectors(ctx context.Context) error {
	return p.transaction(ctx, false, "", func(s *sqlStore) error {
		vectorSearchPath(s)
		s.exec(`CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
			CREATE TABLE IF NOT EXISTS platform_knowledge_vectors (
				tenant_id text NOT NULL, fingerprint text NOT NULL, embedding vector(256) NOT NULL,
				PRIMARY KEY (tenant_id, fingerprint));
			ALTER TABLE platform_knowledge_vectors ENABLE ROW LEVEL SECURITY;
			ALTER TABLE platform_knowledge_vectors FORCE ROW LEVEL SECURITY;
			CREATE INDEX IF NOT EXISTS platform_knowledge_vectors_cosine ON platform_knowledge_vectors
				USING ivfflat (embedding vector_cosine_ops) WITH (lists = 1)`)
		var exists bool
		exists, _ = scanOne[bool](s, `SELECT EXISTS (SELECT 1 FROM pg_policies
			WHERE schemaname = current_schema() AND tablename = 'platform_knowledge_vectors' AND policyname = 'tenant_vectors')`)
		if !exists {
			s.exec(`CREATE POLICY tenant_vectors ON platform_knowledge_vectors
				USING (tenant_id = current_setting('app.tenant_id', true))
				WITH CHECK (tenant_id = current_setting('app.tenant_id', true))`)
		}
		return nil
	})
}

func (p *Postgres) HasKnowledgeVectors(tenant string, keys []string) (map[string]bool, error) {
	found := make(map[string]bool, len(keys))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := p.transaction(ctx, true, tenant, func(s *sqlStore) error {
		rows, err := s.tx.QueryContext(ctx, `SELECT fingerprint FROM platform_knowledge_vectors
			WHERE tenant_id = $1 AND fingerprint = ANY($2)`, tenant, pq.Array(keys))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			found[key] = true
		}
		return rows.Err()
	})
	return found, err
}

func (p *Postgres) RankKnowledgeVectors(tenant string, keys []string, additions map[string][]float64, query []float64, limit int) ([]core.VectorMatch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var matches []core.VectorMatch
	err := p.transaction(ctx, len(additions) == 0, tenant, func(s *sqlStore) error {
		vectorSearchPath(s)
		for key, embedding := range additions {
			value, err := json.Marshal(embedding)
			if err != nil {
				return err
			}
			s.exec(`INSERT INTO platform_knowledge_vectors VALUES ($1, $2, $3::vector)
				ON CONFLICT (tenant_id, fingerprint) DO NOTHING`, tenant, key, string(value))
		}
		if s.err != nil {
			return s.err
		}
		value, err := json.Marshal(query)
		if err != nil {
			return err
		}
		rows, err := s.tx.QueryContext(ctx, `SELECT fingerprint, 1 - (embedding <=> $3::vector)
			FROM platform_knowledge_vectors WHERE tenant_id = $1 AND fingerprint = ANY($2)
			ORDER BY embedding <=> $3::vector LIMIT $4`, tenant, pq.Array(keys), string(value), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var match core.VectorMatch
			if err := rows.Scan(&match.Key, &match.Score); err != nil {
				return err
			}
			matches = append(matches, match)
		}
		return rows.Err()
	})
	return matches, err
}

func vectorSearchPath(s *sqlStore) {
	s.exec("SELECT set_config('search_path', current_setting('search_path') || ', public, extensions', true)")
}
