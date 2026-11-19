-- W1 target schema. Apply in an explicit transaction during a dedicated W1 deployment.
-- The legacy adapter does not automatically apply this migration.
CREATE SCHEMA platform_v2;
SET LOCAL search_path = platform_v2, public;

CREATE TABLE tenants (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('owner','agent')),
  salt TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  totp_secret_enc TEXT,
  totp_step BIGINT NOT NULL DEFAULT 0,
  disabled BOOLEAN NOT NULL DEFAULT false,
  recovery_hashes JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_users_email ON users(lower(email));
ALTER TABLE users ADD UNIQUE (tenant_id, id);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_role ON users(tenant_id, role) WHERE disabled = false;
CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  user_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX idx_sessions_user ON sessions(user_id, expires_at);
CREATE TABLE customers (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  email TEXT,
  phone TEXT,
  tags TEXT[],
  custom_values JSONB,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_customers_email ON customers(tenant_id, lower(email)) WHERE email IS NOT NULL;
CREATE INDEX idx_customers_tags ON customers USING GIN(tags);
CREATE TABLE conversations (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  customer_id TEXT,
  channel TEXT NOT NULL CHECK (channel IN ('web','email','phone','zalo','whatsapp','sms','instagram')),
  status TEXT NOT NULL CHECK (status IN ('open','closed')),
  assignee_id TEXT,
  ai_summary TEXT,
  ai_summary_source TEXT,
  sentiment TEXT,
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_conversations_status ON conversations(tenant_id, status, updated_at DESC);
CREATE INDEX idx_conversations_customer ON conversations(tenant_id, customer_id);
CREATE INDEX idx_conversations_assignee ON conversations(tenant_id, assignee_id) WHERE assignee_id IS NOT NULL;
CREATE TABLE tickets (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  conversation_id TEXT,
  customer_id TEXT,
  subject TEXT NOT NULL,
  content TEXT,
  status TEXT NOT NULL CHECK (status IN ('open','in_progress','resolved')),
  priority TEXT NOT NULL CHECK (priority IN ('low','normal','high','urgent')),
  assignee_id TEXT,
  author_id TEXT,
  responded_by TEXT,
  resolved_by TEXT,
  automation_id TEXT,
  tags TEXT[],
  custom_values JSONB,
  first_response_at TIMESTAMPTZ,
  resolved_at TIMESTAMPTZ,
  response_due_at TIMESTAMPTZ,
  resolution_due_at TIMESTAMPTZ,
  escalated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
  FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_tickets_status ON tickets(tenant_id, status, priority, updated_at DESC);
CREATE INDEX idx_tickets_due ON tickets(tenant_id, status, response_due_at) WHERE status <> 'resolved';
CREATE INDEX idx_tickets_customer ON tickets(tenant_id, customer_id);
CREATE INDEX idx_tags ON tickets USING GIN(tags);
CREATE TABLE messages (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  conversation_id TEXT NOT NULL,
  sender TEXT NOT NULL CHECK (sender IN ('customer','agent','assistant','system','note')),
  content TEXT NOT NULL,
  attachment_key TEXT,
  attachment_name TEXT,
  custom_values JSONB,
  sequence BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id),
  PRIMARY KEY (tenant_id, id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_messages_conv ON messages(tenant_id, conversation_id, created_at, sequence);
CREATE TABLE knowledge_articles (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  public BOOLEAN NOT NULL DEFAULT false,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_kb_slug ON knowledge_articles(tenant_id, slug) WHERE public = true;
CREATE TABLE automations (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  keywords TEXT[] NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  migrated_from TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE TABLE workflow_rules (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  trigger TEXT NOT NULL CHECK (trigger IN ('message_received','ticket_created','ticket_idle','sla_breached')),
  idle_days INT,
  conditions JSONB NOT NULL,
  actions JSONB NOT NULL,
  branch JSONB,
  migrated_from TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_wf_rules_trigger ON workflow_rules(tenant_id, trigger, enabled);
CREATE TABLE workflow_jobs (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  rule_id TEXT NOT NULL,
  FOREIGN KEY (tenant_id, rule_id) REFERENCES workflow_rules(tenant_id, id),
  source_record_id TEXT,
  source_kind TEXT,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','processed','postponed','failed')),
  attempts INT NOT NULL DEFAULT 0,
  next_run_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_wf_jobs_due ON workflow_jobs(tenant_id, status, next_run_at) WHERE status = 'queued';
CREATE TABLE webhooks (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  events TEXT[] NOT NULL,
  secret_ciphertext TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_wh_tenant ON webhooks(tenant_id, enabled);
CREATE TABLE webhook_deliveries (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  webhook_id TEXT NOT NULL,
  FOREIGN KEY (tenant_id, webhook_id) REFERENCES webhooks(tenant_id, id) ON DELETE CASCADE,
  event TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','sent','retry','failed','dead')),
  attempts INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ,
  last_error TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_wh_deliveries_due ON webhook_deliveries(tenant_id, status, next_retry_at)
  WHERE status IN ('pending','retry');
CREATE TABLE webhook_dead_letters (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  delivery_id TEXT NOT NULL,
  event TEXT NOT NULL,
  payload JSONB NOT NULL,
  last_error TEXT,
  failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  replayed BOOLEAN NOT NULL DEFAULT false,
  FOREIGN KEY (tenant_id, delivery_id) REFERENCES webhook_deliveries (tenant_id, id),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_wh_dlq ON webhook_dead_letters(tenant_id, failed_at DESC);
CREATE TABLE notifications (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  user_id TEXT NOT NULL,
  type TEXT NOT NULL,
  resource_kind TEXT,
  resource_id TEXT,
  payload JSONB,
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_notif_unread ON notifications(tenant_id, user_id, created_at DESC) WHERE read_at IS NULL;
CREATE TABLE audit_events (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  actor_id TEXT,
  action TEXT NOT NULL,
  resource_kind TEXT,
  resource_id TEXT,
  before JSONB,
  after JSONB,
  ip INET,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_audit_tenant ON audit_events(tenant_id, created_at DESC);
CREATE TABLE custom_field_definitions (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text','number','boolean','select','date')),
  options JSONB,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

-- Cover the previous, current and next UTC months. The default partition
-- preserves older imports and writes that arrive before maintenance creates a partition.
DO $migration$
DECLARE
  parent_name text;
  month_start date;
  next_month date;
  current_month date := date_trunc('month', now() AT TIME ZONE 'UTC')::date;
  child_name text;
  offset_months int;
BEGIN
  FOREACH parent_name IN ARRAY ARRAY['messages', 'workflow_jobs', 'audit_events'] LOOP
    FOR offset_months IN -1..1 LOOP
      month_start := (current_month + make_interval(months => offset_months))::date;
      next_month := (month_start + interval '1 month')::date;
      child_name := parent_name || '_' || to_char(month_start, 'YYYYMM');
      EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        child_name, parent_name, month_start::text || ' 00:00:00+00', next_month::text || ' 00:00:00+00');
      EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', child_name);
      EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', child_name);
    END LOOP;
    child_name := parent_name || '_default';
    EXECUTE format('CREATE TABLE %I PARTITION OF %I DEFAULT', child_name, parent_name);
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', child_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', child_name);
  END LOOP;
END
$migration$;

-- API and tenant workers must use scoped transactions as a non-owner role
-- without BYPASSRLS. An unset or empty scope is denied.
DO $migration$
DECLARE
  rel text;
  tenant_column text;
BEGIN
  FOREACH rel IN ARRAY ARRAY['tenants', 'users', 'sessions', 'customers', 'conversations', 'tickets', 'messages', 'knowledge_articles', 'automations', 'workflow_rules', 'workflow_jobs', 'webhooks', 'webhook_deliveries', 'webhook_dead_letters', 'notifications', 'audit_events', 'custom_field_definitions'] LOOP
    tenant_column := CASE WHEN rel = 'tenants' THEN 'id' ELSE 'tenant_id' END;
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', rel);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', rel);
    EXECUTE format('CREATE POLICY tenant_scope ON %I USING (%I = NULLIF(current_setting(''app.tenant_id'', true), '''')) WITH CHECK (%I = NULLIF(current_setting(''app.tenant_id'', true), ''''))',
      rel, tenant_column, tenant_column);
  END LOOP;
END
$migration$;
