# UAV Tracking Lab

Dự án nghiên cứu mô phỏng và hiển thị trạng thái UAV theo thời gian thực. Backend dùng Go, gRPC Gateway, NATS JetStream và PostgreSQL; một codebase Flutter chạy trên Android, iOS, web, Windows, macOS và Linux.

## Kiến trúc

```text
Simulation Engine
      │ immutable snapshot
      ▼
Telemetry Pipeline ──► Memory cache + 500-point ring history
      ├──────────────► SSE ──► Flutter canvas renderer
      ├──────────────► NATS JetStream (latest message/UAV)
      └──────────────► PostgreSQL (5-minute sample, 7 days)

gRPC / REST ──► current state, merged history, simulation control
```

Luồng dữ liệu chỉ ghi cache một lần. NATS và PostgreSQL có queue giới hạn nên không tạo backlog làm chậm simulator. Ở 10.000 UAV, Flutter dùng canvas và clustering thay vì tạo 10.000 widget marker.

## Hướng dẫn khởi chạy

Yêu cầu: Docker Desktop đang chạy. Từ thư mục gốc dự án:

```bash
cd /Users/mac/Documents/UAV_tracking
docker compose up --build -d
docker compose ps
curl -sS http://127.0.0.1:8080/healthz
```

Khi `uav-server`, `uav-postgres` và `uav-nats` đều healthy, mở dashboard tại
<http://127.0.0.1:8080>. Các endpoint hữu ích:

- Health: <http://127.0.0.1:8080/healthz>
- Swagger: <http://127.0.0.1:8080/swagger/>
- NATS monitor: <http://127.0.0.1:8222>

Docker tự biên dịch Flutter web và Go server từ source; không dùng binary trong
`bin/`. Nếu image đã được build, dùng `docker compose up -d` để khởi động nhanh.

### Chạy thử 10.000 UAV

Giữ simulator chạy ở 300 ms và đổi quần thể bằng REST:

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/simulation/control \
  -H 'Content-Type: application/json' \
  -d '{"target_drone_count":10000,"update_interval_ms":300,"active":true}'
```

Theo dõi `target_drones`, `current_drones`, `snapshot_age_ms` và các bộ đếm
`dropped_*` trong `/healthz`. Với 10.000 UAV, máy lab nên có ít nhất khoảng
2 GB RAM trống; có thể giảm `MEMORY_HISTORY_POINTS` khi máy yếu.

### Chia sẻ bằng Cloudflare Tunnel (khuyến nghị thay ngrok)

Cài Cloudflare Tunnel một lần trên macOS:

```bash
brew install cloudflared
```

Trong terminal khác, khởi chạy tunnel tới backend local:

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

Lệnh sẽ in URL dạng `https://<random>.trycloudflare.com`. Mở URL đó để dùng
dashboard Flutter đã build sẵn; API và SSE dùng same-origin nên không cần chạy
Flutter lần nữa. Tunnel Quick Tunnel chỉ dành cho nghiên cứu, URL thay đổi khi
khởi động lại và không có SLA.

Nếu muốn chạy Flutter web ở chế độ development với tunnel backend:

```bash
cd /Users/mac/Documents/UAV_tracking/app
flutter run -d chrome --dart-define=API_BASE_URL=https://<random>.trycloudflare.com
```

Ngrok free không còn là đường dẫn mặc định: interstitial `ERR_NGROK_6024` và
quota bandwidth có thể chặn toàn bộ SSE. Nếu vẫn dùng ngrok, thay URL trong
Settings/`API_BASE_URL` và chỉ dùng cho payload nhỏ.

### Kết nối từ điện thoại/laptop trong cùng LAN

Lấy IP máy chạy Docker (ví dụ `192.168.1.10`), rồi chạy Flutter:

