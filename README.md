<p align="center">
  <img src="FRONTEND/public/nexa-192.png" alt="Logo Nexa" width="88">
</p>

<h1 align="center">Nexa · Business Automation</h1>

<p align="center">Nền tảng chăm sóc khách hàng và tự động hóa quy trình cho doanh nghiệp.</p>

> **Đang phát triển** · Dự án chưa có bản web công khai. Bạn có thể chạy Nexa trên máy cá nhân theo hướng dẫn bên dưới.

## Giới thiệu

Nexa tập trung hồ sơ khách hàng, hội thoại và phiếu hỗ trợ vào một không gian làm việc. Đội ngũ hỗ trợ có thể phân công công việc, theo dõi thời hạn phản hồi và tự động hóa các thao tác lặp lại.

## Tính năng

| Nhóm | Khả năng |
| --- | --- |
| Hỗ trợ khách hàng | Quản lý hồ sơ, hội thoại, phiếu hỗ trợ, ghi chú nội bộ và câu trả lời mẫu. |
| Kênh liên lạc | Widget chat cập nhật trực tiếp qua SSE. Email gửi và nhận qua Resend khi được cấu hình. |
| Kho kiến thức và AI | Quản lý bài hướng dẫn, xuất bản bài trong trung tâm trợ giúp và dùng AI hỗ trợ soạn phản hồi khi cấu hình OpenAI. |
| Tự động hóa | Thiết lập workflow theo sự kiện, điều kiện và nhánh xử lý. Lên lịch gửi báo cáo. |
| Báo cáo | Theo dõi khối lượng hỗ trợ, thời hạn SLA và mức độ hài lòng của khách hàng (CSAT). |
| Quản trị | Quản lý doanh nghiệp, tài khoản owner và agent, phiên đăng nhập và nhật ký hoạt động. |

## Công nghệ và cấu trúc

| Thành phần | Công nghệ và vai trò |
| --- | --- |
| [BACKEND](BACKEND/) | API và nghiệp vụ viết bằng Go. Lưu dữ liệu bằng JSON hoặc PostgreSQL. |
| [FRONTEND](FRONTEND/) | Giao diện tiếng Việt dùng Next.js, React, TypeScript và Tailwind CSS. |
| [compose.yaml](compose.yaml) | Khởi chạy frontend, backend và PostgreSQL cho môi trường local. |

## Chạy trên máy cá nhân

Nếu chưa có mã nguồn, chạy các lệnh sau trong thư mục bạn muốn lưu dự án. Nếu đã có mã nguồn, mở terminal tại thư mục gốc của dự án.

```powershell
git clone https://github.com/thankiemsama/Business-Automation.git
cd Business-Automation
```

### Docker Compose

Cần Docker và Docker Compose. Từ thư mục gốc, chạy:

```powershell
docker compose up --build
```

Mở [http://127.0.0.1:3000](http://127.0.0.1:3000).

### Go và Node.js

Cần Go 1.24 trở lên và Node.js 24. Mở hai terminal tại thư mục gốc của dự án.

Chạy API trong terminal thứ nhất.

```powershell
cd BACKEND
go run ./cmd/api
```

API mặc định nghe tại `http://127.0.0.1:8080` và lưu dữ liệu bằng JSON nếu chưa cấu hình `DATABASE_URL`. Kiểm tra API qua [http://127.0.0.1:8080/healthz](http://127.0.0.1:8080/healthz).

Chạy giao diện trong terminal thứ hai.

```powershell
cd FRONTEND
npm ci
npm run dev
```

Mở [http://localhost:3000](http://localhost:3000).

Với cả hai cách chạy, chọn **Tạo tài khoản** để tạo doanh nghiệp và tài khoản chủ doanh nghiệp. Sau khi đăng nhập, vào **Đội ngũ** để thêm nhân viên.

## Tài liệu

- [Backend](BACKEND/README.md) — cấu hình lưu trữ, API và kiểm thử.
- [Frontend](FRONTEND/README.md) — cấu hình môi trường và kiểm tra giao diện.
- [Vận hành và tích hợp](OPERATIONS.md) — chat, email, OpenAI, workflow và các tùy chọn quản trị.
