ALTER TABLE platform_records ADD COLUMN pending_workflow_at timestamptz;

UPDATE platform_records
SET pending_workflow_at = COALESCE((data->>'created_at')::timestamptz, '0001-01-01 00:00:00+00'::timestamptz)
WHERE kind = 'workflow_jobs' AND data->'enabled' = 'true'::jsonb;

CREATE INDEX platform_records_pending_workflow_idx
    ON platform_records (pending_workflow_at, id, tenant_id)
    WHERE pending_workflow_at IS NOT NULL;
