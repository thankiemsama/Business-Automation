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
- **Stripe** dùng `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRO_PRICE`, `STRIPE_ENTERPRISE_PRICE` và `APP_ORIGIN`. Trỏ webhook tới `POST /api/v1/stripe/webhook` trên backend, bật `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted` và `invoice.paid`, rồi cấu hình Billing Portal.
- Cấu hình hạn mức bằng `NEXA_BILLING_<FREE|PRO|ENTERPRISE>_<TICKETS|AI>_LIMIT`. Biến rỗng hoặc 0 nghĩa là không giới hạn. Hạn mức tính theo tháng dương lịch, giờ Việt Nam. Tạo phiếu và giữ lượt gọi AI dùng chung giao dịch với bộ đếm tương ứng. Vượt mức trả HTTP 402. Hạn mức AI theo ngày của tenant vẫn áp dụng.
- Đặt `NEXA_BILLING_<PRO|ENTERPRISE>_<TICKETS|AI>_INVOICE_CREDITS` để cấp thêm lượt trong tháng khi Stripe xác nhận hóa đơn đăng ký mới hoặc gia hạn đã thanh toán, khớp subscription, customer và price. Mỗi hóa đơn được cấp một lần. Phần vượt mức cơ bản tiêu thụ lượt đã cấp. Nexa không phát sinh khoản thu theo lượt. Owner xem số dùng và lượt còn lại trong bảng mức sử dụng, hoặc đăng ký webhook Nexa `credit_balance.low` để nhận cảnh báo ở ngưỡng 80%.
- **CRM** chỉ bật cho `NEXA_CRM_TENANT` đã cấu hình. HubSpot dùng `HUBSPOT_TOKEN`. Salesforce dùng `SALESFORCE_ORIGIN`, `SALESFORCE_TOKEN` và `SALESFORCE_EXTERNAL_ID_FIELD`. Odoo JSON-2 dùng `ODOO_ORIGIN`, `ODOO_API_KEY`, `ODOO_EXTERNAL_ID_FIELD` và tùy chọn `ODOO_DATABASE`. Tạo trường external ID duy nhất trên hệ thống đích. Màn hình Quản trị đồng bộ một chiều từng liên hệ.
- **Giao diện** dùng next-intl cho shell, đăng nhập cơ bản, phím tắt và trợ giúp. Các màn hình nghiệp vụ vẫn có nội dung tiếng Việt. Dùng `?` để mở phím tắt. PWA chỉ lưu trang ngoại tuyến tĩnh, không cache dữ liệu khách hàng.

## Analytics và workflow

- Trong **Tự động hóa → Định tuyến theo kỹ năng**, owner thêm nhóm, thành viên và giới hạn số hội thoại đang xử lý trên mỗi thành viên. Bật tự động phân công sau khi thêm quy tắc. Nexa dùng quy tắc khớp đầu tiên theo kênh, mức ưu tiên và nhãn hội thoại hoặc khách hàng. Đưa quy tắc nhãn VIP lên trước quy tắc chung.
- Với hội thoại mới, Nexa chọn thành viên có ít hội thoại đang xử lý nhất trong nhóm chính. Khi tải bằng nhau, ưu tiên người lâu nhất chưa được hệ thống phân công. Khi nhóm chính hết chỗ, Nexa xét nhóm dự phòng. Nếu cả hai nhóm hết chỗ, hội thoại xuất hiện trong danh sách chưa phân công để đội hỗ trợ nhận việc.
- Trong **Quản trị → Truy cập và thương hiệu**, owner có thể bật danh thiếp và dùng tên hiển thị hiện có, lời giới thiệu và liên kết HTTPS. Nexa gắn một danh thiếp vào phản hồi của nhân viên trong mỗi hội thoại. Widget hiển thị liên kết bấm được, Zalo OA và WhatsApp nhận chữ ký văn bản trong giới hạn độ dài của kênh.
- Trong cùng trang, owner có thể bật quy tắc bàn giao chatbot. Khi khách yêu cầu gặp nhân viên, chatbot bàn giao qua phiếu hỗ trợ. Cấu hình có thể nâng ngưỡng tin cậy tối thiểu. Dấu vết ghi lý do, mã chính sách, mô hình và mã hội thoại để phục vụ đối soát nội bộ.

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
- Sau 8 lần gửi không thành công, owner có thể mở **Tích hợp → Webhook → DLQ / Thử lại** trong 30 ngày. Lượt gửi lại dùng URL hiện tại của webhook, giữ nguyên nội dung và mã `X-Nexa-Delivery`, rồi bắt đầu lại chu kỳ thử. Sau 30 ngày, lượt gửi lỗi hết hạn hiển thị và gửi lại.
- API công khai có quyền đọc khách hàng, phiếu và hội thoại. Dùng `Authorization: Bearer nexa_…`. Mở Swagger UI tại `/api-docs` trên frontend để thử API. Tài liệu OpenAPI trực tiếp nằm ở `/api/public/openapi.json` trên backend.
- Zalo OA, WhatsApp và SMS qua Twilio nhận tin qua webhook và gửi qua hàng đợi. Messenger và Instagram có màn hình lưu cấu hình. Hồ sơ 360° hiển thị lịch sử hỗ trợ và các kênh đã có dữ liệu. LTV theo doanh thu cần nguồn dữ liệu giao dịch.

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

