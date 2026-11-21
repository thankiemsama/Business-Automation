# Cutover Schema v2 (platform_v2) — Kế hoạch và điều kiện bật RLS

Tài liệu này là kế hoạch hành động cho việc chuyển adapter vận hành chính
từ schema "tương thích" (bảng `platform_records` poly-table) sang schema
đích `platform_v2` (17 bảng riêng, RLS theo tenant, partition tháng,
pgvector) đã định nghĩa sẵn trong `BACKEND/internal/repository/migrations`.

Phạm vi: **chỉ lập kế hoạch và điều kiện phát hành**, chưa tự động cutover.
Cutover là thao tác vận hành có kiểm soát, thực hiện khi các điều kiện
bên dưới được đáp ứng.

## 1. Hiện trạng (điểm xuất phát)

| Thành phần | Trạng thái |
| --- | --- |
| Migrations v2 (`0002_rls_partition.sql`, `0003_vector.sql`) | Đã viết sẵn, chạy trên database staging riêng |
| Adapter đang vận hành | Dùng schema tương thích, tự áp dụng `003_tenant_record_key.sql` |
| RLS | **Chưa bật** — cách ly tenant hiện dựa ở service layer (rủi ro R10) |
| Partition (`messages`, `workflow_jobs`, `audit_events`) | Định nghĩa trong v2, chưa chạy ở vận hành |
| pgvector (embedding KB) | `0003_vector.sql` đã có; adapter bật khi extension khả dụng |

## 2. Điều kiện phát hành (đóng hết mục nào mới được làm bước đó)

### Gating trước khi cutover
- [ ] SSE fanout NATS đã triển khai và test cross-replica (Task 1)
- [ ] Scheduler workflow query-based + fairness (Task 2) hoạt động ổn định
- [ ] Load test baseline (k6 REST + SSE harness) đã có số p95/p99 trước cutover
- [ ] Go race detector chạy sạch (Docker/Linux CI — máy Windows không chạy được, xem Task 4)
- [ ] Backup `pg_dump` + diễn tập `pg_restore` vào database trống (RTO < 30 phút)

### Dual-read (48h)
- [ ] Chạy migration v2 trên staging (database riêng, không đụng production)
- [ ] Adapter đọc song song: schema tương thích (nguồn sự thật) + schema v2 (kiểm tra)
- [ ] So khớp số lượng và nội dung từng bảng; diff = 0 trong 48h liên tục
- [ ] Kiểm chứng RLS policy với role không sở hữu bảng: tenant A không đọc được tenant B

### Cutover
- [ ] Chọn cửa sổ bảo trì, đóng ghi (read-only) trong thời gian cutover
- [ ] Chạy lại migration v2 trên production database, xác nhận migration idempotent
- [ ] Bật adapter đọc/ghi sang schema v2 (flag `NEXA_USE_SCHEMA_V2=true`)
- [ ] Bật `set_config('app.tenant_id', ...)` per transaction trong mọi luồng API + worker
- [ ] Chạy smoke test toàn bộ endpoint (`/healthz`, đăng nhập, tạo ticket, webhook)
- [ ] Quan sát 24h: p95, error rate, lock contention (advisory lock metric)

### Sau cutover
- [ ] Giữ schema tương thích đọc được trong 30 ngày (hồi quy khẩn cấp)
- [ ] Rà soát partition: cron tạo partition tháng kế tiếp, auto-drop 90 ngày (jobs) / 2 năm (audit)
- [ ] Xóa bảng tương thích sau 30 ngày nếu không có sự cố

## 3. Hồi quy khẩn cấp (rollback)

- Flag `NEXA_USE_SCHEMA_V2=false` đưa adapter về schema tương thích (nhanh, không migrate ngược)
- Dữ liệu ghi trong thời gian schema v2 được đồng bộ ngược về `platform_records` trong giai đoạn dual-write (nếu bật); nếu chưa đồng bộ ngược thì phục hồi từ backup `pg_dump` cửa sổ cutover
- RLS policy không tự xóa khi rollback — giữ policy, chỉ đổi adapter; tránh tenant leak do role vận hành

## 4. Điều kiện chưa nên làm (giữ nguyên từ AUDIT_FOUNDATION.md)

- Không thêm tính năng CRUD mới, LLM provider mới, kênh mới trong thời gian cutover
- Mobile native, SSO provider mới: chờ sau khi schema v2 ổn định 30 ngày
- Marketplace / Data Intelligence: xây trên schema v2 + RLS, không xây trên schema tương thích

## 5. Trách nhiệm và thứ tự thực thi

| Bước | Người thực hiện | Công cụ | Trạng thái |
| --- | --- | --- | --- |
| SSE fanout NATS (Task 1) | Backend | `internal/service/fanout.go` | ✅ Đã hoàn thành |
| Scheduler query-based + fairness (Task 2) | Backend | `internal/service/workflow_jobs.go` | ✅ Đã hoàn thành |
| Load test baseline (Task 5) | Backend/QA | `loadtest/staging.js`, `loadtest/sse-latency.mjs` | ✅ Kịch bản đã có |
| Race detector (Task 4) | CI/Linux | `scripts/race-detector.ps1` | ✅ Script Docker/NoDocker |
| Dual-read 48h | QA + SRE | Migration staging + diff script | ⬜ Chờ gate |
| Cutover production | SRE | Flag + runbook | ⬜ Chờ gate |
