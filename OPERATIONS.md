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

Trong cùng tenant, các chuỗi thư mới từ cùng địa chỉ email được liên kết với một hồ sơ khách đang hoạt động. Nếu đã có hồ sơ trùng, owner có thể gộp hồ sơ từ màn hình chi tiết khách hàng. Hai hồ sơ cần có cùng email hoặc một hồ sơ chưa có email. Lịch sử chuyển sang hồ sơ đích và thời hạn phiên cổng khách được giữ nguyên.

Cấu hình biến môi trường cho tiến trình Go:

| Biến | Tác dụng |
|---|---|
| `APP_ORIGIN` | URL HTTPS công khai của frontend, dùng trong liên kết mời và khôi phục. |
| `RESEND_API_KEY` | Khóa Resend dùng để gửi email. |
| `EMAIL_FROM` | Địa chỉ gửi đã xác minh trên Resend. |
| `SUPPORT_REPLY_TO` | Địa chỉ hộp thư hỗ trợ nhận phản hồi. |
| `INBOUND_EMAIL_TENANT` | Tenant đích. Có thể lấy từ URL widget hoặc `/api/v1/auth/me`. |
| `INBOUND_EMAIL_SECRET` | Bí mật ngẫu nhiên từ 32 ký tự dùng giữa gateway nhận thư và API. |

Nếu dùng gateway hộp thư, cấu hình gateway để chuẩn hóa email nhận được thành JSON và gửi `POST /api/v1/email/inbound` trực tiếp đến backend. Gateway truyền `Authorization: Bearer <INBOUND_EMAIL_SECRET>`. Giữ nguyên `id` giữa các lần thử lại và dùng cùng `thread_id` cho chuỗi thư.

```json
{
  "id": "message-unique-id",
  "thread_id": "existing-mail-thread-id",
  "from": "Khach Hang <customer@example.com>",
  "subject": "Yêu cầu hỗ trợ",
  "text": "Tôi cần trợ giúp."
}
```

Gửi email trong hội thoại trả HTTP `202` và trạng thái `pending` sau khi lưu hàng chờ. Worker gửi và thử lại theo khoảng chờ tăng dần trong tối đa 23 giờ tính từ lúc tạo tin, dùng cùng mã gửi phía Resend. Nút **Kiểm tra lại** dùng chung lịch gửi và cơ chế giữ lượt với worker.

- Trạng thái `sent` cho biết Resend đã tiếp nhận yêu cầu. Dùng `email_provider_id` trong bản ghi tin nhắn để tra trạng thái phát thư thực tế trong Resend. Mốc phản hồi SLA được lưu khi Resend tiếp nhận email.
- Trạng thái `failed` dừng gửi tự động và phát thông báo trong workspace. Cấu hình hệ thống tập trung log cảnh báo theo sự kiện `email reply requires reconciliation`. Log có tenant, mã tin nhắn, số lần thử và mã lỗi gần nhất.
- Khi không có mã Resend, đối chiếu người nhận, tiêu đề và giờ gửi với nhật ký Resend trước khi gửi một tin mới. Việc gửi lại sau cửa sổ 23 giờ cần xác nhận thủ công vì yêu cầu trước đó có thể đã được tiếp nhận trong khi ứng dụng mất kết nối.

## AI nâng cao

Đặt `OPENAI_API_KEY` và tùy chọn `OPENAI_CHAT_MODEL` trên backend. Model soạn nháp mặc định là `gpt-4.1-mini`.