### Nhà cung cấp mô hình

Chọn một nhà cung cấp cho mỗi bản triển khai bằng `LLM_PROVIDER`. Mặc định là `openai` với các biến hiện có.

| Giá trị | Cấu hình backend | Khả năng |
| --- | --- | --- |
| `openai` | `OPENAI_API_KEY`, tùy chọn `OPENAI_CHAT_MODEL` | Chat, vector, chép lời |
| `deepseek` | `DEEPSEEK_API_KEY`, `LLM_CHAT_MODEL` | Chat và JSON. Các tính năng cần vector hoặc chép lời yêu cầu nhà cung cấp hỗ trợ. |
| `ollama` | `LLM_CHAT_MODEL`, tùy chọn `LLM_BASE_URL` và `LLM_API_KEY` | Chat và JSON. Có thể đặt `LLM_EMBEDDING_MODEL` nếu model hỗ trợ 256 chiều. |

Ollama mặc định dùng `http://127.0.0.1:11434/v1`. Với Docker, đặt URL có thể truy cập từ container backend, gồm hậu tố `/v1`. Ollama dùng vector trong bộ nhớ của backend. Khi đổi model, hiệu chuẩn lại ngưỡng trước khi bật quyết định tự động.

### QA thực thời

Owner bật **Bật QA thực thời cho bản nháp phản hồi** trong **Quản trị → Truy cập và thương hiệu** và có thể nhập quy trình, từ cấm để mô hình đối chiếu. Trong hội thoại, nhân viên chọn cho phép kiểm tra trước khi bản nháp và ngữ cảnh được gửi đến nhà cung cấp AI. Trình soạn xếp hàng sau khi người dùng ngừng nhập và vẫn cho phép gửi tin trong lúc chờ. Worker dùng quota AI theo doanh nghiệp. Kết quả là gợi ý để nhân viên cân nhắc. Owner xem tổng số nhãn cảnh báo theo loại, nhân viên và ngày trong **Phân tích → Kiểm tra chất lượng**.

