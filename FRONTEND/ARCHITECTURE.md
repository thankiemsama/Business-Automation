# Kiến trúc frontend

## Phân tầng

```text
App Router → Session gate / Module page → Component / Hook
                                  ↓
                         Repository contracts
                                  ↓
                        FetchHttpClient adapter
                                  ↓
                     Next.js BFF (same-origin)
                                  ↓
                      Go API /api/v1
```

`shared/domain` định nghĩa Customer, Conversation, Ticket, Knowledge, Automation, Message và User riêng, không dùng `Record` chung cho toàn bộ giao diện. Input loại bỏ ID/timestamp; serializer theo module chỉ gửi field DTO hợp lệ. PUT gửi đầy đủ trường nghiệp vụ.

Đây là phân tầng thực dụng: UI điều phối repository trực tiếp cho CRUD đơn giản, không thêm tầng use case rỗng chỉ để đủ tên Clean Architecture.

## OOP, SOLID và patterns

- **Encapsulation/composition:** HTTP client và các repository là class, giữ dependency private và nhận qua constructor. React component dùng function, không ép thành class.
- **Repository:** `ResourceRepository<T>` cung cấp list/get/save. Adapter xử lý URL/query/DTO, component không tự ghép endpoint.
- **Dependency Injection + Composition Root:** `createServices(HttpClient)` nối adapter; ServicesProvider tạo bộ dependency ổn định cho mỗi cây ứng dụng. Có thể inject HTTP adapter giả để test.
- **Strategy/Registry:** mỗi module có `ModuleDefinition<K>` với fields/defaults/serializer riêng. `resourceModules` được kiểm tra bằng mapped type, editor và collection dùng lại giữa các module.
- **SRP:** HTTP, phiên, query lifecycle, resource editor, field renderer, relation picker và conversation thread có trách nhiệm riêng.
- **OCP:** hành vi CRUD được cấu hình qua định nghĩa module; nghiệp vụ hội thoại dùng UI chuyên biệt.
- **LSP:** adapter phải giữ contract dữ liệu/lỗi/phân trang. `implements` không tự bảo đảm hành vi; integration tests kiểm tra các contract HTTP với backend thật.
- **ISP:** HttpClient chỉ có request; auth/dashboard/chat có repository riêng, không nhồi tất cả vào resource contract.
- **DIP:** repository phụ thuộc HttpClient interface; editor nhận repository qua props; Composition Root chọn concrete implementation.

Không thêm Singleton, event bus hoặc hierarchy để thể hiện pattern. ConversationRepository mở rộng resource adapter bằng messages/send/reply, giữ các thao tác gốc.

## Quản lý state

| State              | Nơi quản lý               | Quy tắc                                                                |
| ------------------ | ------------------------- | ---------------------------------------------------------------------- |
| Dependency         | ServicesProvider          | Tạo một lần mỗi cây                                                    |
| Phiên              | SessionGate + UserContext | `/auth/me` xác minh, resource 401 làm hết phiên                        |
| Dữ liệu API        | useQuery + repository     | Loading/error/data; AbortController và generation guard bỏ response cũ |
| Search/filter/page | Module page               | Đổi filter đưa page về 1                                               |
| Form/mutation      | Editor/thread/team        | Local state, khóa khi lưu, giữ nội dung khi lỗi                        |
| Selection          | ConversationsPage         | Thread có key theo ID, đổi hội thoại reset draft/lịch sử               |
| Navigation         | App Router                | Route riêng, layout giữ phiên                                          |

Khi loader đổi, dữ liệu của query trước không hiển thị như kết quả query mới. Mutation thành công mới đóng form và reload danh sách. Không tự retry POST để tránh gửi trùng khi chưa rõ kết quả mạng.

`useQuery` chưa phải query cache: không deduplicate giữa module, stale time, offline persistence hoặc background retry. Khi quy mô tăng có thể thay bằng TanStack Query/SWR sau repository. Token không lưu trong localStorage/sessionStorage/context.

## API và bảo mật phiên

Browser chỉ gọi `/api/platform/*`. BFF whitelist đường dẫn, chỉ export GET/POST/PUT, kiểm tra origin mutation, giới hạn body 64 KiB, timeout 15 giây, không cache response cá nhân. URL backend là server-only; browser không chọn URL đích.

Login chuyển bearer token thành cookie HttpOnly, SameSite=Lax, path=/, hạn theo `expires_in`, Secure khi HTTPS. JSON trả browser chỉ có `authenticated: true`. Logout gọi backend thu hồi token rồi xóa cookie. Nếu không kết nối backend, UI báo lỗi và cho thử lại thay vì khẳng định đã thu hồi phiên.

UI ẩn thao tác owner-only với agent; backend vẫn quyết định authorization và tenant. Owner mới sửa knowledge/automation hoặc tạo agent. BFF không thay validation và quyền của Go.

Đặt `APP_ORIGIN` chính xác khi reverse proxy thay hostname/protocol. Backend auth limiter hiện thấy IP của BFF nên giới hạn 20 auth/phút dùng chung qua proxy. Khi public cần thiết kế rate limit ở proxy đáng tin; không tùy tiện tin X-Forwarded-For từ client.

## Data và giới hạn

Relation picker có search/phân trang, không chỉ lấy 100 mục rồi coi là toàn bộ dữ liệu. Message phân trang theo thứ tự backend; gửi tin khi ở trang trước đưa người dùng đến trang chứa tin mới. Dashboard chỉ dùng số liệu có nguồn: tỷ lệ giải quyết = (tickets - open_tickets) / tickets, 0 nếu chưa có ticket.

Chưa có realtime, virtualized list, đồng bộ phiên giữa nhiều tab, refresh token, component tests hoặc bộ UI tests tự động trong CI. Backend không có DELETE/list-team nên UI không giả lập. Kiểm thử tích hợp có session, exact PUT DTO, keyword reply/automation, closed conversation, roles và tenant isolation; không coi đây là chứng nhận production readiness.