- RAG xếp hạng theo cosine tối đa 5 bài từ 100 bài đang bật cập nhật gần nhất. `text-embedding-3-small` tạo 256 chiều. Khi mở tìm kiếm hoặc tạo bản nháp lần đầu, backend tự tạo vector còn thiếu và lưu theo tenant, ID bài viết và nội dung. PostgreSQL cần pgvector và quyền tạo hoặc sử dụng extension. Backend dùng bảng có RLS theo tenant và chỉ mục IVFFlat. File store hoặc PostgreSQL thiếu pgvector dùng cosine trong bộ nhớ. Trình soạn nháp giữ link tới bài được dùng làm nguồn.
- Triage tự lưu nhãn, ưu tiên và cảm xúc khi kết quả đạt ngưỡng. Các gợi ý chưa đạt ngưỡng chờ nhân viên duyệt trong chi tiết phiếu. `AI_TRIAGE_CONFIDENCE_THRESHOLD` mặc định 0,85.
- `AI_CHATBOT_AUTO_REPLY=true` bật bot cho widget web, Zalo OA và WhatsApp ở hội thoại chưa phân công. `AI_CHATBOT_CONFIDENCE_THRESHOLD` mặc định 0,70. Bot dùng bài đã bật và công khai để trả lời câu hỏi nghiệp vụ. Thiếu căn cứ, khách tức giận hoặc có tệp sẽ tạo phiếu bàn giao. Sau câu trả lời có nguồn, khách có thể trả lời **Đã giải quyết** để bot đóng hội thoại. Khách dùng Zalo hoặc WhatsApp sẽ nhận tin mời chấm CSAT từ 1 đến 5.
- Đặt `AI_TRIAGE_CALIBRATION` và `AI_CHATBOT_CALIBRATION` bằng hai mảng JSON, mỗi mảng gồm 10 phần tử `{"correct":0,"total":0}` tương ứng các khoảng điểm 0–0,1, 0,1–0,2,…, 0,9–1. Đếm kết quả đúng trên bộ mẫu đã gắn nhãn, tách riêng từng tác vụ và dùng đúng model đang triển khai. Với triage, chỉ tính đúng khi nhãn, ưu tiên và cảm xúc đều được chấp nhận. Với chatbot, kiểm cả căn cứ, nội dung và quyết định trả lời. Mỗi khoảng cần ít nhất 20 mẫu. Điều kiện tự động dùng cận dưới Wilson 95% của tỷ lệ đúng. Thiếu dữ liệu hiệu chuẩn thì triage chờ duyệt và chatbot bàn giao. Hiệu chuẩn lại khi đổi model hoặc dữ liệu thực tế thay đổi.
- SLA ước lượng p50/p90 cho phản hồi đầu tiên và xử lý bằng hồi quy phân vị riêng theo tenant. Biến dự báo gồm kênh, ưu tiên, giờ tạo phiếu theo ICT và tải hàng chờ của người phụ trách. Tải quá khứ được dựng từ vòng đời ticket và người phụ trách hiện lưu. Dùng tối đa 2.000 mẫu hoàn tất gần nhất trong 60 ngày cho mỗi mốc. Dưới 20 mẫu, backend dùng mốc mặc định. `AI_SLA_AUTO_ESCALATE=true` chuyển phiếu có nguy cơ trễ cho owner.
- Ghi âm tối đa 45 giây hoặc tải tệp tối đa 4 MiB. `gpt-transcribe` hoặc `OPENAI_TRANSCRIPTION_MODEL=whisper-1` tạo bản chép lời. Backend xác định ý định và cảm xúc từ nội dung chép lời, sau đó cho phép chỉnh văn bản trước khi gửi. `AI_CHAT_WIDGET_VOICE=true` bật ghi âm cho khách đã có phiên chat.
- Báo cáo phân tích theo khoảng ngày hiển thị số và tỷ lệ hội thoại bot tự giải quyết đã có xác nhận của khách, tỷ lệ yêu cầu giải quyết trước khi tạo phiếu, CSAT trung bình theo kênh, thời gian phản hồi trung bình theo ưu tiên và chi phí AI ước tính cho mỗi phiếu mới. Đặt `AI_USD_RATES` dạng JSON theo tên model. Dùng `input_per_million` và `output_per_million` cho token, hoặc `per_minute` cho âm thanh tính theo thời lượng. Báo cáo chia tổng chi phí AI có đơn giá trong kỳ cho số phiếu mới cùng kỳ và hiển thị số lượt chưa định giá.

## Zalo OA, WhatsApp và Resend inbound

1. Đặt `NEXA_CHANNEL_CREDENTIALS` bằng JSON sau trên backend, thay các giá trị ví dụ bằng thông tin từ kho bí mật của môi trường triển khai.

   ```json
   {
     "tenant-id": {
       "zalo": {
         "account_id": "111",
         "app_id": "222",
         "access_token": "<OA access token>",
         "webhook_secret": "<OA Secret Key>"
       },
       "whatsapp": {
         "account_id": "333",
         "access_token": "<Cloud API token>",
         "webhook_secret": "<Meta App Secret>",
         "verify_token": "<Meta Webhook Verify Token>",
         "graph_version": "v25.0"
       }
     }
   }
   ```

