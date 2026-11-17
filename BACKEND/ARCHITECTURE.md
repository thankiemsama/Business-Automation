# Kiến trúc và tiêu chí chất lượng

Mục tiêu của lần refactor này là nâng mức tổ chức mã và khả năng kiểm chứng của backend MVP, hướng tới khoảng 8/10 trong phạm vi đồ án/local backend. Đây không phải điểm đánh giá độc lập hoặc chứng nhận production readiness.

## Cấu trúc

```text
BACKEND/
  cmd/api/                 Startup, HTTP server, graceful shutdown
  internal/
    app/                   Composition Root
    api/                   Routes, controller, middleware, DTO, response
    core/                  User, Session, Record, error, transaction contracts
    repository/            File adapter, private snapshots và database state
    service/               Auth, Resource, Conversation, Dashboard
                           ResourcePolicy, ReplyProvider, AutomationRunner
  scripts/demo.ps1          Luồng thử qua HTTP thật
  go.mod
```

## OOP theo Go

Các service giữ dependency trong field private và được tạo qua constructor. Hành vi được đóng gói trong method của struct; interface hỗ trợ đa hình. Composition được dùng để kết hợp service với thuật toán và hạ tầng. Không cần mô phỏng class hierarchy hay kế thừa kiểu Java/C#.

`core.Record` vẫn dùng chung cho nhiều loại bản ghi để giữ định dạng file MVP. Đây là giới hạn có chủ đích; DTO đầu vào đã tách riêng theo tài nguyên và không cho ghi các field nghiệp vụ không thuộc tài nguyên đó. Một bước tiếp theo là model domain riêng, typed repository và database schema tương ứng.

## SOLID đang áp dụng

| Nguyên tắc | Cách áp dụng thực tế |
| --- | --- |
| S — Single Responsibility | Auth, Resource, Conversation, Dashboard được tách; HTTP serialization, DTO, middleware và persistence không nằm trong một service |
| O — Open/Closed | Registry ResourcePolicy cho validation/quyền ghi; thay ReplyProvider hoặc AutomationRunner qua constructor; test chứng minh thêm policy mà không sửa ResourceService |
| L — Liskov Substitution | Provider giả thay local provider qua cùng interface; contract lỗi phải được trả về và transaction rollback; file adapter được kiểm tra bằng tình huống lỗi và concurrent writes |
| I — Interface Segregation | HTTP sử dụng AuthUseCases, ResourceUseCases, ConversationUseCases, DashboardUseCases; transaction phân biệt Reader và Writer; port Clock/IDGenerator/Hasher riêng |
| D — Dependency Inversion | Service phụ thuộc core.Repository và các port, không import concrete file adapter; `internal/app` chịu trách nhiệm nối implementation |

Reader/Writer hiện vẫn gộp thao tác tài khoản và nghiệp vụ trong một Unit of Work, chưa phải interface cực nhỏ cho từng aggregate. LSP không thể được bảo đảm chỉ bằng khai báo interface; các test cung cấp bằng chứng cho những hành vi đã kiểm chứng.

## Design patterns có sử dụng

- **Repository + Unit of Work:** tất cả thao tác trong Update cùng commit hoặc cùng rollback. Tin nhắn và ticket automation không thể chỉ lưu một nửa. Reader trả bản sao và lọc tenant; không đưa map lưu trữ cho nghiệp vụ.
- **Strategy:** CustomerPolicy / ConversationPolicy / TicketPolicy / KnowledgePolicy / AutomationPolicy; LocalKnowledgeProvider; KeywordAutomation. Mỗi thuật toán có port để thay thế.
- **Dependency Injection + Composition Root:** constructor nhận dependency, `internal/app.NewHandler` nối chúng; không dùng service locator hay biến toàn cục chứa service.
- **Decorator:** middleware bao HTTP handler để xác thực, thêm header và phục hồi panic.

Không thêm Singleton, Observer hoặc Abstract Factory khi chưa có nhu cầu nghiệp vụ cụ thể.

## Các bất biến đã siết lại

- Tenant do identity/server quyết định; khóa ngoại khách hàng/hội thoại phải thuộc tenant hiện tại.
- Không đổi khách hàng của hội thoại đã tồn tại; ticket phải khớp customer của hội thoại.
- Ticket automation có automation_id riêng, không dùng name để lưu ID nguồn. Ticket cũ lưu ID trong name vẫn được nhận diện để tránh tạo lặp.
- Sequence của tin nhắn tăng trong cùng transaction; phản hồi không phụ thuộc vào timestamp phải khác nhau.
- Provider lỗi thì không lưu phản hồi; automation lỗi thì rollback cả tin nhắn và ticket.
- File chỉ được thay thế sau khi ghi và sync file tạm thành công; lỗi lưu không công bố state mới. Clone không còn bỏ qua lỗi JSON marshal/unmarshal.

## Kiểm chứng và giới hạn

Xem WORKLOG.md cho lệnh và kết quả chạy thực tế. Test có thay thế dependency thật sự, clock giả để kiểm tra expiry, persistence failure và thao tác đồng thời; không chỉ kiểm tra constructor hoặc getter.

Vẫn là file store một process, chưa có SQL/migration, LLM thực, frontend, queue/outbox, audit trail hay kiểm thử tải production. ReplyProvider hiện chạy bên trong transaction cục bộ: chỉ dùng provider nhanh, không có side effect. Khi tích hợp LLM qua mạng, cần tách đọc snapshot → gọi provider ngoài transaction có context/timeout → lưu với kiểm tra version/idempotency, thay vì giữ khóa file trong lúc gọi mạng.

Sai lỗi domain hiện vẫn mang HTTP status để tương thích response MVP. Một backend lớn hơn nên dùng mã lỗi domain độc lập và map status tại HTTP boundary. Giữ những giới hạn này công khai thay vì coi việc dùng pattern là đủ để đạt production quality.

## Quy ước Clean Code

Tên biến trong nghiệp vụ mô tả vai trò; hằng số nghiệp vụ tập trung tại core/constants.go, giới hạn kỹ thuật đặt cạnh thành phần sử dụng. Constructor nằm cùng service. Các hàm orchestration gọi helper theo ý nghĩa nghiệp vụ, dùng guard clauses cho lỗi và tránh boolean flag đổi hành vi endpoint. Giữ comment cho lý do/tương thích quan trọng thay vì diễn giải từng dòng lệnh. Import chuẩn Go đứng trước import nội bộ, toàn bộ mã định dạng bằng gofmt.
