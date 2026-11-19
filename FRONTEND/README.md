# Nexa — Customer Experience Workspace

Giao diện tiếng Việt cho backend Go trong `../BACKEND`, dùng Next.js App Router, React, TypeScript và Tailwind CSS. Dashboard đọc số liệu thật từ API.

## Chạy local

Terminal 1:

```powershell
cd D:\GOLANG\BACKEND
go run ./cmd/api
```

Terminal 2:

```powershell
cd D:\GOLANG\FRONTEND
npm install
npm run dev
```

Mở `http://localhost:3000`, chọn **Tạo tài khoản** để tạo owner và doanh nghiệp mới. Mật khẩu từ 12–128 byte theo backend. Owner thêm nhân viên tại **Đội ngũ**. Không có tài khoản mặc định.

Backend mặc định `http://127.0.0.1:8080`. Để đổi, sao chép `.env.example` thành `.env.local`, sửa `BACKEND_URL` rồi khởi động lại Next.js. Đây là biến server-only. Nếu reverse proxy đổi host/protocol, đặt `APP_ORIGIN` thành origin công khai chính xác, ví dụ `https://support.example.com`.

## Chức năng

- Đăng ký, đăng nhập, kiểm tra phiên, đăng xuất thu hồi token.
- Dashboard: khách hàng, hội thoại, tin nhắn, ticket và tiến độ giải quyết.
- Khách hàng: tìm kiếm, phân trang, tạo và cập nhật.
- Hội thoại: lọc trạng thái, tạo/sửa/đóng/mở lại; lịch sử phân trang; gửi tin nhân viên hoặc ghi nhận tin khách hàng; phản hồi từ kiến thức và cảnh báo cần người hỗ trợ.
- Ticket: tạo/sửa, tìm kiếm, lọc trạng thái, độ ưu tiên và giải quyết.
- Kiến thức/automation: tạo, cập nhật, bật/tắt; agent chỉ xem.
- Owner xem thành viên, mời nhân viên và tạo tài khoản agent.
- Responsive desktop/mobile, modal native hỗ trợ focus/Escape, loading, empty state, lỗi và thử lại.

Không có DELETE theo hợp đồng backend: đóng hội thoại, resolve ticket hoặc tắt bài/quy tắc.

## Cấu trúc

```text
src/
  app/                     Routes, layout, metadata và BFF API
    (workspace)/           Các route nghiệp vụ dùng chung session/shell
    api/platform/          Proxy giới hạn endpoint và cookie HttpOnly
  modules/
    auth/                  Đăng nhập/đăng ký
    dashboard/             Thống kê
    customers/             Định nghĩa nghiệp vụ khách hàng
    conversations/         Danh sách, lịch sử, composer và form
    tickets/               Định nghĩa ticket
    knowledge/             Định nghĩa kho kiến thức
    automations/           Định nghĩa quy tắc
    team/                  Cấp tài khoản agent
    resources/             Registry, collection, editor, field, relation picker
  shared/
    domain/                Entity/DTO/query contracts
    api/                   HTTP port và Repository adapters
    hooks/                 Vòng đời dữ liệu bất đồng bộ
    providers/             Composition Root, dependency và session context
    ui/                    Shell, icons và primitives
scripts/integration.test.mjs
```

Các màn hình nhiều chức năng dùng tham số URL `section` để mở trực tiếp từng mục. Composition root trong `shared/providers` cấp repository qua cổng `HttpClient`. Module sở hữu state nhập liệu và gọi `useQuery` cho dữ liệu server; component điều hướng chỉ chọn và gắn nội dung đang mở.

Xem `ARCHITECTURE.md` về OOP, SOLID, patterns, state và giới hạn.

## Kiểm tra

```powershell
npm run lint
npm run typecheck
npm run format:check
npm run build
```

Production: `npm run build` rồi `npm start`.

Integration test tạo dữ liệu nên phải chạy backend riêng. Terminal 1:

```powershell
cd D:\GOLANG\BACKEND
$env:HTTP_ADDR = '127.0.0.1:9080'
$env:DATA_FILE = Join-Path $env:TEMP ('nexa-test-' + [guid]::NewGuid() + '.json')
go run ./cmd/api
```

Terminal 2:

```powershell
cd D:\GOLANG\FRONTEND
$env:BACKEND_URL = 'http://127.0.0.1:9080'
npm run dev -- --hostname 127.0.0.1 --port 3001
```

Terminal 3: `cd D:\GOLANG\FRONTEND` rồi `npm run test:integration`. Đổi URL bằng `TEST_FRONTEND_URL` nếu cần. Bộ test bao gồm cookie, origin, CRUD/PUT, tìm kiếm/phân trang, automation, phản hồi, hội thoại đóng, quyền owner/agent, tenant isolation và thu hồi phiên.

## Giới hạn

MVP local, chưa realtime/polling, push notification hay cache server-state toàn ứng dụng. Dùng **Làm mới** để nhận cập nhật từ người khác. Phản hồi kiến thức dùng tìm từ khóa cục bộ của backend, chưa có LLM. Web/email/phone là phân loại nội bộ, chưa tích hợp nhà cung cấp. Trước khi public cần bổ sung quy trình mời nhân viên/đổi mật khẩu và vận hành phù hợp.
