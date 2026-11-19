# AUDIT: Foundation kỹ thuật Nexa (Backend Go + File Repository)

**Ngày audit:** hôm nay
**Phạm vi:** `BACKEND/internal/repository/file.go`, `core/models.go`, `service/webhooks.go`, `service/workflow_jobs.go`, `service/workflow_rules.go`, `app/app.go`
**Kết luận:** Code hiện tại **sạch, có kỷ luật** (SOLID, UoW, atomic write, idempotency), nhưng **foundation không chịu được 100+ tenant production**. Mọi tính năng AI/vertical/ecosystem nên xây trên nền SQL + queue trước.

---

## 1. Hiện trạng (bằng chứng từ code)

| Lớp | File | Vấn đề chính |
|---|---|---|
| Persistence | `repository/file.go` | Toàn bộ state = 1 JSON file, đọc 1 lần lúc startup, giữ trong RAM. Mỗi `Update()` = `clone(state)` + `json.MarshalIndent` + atomic rename. |
| Schema | `core/models.go` | `Record` là **poly-table**: customer, conversation, ticket, message, knowledge, automation, webhook, job, notification… đều nằm chung 1 map, phân biệt bằng `Kind`. `AttachmentData string` = base64 trong DB. |
| Auth | `core/models.go` | `User.RecoveryHashes`, `TOTPSecret` lưu plaintext trong JSON. Không có hash cho secret. |
| Webhook | `service/webhooks.go` | Outbox pattern có, retry exponential (1m→64m, 8 lần), nhưng **1 process, 3s tick, batch 8**. Không DLQ, không fairness multi-tenant. |
| Workflow | `service/workflow_jobs.go` | `scheduleIdle` scan **toàn bộ tenant × ticket × rule** mỗi tick. `runJobs` batch 25, sort by CreatedAt → tenant có nhiều job "đói". |
| App | `app/app.go` | 6 goroutine background (workspace, admin, ai, webhooks, operations, csat) đều poll repo. 1 panic → mất in-memory state. |
| MFA | `core/models.go` | `TOTPSecret` plaintext. Khi lên Postgres phải chuyển sang encrypted blob. |

---

## 2. 10 rủi ro lớn nhất (kèm bằng chứng + cách fix)

### R1 — Chi phí I/O tăng O(n²) tổng hợp
**Bằng chứng:** `file.go:Update()` → `clone(repository.state)` rồi `persist(nextState)` serialize **toàn bộ** JSON. Với 100k records (messages chiếm đa số), mỗi update = 50–100MB JSON. 100 rps = **5–10GB/s I/O** → chết sau vài giờ.
**Fix:** SQL repository (Postgres), mỗi write = 1 row UPDATE, O(1).

### R2 — 1 process, không scale, không HA
**Bằng chứng:** `sync.RWMutex` in-process + 6 goroutine poll. Không thể chạy 2 replica (2 process đọc cùng file → race).
**Fix:** 2–4 replica + Postgres (leader/follower), queue NATS/Redis cho background jobs.

### R3 — Không có transaction ACID
**Bằng chứng:** `file.go:Update()` clone → mutate → persist → `repository.state = clone(nextState)`. Nếu panic giữa persist, in-memory lệch với file. Không có row-level isolation giữa các tenant.
**Fix:** Postgres transaction + savepoint cho UoW, `BEGIN … COMMIT` per write.

### R4 — Leak: "deleted" = `Enabled=false`
**Bằng chứng:** `webhooks.go:Remove` set `Enabled=false`, không xóa. `workflow_jobs.go:runJob` cũng vậy. Map không bao giờ shrink. 1000 tenant × 50 webhook × 8 retry = **hàng triệu zombie records** → RAM tăng vô hạn.
**Fix:** Bảng `webhook_deliveries` có TTL (purge sau 7 ngày), `workflow_jobs` partition by month + auto-drop.

### R5 — Không có migration
**Bằng chứng:** Đổi schema = patch JSON thủ công. 100 tenant production = rủi ro mất data.
**Fix:** `golang-migrate`, run trước deploy (init container), version-controlled.

