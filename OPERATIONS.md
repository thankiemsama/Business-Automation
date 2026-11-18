# Vận hành hỗ trợ khách hàng

- Trong **Hộp hội thoại** và **Phiếu hỗ trợ**, chọn **Nhận việc** hoặc chọn người phụ trách. Dùng **Của tôi** và **Chưa phân công** để lọc. Hộp hội thoại và widget nhận cập nhật trực tiếp qua SSE.
- Trong **Đội ngũ**, chủ doanh nghiệp có thể chỉnh sửa, vô hiệu hóa và gửi lời mời. Nhân viên đổi mật khẩu trong **Tài khoản**. Liên kết khôi phục có hiệu lực 30 phút và lời mời có hiệu lực 48 giờ. Đổi hoặc đặt lại mật khẩu kết thúc các phiên đăng nhập cũ.
- Trong hội thoại, chọn **Ghi chú nội bộ** để trao đổi trong đội. Chủ doanh nghiệp quản lý **Câu trả lời mẫu** ngay trong trình soạn tin. Tệp PNG, JPEG và PDF có giới hạn 32 KiB.
- SLA dùng giờ liên tục kể từ khi tạo phiếu. Hạn phản hồi/xử lý theo mức thấp, thường, cao và khẩn lần lượt là 8/72, 4/48, 2/24 và 1/8 giờ. Phiếu hiện cảnh báo trước 30 phút. **Chuyển cấp** gán phiếu quá hạn cho chủ doanh nghiệp. Phản hồi được ghi khi nhân viên trả lời trong hội thoại liên kết. Với phiếu độc lập, chọn **Ghi nhận đã phản hồi** sau khi liên hệ khách hàng.
- **Báo cáo hỗ trợ** trên trang tổng quan lọc theo ngày Việt Nam và hiển thị theo nhân viên. Tồn là số phiếu chưa xử lý tại thời điểm xem. Khách hàng có thể chấm mức hài lòng sau khi hội thoại đóng.

## Quản trị, bảo mật và tích hợp doanh nghiệp

- **Bảo mật** cho owner và agent quản lý TOTP, mã khôi phục dùng một lần và các phiên đang hoạt động. Đặt `NEXA_SECURITY_KEY` bằng khóa ngẫu nhiên 32 byte mã hóa base64 trên backend. Lưu dự phòng khóa riêng khỏi dữ liệu.
- **Quản trị** có nhật ký thay đổi, xuất JSON theo tenant hoặc khách hàng và bản sao lưu AES-GCM. Nhật ký lưu trạng thái trước–sau và mã băm cho trường nhạy cảm. Sao lưu bao gồm hồ sơ nghiệp vụ, không gồm session token và khóa đăng nhập. Giải mã bằng `go run ./cmd/backup-unseal -tenant TENANT_ID -input nexa-backup.enc -output nexa-data.json` với cùng khóa bảo mật. Công cụ tạo tệp mới và không nhập đè dữ liệu.
- **Allowlist** áp dụng với phiên nhân viên và API công khai có khóa. Khi chạy qua Next.js trên loopback, đặt cùng `NEXA_PROXY_SECRET` (ít nhất 32 ký tự) cho frontend và backend. Chỉ bật `NEXA_TRUST_INGRESS_IP=true` khi ingress tin cậy đã ghi đè `X-Nexa-Ingress-IP` và chặn truy cập công khai trực tiếp vào frontend/backend nội bộ. Dùng CIDR bao gồm IP hiện tại.
- **SSO** dùng `OIDC_GOOGLE_ISSUER`, `OIDC_OKTA_ISSUER` hoặc `OIDC_MICROSOFT_ISSUER`, cùng cặp `*_CLIENT_ID` và `*_CLIENT_SECRET`. Google có issuer mặc định. Microsoft cần issuer cụ thể của directory. Đăng ký callback `APP_ORIGIN/api/oidc/{google|okta|microsoft}/callback`. Liên kết chủ thể nhà cung cấp với tài khoản đã có bằng `NEXA_OIDC_BINDINGS={"https://accounts.google.com|SUB":"agent@example.com"}`. Tài khoản đã bật TOTP vẫn cần mã sau SSO.
- **White-label** hỗ trợ tên, logo HTTPS, màu và ánh xạ tên miền. Cần tự trỏ DNS, cài TLS và chuyển tiếp host tại ingress. Callback SSO dùng `APP_ORIGIN` chuẩn.
- **Stripe** dùng `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRO_PRICE`, `STRIPE_ENTERPRISE_PRICE` và `APP_ORIGIN`. Trỏ webhook tới `POST /api/v1/stripe/webhook` trên backend, bật các sự kiện subscription created/updated/deleted và cấu hình Billing Portal. Gói mặc định là Free. Số liệu theo tháng gồm tin nhắn, phiếu và yêu cầu đã xác thực. Bộ đếm yêu cầu ghi theo lô; chưa áp hạn mức gói.
- **CRM** chỉ bật cho `NEXA_CRM_TENANT` đã cấu hình. HubSpot dùng `HUBSPOT_TOKEN`. Salesforce dùng `SALESFORCE_ORIGIN`, `SALESFORCE_TOKEN` và `SALESFORCE_EXTERNAL_ID_FIELD`. Odoo JSON-2 dùng `ODOO_ORIGIN`, `ODOO_API_KEY`, `ODOO_EXTERNAL_ID_FIELD` và tùy chọn `ODOO_DATABASE`. Tạo trường external ID duy nhất trên hệ thống đích. Màn hình Quản trị đồng bộ một chiều từng liên hệ.
- **Giao diện** dùng next-intl cho shell, đăng nhập cơ bản, phím tắt và trợ giúp. Các màn hình nghiệp vụ vẫn có nội dung tiếng Việt. Dùng `?` để mở phím tắt. PWA chỉ lưu trang ngoại tuyến tĩnh, không cache dữ liệu khách hàng.