```bash
cd /Users/mac/Documents/UAV_tracking/app
flutter run -d chrome --dart-define=API_BASE_URL=http://192.168.1.10:8080
flutter run -d <device-id> --dart-define=API_BASE_URL=http://192.168.1.10:8080
```

Điện thoại và máy backend phải cùng mạng Wi-Fi; firewall macOS phải cho phép
TCP port `8080`. Đây là lựa chọn ổn định nhất để benchmark 10.000 UAV.

### Dừng và xem log

```bash
docker compose logs -f server
docker compose down                 # giữ nguyên volume PostgreSQL
docker compose down -v              # chỉ dùng khi muốn xóa dữ liệu nghiên cứu
```

## Chạy khi phát triển

Backend yêu cầu Go 1.25, PostgreSQL và NATS:

```bash
make build
./bin/server
```

Flutter 3.44:

```bash
cd app
flutter pub get
flutter run -d chrome
flutter run -d macos
flutter run -d <device-id> --dart-define=API_BASE_URL=http://192.168.1.10:8080
```

Mobile/desktop có thể đổi và lưu Server URL trong nút Settings. Web mặc định dùng same-origin.

## Cấu hình backend

| Biến | Mặc định | Giới hạn / ý nghĩa |
|---|---:|---|
| `TARGET_DRONE_COUNT` | `10` | 1–10.000 |
| `UPDATE_INTERVAL_MS` | `300` | 100–2.000 ms |
| `SSE_INTERVAL_MS` | `300` | 100–5.000 ms |
| `MEMORY_HISTORY_POINTS` | `500` | Ring history/UAV |
| `HISTORY_SAMPLE_INTERVAL` | `5m` | Chu kỳ ghi PostgreSQL |
| `RETENTION_DAYS` | `7` | Partition retention |
| `NATS_MAX_BYTES` | `134217728` | Giới hạn JetStream RAM |

Xem `docker-compose.yml` cho DSN/port đầy đủ.

## Kiểm tra và build

```bash
make test
make analyze
make web
make macos
```

CI build web/Android/Linux trên Ubuntu, iOS/macOS trên macOS và Windows trên Windows. Build iOS trong CI dùng `--no-codesign`; ký và phát hành store nằm ngoài phạm vi nghiên cứu.

Codegen protobuf là thao tác riêng, không chạy ngầm khi build:

```bash
make proto
```

### Baseline hiệu suất

Microbenchmark ngày 2026-08-12 trên Apple M1 Pro với 10.000 UAV:

| Hot path | Thời gian | Allocation |
|---|---:|---:|
| Simulator tick | ~0,89 ms | 2 allocs/op |
| Cache `SetBatch` | ~0,46 ms | 329 allocs/op |
| SSE JSON stream encode | ~5,43 ms | 10.001 allocs/op |

Đây là benchmark CPU cục bộ, không thay thế bài đo FPS/heap 10 phút với NATS, PostgreSQL và client qua LAN. Chi tiết trạng thái xác minh nằm trong audit.

Smoke end-to-end 21 giây đạt snapshot age 50 ms và không drop ở 10.000 UAV/300 ms; RSS server khoảng 708 MiB với cấu hình ring mặc định 500 điểm/UAV. Có thể giảm `MEMORY_HISTORY_POINTS` nếu máy lab ít RAM.

## Giới hạn nghiên cứu

- Không có auth, RBAC, HA hoặc cloud autoscaling.
- HTTP LAN được cho phép để demo; khi đưa ra mạng công cộng phải dùng HTTPS và giới hạn CORS.
- Chế độ 10.000 UAV dùng clustering thích ứng; muốn chọn từng UAV cần zoom đến khi còn tối đa khoảng 2.000 mục trong viewport.
- Mốc hiệu suất mục tiêu: map ≥20 FPS, UI phản hồi <100 ms và dữ liệu trễ <1 giây trên máy lab/LAN.

Đánh giá chi tiết nằm tại [docs/AUDIT.md](docs/AUDIT.md).
