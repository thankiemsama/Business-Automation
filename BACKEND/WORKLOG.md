# Worklog — 2026-11-17

## Phạm vi đã thực hiện

Tham khảo ARCHITECTURE.md, AuthController và BaseResponse của backend Badminton C#/.NET. Xây mới Go backend MVP trong BACKEND; không chỉnh sửa dự án tham khảo. Dùng phân lớp API/service/core repository contract/file adapter.

## Đã kiểm chứng

- `go test ./... -count=1`: PASS, 4 test functions (3 HTTP workflow/validation/rate-limit, 1 repository rollback/concurrency).
- `go vet ./...`: PASS.
- `go build -o ./data/api.exe ./cmd/api`: PASS.
- Chạy binary trên 127.0.0.1:18089 và `./scripts/demo.ps1 -BaseUrl http://127.0.0.1:18089`: PASS. Phản hồi có source_id và needs_human=false; dashboard có 1 customer, 1 conversation, 1 automation, 1 knowledge article, 2 messages, 1 open ticket. Tiến trình thử nghiệm đã dừng.
- Test persistence mở lại file sau nhiều lần cập nhật, xác nhận dữ liệu và session vẫn sử dụng được. Test rollback, snapshot read isolation và 20 goroutine cập nhật không mất bản ghi.

Dùng GOCACHE=D:\GOLANG\.go-cache do quyền ghi cache mặc định bị chặn. Không kiểm chứng race detector, production load, SQL hoặc nhà cung cấp AI/kênh liên lạc. Smoke artifacts nằm trong BACKEND/data đã được ignore; dữ liệu mặc định data/platform.json chưa được seed.

## Giới hạn hiện tại

Trả lời bằng local keyword search, không gọi LLM. File store chỉ hỗ trợ một process; không phải production database. Chưa có frontend hoặc tích hợp kênh thực. Xem README để biết API, cấu hình và hướng mở rộng.

## Refactor OOP / SOLID — 2026-11-17

### Thay đổi

- Xóa năm thư mục khung rỗng API/CORE/MODELS/SERVICES/TESTS. Xóa binary, log, dữ liệu demo/smoke và compiler cache do các lượt thực hiện này tạo; không xóa dữ liệu platform của người dùng.
- Tách Service tổng thành AuthService, ResourceService, ConversationService, DashboardService; dependency private, constructor injection.
- Thêm Composition Root trong internal/app và chia nhỏ HTTP controller/middleware/DTO. HTTP phụ thuộc use-case ports theo từng trách nhiệm.
- Reader/Writer và Unit of Work che map lưu trữ; private databaseState trong adapter. Clone trực tiếp map/slice, read snapshot ngoài khóa; ghi chỉ công bố sau persistence thành công.
- ResourcePolicy registry, ReplyProvider và AutomationRunner Strategy; Clock/IDGenerator/PasswordHasher có thể thay thế.
- DTO riêng cho từng loại tài nguyên; metadata client không quyết định tenant. Cấm đổi customer của hội thoại; automation_id tách riêng khỏi name; hỗ trợ ticket cũ. Sequence tin nhắn xử lý timestamp trùng nhau.

### Kết quả kiểm chứng sau refactor

- `go test ./... -count=1`: PASS, 12 test functions (4 API, 2 repository, 6 service).
- `go vet ./...`: PASS.
- `go build -o ./data/api.exe ./cmd/api`: PASS.
- Binary mới + `scripts/demo.ps1` trên 127.0.0.1:18089: PASS, nguồn kiến thức đúng, 2 messages (assistant sequence=2), 1 open ticket. Tiến trình đã dừng.
- Test mới: expiry bằng injected clock; provider replacement/failure rollback; rollback toàn bộ message khi automation lỗi; 12 concurrent inbound messages tạo một ticket và sequence riêng; thêm policy mới không sửa service; legacy ticket deduplication; persistence failure không công bố state; typed DTO và cấm đổi customer của hội thoại.
- Không chạy race detector: môi trường không tìm thấy gcc/C compiler trên PATH. Test goroutine đã chạy nhưng không thay thế bằng chứng race-detector.

Mục tiêu chất lượng khoảng 8/10 áp dụng cho cấu trúc MVP, không phải đánh giá production. Các giới hạn và cách áp dụng pattern được ghi cụ thể trong ARCHITECTURE.md.

## Clean Code — 2026-11-17

- Đổi tên biến nghiệp vụ thành user/store/record/conversation thay cho các tên một chữ khó đọc; đặt constructor cạnh service tương ứng.
- Tập trung tên tài nguyên, role, status, sender, channel, priority và thời hạn session trong core/constants.go. HTTP dùng net/http constants. Đặt tên cho các giới hạn request, password, pagination và auth concurrency.
- Tách strategies.go thành knowledge.go và automation.go; chia thuật toán thành searchTokens, scoreArticle, hasOpenAutomationTicket, newTicket và matchesAnyKeyword.
- Loại bỏ boolean flag chọn đăng ký/đăng nhập; dùng authAction. Tái sử dụng capacity guard và trả token response có kiểu rõ ràng.
- Pagination tách parse/validate/filter/response; giữ items là JSON array ngay cả khi kết quả rỗng.
- Chuẩn bị metadata bản ghi và lưu tin nhắn dùng helper riêng; lưu snapshot tách persist/writeSnapshot với guard clauses và đóng file trên mọi nhánh.
- Chuẩn hóa import, khoảng cách declaration và định dạng composite literals; giữ nguyên route, định dạng dữ liệu lưu và giá trị JSON nghiệp vụ.