## Analytics và workflow

- **Báo cáo hỗ trợ** dùng ngày Việt Nam và tuần bắt đầu thứ Hai. Biểu đồ dùng phiếu tạo trong kỳ. SLA tính trên các hạn phản hồi và xử lý đã đến trong kỳ, ghi nhận người thực hiện khi có và người đang phụ trách khi còn chờ. Kỳ lịch sử được dựng lại từ dữ liệu hiện có, bao gồm ưu tiên hiện tại. Phiếu cũ thiếu mốc giải quyết có thể thiếu dữ liệu SLA.
- Cohort nhóm khách theo tuần mở phiếu đầu tiên và đo tỷ lệ mở lại phiếu trong W1–W8. Các tuần chưa kết thúc để trống. PDF tải từ dashboard và dùng cùng khoảng ngày.
- **Workflow và lịch gửi** cho chủ doanh nghiệp tạo chuỗi hành động, điều kiện AND và nhánh đúng/sai. Trigger hỗ trợ phiếu mới, tin nhắn khách, phiếu không hoạt động và lần vi phạm SLA đầu tiên. Quy tắc từ khóa cũ tiếp tục chạy cho đến khi chủ doanh nghiệp chọn **Chuyển sang workflow**. Sau khi chuyển, quy tắc cũ tắt và workflow giữ cách khớp ít nhất một từ khóa. Hành động tự giải quyết phiếu dùng ngày cập nhật gần nhất của phiếu hoặc hội thoại liên quan làm mốc chờ.
- Worker kiểm tra trigger theo lịch khoảng 15 giây. Hành động và bản ghi chống lặp được lưu cùng giao dịch. Phiếu do workflow tạo không kích hoạt trigger phiếu mới và không thể sinh thêm phiếu qua một workflow khác.
- Lịch tuần/tháng gửi tóm tắt và CSV của kỳ đã kết thúc gần nhất tới thành viên đang hoạt động. Cần `RESEND_API_KEY` và `EMAIL_FROM`. Lịch dùng giờ Việt Nam; ngày gửi hàng tháng từ 1 đến 28. Lịch và workflow chỉ tự chạy khi tiến trình Go đang hoạt động.

## Công cụ không gian làm việc

