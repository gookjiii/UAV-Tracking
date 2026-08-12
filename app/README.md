# UAV Tracking Lab — Flutter client

Một Flutter codebase cho Android, iOS, web, Windows, macOS và Linux.

```bash
cd /Users/mac/Documents/UAV_tracking/app
flutter pub get
flutter analyze
flutter test
flutter run -d chrome
```

Web mặc định kết nối same-origin. Native mặc định dùng `http://127.0.0.1:8080`; đổi server LAN trong Settings hoặc khi chạy:

```bash
flutter run -d <device> --dart-define=API_BASE_URL=http://192.168.1.10:8080
```

Để chia sẻ backend ra Internet, dùng Cloudflare Quick Tunnel:

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

Mở URL `https://<random>.trycloudflare.com` được in ra. Nếu chạy Flutter ở chế
độ development hoặc trên thiết bị native, truyền URL backend:

```bash
flutter run -d chrome --dart-define=API_BASE_URL=https://<random>.trycloudflare.com
flutter run -d <device-id> --dart-define=API_BASE_URL=https://<random>.trycloudflare.com
```

Với 10.000 UAV, app tự dùng SSE chunked (`?chunked=1`) để tránh một event quá
lớn. Ngrok free có quota bandwidth/interstitial nên không phù hợp cho stream
liên tục; Cloudflare Tunnel hoặc LAN được khuyến nghị. Sau khi sửa code, dùng
hot restart (`R`) hoặc refresh toàn bộ trang để tạo lại SSE connection.

UI giữ tiếng Anh để thống nhất thuật ngữ kỹ thuật. State management dùng Provider/Selector; realtime dùng SSE qua `package:http`; server URL được lưu bằng `SharedPreferencesAsync`.
