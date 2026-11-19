# AI Customer Service & Business Automation Platform

Backend MVP viết bằng Go, tham khảo cách phân lớp của `D:\BadmintonBooking\Backend`. Nghiệp vụ booking/payment/sân cầu lông được thay bằng customer service và automation. Không sao chép tài khoản, cấu hình hay bí mật của dự án mẫu.

## Chạy trên Windows / PowerShell

Cần Go 1.24 trở lên và các module được khai báo trong `go.mod`.

```powershell
cd D:\GOLANG\BACKEND
go run ./cmd/api
```

API mặc định: `http://127.0.0.1:8080`, kiểm tra bằng `GET /healthz`.

Cấu hình tùy chọn (biến môi trường được đọc trực tiếp, không tự nạp file .env):

```powershell
$env:HTTP_ADDR = '127.0.0.1:8080'
$env:DATA_FILE = 'data/platform.json'
go run ./cmd/api
```

Dữ liệu được lưu sau mỗi thay đổi và tồn tại sau khi khởi động lại. Chỉ chạy **một process** trên mỗi file dữ liệu. File chứa dữ liệu khách hàng và thông tin xác thực đã băm; không đưa file vào Git.

### Chuyển dữ liệu hiện có sang PostgreSQL

Backend dùng PostgreSQL khi có `DATABASE_URL`. Lấy chuỗi kết nối từ kho bí mật của môi trường triển khai và dùng TLS theo cấu hình máy chủ. Khi khởi động backend áp dụng migration có phiên bản trong `internal/repository/migrations`. Tài khoản kết nối cần quyền tạo và cập nhật bảng trong schema dành cho ứng dụng.

1. Dừng backend chế độ JSON và sao lưu nguyên tệp `DATA_FILE`. Chọn cơ sở dữ liệu PostgreSQL trống để chuyển dữ liệu.
2. Đặt `DATABASE_URL` trỏ đến cơ sở dữ liệu đích và `DATA_FILE` trỏ đến bản chụp JSON. Chạy từ thư mục `BACKEND`:

   ```powershell
   go run ./cmd/api import-json
   ```

   Lệnh nhập user, session và record trong cùng một transaction. Nếu thành công, log trả số lượng từng loại. Lệnh từ chối cơ sở dữ liệu đã có dữ liệu hoặc đã nhập trước đó. Với hệ thống mới, chạy thẳng `go run ./cmd/api` để tạo schema.
3. Đối chiếu số lượng với bản chụp và kiểm tra đăng nhập, dữ liệu gần nhất và quyền theo tenant trên môi trường thử nghiệm. Kiểm tra tài khoản Resend trước khi chạy worker trên bản sao có email đang chờ. Khởi động backend với `DATABASE_URL` khi sẵn sàng tiếp nhận thao tác ghi.
4. Giữ lại snapshot JSON để quay lại trong thời gian kiểm tra trước khi mở ghi. Sau khi PostgreSQL đã nhận thao tác mới, phục hồi từ bản sao lưu PostgreSQL để giữ dữ liệu mới. Dùng `pg_dump` định dạng custom và diễn tập `pg_restore` vào database trống trước khi triển khai. Dữ liệu xác thực, session và nội dung khách hàng nằm trong bản sao lưu, nên lưu bản sao trong kho được bảo vệ.

Mỗi cập nhật SQL ghi các row liên quan. Các transaction ghi đi qua một advisory lock chung giữa các tiến trình backend để giữ các ràng buộc nghiệp vụ hiện tại. Luồng đọc dùng snapshot SQL nhất quán. SSE vẫn dùng bộ nhớ từng tiến trình, vì vậy triển khai nhiều replica cần kế hoạch đồng bộ sự kiện riêng.

Để kiểm thử migration, import, rollback và các ghi đồng thời, đặt `TEST_DATABASE_URL` là URL của database PostgreSQL kiểm thử rồi chạy `go test ./...`. Bài kiểm thử tạo và dọn schema riêng trên database đó.

`core.Writer.Savepoint` cho phép rollback một thao tác con và trả lỗi để caller quyết định tiếp tục giao dịch ngoài. PostgreSQL dùng savepoint SQL, chế độ file dùng snapshot con. Workspace ghi nhận hiệu ứng sau commit từ thao tác con thành công.

### Schema đích W1

Hai file `0002_rls_partition.sql` và `0003_vector.sql` định nghĩa schema đích `platform_v2` với 17 bảng theo audit §3.1. Chạy từng file trong một transaction, theo thứ tự, trên database staging riêng. Adapter hiện tại vẫn dùng schema tương thích đã có dữ liệu và chỉ tự áp dụng các migration được khai báo trong `postgres.go`, gồm migration khóa ghép `003_tenant_record_key.sql`.