### R6 — Không có observability
**Bằng chứng:** Không metrics, không tracing, không structured log (dùng `slog` chuẩn, không OpenTelemetry). Không biết SLO, debug incident khó.
**Fix:** OTel SDK (trace+metric), Prometheus, Grafana, Sentry, zerolog. 5 SLO: p95 API < 300ms, p99 webhook delivery < 5s, ticket SLA breach < 2%, queue depth < 100, error rate < 0.1%.

### R7 — Webhook backoff có ceiling, không DLQ
**Bằng chứng:** `webhooks.go:dispatch` → `time.Minute * time.Duration(1<<min(current.Sequence, 6))` = max 64 phút. Sau 8 lần = "failed" vĩnh viễn. Consumer down 1h → mất event.
**Fix:** DLQ bảng riêng + admin UI replay. Retry max 24h (exponential + jitter), sau đó chuyển `dead_letter`.

### R8 — Workflow scheduler O(T × N × R) mỗi tick
**Bằng chứng:** `workflow_jobs.go:scheduleIdle` → `for _, tenant := range store.Tenants() { for _, rule := range rules { for _, ticket := range tickets { … } } }`. 100 tenant × 500 ticket × 10 rule = **500k so sánh mỗi tick**. 10s/tick → CPU pin.
**Fix:** Index theo `(tenant_id, status, updated_at)` + `WHERE updated_at < now() - idle_days`. Query O(1) per tenant, không scan.

### R9 — Attachment base64 trong DB
**Bằng chứng:** `models.go:AttachmentData string` (32 KiB limit). 1000 attachments = 32MB bloat JSON. Không có CDN, không có separate storage.
**Fix:** MinIO/S3 (object storage), DB chỉ lưu `attachment_key`. Migration: base64 → upload S3 → delete base64.

### R10 — Multi-tenant isolation phụ thuộc service layer
**Bằng chứng:** Mọi query = `Records(tenant, kind)` → in-memory filter. Nếu có bug 1 dòng = cross-tenant leak. Khi lên Postgres, phải design lại (tenant_id + RLS + partition).
**Fix:** Row-Level Security (Postgres 15) + composite PK `(tenant_id, id)` + app set `app.tenant_id` per query.

---

## 3. Thiết kế SQL (Postgres 15+)

### 3.1 Schema (DDL)