- Dùng **Ctrl+K** hoặc **Cmd+K** để mở tìm nhanh. Chọn **Sáng**, **Tối** hoặc **Theo hệ thống** trên thanh công cụ.
- Nhãn được nhập bằng dấu phẩy. Bộ lọc nhãn khớp tất cả nhãn được chọn. Chế độ xem đã lưu thuộc tài khoản hiện tại.
- Có thể sửa tối đa 100 mục trong một thao tác hàng loạt. Nếu một mục không hợp lệ, toàn bộ thao tác bị hủy.
- Báo cáo, khách hàng và phiếu hỗ trợ hỗ trợ CSV và XLSX. Có thể truyền `since=YYYY-MM-DD` và `to=YYYY-MM-DD` vào `/api/v1/reports/export`.
- Trung tâm thông báo lưu tin nhắn mới, lượt phân công và lần vi phạm SLA đầu tiên của phiếu. Với phiếu hoặc hội thoại chưa phân công, thông báo đến các thành viên đang hoạt động.
- SSE cần proxy chuyển tiếp luồng và tắt bộ đệm. Workspace dùng `GET /api/v1/events` với `Last-Event-ID` hoặc `since` để đồng bộ lại khi kết nối gián đoạn. Widget sử dụng luồng riêng theo phiên khách.
- Web Push cần HTTPS hoặc localhost, `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` và `VAPID_SUBJECT` trên backend. Tạo cặp khóa VAPID một lần và giữ nguyên trong các lần triển khai. Đặt chủ thể là địa chỉ liên hệ vận hành, ví dụ `mailto:admin@example.com`. Mỗi người dùng bật Web Push trong **Thông báo** trên từng thiết bị. Push chỉ hiển thị nội dung chung, chi tiết nằm trong ứng dụng.

## Trung tâm trợ giúp và tích hợp
- Bật **Xuất bản trong trung tâm trợ giúp công khai** cho bài được phép chia sẻ. Đường dẫn đa doanh nghiệp là `/kb/{tenant}` và `/kb/{tenant}/{slug}`. Bài nội bộ có thể tiếp tục dùng để soạn nháp.
- Cổng khách dùng `/portal/{tenant}`. Mã email chỉ dùng một lần. Cổng cho gửi PNG, JPEG hoặc PDF tối đa 32 KiB khi tạo phiếu. Email đăng nhập và khảo sát CSAT cần cấu hình Resend và `APP_ORIGIN`.
- Webhook hỗ trợ `ticket.created`, `message.new`, `sla.breached` và `csat.submitted`. Đích phải là máy chủ công khai. Xác thực HMAC-SHA256 qua `X-Nexa-Signature`, dùng `X-Nexa-Delivery` để khử trùng lặp. Phản hồi ngoài 2xx được thử lại với giãn cách.
- API công khai có quyền đọc khách hàng, phiếu và hội thoại. Dùng `Authorization: Bearer nexa_…`. Mở Swagger UI tại `/api-docs` trên frontend để thử API. Tài liệu OpenAPI trực tiếp nằm ở `/api/public/openapi.json` trên backend.
- Zalo, Messenger, WhatsApp, SMS và Instagram hiện chỉ có màn hình lưu cấu hình. Chưa có adapter nhà cung cấp. Hồ sơ 360° hiển thị lịch sử hỗ trợ và các kênh đã có dữ liệu. LTV theo doanh thu cần nguồn dữ liệu giao dịch.

## Chat trên website

Trong **Hộp hội thoại**, chủ doanh nghiệp chọn **Mở widget hỗ trợ** để lấy URL công khai. Có thể dùng URL làm liên kết hoặc nhúng trang đó bằng iframe:

```html
<iframe
  src="https://support.example.com/chat/TENANT_ID"
  title="Hỗ trợ khách hàng"
  width="380"
  height="640"
  style="max-width: 100%; border: 0"
></iframe>
```

## Email

Cấu hình biến môi trường cho tiến trình Go:

| Biến | Tác dụng |
|---|---|
| `APP_ORIGIN` | URL HTTPS công khai của frontend, dùng trong liên kết mời và khôi phục. |
| `RESEND_API_KEY` | Khóa Resend dùng để gửi email. |
| `EMAIL_FROM` | Địa chỉ gửi đã xác minh trên Resend. |
| `SUPPORT_REPLY_TO` | Địa chỉ hộp thư hỗ trợ nhận phản hồi. |
| `INBOUND_EMAIL_TENANT` | Tenant đích. Có thể lấy từ URL widget hoặc `/api/v1/auth/me`. |
| `INBOUND_EMAIL_SECRET` | Bí mật ngẫu nhiên từ 32 ký tự dùng giữa gateway nhận thư và API. |