- RLS trên schema đích dùng tenant scope cục bộ của transaction. Luồng API và worker cần mở transaction với tenant xác định từ phiên đã xác thực hoặc từ job đã claim. Runtime dùng role không sở hữu bảng và không có `BYPASSRLS`; luồng xác thực toàn cục cần phương thức truy cập riêng được cấp quyền hẹp.
- Ba bảng phân vùng gồm `messages`, `workflow_jobs`, `audit_events`. Migration tạo partition theo tháng UTC cho tháng trước, tháng hiện tại, tháng kế tiếp và một partition mặc định cho dữ liệu lịch sử. Công việc tạo partition tiếp theo và chính sách lưu giữ thuộc phần vận hành trước cutover.
- Dùng database kiểm thử cô lập có quyền tạo role, với `TEST_DATABASE_URL`, rồi chạy `go test -v ./internal/repository -run TestTargetSchemaRLSAndPartitions`. Ca pgvector cần máy chủ có sẵn extension `vector`. Trên máy có Docker, chạy `go test -tags testcontainers -v ./internal/repository -run TestTargetSchemaInContainer` để khởi động PostgreSQL 16 có pgvector và bắt buộc kiểm tra cả migration vector.

## Kiến trúc

| Thành phần | Vai trò tương ứng dự án mẫu |
| --- | --- |
| `cmd/api` | Startup, cấu hình và graceful shutdown |
| `internal/app` | Composition Root, khởi tạo và nối dependency |
| `internal/api` | Controllers, HTTP validation, authentication, pagination |
| `internal/service` | Auth / Resource / Conversation / Dashboard; policy và provider strategies |
| `internal/core` | Core, entity models và Repository contract |
| `internal/repository` | Persistence adapter; cập nhật đồng bộ, rollback khi lỗi |

`app` nối các implementation. HTTP phụ thuộc các use-case interface riêng; service phụ thuộc Repository, Clock, IDGenerator, PasswordHasher, ReplyProvider và AutomationRunner. File adapter triển khai Repository/Unit of Work với Reader/Writer; map lưu trữ được giữ private trong adapter.

Request dùng DTO riêng cho customer/conversation/ticket/knowledge/automation. `Record` vẫn là dạng dữ liệu chung để giữ tương thích file cũ, chưa phải mô hình domain chuyên biệt đầy đủ. Xem [ARCHITECTURE.md](ARCHITECTURE.md) để biết OOP, SOLID, pattern và giới hạn thực tế.

Khi sửa customer, conversation, ticket, knowledge, template hoặc automation bằng `PUT`, gửi `version` từ bản ghi đã dùng để mở form. Thiếu phiên bản trả HTTP `428`. Phiên bản cũ trả HTTP `409`. Cập nhật thành công trả bản ghi với phiên bản mới. Backend gán phiên bản ban đầu cho dữ liệu JSON cũ khi mở hoặc nhập.

## Chức năng đã có

- Đăng ký tạo một doanh nghiệp độc lập và tài khoản owner. Email đăng nhập duy nhất toàn hệ thống.
- Đăng nhập bằng opaque Bearer token có hạn 24 giờ, đăng xuất thu hồi phiên hiện tại; token chỉ lưu dạng SHA-256. Mật khẩu PBKDF2-HMAC-SHA256, salt riêng và 210.000 vòng lặp.
- Owner tạo agent cùng doanh nghiệp. Agent được thao tác khách hàng, hội thoại, tin nhắn, ticket và xem dashboard; chỉ owner sửa kho kiến thức và quy tắc automation.
- Khách hàng, hội thoại theo kênh web/email/phone, lịch sử tin nhắn và ticket có trạng thái/độ ưu tiên.
- Kho kiến thức và phản hồi tự động dựa trên số từ trùng khớp. Chỉ dùng bài có `enabled: true`, trả `source_id`; không tìm thấy thì `needs_human: true`. Đây là tìm kiếm cục bộ, **chưa phải LLM hoặc semantic RAG**. Nhân viên cần xử lý tiếp; cờ này chưa tự phân công agent.
- Automation: khi tin nhắn từ customer chứa ít nhất một từ khóa, tạo ticket. Mỗi quy tắc chỉ có một ticket chưa resolved trên mỗi hội thoại. Quy tắc chỉ chạy khi tạo tin nhắn mới, không quét lại lịch sử.
- Dashboard đếm tài nguyên và ticket chưa giải quyết; danh sách có phân trang, lọc trạng thái, tìm kiếm.
- HTTP body tối đa 64 KiB, từ chối field không xác định; giới hạn 20 request auth/IP/phút, không tin X-Forwarded-For; tối đa 4 tác vụ hash đồng thời tại endpoint đăng ký/đăng nhập/tạo agent.

## API

Tất cả đường dẫn sau có tiền tố `/api/v1`. Ngoại trừ register/login, cần header `Authorization: Bearer <access_token>`.

