ALTER TABLE platform_records
    DROP CONSTRAINT platform_records_pkey,
    ADD PRIMARY KEY (tenant_id, id);