2. Trong **Tích hợp → Kênh**, nhập JSON công khai gồm `account_id` và thêm `app_id` cho Zalo. `account_id` là OA ID cho Zalo hoặc Phone Number ID cho WhatsApp. Bật và lưu kênh. Dùng phiên bản Graph còn được hỗ trợ cho ứng dụng. Xoay access token Zalo trong kho bí mật và nạp lại cấu hình backend trước khi token hết hạn.
3. Đăng ký HTTPS callback trực tiếp vào backend tại `/api/v1/channel-webhooks/{tenant}/zalo` hoặc `/api/v1/channel-webhooks/{tenant}/whatsapp`. Proxy cần giữ nguyên raw body và header chữ ký. Chọn đúng callback cho tài khoản đã khai báo và đăng ký sự kiện tin nhắn. Các endpoint quản trị gửi thử và DLQ dùng BFF và phiên owner.
4. Từ một tài khoản khách đã mở phiên nhắn tin hợp lệ, gửi tin vào Zalo OA hoặc WhatsApp, kiểm tra hội thoại và trả lời từ hộp thư. Dùng **Gửi tin kiểm thử** để kiểm tra outbound. Chức năng gửi dùng văn bản trong cửa sổ nhắn tin của nhà cung cấp. Tệp đến được đánh dấu để nhân viên kiểm tra trên kênh nguồn.
5. Tin đến được dedup bằng ID đã xác thực của nhà cung cấp trong phạm vi tenant và tài khoản. Mỗi lượt gửi có `X-Nexa-Delivery` cố định. Trạng thái **Nhà cung cấp đã tiếp nhận** xác nhận API gửi trả mã tin nhắn. Hàng đợi thử lại khi bị giới hạn tần suất và chuyển lỗi vĩnh viễn hoặc kết quả chưa xác định vào DLQ. Khi chưa xác định, đối soát trên kênh trước khi chọn **Thử lại**.
6. Để Resend gửi webhook trực tiếp, đăng ký `email.received` trỏ tới `POST /api/v1/email/resend`. Đặt `INBOUND_EMAIL_TENANT`, `RESEND_INBOUND_ADDRESS`, `RESEND_INBOUND_WEBHOOK_SECRET` và `RESEND_INBOUND_API_KEY` có quyền đọc thư đến. Backend xác thực Svix và lấy nội dung văn bản từ Resend. Gateway cũ vẫn có thể dùng `/api/v1/email/inbound` với bearer token và `X-Nexa-Delivery` cố định cho mỗi lần gửi lại.

