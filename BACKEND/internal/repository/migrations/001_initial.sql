CREATE TABLE platform_users (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    email text NOT NULL,
    data jsonb NOT NULL
);
CREATE INDEX platform_users_tenant_idx ON platform_users (tenant_id);
CREATE INDEX platform_users_email_idx ON platform_users (email);

CREATE TABLE platform_sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    data jsonb NOT NULL
);
CREATE INDEX platform_sessions_user_idx ON platform_sessions (user_id);
CREATE INDEX platform_sessions_expiry_idx ON platform_sessions (expires_at);

CREATE TABLE platform_records (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    kind text NOT NULL,
    pending_email_at timestamptz,
    data jsonb NOT NULL
);
CREATE INDEX platform_records_tenant_kind_idx ON platform_records (tenant_id, kind);
CREATE INDEX platform_records_conversation_messages_idx ON platform_records (tenant_id, (data->>'conversation_id'))
    WHERE kind = 'messages';
CREATE INDEX platform_records_due_email_idx ON platform_records (pending_email_at, id)
    WHERE pending_email_at IS NOT NULL;

CREATE TABLE platform_imports (
    source text PRIMARY KEY,
    imported_at timestamptz NOT NULL DEFAULT now()
);
