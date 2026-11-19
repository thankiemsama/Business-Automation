# Giai đoạn 3 — Business & Ecosystem

## Vertical SaaS

Khi đăng ký bằng email, chủ workspace có thể chọn Thương mại điện tử, F&B, SaaS, Giáo dục hoặc Logistics. Lựa chọn mặc định là workspace trống. Một lần ghi dữ liệu tạo cả chủ workspace lẫn bộ mẫu. Workspace hiện có và luồng đăng ký SSO giữ nguyên.

Mỗi bộ mẫu có một bài KB nội bộ, một workflow nhận diện tin nhắn, một workflow cảnh báo quá hạn SLA ưu tiên cao và một câu trả lời mẫu mời đánh giá CSAT. Workflow bắt đầu ở trạng thái tắt. Quản trị viên có thể xem, sửa và bật trong màn hình Workflow. Mẫu CSAT chỉ chèn văn bản khi agent chọn trong trình soạn tin. Mục tiêu SLA và cơ chế khảo sát hiện hành tiếp tục áp dụng.

Đo thời gian từ lúc bắt đầu đăng ký đến khi chủ workspace thấy đúng bộ mẫu. Báo cáo p50, p90 và tỷ lệ đăng ký thất bại theo ngành trước khi công bố mục tiêu 30 giây.

## Trình tự triển khai và điều kiện phát hành

| Hạng mục | Phạm vi đầu tiên | Điều kiện phát hành |
| --- | --- | --- |
| Marketplace | Danh mục mẫu có phiên bản, duyệt thủ công, cài đặt bản sao vào workspace. Bắt đầu với workflow và prompt khai báo. Widget chạy mã và thanh toán cho nhà phát hành triển khai sau. | Kiểm tra quyền sở hữu, cách ly tenant, cài lặp, gỡ cài và thu hồi phiên bản. Chốt hoa hồng 15–20%, hoàn tiền và đối soát trước khi thu phí. |
| White-label và usage | Xác minh quyền sở hữu domain trước khi định tuyến. Ghi nhận tin nhắn, phiếu và vi phạm SLA bằng định danh sự kiện ổn định. Chạy đối soát usage trước khi tính overage. | Kiểm tra TLS, domain trùng, cookie và định tuyến sai tenant. Đối soát sự kiện trùng, đến muộn và lỗi gửi trước khi bật tính phí. |
| Data Intelligence | Kết nối nguồn giao dịch có định danh khách hàng, loại tiền, hoàn tiền và thời điểm thống nhất. Tính LTV thực thu trước khi thử dự báo churn. | Chốt định nghĩa churn, tách dữ liệu theo thời gian, kiểm tra rò rỉ dữ liệu và đo chất lượng trên tập kiểm chứng. |
| Tích hợp | Thực hiện Slack, LINE và Telegram theo từng kênh. Làm thử Zendesk và Intercom bằng import có chế độ xem trước. | Dùng tài khoản thử nghiệm để kiểm tra xác thực webhook, sự kiện trùng và giới hạn gửi. Đối soát số bản ghi, ánh xạ và khôi phục import trước khi di chuyển dữ liệu thật. |
| On-prem / Hybrid | Thử triển khai Compose và Postgres trong môi trường riêng. Xác nhận yêu cầu mạng, lưu trữ, cập nhật ngoại tuyến và giấy phép. Đánh giá hàng đợi NATS theo yêu cầu triển khai. | Kiểm tra sao lưu và khôi phục, nâng cấp, cách ly tenant, giám sát và xử lý khi license hết hạn. |
| Mobile native | Xem xét sau Vertical SaaS và Marketplace dựa trên số liệu Web Push và PWA của agent di động. | Xác định khoảng trống về độ tin cậy thông báo hoặc thao tác di động trước khi cam kết React Native và push native. |