```sql
-- ============ CORE ============
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
  email CITEXT NOT NULL UNIQUE,          -- toàn hệ thống (theo spec hiện có)
  role TEXT NOT NULL CHECK (role IN ('owner','agent')),
  salt TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  totp_secret_enc TEXT,                  -- encrypted (AGE/PQCipher), không plaintext
  totp_step BIGINT NOT NULL DEFAULT 0,
  disabled BOOLEAN NOT NULL DEFAULT false,
  recovery_hashes JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_role ON users(tenant_id, role) WHERE disabled = false;

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,           -- SHA-256 của opaque token
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id, expires_at);

-- ============ BUSINESS (thay poly Record) ============
CREATE TABLE customers (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  email CITEXT,
  phone TEXT,
  tags TEXT[],
  custom_values JSONB,
  notes TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_customers_email ON customers(tenant_id, email) WHERE email IS NOT NULL;
CREATE INDEX idx_customers_tags ON customers USING GIN(tags);

CREATE TABLE conversations (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
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
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_conversations_status ON conversations(tenant_id, status, updated_at DESC);
CREATE INDEX idx_conversations_customer ON conversations(tenant_id, customer_id);
CREATE INDEX idx_conversations_assignee ON conversations(tenant_id, assignee_id) WHERE assignee_id IS NOT NULL;

CREATE TABLE tickets (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
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
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_tickets_status ON tickets(tenant_id, status, priority, updated_at DESC);
CREATE INDEX idx_tickets_due ON tickets(tenant_id, status, response_due_at) WHERE status <> 'resolved';
CREATE INDEX idx_tickets_customer ON tickets(tenant_id, customer_id);
CREATE INDEX idx_tags ON tickets USING GIN(tags);

CREATE TABLE messages (
  id BIGINT GENERATED ALWAYS AS IDENTITY,
  tenant_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  sender TEXT NOT NULL CHECK (sender IN ('customer','agent','assistant','system')),
  content TEXT NOT NULL,
  attachment_key TEXT,                   -- S3/MinIO key, không base64
  attachment_name TEXT,
  custom_values JSONB,
  sequence BIGINT NOT NULL,               -- giữ thứ tự (theo spec hiện có)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
-- Partition monthly để bảng không phình
CREATE TABLE messages (
  id BIGINT GENERATED ALWAYS AS IDENTITY,
  tenant_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  sender TEXT NOT NULL,
  content TEXT NOT NULL,
  attachment_key TEXT,
  attachment_name TEXT,
  custom_values JSONB,
  sequence BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_messages_conv ON messages(tenant_id, conversation_id, created_at, sequence);

CREATE TABLE knowledge_articles (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  slug TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  public BOOLEAN NOT NULL DEFAULT false,
  embedding VECTOR(1536),                 -- pgvector, tối đa 100 bài gần nhất (theo spec)
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_kb_slug ON knowledge_articles(tenant_id, slug) WHERE public = true;
CREATE INDEX idx_kb_embedding ON knowledge_articles USING ivfflat (embedding vector_l2_ops);

CREATE TABLE automations (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  keywords TEXT[] NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  migrated_from TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

-- ============ WORKFLOW (thay poly Record) ============
CREATE TABLE workflow_rules (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  trigger TEXT NOT NULL CHECK (trigger IN ('message_received','ticket_created','ticket_idle','sla_breached')),
  idle_days INT,
  conditions JSONB NOT NULL,              -- {channel, priority, status, contains, keywords_any}
  actions JSONB NOT NULL,                  -- [{type, value}, …]
  branch JSONB,                            -- {conditions, then, otherwise}
  migrated_from TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX idx_wf_rules_trigger ON workflow_rules(tenant_id, trigger, enabled);

CREATE TABLE workflow_jobs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  rule_id TEXT NOT NULL REFERENCES workflow_rules(tenant_id, id),
  source_record_id TEXT,
  source_kind TEXT,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','processed','postponed','failed')),
  attempts INT NOT NULL DEFAULT 0,
  next_run_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wf_jobs_due ON workflow_jobs(tenant_id, status, next_run_at) WHERE status = 'queued';
-- Partition monthly (auto-drop sau 90 ngày)
CREATE TABLE workflow_jobs (
  …
) PARTITION BY RANGE (created_at);

-- ============ WEBHOOK (tách khỏi poly Record) ============
CREATE TABLE webhooks (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  events TEXT[] NOT NULL,
  secret_hash TEXT NOT NULL,             -- SHA-256, không plaintext
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wh_tenant ON webhooks(tenant_id, enabled);

CREATE TABLE webhook_deliveries (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
  event TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','sent','retry','failed','dead')),
  attempts INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ,
  last_error TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wh_deliveries_due ON webhook_deliveries(tenant_id, status, next_retry_at)
  WHERE status IN ('pending','retry');
-- TTL: purge sau 7 ngày (cron: DELETE WHERE updated_at < now() - interval '7 days')

CREATE TABLE webhook_dead_letters (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  delivery_id TEXT NOT NULL,
  event TEXT NOT NULL,
  payload JSONB NOT NULL,
  last_error TEXT,
  failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  replayed BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_wh_dlq ON webhook_dead_letters(tenant_id, failed_at DESC);

-- ============ NOTIFICATION (tách khỏi poly Record) ============
CREATE TABLE notifications (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  type TEXT NOT NULL,
  resource_kind TEXT,
  resource_id TEXT,
  payload JSONB,
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notif_unread ON notifications(tenant_id, user_id, created_at DESC) WHERE read_at IS NULL;

-- ============ AUDIT (mới, bắt buộc cho GDPR) ============
CREATE TABLE audit_events (
  id BIGINT GENERATED ALWAYS AS IDENTITY,
  tenant_id TEXT NOT NULL,
  actor_id TEXT,
  action TEXT NOT NULL,
  resource_kind TEXT,
  resource_id TEXT,
  before JSONB,
  after JSONB,
  ip INET,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_audit_tenant ON audit_events(tenant_id, created_at DESC);
-- Partition monthly, auto-drop sau 2 năm

-- ============ CUSTOM FIELDS (mở rộng) ============
CREATE TABLE custom_field_definitions (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  kind TEXT NOT NULL,                     -- 'customer','ticket','conversation'
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('text','number','boolean','select','date')),
  options JSONB,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);
-- Giá trị lưu trong custom_values JSONB của từng bảng (customers, tickets, conversations)
```

