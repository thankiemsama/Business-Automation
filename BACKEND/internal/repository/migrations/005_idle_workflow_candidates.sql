CREATE INDEX platform_records_idle_ticket_candidates_idx
    ON platform_records (workflow_activity_at, tenant_id, id)
    WHERE kind = 'tickets' AND workflow_activity_at IS NOT NULL;

CREATE INDEX platform_records_idle_workflow_jobs_idx
    ON platform_records (tenant_id, (data->>'automation_id'), (data->>'conversation_id'))
    WHERE kind = 'workflow_jobs' AND data->>'subject' = 'ticket_idle'
      AND COALESCE(data->>'status', '') <> 'postponed';
