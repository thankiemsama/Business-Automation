ALTER TABLE platform_records ADD COLUMN workflow_activity_at timestamptz;

UPDATE platform_records
SET workflow_activity_at = CASE
    WHEN kind = 'tickets' THEN COALESCE(
        NULLIF((data->>'updated_at')::timestamptz, '0001-01-01 00:00:00+00'::timestamptz),
        (data->>'created_at')::timestamptz, '0001-01-01 00:00:00+00'::timestamptz)
    ELSE (data->>'updated_at')::timestamptz
END
WHERE kind = 'conversations' OR (kind = 'tickets' AND data->>'status' IS DISTINCT FROM 'resolved');

CREATE INDEX platform_records_idle_ticket_idx
    ON platform_records (tenant_id, workflow_activity_at, id)
    WHERE kind = 'tickets' AND workflow_activity_at IS NOT NULL;
