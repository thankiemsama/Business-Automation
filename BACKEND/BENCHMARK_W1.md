# W1 · Benchmark ghi một ticket trên 100.000 bản ghi

Đo ngày 19/11/2026 trên Windows amd64, Intel Core i5-4210U, với PostgreSQL 16.11 chạy trên loopback. Hai adapter dùng cùng một bộ dữ liệu giả lập gồm 100.000 ticket trải trên 100 mã tenant. Mỗi lượt đọc và cập nhật một ticket cố định qua Unit of Work; bộ đếm bắt đầu trước transaction và kết thúc sau commit hoặc ghi đồng bộ file. Nạp dữ liệu nằm ngoài thời gian đo.

| Adapter | Lượt ghi | Trung bình | p95 ghi | Byte cấp phát mỗi lượt |
| --- | ---: | ---: | ---: | ---: |
| File JSON | 60 | 1.045,192 ms | 1.410 ms | 462.852.811 |
| PostgreSQL hiện tại | 60 | 3,048 ms | 4,303 ms | 10.068 |

p95 giảm khoảng **328 lần**, đạt ngưỡng **100 lần** cho phép đo cục bộ này. Kết quả phản ánh một luồng ghi tuần tự qua adapter hiện tại, với fixture gồm ticket và không có tệp đính kèm. Cần đo lại khi chạy song song trên schema đích và vùng staging để xác định công suất, p95 API và độ trễ khi chờ khóa.

Tái lập từ thư mục `BACKEND` với `TEST_DATABASE_URL` trỏ đến PostgreSQL thử nghiệm. Benchmark tạo và dọn một schema riêng.

```powershell
go test ./internal/repository -run '^$' -bench '^BenchmarkSingleTicketWrite100k$' -benchtime=60x -count=1
```
