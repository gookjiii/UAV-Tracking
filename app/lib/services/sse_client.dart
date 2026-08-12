import 'dart:convert';

import 'package:http/http.dart' as http;

import 'drone_api_service.dart';

class SseEvent {
  const SseEvent({this.id, this.event = 'message', required this.data});

  final String? id;
  final String event;
  final String data;
}

class SseClient {
  const SseClient(this._client);

  final http.Client _client;

  Stream<SseEvent> connect(Uri uri) async* {
    final request =
        http.Request('GET', uri)
          ..headers['Accept'] = 'text/event-stream'
          ..headers['Cache-Control'] = 'no-cache'
          // ngrok's free browser interstitial (ERR_NGROK_6024) otherwise
          // replaces the stream with HTML when the caller is Chrome.
          ..headers['ngrok-skip-browser-warning'] = 'true';
    final response = await _client.send(request);
    if (response.statusCode != 200) {
      throw ApiException(
        'SSE connection returned HTTP ${response.statusCode}',
        statusCode: response.statusCode,
      );
    }

    String? id;
    var event = 'message';
    final dataLines = <String>[];
    await for (final line in response.stream
        .transform(utf8.decoder)
        .transform(const LineSplitter())) {
      if (line.isEmpty) {
        if (dataLines.isNotEmpty) {
          yield SseEvent(id: id, event: event, data: dataLines.join('\n'));
        }
        id = null;
        event = 'message';
        dataLines.clear();
        continue;
      }
      if (line.startsWith(':')) {
        continue;
      }
      final separator = line.indexOf(':');
      final field = separator < 0 ? line : line.substring(0, separator);
      var value = separator < 0 ? '' : line.substring(separator + 1);
      if (value.startsWith(' ')) {
        value = value.substring(1);
      }
      switch (field) {
        case 'id':
          id = value;
        case 'event':
          event = value;
        case 'data':
          dataLines.add(value);
      }
    }
  }
}