### 3.2 Row-Level Security (Postgres 15)

```sql
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_customers ON customers
  USING (tenant_id = current_setting('app.tenant_id')::text);

-- Lặp lại cho conversations, tickets, messages, knowledge_articles, …
```

App đặt tenant trước mỗi query:
```go
conn, _ := pool.Acquire(ctx)
defer conn.Release()
if _, err := conn.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil { … }
```

### 3.3 Partition

- `messages`: RANGE (created_at), monthly. 100k messages/tháng × 12 = quản lý được.
- `workflow_jobs`: RANGE (created_at), monthly, auto-drop 90 ngày.
- `audit_events`: RANGE (created_at), monthly, auto-drop 2 năm.
- `webhook_deliveries`: TTL purge 7 ngày (cron).

### 3.4 Index cần thiết (đảm bảo query O(log n), không scan)

| Query (hiện tại) | Index |
|---|---|
| `Records(tenant, kind)` | Composite PK `(tenant_id, id)` + `idx_kind_status` |
| Ticket chưa resolved theo priority | `idx_tickets_status (tenant_id, status, priority, updated_at DESC)` |
| SLA breach (overdue) | `idx_tickets_due (tenant_id, status, response_due_at)` |
| Tin nhắn theo hội thoại | `idx_messages_conv (tenant_id, conversation_id, created_at, sequence)` |
| Workflow idle | `idx_wf_rules_trigger` + `tickets.updated_at < now() - idle_days` |
| Webhook pending | `idx_wh_deliveries_due` (partial index) |
| Tag/keyword search | `GIN(tags)`, `GIN(keywords)` |
| Embedding (semantic) | `ivfflat (embedding vector_l2_ops)` (pgvector) |

### 3.5 Queue + Outbox

- **Broker:** NATS JetStream (Go-native, nhẹ) hoặc Redis Streams (nếu đã có Redis).
- **Outbox pattern:** Trong transaction nghiệp vụ, insert row vào `webhook_deliveries` / `workflow_jobs`. Poller claim bằng `SELECT … FOR UPDATE SKIP LOCKED`, retry exponential + jitter, max 24h, sau đó chuyển `dead_letters`.
- **Workers:** 5 concurrent per broker, HPA theo queue depth.
- **Fairness:** Round-robin per tenant (không tenant nào "đói" như hiện tại).

---

## 4. Migration plan (40 ngày, 6 tuần)

| Tuần | Track | Việc | Deliverable |
|---|---|---|---|
| **W1** | Schema | DDL + golang-migrate (15 bảng, RLS, partition) | `migrations/0001_init.up.sql`, testcontainers-go |
| | | `internal/repository/sql.go` (Postgres adapter, giữ UoW) | Same interface `Repository`, drop-in thay file.go |
| | | Regression: 13 test functions + new SQL tests | CI green |
| **W2** | SQL → prod | Cutover file → Postgres (blue-green, dual-read 48h) | 0 P0, p95 < 500ms |
| | | Object storage (MinIO/S3) cho attachments, migration base64 → key | `attachment_key` |
| | | Load test k6: 500 rps, 1000 users, 24h | Report: p95, p99, error rate |
| **W3** | Queue | NATS JetStream + outbox + DLQ | Webhook + workflow qua broker |
| | | Fairness per-tenant (round-robin) | Không tenant nào "đói" |
| | | E2E Playwright (bổ sung integration test Node hiện có) | UI flow: register → ticket → webhook |
| **W4** | Observability | OpenTelemetry (trace+metric) + Prometheus + Grafana + Sentry | 10 SLO dashboard |
| | | zerolog structured log, log level per-tenant | — |
| | | Docker Compose (dev) + k8s manifests (prod, 2–4 replicas, HPA) | CI/CD: lint → test → build → migrate → deploy |
| **W5** | Security & DR | Secrets: Vault / sealed-secrets | Không secret trong image |
| | | Backup: pg_dump hàng giờ (S3) + PITR (WAL archive) + restore drill | RPO < 1h, RTO < 30min |
| | | SAST (gosec, semgrep-golang) + DAST (OWASP ZAP) + SBOM (syft) | 0 critical CVE |
| | | Load test mục tiêu: 2000 rps, p95 < 300ms | Pass |
| **W6** | Buffer | Hotfix, docs, onboarding nội bộ, cutover final | Go-live |