Tham khảo [kiến trúc OA OpenAPI](https://oa.zalo.me/home/documents/guides/Khoi-tao-ung-dung-va-cap-quyen_117071366476220195), [WhatsApp Business Platform](https://developers.facebook.com/documentation/business-messaging/whatsapp/overview) và [Resend Receiving API](https://resend.com/docs/api-reference/emails/retrieve-received-email).

## Quan sát backend và workflow nền

Đặt `METRICS_TOKEN` bằng bí mật ngẫu nhiên ít nhất 32 byte để bật `GET /metrics` trên backend. Cấu hình Prometheus gửi header `Authorization: Bearer …` từ mạng nội bộ. Endpoint mặc định trả 404 khi chưa cấu hình hoặc thiếu quyền.

- Dùng `platform_http_requests_total` theo nhóm status để theo dõi lưu lượng và tỷ lệ 5xx. Dùng histogram `platform_http_request_duration_seconds` để tính p95/p99 bằng `histogram_quantile`. Kết nối SSE được loại khỏi histogram.
- Theo dõi histogram `platform_postgres_write_lock_wait_seconds` và bộ đếm `platform_postgres_write_lock_errors_total` khi tăng số replica hoặc tải ghi.
- Migration `002_workflow_activity.sql` điền lại thời điểm hoạt động từ dữ liệu hiện có và lập chỉ mục ticket chưa giải quyết. Hãy đo thời gian migration trên bản sao staging trước khi nâng cấp database lớn vì bước backfill cập nhật các row hiện có.
- Migration `004_pending_workflow_jobs.sql` lập chỉ mục job đang chờ. PostgreSQL trả tối đa 25 job mỗi lượt theo thứ tự tạo; worker lấy khóa row bằng `FOR UPDATE SKIP LOCKED` rồi ghi kết quả và cập nhật nghiệp vụ trong cùng transaction. Lịch sử job hoàn thành nằm ngoài chỉ mục hàng chờ. Worker workflow, API danh sách/chi tiết, dashboard, đọc ngữ cảnh cho bản nháp, lưu tài nguyên, gửi tin và các thao tác phân công/SLA trên phiếu đã dùng transaction theo tenant. Đọc có phạm vi không lấy advisory lock ghi. PostgreSQL giữ khóa riêng của tenant cùng một khóa tương thích dùng chung ở chế độ shared; các luồng ghi cũ vẫn lấy khóa tương thích ở chế độ exclusive. Schema `platform_v2` và role RLS thuộc bước cutover riêng.

## Docker Compose và kiểm tra tích hợp

Từ thư mục gốc, dùng Docker với Compose v2 và chạy:

```powershell
docker compose up --build --wait
```

Mở `http://127.0.0.1:3000`. Nếu cổng 3000 đã được dùng, đặt `$env:NEXA_PORT = '3001'` trước khi chạy Compose và mở cổng tương ứng. Backend và PostgreSQL chỉ lắng nghe trong mạng Compose. Probe `GET /readyz` của backend trả 204 khi sẵn sàng và 503 nếu PostgreSQL không phản hồi. Endpoint `GET /healthz` tiếp tục kiểm tra tiến trình. Volume `postgres-data` giữ dữ liệu khi chạy `docker compose down`.

Stack này dùng thông tin đăng nhập cố định cho môi trường thử nghiệm cục bộ. Khi chạy bộ kiểm thử BFF, luôn dùng stack và volume riêng vì test tạo workspace và dữ liệu nghiệp vụ. Chạy từ thư mục gốc:

```powershell
$env:NEXA_PORT = '3001'
docker compose -p nexa-test up --build --wait
$env:TEST_FRONTEND_URL = 'http://127.0.0.1:3001'
npm --prefix FRONTEND run test:integration
docker compose -p nexa-test down
```

GitHub Actions chạy kiểm thử Go với PostgreSQL/pgvector, kiểm tra lint và TypeScript, build hai image và chạy kiểm thử BFF qua stack Compose riêng. Pipeline kết thúc ở bước xác nhận bản build. Triển khai tự động cần một môi trường đích, quyền truy cập registry và quy trình rollback.


## Chạy thử trên cloud quản lý

- Tạo staging riêng với dữ liệu giả lập. Dùng PostgreSQL quản lý có backup và PITR, object storage tương thích S3 qua danh tính dịch vụ, và bộ thu Prometheus trong mạng nội bộ. Ghi lại vùng triển khai và quyền của role API, worker, migration trước đợt cutover.
- Diễn tập PITR vào một instance riêng. Tắt gửi email, webhook và các tích hợp có tác dụng bên ngoài trước khi mở bản phục hồi. Đối chiếu số lượng bản ghi theo tenant, kiểm tra đăng nhập và quyền truy cập, rồi đo thời điểm dữ liệu mới nhất đã phục hồi và tổng thời gian khôi phục để xác định RPO/RTO thực tế.
- Chạy `k6 run BACKEND/loadtest/staging.js` với `CONFIRM_ISOLATED_STAGING=yes`, `STAGING_API_URL` trỏ backend staging, và `STAGING_API_TOKENS` gồm các session token ngắn hạn của tenant thử, phân cách bằng dấu phẩy. Nạp token từ kho bí mật hoặc biến môi trường của runner. Tắt tích hợp gửi ngoài và dùng database có thể dọn lại vì bài test có tạo ticket.
- Mặc định bài chạy thử ở 5 RPS trong 30 giây với 10% thao tác ghi. Tăng dần bằng `STAGING_RPS` và chỉnh `STAGING_VUS`, `STAGING_DURATION`, `STAGING_WRITE_PERCENT` theo mục tiêu. Ngưỡng mặc định là p95 dưới 300 ms, p99 dưới 1.200 ms, tỷ lệ sai status dưới 0,1% và không có vòng lặp bị bỏ. Đối chiếu từng đợt với histogram chờ khóa PostgreSQL trước khi tăng tiếp. Dùng `STAGING_P95_MS=500` cho cổng cutover W2. Sau khi staging đạt các đợt ngắn, đặt `STAGING_RPS=2000`, `STAGING_VUS=1000` và `STAGING_DURATION=24h` để chạy đợt W5; bộ phát tải phải đủ năng lực để duy trì tốc độ đã đặt.