| Method | Path | Mục đích |
| --- | --- | --- |
| POST | `/auth/register` | `{name,email,password}` → owner và tenant mới |
| POST | `/auth/login` | `{email,password}` → access_token |
| GET | `/auth/me` | Thông tin người dùng hiện tại |
| POST | `/auth/logout` | Thu hồi token hiện tại |
| POST | `/team` | Owner tạo agent: `{name,email,password}` |
| GET | `/dashboard` | Thống kê doanh nghiệp hiện tại |
| GET, POST | `/customers` | Danh sách / tạo khách hàng |
| GET, PUT | `/customers/{id}` | Xem / thay thế khách hàng |
| GET, POST | `/conversations` | Danh sách / tạo hội thoại |
| GET, PUT | `/conversations/{id}` | Xem / thay thế hội thoại |
| GET, POST | `/tickets` | Danh sách / tạo ticket |
| GET, PUT | `/tickets/{id}` | Xem / thay thế ticket |
| GET, POST | `/knowledge` | Danh sách / tạo bài kiến thức |
| GET, PUT | `/knowledge/{id}` | Xem / thay thế bài |
| GET, POST | `/automations` | Danh sách / tạo quy tắc |
| GET, PUT | `/automations/{id}` | Xem / thay thế quy tắc |
| GET, POST | `/conversations/{id}/messages` | Lịch sử / tạo tin nhắn `{content,sender}` |
| POST | `/conversations/{id}/reply` | Trả lời tin nhắn khách hàng mới nhất |

Request tạo từng tài nguyên:

```json
{"name":"Nguyễn An","email":"an@example.com","phone":"0900000000"}
```
```json
{"customer_id":"CUSTOMER_ID","channel":"web","status":"open"}
```
```json
{"subject":"Yêu cầu hoàn tiền","customer_id":"CUSTOMER_ID","conversation_id":"CONVERSATION_ID","status":"open","priority":"high"}
```
```json
{"subject":"Chính sách hoàn tiền","content":"Hoàn tiền trong 7 ngày.","enabled":true}
```
```json
{"name":"Tiếp nhận khiếu nại","keywords":["hoàn tiền","khiếu nại"],"enabled":true}
```

- PUT thay thế toàn bộ dữ liệu nghiệp vụ, cần gửi đủ trường bắt buộc. ID, tenant, timestamps do server quyết định.
- Ticket status: `open`, `in_progress`, `resolved`; priority: `low`, `normal`, `high`, `urgent`.
- Hội thoại status: `open`, `closed`. Hội thoại đóng không nhận thêm tin nhắn/phản hồi tự động. Không được đổi customer_id của hội thoại đã tạo.
- Sender đầu vào: `customer` hoặc `agent`. `assistant` chỉ do server tạo. Endpoint tin nhắn dành cho nhân viên/tích hợp đã đăng nhập, chưa phải public chat widget.
- Danh sách: `?page=1&limit=20&q=keyword&status=open`, limit tối đa 100. Kết quả tăng dần theo thời điểm tạo; tin nhắn mới có sequence để bảo đảm thứ tự ngay cả khi timestamp trùng nhau. Tin nhắn cũ không có sequence vẫn đọc được.
- Không có DELETE; đóng hội thoại, resolve ticket hoặc tắt rule/bài bằng `enabled: false`.
- Response: `{ "data": ..., "message": "...", "code": "SUCCESS" }`; danh sách nằm trong `data.items` kèm total/page/limit.
- Lỗi trả HTTP 400/401/403/404/409/415/429/500; tài nguyên tenant khác trả 404.

## Thử toàn bộ luồng

Trong terminal khác khi API đang chạy:

```powershell
cd D:\GOLANG\BACKEND
./scripts/demo.ps1
```

Script tạo doanh nghiệp demo mới với email ngẫu nhiên, thêm khách hàng, quy tắc, kiến thức và hội thoại; gửi tin nhắn để tạo ticket rồi yêu cầu phản hồi. Script chủ động tạo dữ liệu, chỉ dùng trên API local của bạn.

## Kiểm tra

```powershell
go test ./... -count=1
go vet ./...
```

Nếu cache mặc định bị chặn quyền ghi trong Codex:

```powershell
$env:GOCACHE = 'D:\GOLANG\.go-cache'
go test ./... -count=1
go vet ./...
```

13 test functions kiểm tra luồng HTTP/nghiệp vụ, tenant isolation, chống giả tenant, owner/agent, DTO validation/rate limit, token expiry với clock giả, thu hồi token, đọc lại file, rollback khi persistence/provider/automation lỗi, provider thay thế, mở rộng policy, automation song song và tương thích ticket cũ.

## Giới hạn và hướng mở rộng

Đây là backend MVP chạy local, chưa phải hệ thống production hoàn chỉnh. Chưa có giao diện, tích hợp mô hình AI thật, embeddings/vector search, webhook Zalo/Facebook/email, workflow nhiều bước/scheduler, queue/retry, SLA, phân công nhân viên, audit log, billing, xác minh email hay khôi phục mật khẩu. Kênh hiện là thuộc tính phân loại, chưa gửi/nhận qua nhà cung cấp thật.

File repository đọc/ghi toàn bộ state và chỉ khóa trong một process; không dùng cho nhiều replica hay dữ liệu lớn. Chưa kiểm chứng mất điện, phục hồi backup hoặc tải production. Bước phát triển tiếp theo là SQL repository có transaction và migration, provider AI có timeout/giới hạn chi phí, queue/outbox và xác thực webhook. Trước khi public cần HTTPS, quy trình mời/xác minh tài khoản và cơ chế vận hành phù hợp.