Trong **Quản trị → Truy cập và thương hiệu**, bật **QA tự động cho văn bản, ảnh và bản chép lời** để xếp hàng từng tin nhắn mới và từng đoạn chép lời cuộc gọi nhập vào Nexa. Quản trị viên nên chọn model hỗ trợ ảnh PNG/JPEG qua Chat Completions. Khi dùng kho object, worker đọc ảnh qua khóa tệp đã xác minh. Ghi âm nhập vào Nexa đi qua bước chép lời trước khi được đánh giá rủi ro. Nội dung ảnh chỉ được gửi khi mô hình được cấu hình và tính năng được bật. [Giao diện ảnh trong Chat Completions](https://developers.openai.com/api/docs/guides/images-vision?api-mode=chat)

Trong **Phân tích → Kiểm tra chất lượng**, owner xem trạng thái, loại dữ liệu, nhãn và đường dẫn hội thoại có cảnh báo. Các lượt hết quota sẽ chờ kỳ kế tiếp. Tệp ngoài PNG/JPEG và nội dung ngoài kênh mà Nexa chỉ có tên tệp được đánh dấu chưa hỗ trợ hoặc đánh giá một phần. Phiên bản hiện tại xếp hàng dữ liệu được ghi từ lúc bật.

### AI Sales, CS và Ops

Trong **Quản trị → Truy cập và thương hiệu → Tính năng**, bật từng vai trò và nhập SOP riêng, tối đa 2.000 ký tự mỗi vai trò.

- **AI Sales** đề xuất một bước tư vấn khi có lead QR mới. Owner đọc gợi ý trong kết quả chiến dịch. Gợi ý dùng cho nhân viên duyệt và bị ẩn sau khi khách ngừng nhận lời nhắc.
- **AI CS** áp SOP nhóm hỗ trợ trong câu trả lời của chatbot hiện có. Quy tắc nguồn tri thức, ngưỡng tin cậy và bàn giao theo cấu hình vẫn áp dụng. Bật AI CS cùng luồng chatbot hiện có để đưa SOP vào sử dụng.
- **AI Ops** đề xuất bước xử lý nội bộ khi dự báo phát hiện rủi ro SLA mới. AI Ops dùng SOP và số liệu dự báo, gửi gợi ý qua thông báo nội bộ nếu phiếu còn rủi ro.


- RAG xếp hạng theo cosine tối đa 5 bài từ 100 bài đang bật cập nhật gần nhất. `text-embedding-3-small` tạo 256 chiều. Khi mở tìm kiếm hoặc tạo bản nháp lần đầu, backend tự tạo vector còn thiếu và lưu theo tenant, ID bài viết và nội dung. PostgreSQL cần pgvector và quyền tạo hoặc sử dụng extension. Backend dùng bảng có RLS theo tenant và chỉ mục IVFFlat. File store hoặc PostgreSQL thiếu pgvector dùng cosine trong bộ nhớ. Trình soạn nháp giữ link tới bài được dùng làm nguồn.
- Triage tự lưu nhãn, ưu tiên và cảm xúc khi kết quả đạt ngưỡng. Các gợi ý chưa đạt ngưỡng chờ nhân viên duyệt trong chi tiết phiếu. `AI_TRIAGE_CONFIDENCE_THRESHOLD` mặc định 0,85.
- `AI_CHATBOT_AUTO_REPLY=true` bật bot cho widget web, Zalo OA và WhatsApp ở hội thoại chưa phân công. `AI_CHATBOT_CONFIDENCE_THRESHOLD` mặc định 0,70. Bot dùng bài đã bật và công khai để trả lời câu hỏi nghiệp vụ. Thiếu căn cứ, khách tức giận hoặc có tệp sẽ tạo phiếu bàn giao. Sau câu trả lời có nguồn, khách có thể trả lời **Đã giải quyết** để bot đóng hội thoại. Khách dùng Zalo hoặc WhatsApp sẽ nhận tin mời chấm CSAT từ 1 đến 5.
- Đặt `AI_TRIAGE_CALIBRATION` và `AI_CHATBOT_CALIBRATION` bằng hai mảng JSON, mỗi mảng gồm 10 phần tử `{"correct":0,"total":0}` tương ứng các khoảng điểm 0–0,1, 0,1–0,2,…, 0,9–1. Đếm kết quả đúng trên bộ mẫu đã gắn nhãn, tách riêng từng tác vụ và dùng đúng model đang triển khai. Với triage, chỉ tính đúng khi nhãn, ưu tiên và cảm xúc đều được chấp nhận. Với chatbot, kiểm cả căn cứ, nội dung và quyết định trả lời. Mỗi khoảng cần ít nhất 20 mẫu. Điều kiện tự động dùng cận dưới Wilson 95% của tỷ lệ đúng. Thiếu dữ liệu hiệu chuẩn thì triage chờ duyệt và chatbot bàn giao. Hiệu chuẩn lại khi đổi model hoặc dữ liệu thực tế thay đổi.
- SLA ước lượng p50/p90 cho phản hồi đầu tiên và xử lý bằng hồi quy phân vị riêng theo tenant. Biến dự báo gồm kênh, ưu tiên, giờ tạo phiếu theo ICT và tải hàng chờ của người phụ trách. Tải quá khứ được dựng từ vòng đời ticket và người phụ trách hiện lưu. Dùng tối đa 2.000 mẫu hoàn tất gần nhất trong 60 ngày cho mỗi mốc. Dưới 20 mẫu, backend dùng mốc mặc định. `AI_SLA_AUTO_ESCALATE=true` chuyển phiếu có nguy cơ trễ cho owner.
- Ghi âm tối đa 45 giây hoặc tải tệp tối đa 4 MiB. `gpt-transcribe` hoặc `OPENAI_TRANSCRIPTION_MODEL=whisper-1` tạo bản chép lời. Backend xác định ý định và cảm xúc từ nội dung chép lời, sau đó cho phép chỉnh văn bản trước khi gửi. `AI_CHAT_WIDGET_VOICE=true` bật ghi âm cho khách đã có phiên chat.
- Báo cáo phân tích theo khoảng ngày hiển thị số và tỷ lệ hội thoại bot tự giải quyết đã có xác nhận của khách, tỷ lệ yêu cầu giải quyết trước khi tạo phiếu, CSAT trung bình theo kênh, thời gian phản hồi trung bình theo ưu tiên và chi phí AI ước tính cho mỗi phiếu mới. Đặt `AI_USD_RATES` dạng JSON theo tên model. Dùng `input_per_million` và `output_per_million` cho token, hoặc `per_minute` cho âm thanh tính theo thời lượng. Báo cáo chia tổng chi phí AI có đơn giá trong kỳ cho số phiếu mới cùng kỳ và hiển thị số lượt chưa định giá.

### Kiểm tra chất lượng, trích xuất và agent

- Bật từng tính năng trong **Quản trị → Truy cập và thương hiệu**. Kiểm tra chất lượng xếp hàng cho các lần giải quyết phiếu sau khi bật. Phiếu thiếu phản hồi của nhân viên có thể có nhãn và ghi nhận về khách hàng mà không có điểm. Xem kết quả ở chi tiết phiếu và ghi nhận gần nhất trong hồ sơ khách hàng 360°.
- Dashboard tính tỷ lệ chấm điểm trên các lượt đã xếp hàng trong kỳ. Số gợi ý chuyển duyệt, được chấp nhận và tự áp dụng dùng ngày phát sinh của từng sự kiện. Gợi ý được chấp nhận ở kỳ sau xuất hiện trong kỳ sau.
- **Quản trị → Trích xuất hàng loạt** hỗ trợ tối đa 50 phiếu và 10 trường mỗi đợt. AI đọc tiêu đề và mô tả để điền trường trống. Tiến độ phân biệt phiếu đã điền, bỏ qua, lỗi và đang chờ.
- Bản nháp FAQ dùng các trường hợp bot đã bàn giao vì tìm kho kiến thức công khai không có kết quả. Khi nhân viên trả lời và giải quyết phiếu, worker soạn bài riêng tư và để tắt. Chủ doanh nghiệp duyệt trong **Kho kiến thức** trước khi bật dùng hoặc công khai.
- Agent chỉ tra số liệu hỗ trợ của khách gắn với phiếu. Đề xuất giải quyết và gửi CSAT cần người tạo đề xuất duyệt trong 15 phút. Nếu bật chế độ tự giải quyết, khách phải gửi đúng «Đã giải quyết» sau phản hồi của nhân viên và hội thoại phải có đúng một phiếu đang mở, cùng khách hàng, với mức ưu tiên dưới Khẩn. Nhật ký ghi công cụ và phiếu liên quan.
- **AI teammate** có preset cảnh báo SLA và xử lý inbox. Preset inbox dùng thống kê hiệu chuẩn và kho kiến thức công khai của chatbot. Biến `AI_CHATBOT_AUTO_REPLY` vẫn điều khiển cấu hình chung của tiến trình.
- Quota theo lượt gọi OpenAI được đặt theo từng doanh nghiệp và đặt lại vào 00:00 giờ Việt Nam. Giá trị 0 cho phép không giới hạn. Quota đếm lượt gọi, kể cả lượt đã gửi mà nhà cung cấp trả lỗi. Job kiểm tra, trích xuất và soạn bài chuyển sang ngày tiếp theo khi hết quota. Một tác vụ có thể dùng nhiều lượt nếu vừa tạo vector vừa gọi mô hình.
- Owner có thể xuất **Câu trả lời mẫu** bằng mật khẩu và nhập mã ở doanh nghiệp khác. Mã chứa tiêu đề và nội dung đã mã hóa. Nhập lại cùng mã trong cùng doanh nghiệp dùng lại bản đã nhập.

### AI Sparring

- Owner bật **AI Sparring** trong **Quản trị → Truy cập và thương hiệu**. Tại **AI Sparring → Ngân hàng kịch bản**, chọn hội thoại nguồn và xem trước tối đa 24 lượt thoại. Nexa che các định danh phổ biến và các tên đã biết ngay trên máy chủ. Owner cần bỏ các chi tiết có thể nhận dạng còn sót, kể cả tên tự do, địa chỉ hoặc dữ kiện hiếm, trước khi duyệt. Bản xem trước không gọi OpenAI. Chỉ đưa vào ngân hàng các hội thoại mà doanh nghiệp có quyền sử dụng cho đào tạo.
- Mỗi phiên 1v1 có tối đa 8 lượt trả lời của học viên. AI đóng vai khách khó tính. Khi nộp bài, chọn giải quyết hoặc chuyển người xử lý. Hàng đợi dùng quota AI của doanh nghiệp và trả điểm cùng coaching. Phiên nằm riêng khỏi inbox và kết quả chỉ hiển thị cho học viên.
- Owner chọn **Kiểm định AI agent** sau khi tạo ít nhất một kịch bản khách đã xác nhận giải quyết và một kịch bản cần chuyển người xử lý. Model làm hai bài mô phỏng tự động. Mỗi bài cần ít nhất 80/100, đúng hành động và không bị đánh dấu xử lý thiếu an toàn. Điểm của học viên không được dùng để cấp chứng chỉ.
- Chứng chỉ nội bộ có hiệu lực 30 ngày và gắn với doanh nghiệp, model và phiên bản quy tắc giải quyết. Lưu trữ một kịch bản mà chứng chỉ dựa vào hoặc kiểm định lại không đạt sẽ khiến chứng chỉ mất hiệu lực. Kiểm định lại khi đổi phiên bản model. Cả đề xuất agent có người duyệt và luồng agent tự giải quyết đều yêu cầu chứng chỉ. Quyền xử lý thủ công của nhân viên tiếp tục theo phân quyền phiếu.

- Ngân hàng gợi ý hội thoại từ QA, sự kiện phân loại AI và các lượt agent có kết quả. Owner duyệt ẩn danh và gán quyết định chuẩn. Mỗi đợt kiểm định lưu bộ case trong `eval_sets` và chạy lại prompt ra quyết định trên dữ liệu mô phỏng. Khi ứng dụng khởi động sau khi đổi prompt, tenant từng chứng nhận model đó sẽ được xếp hàng kiểm định lại với bộ case cũ còn hoạt động. Giảm quá 5 điểm ở một case so với lần đạt gần nhất trên cùng bộ case sẽ chặn cấp chứng chỉ.
- **Quản trị → Dấu vết AI agent** hiển thị tối đa 200 sự kiện trong 7 ngày. Bản ghi chỉ gồm mã lượt, công cụ, kết quả, model, phiên bản prompt và thời điểm. Bộ đệm loại sự kiện hết hạn khi ghi tiếp. Chi phí AI tiếp tục tính theo metering của tenant.

### WFM và Video KB

- **Tổng quan → Phân tích** dự báo 7 ngày đầy đủ tiếp theo từ mốc báo cáo, dùng tối đa tám ngày cùng thứ và tối thiểu hai mẫu. Ngày chưa kết thúc không được dùng làm mẫu. Báo cáo lịch sử dự báo tại mốc cuối kỳ.
- Tỷ lệ phiếu AI chạm rồi nhân viên giải quyết dùng các phiếu đã giải quyết trong kỳ có dấu vết triage, agent hoặc bot trước lúc giải quyết. Phân nhóm theo kênh và ưu tiên. Phiếu áp dụng đề xuất của AI được tính cho AI dù có người duyệt. Phiếu thiếu nguồn xác định người giải quyết hiển thị riêng.
- Bài kiến thức hỗ trợ liên kết video HTTPS và bản chép lời. Tìm ngữ nghĩa đưa phần đầu bản chép lời vào chỉ mục và trả liên kết video cùng bài. Bài công khai hiển thị liên kết và bản chép lời.

### Ghi âm cuộc gọi và QR chiến dịch

- Khi bật **Nhập ghi âm và chép lời cuộc gọi**, owner chọn hồ sơ khách hàng và xác nhận quyền xử lý trước khi gửi tệp tối đa 4 MiB tới OpenAI. Bản chép lời nằm dưới dạng ghi chú nội bộ trong hội thoại Điện thoại. Nexa lưu bản chép lời và không lưu tệp âm thanh.
- Với dữ liệu ghi âm và hội thoại gửi tới nhà cung cấp, xác định căn cứ xử lý, thông báo cho người liên quan, thời hạn lưu, quyền tiếp cận và nghĩa vụ chuyển dữ liệu cá nhân ra nước ngoài nếu áp dụng. Đối chiếu [Luật Bảo vệ dữ liệu cá nhân](https://vanban.chinhphu.vn/?classid=1&docid=214590&pageid=27160&typegroupid=) và [Nghị định 356/2025/NĐ-CP](https://vanban.chinhphu.vn/?classid=1&docid=216387&pageid=27160).
- Tạo mã tại **Tích hợp → QR thu hút khách hàng** sau khi cấu hình kênh và `APP_ORIGIN`. Trang đích mở tài khoản kênh đã kết nối. Với Zalo, khách sao chép và gửi mã trong chat. Với WhatsApp, nội dung mã được điền sẵn. Lượt mở trang có thể bao gồm truy cập lặp. Lead được ghi khi Nexa nhận mã qua webhook.
- Lời nhắc tư vấn ở mốc 1 và 8 giờ cần sự đồng ý của khách và dừng khi khách nhắn thêm hoặc gửi STOP. Nexa kiểm tra lại hạn khi gửi và khi dời qua giờ yên lặng. Cửa sổ tính từ lần khách gửi mã xác thực là 48 giờ trên Zalo OA và 24 giờ với tin văn bản thường trên WhatsApp. Với cuộc gọi, SMS hoặc email quảng cáo, đối chiếu thêm Nghị định 91/2020/NĐ-CP. [Tin tư vấn Zalo OA](https://oa.zalo.me/home/documents/guides/tin-tu-van) · [Chính sách WhatsApp Business](https://whatsappbusiness.com/policy/) · [Nghị định 91/2020/NĐ-CP](https://vanban.chinhphu.vn/?docid=200773&pageid=27160)

- Lời nhắc từ QR gửi 08:00–20:00 giờ Việt Nam, tối đa hai tin trong 24 giờ trên cùng tài khoản kênh và người nhận. Tin chờ qua cửa sổ tương tác bị hủy. Tỷ lệ phản hồi lấy số khách có phản hồi chia cho số khách có tin được nhà cung cấp nhận gửi. Có tùy chọn tạo một phiếu theo dõi từ yêu cầu tư vấn, báo giá hoặc hỗ trợ. Chỉ số này chưa bao gồm xác nhận đã đọc.

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

### SMS qua Twilio

1. Chọn số Twilio có khả năng gửi và nhận SMS. Trong **Tích hợp → Kênh → SMS**, bật kênh và lưu `{"account_id":"+12025550123"}` với số thật ở dạng E.164.
2. Thêm đối tượng `sms` dưới khóa tenant trong `NEXA_CHANNEL_CREDENTIALS` trên backend. Đặt cả `access_token` và `webhook_secret` bằng Auth Token của tài khoản Twilio tương ứng.

   ```json
   {
     "account_id": "+12025550123",
     "twilio_sid": "<Twilio Account SID>",
     "access_token": "<Twilio Auth Token>",
     "webhook_secret": "<Twilio Auth Token>",
     "callback_url": "https://api.example.com/api/v1/channel-webhooks/<tenant>/sms"
   }
   ```

3. Trong Twilio, cấu hình **A message comes in** dùng HTTP POST đến đúng URL HTTPS đó, không thêm query string. Proxy chuyển nguyên body và header `X-Twilio-Signature`. Nexa dùng Message SID để bỏ qua callback trùng lặp.
4. Gửi SMS vào số đã cấu hình và phản hồi trong hội thoại Nexa. Owner cũng có thể gửi tin kiểm thử và xem hàng đợi lỗi trong **Tích hợp → Kênh → SMS**. Trạng thái đã gửi ghi nhận Twilio tiếp nhận yêu cầu. Twilio quản lý STOP/START. Phạm vi adapter là SMS văn bản hai chiều, gồm tin kiểm thử và phản hồi hỗ trợ.

## Quan sát backend và workflow nền

Đặt `METRICS_TOKEN` bằng bí mật ngẫu nhiên ít nhất 32 byte để bật `GET /metrics` trên backend. Cấu hình Prometheus gửi header `Authorization: Bearer …` từ mạng nội bộ. Endpoint mặc định trả 404 khi chưa cấu hình hoặc thiếu quyền.

- Dùng `platform_http_requests_total` theo nhóm status để theo dõi lưu lượng và tỷ lệ 5xx. Dùng histogram `platform_http_request_duration_seconds` để tính p95/p99 bằng `histogram_quantile`. Kết nối SSE được loại khỏi histogram.
- Theo dõi histogram `platform_postgres_write_lock_wait_seconds` và bộ đếm `platform_postgres_write_lock_errors_total` khi tăng số replica hoặc tải ghi.
- Migration `002_workflow_activity.sql` điền lại thời điểm hoạt động từ dữ liệu hiện có và lập chỉ mục ticket chưa giải quyết. Hãy đo thời gian migration trên bản sao staging trước khi nâng cấp database lớn vì bước backfill cập nhật các row hiện có.
- Migration `004_pending_workflow_jobs.sql` lập chỉ mục job đang chờ. PostgreSQL trả tối đa 25 job mỗi lượt theo thứ tự tạo; worker lấy khóa row bằng `FOR UPDATE SKIP LOCKED` rồi ghi kết quả và cập nhật nghiệp vụ trong cùng transaction. Lịch sử job hoàn thành nằm ngoài chỉ mục hàng chờ. Worker workflow, API danh sách/chi tiết, dashboard, đọc ngữ cảnh cho bản nháp, lưu tài nguyên, gửi tin và các thao tác phân công/SLA trên phiếu đã dùng transaction theo tenant. Đọc có phạm vi không lấy advisory lock ghi. PostgreSQL giữ khóa riêng của tenant cùng một khóa tương thích dùng chung ở chế độ shared; các luồng ghi cũ vẫn lấy khóa tương thích ở chế độ exclusive. Schema `platform_v2` và role RLS thuộc bước cutover riêng.

## Docker Compose và kiểm tra tích hợp

### Nexa Private ngoại tuyến

Trên máy chuẩn bị có mạng và cùng kiến trúc với máy đích, chạy `bash scripts/nexa-private.sh pack /duong-dan/nexa-bundle`. Thêm `--with-ai` để kèm image Ollama. Chuyển cả thư mục qua kênh phân phối nội bộ đã xác thực. Image và manifest có checksum để kiểm tra hỏng dữ liệu khi chuyển.

Trên máy đích có Docker Compose, Bash và OpenSSL, chạy `bash /duong-dan/nexa-bundle/nexa-private.sh install /duong-dan/nexa-bundle`. Lệnh kiểm tra checksum, load image, tạo mật khẩu cục bộ ở lần đầu và khởi động với `--pull never --no-build`. Giao diện mặc định ở `http://127.0.0.1:3000`. Backend, Postgres, bucket và Ollama chỉ tham gia mạng Docker nội bộ. Để phục vụ qua reverse proxy nội bộ, đặt `NEXA_PRIVATE_ORIGIN` đúng origin công khai của proxy và cấu hình TLS tại proxy.

Để dùng AI cục bộ, chép model có giấy phép phù hợp và Modelfile vào thư mục `models` của bundle. Thêm `COMPOSE_PROFILES=local-ai`, `NEXA_PRIVATE_LLM_PROVIDER=ollama` và `NEXA_PRIVATE_CHAT_MODEL=<tên-model>` vào `private.env`. Sau khi khởi động, chạy `docker compose --env-file private.env -f compose.private.yaml exec ollama ollama create <tên-model> -f /models/Modelfile` từ thư mục bundle. Trong Modelfile, dùng đường dẫn model dưới `/models`. Ollama chạy với cloud tắt. [Ollama trong Docker](https://docs.ollama.com/docker) · [Chế độ local-only](https://docs.ollama.com/faq)

Giữ lại `private.env` qua các lần nâng cấp và sao lưu Postgres, kho object cùng cấu hình trước khi nạp phiên bản mới. Bộ cài này dành cho mạng riêng. Phát hành appliance sản xuất còn cần diễn tập khôi phục, kiểm thử môi trường cách ly và chốt cơ chế license.

### Kho tệp tương thích MinIO/S3

Compose mặc định dùng Silo, nhánh tương thích MinIO, cùng volume `minio-data` và dịch vụ khởi tạo bucket riêng tư `nexa-attachments`. Đặt `NEXA_MINIO_IMAGE` và `NEXA_MINIO_CLIENT_IMAGE` nếu dùng image MinIO và client trong registry nội bộ. Đặt `NEXA_MINIO_USER` và `NEXA_MINIO_PASSWORD` trước khi chạy ngoài môi trường local.

Triển khai backend riêng có thể đặt `ATTACHMENT_S3_ENDPOINT`, `ATTACHMENT_S3_BUCKET`, `ATTACHMENT_S3_ACCESS_KEY`, `ATTACHMENT_S3_SECRET_KEY` và tùy chọn `ATTACHMENT_S3_REGION` trên bucket đã tạo. Dùng tài khoản được phép đọc và ghi object trong bucket. Khi không đặt endpoint, backend lưu base64 như trước. Khi đã dùng kho object, sao lưu bucket hoặc volume cùng cơ sở dữ liệu, rồi phục hồi cả hai trước khi mở backend.

Đặt `ATTACHMENT_MIGRATE_ON_READ=true` để chuyển từng tệp cũ khi được đọc qua luồng đã xác thực. Nexa ghi object, đọc lại để kiểm tra nội dung và chỉ thay bản ghi nếu base64 gốc vẫn khớp. Tệp cũ tiếp tục đọc được nếu lần chuyển thất bại. Giới hạn tải lên và định dạng API hiện giữ nguyên.

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
## Triển khai lên Render Free

Blueprint tạo Web Service và PostgreSQL 16 gói Free tại Singapore, trong môi trường Production của project Business-Automation. Next.js và Go chạy chung một container. API chỉ nghe trên loopback và tự chạy migration khi khởi động.

1. Trong [Render Dashboard](https://dashboard.render.com/), chọn **New → Blueprint**, kết nối repo `thankiemsama/Business-Automation` và chọn nhánh `main`. Kiểm tra hai tài nguyên đều dùng gói **Free** rồi áp dụng Blueprint.
2. Khi service báo **Live**, mở URL của `nexa-web`, đăng ký hoặc đăng nhập, rồi tạo và tải lại một ticket để kiểm tra API và PostgreSQL.

Render Free có thể ngủ sau 15 phút không hoạt động. PostgreSQL Free hết hạn sau 30 ngày và không có bản sao lưu do Render quản lý. Tích hợp nhận webhook bên ngoài cần ingress riêng cho các đường dẫn callback.

## Multi-replica: SSE fanout qua NATS

- Moi backend replica co bo phat su kien SSE rieng trong bo nho (memory fanout). Khi chi chay 1 replica, mo hinh nay duoc dung. Khi chay 2+ backend replicas, phai bat NATS de su kien duoc fan ra tat ca replicas.
- Cau hinh (moi backend): dat NEXA_NATS_URL (vi du nats://nats:4222), tuy chon NEXA_NATS_USER / NEXA_NATS_PASSWORD, tuy chon NEXA_FANOUT_EPOCH (chung mot gia tri cho tat ca replicas de dong cursor).
- compose.yaml da co service nats (JetStream, memory stream NEXA_SSE). Bat vi: NEXA_NATS_URL=nats://nats:4222 trong bien .env roi docker compose up -d.
- Khong dat NEXA_NATS_URL: backend van chay binh thuong voi fanout in-memory (1 process). Chi chay nhieu replica SAU khi da bat NATS va da do load (xem CUTOVER_SCHEMA_V2.md).
- Khoi luong: stream NEXA_SSE dung memory storage, toi da 1 gio. Khong can ban dau vao NATS.
- Kiem chung: chay loadtest/sse-latency.mjs (node loadtest/sse-latency.mjs voi NEXA_SSE_API_URL va NEXA_SSE_TOKENS) de do muong refresh SSE sau tung viec ghi. Tat ca streams phai nhan refresh trong han; stream nao khong bao gio nhan tu nhieu la dau hieu fanout chua duoc noi NATS.