---

## 5. Việc làm ngay (tuần 1, bắt đầu hôm nay)

1. **Draft DDL** (trên) → commit vào `BACKEND/migrations/0001_init.up.sql`.
2. **Code `sql.go` skeleton**: implement `Repository` interface trên Postgres (pgxpool), giữ UoW (transaction + savepoint).
3. **Testcontainers-go**: 13 test functions hiện có chạy trên Postgres, không cần file JSON.
4. **Benchmark**: file.go vs sql.go với 100k records, so sánh p95 write latency (mục tiêu: giảm 100×).
5. **Decision gate (Day 5):** Nếu sql.go pass benchmark + regression → tiếp tục W2. Nếu chưa → buffer 3 ngày.

---

## 6. Điều KHÔNG nên làm trong 40 ngày

- Không thêm tính năng CRUD mới (ticket, KB, customer…).
- Không thêm LLM provider mới (focus OpenAI).
- Không thêm kênh mới (web, email hiện có đủ).
- Không build mobile native (PWA đủ).
- Không vội SSO provider mới (OIDC generic đã có).

Mọi tính năng mới (autonomous agent, vertical, marketplace) **đợi sau W6** khi foundation ổn.

---

## 7. Phụ lục: mapping Record → bảng mới

| `Kind` (current) | Bảng mới | Ghi chú |
|---|---|---|
| `customer` | `customers` | + `tags TEXT[]`, `custom_values JSONB` |
| `conversation` | `conversations` | + `channel` enum mở rộng (zalo, whatsapp, sms) |
| `ticket` | `tickets` | + SLA fields, `first_response_at`, `resolved_by` |
| `message` | `messages` | partition monthly, `attachment_key` thay base64 |
| `knowledge` | `knowledge_articles` | + `embedding VECTOR(1536)` (pgvector) |
| `automation` | `automations` | giữ, dùng khi chưa migrate sang `workflow_rules` |
| `webhooks` | `webhooks` | `secret_hash` thay plaintext |
| `webhook_deliveries` | `webhook_deliveries` | + `dead_letters` |
| `workflow_rules` | `workflow_rules` | JSONB cho conditions/actions (giữ linh hoạt) |
| `workflow_jobs` | `workflow_jobs` | partition monthly, auto-drop 90 ngày |
| `notification` (implicit) | `notifications` | bảng riêng |
| `*` (audit) | `audit_events` | mới, partition monthly, keep 2 năm |

---

## 8. Phụ lục: checklist go-live (W6)

- [ ] Postgres 15+ production (managed: RDS/GCP CloudSQL/DO)
- [ ] NATS JetStream (managed hoặc self-hosted 3-node)
- [ ] MinIO/S3 cho attachments
- [ ] 2–4 backend replicas (k8s), HPA CPU 70%
- [ ] Migration pipeline (init container)
- [ ] pg_dump hàng giờ + PITR (WAL archive)
- [ ] OTel + Prometheus + Grafana + Sentry
- [ ] Load test 2000 rps pass (p95 < 300ms)
- [ ] Security: 0 critical CVE, 0 secret trong image
- [ ] Backup restore drill (RTO < 30min)
- [ ] Runbook: incident, rollback, cutover
- [ ] Onboarding tenant mới (5 phút)
