import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:uav_tracking_app/services/sse_client.dart';

void main() {
  test('parses snapshot events and ignores heartbeat comments', () async {
    http.BaseRequest? capturedRequest;
    final client = MockClient((request) async {
      capturedRequest = request;
      return http.Response(
        ': keep-alive\n\nid: 7\nevent: snapshot\ndata: [{"drone_id":"UAV-1"}]\n\n',
        200,
        headers: {'content-type': 'text/event-stream'},
      );
    });
    final events =
        await SseClient(
          client,
        ).connect(Uri.parse('http://localhost/stream')).toList();

    expect(events, hasLength(1));
    expect(events.single.id, '7');
    expect(events.single.event, 'snapshot');
    expect(events.single.data, contains('UAV-1'));
    expect(capturedRequest?.headers['ngrok-skip-browser-warning'], 'true');
  });
}
