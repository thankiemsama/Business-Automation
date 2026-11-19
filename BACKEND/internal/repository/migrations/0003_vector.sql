-- W1 target schema, applied after 0002_rls_partition.sql.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
ALTER TABLE platform_v2.knowledge_articles ADD COLUMN embedding public.vector(1536);
CREATE INDEX idx_kb_embedding ON platform_v2.knowledge_articles
    USING hnsw (embedding public.vector_cosine_ops)
    WHERE enabled AND embedding IS NOT NULL;