Kiểm chứng cuối: `go test ./... -count=1` PASS (13 test functions; thêm 4 subcases về phân trang/lọc), `go vet ./...` PASS, `go build` PASS, `gofmt -l` không còn file cần định dạng. Cache/binary kiểm tra đặt trong TEMP, không tạo thêm thư mục build trong dự án. Không chạy thêm demo HTTP vì bộ HTTP regression đã qua và không có tích hợp bên ngoài thay đổi.

## Multi-replica hardening � 2026-11-21

### Task 1 � SSE fanout multi-replica (NATS JetStream)

- Moi backend replica truong co watchers SSE trong bo nho (workspace.go), chi dung
  cho 1 process. Tach ra interface EventFanout (internal/service/fanout.go):
  - MemoryFanout (mac dinh, giu hinh vi hien co: cursor epoch-revision, giao
    khong block, deo mot cursor moi nhat cho watcher cham).
  - NATSFanout (bat khi dat NEXA_NATS_URL): stream NEXA_SSE (memory, MaxAge 1h),
    subject sse.{scope}, moi replica publish + subscribe chung mot stream,
    cursor = stream sequence (dong tren tat ca replicas). Epoch chia se de qua
    NEXA_FANOUT_EPOCH. Local subscription tai tung replica chuyen event vao
    watchers (kem ca event do chinh no sinh ra), khong lan luon.
- workspace.go doc Watch/Publish sang fanout; hoi thoei khach (guest scope
  chat:t:c) va tenan scope deu di chung mot duong. app.go noi vao:
  NATSFanoutFromEnvironment + Close khi ctx xong.
- Test (fanout_test.go, fanout_stress_test.go): Watch/Publish/stop, deo watcher
  cham khong block, cursor monoton, song song 16 watchers x 32 tenan + 8
  publisher (250ms), round-trip NATS 2 ket noi (test NATSFanoutRoundTrip,
  skip neu khong co NEXA_NATS_TEST_URL).
- compose.yaml: them service nats (nats:2.10-alpine --js, volume nats-data)
  va cac bien NEXA_NATS_URL/USER/PASSWORD + NEXA_FANOUT_EPOCH cho backend.

### Task 2 � Workflow scheduler fairness

- runJobs/runAIJobs lay batch 25 theo thu tu hien han (duoi) nhu cu, nhung
  gio gi han 5 job/tenan moi batch (capJobsPerTenant, workflow_jobs.go).
  Tenan "on" khong con chan ngat 25 slot; job con lai van pending, duoc
  lay o tick tiep (15s). Job da claim van an toan nhieu replica vi Postgres
  LockWorkflowJob dung FOR UPDATE SKIP LOCKED va enqueue dedup (tokenHash
  workflow:rule:ticket) + advisory lock chung.
- Test (workflow_fairness_test.go): cap FIFO 3 tenan x 6 job = chi giu
  5/tenan, batch nho/ r?ng tra day du, khong doi thu tu.

### Task 4 � Race detector (Windows khong chay duoc)

- Go race detector can cgo + C toolchain Linux/macOS. Masine nay (Windows)
  khong co Docker/WSL distro; MSYS2 gcc 16 (ucrt64) khong host duoc cgo
  (b�?c -E -dM im l?ng th?t b?i khi Go cgo g?i gcc nh� ti?n tr?nh con Windows,
  du chi -c chi dung duoc trong MSYS2). K�t luan: chay tren Linux/Docker.
- scripts/race-detector.ps1: -NoDocker (ch?y tr�n Linux host) hoac chay
  docker run golang:1.24-bookworm + go test -race ./... -count=1 (cache
  go-cache volume).

### Task 5 � Load test

- loadtest/staging.js (k6, co san): REST read/write theo STAGING_RPS,
  threshold p95 < STAGING_P95_MS.
- loadtest/sse-latency.mjs (moi, Node 18+, khong thu vien): mo
  NEXA_SSE_STREAMS luon SSE, goi NEXA_SSE_WRITES viec ghi, do thoi gian
  ten stream nhan luot refresh (deadline NEXA_SSE_REFRESH_DEADLINE_MS).
  Bao cao p50/p95/p99 + so stream bi do (0 refresh) = dau hieu fanout
  chua duoc nois NATS. Exit 1 neu co miss.

### Task 3 � Cutover schema v2 (k? ho?ch)

- CUTOVER_SCHEMA_V2.md: dieu kien gate (Task 1-5 xong + backup drill),
  dual-read 48h, cutover qua NEXA_USE_SCHEMA_V2, rollback 30 ngay,
  khong tao ten hanh CRUD/kenh/LLM trong thoi gian cutover.

### Kiem chung

- go build ./... OK; go vet ./... OK; go test ./... -count=1 PASS
  (api, repository, service, telemetry). go.mod nhat nats.go v1.56.0
  (direct), go directive 1.26 (khop Dockerfile golang:1.26-bookworm).