Cấu hình gateway hộp thư để chuẩn hóa email nhận được thành JSON và gửi `POST /api/v1/email/inbound` trực tiếp đến backend. Gateway truyền `Authorization: Bearer <INBOUND_EMAIL_SECRET>`. Giữ nguyên `id` giữa các lần thử lại và dùng cùng `thread_id` cho chuỗi thư.

```json
{
  "id": "message-unique-id",
  "thread_id": "existing-mail-thread-id",
  "from": "Khach Hang <customer@example.com>",
  "subject": "Yêu cầu hỗ trợ",
  "text": "Tôi cần trợ giúp."
}
```

Khi email gửi trong hội thoại chưa thành công, giao diện hiện **Email chưa gửi · Thử lại**. Có thể thử lại trong 23 giờ với cùng mã gửi để tránh gửi trùng.

## AI nâng cao

Cấu hình `OPENAI_API_KEY` trên backend. Có thể đặt `OPENAI_CHAT_MODEL` để chọn model soạn nháp, mặc định là `gpt-4.1-mini`. Tìm kiếm dùng `text-embedding-3-small` trên tối đa 100 bài kiến thức đang bật gần đây nhất. Khi nhân viên chọn **Soạn nháp từ kiến thức**, backend gửi câu hỏi và nội dung kiến thức cần tìm đến OpenAI. Bản nháp được đặt trong ô soạn cùng nguồn để nhân viên kiểm tra và quyết định gửi. Khi thiếu dữ kiện, hệ thống mở phiếu bàn giao hoặc dùng phiếu đang mở của hội thoại. Nếu thiếu khóa, chức năng soạn nháp dùng tìm kiếm từ khóa trong kho kiến thức.

- Tìm kiếm nội bộ dùng từ khóa và cosine similarity, tối đa 5 kết quả. Vector bài viết được giữ trong bộ nhớ theo tenant và nội dung. Bản nháp lấy tối đa 4 nguồn. Tóm tắt được tạo khi mở hội thoại hoặc phiếu có ít nhất 8 tin nhắn, bỏ qua ghi chú nội bộ và lưu vào phiếu liên quan.
- Tiến trình nền phân loại cảm xúc và đề xuất nhãn, độ ưu tiên cho dữ liệu mới trong 24 giờ gần nhất. Nhân viên xác nhận trước khi thay đổi phiếu.
- Bật `AI_CHATBOT_AUTO_REPLY=true` để tự trả lời cho chat web chưa phân công. AI dùng các bài đã bật và xuất bản, hoặc hỗ trợ chung cho một số lời chào/yêu cầu trợ giúp đơn giản. Nội dung thiếu căn cứ, có tệp hoặc khách tức giận sẽ được bàn giao. Điểm tin cậy từ mô hình dùng để lọc và chưa được hiệu chuẩn thành xác suất.
- Bộ dự báo SLA dùng trung vị lịch sử và hàng chờ. Mặc định chỉ cảnh báo. Bật `AI_SLA_AUTO_ESCALATE=true` để tự chuyển phiếu có nguy cơ cao cho chủ doanh nghiệp. Quy tắc hiện áp dụng sau khi đã dùng ít nhất 75% thời hạn.
- Ghi âm trong trình soạn hội thoại giới hạn 45 giây hoặc tải tối đa 4 MiB. `gpt-transcribe` chuyển âm thanh thành bản nháp để nhân viên xem trước. Máy chủ chuyển âm thanh sang OpenAI khi người dùng thao tác. Bật `AI_CHAT_WIDGET_VOICE=true` để cho phép khách đang có phiên chat chuyển tối đa 5 đoạn/giờ. `OPENAI_TRANSCRIPTION_MODEL=whisper-1` cho phép dùng Whisper cũ trong thời gian OpenAI còn hỗ trợ.
