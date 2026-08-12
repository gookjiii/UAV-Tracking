import 'dart:convert';

import 'package:http/http.dart' as http;

import '../models/drone.dart';

class ApiException implements Exception {
  const ApiException(this.message, {this.statusCode});

  final String message;
  final int? statusCode;

  @override
  String toString() => message;
}

class SimulationControlResult {
  const SimulationControlResult({
    required this.success,
    required this.activeDrones,
    required this.message,
  });

  final bool success;
  final int activeDrones;
  final String message;
}

class DroneApiService {
  DroneApiService(this._client, String baseUrl) : _baseUrl = baseUrl;

  final http.Client _client;
  String _baseUrl;

  set baseUrl(String value) => _baseUrl = value;

  static const _researchHeaders = <String, String>{
    'ngrok-skip-browser-warning': 'true',
  };

  Uri streamUri({bool chunked = false}) {
    final uri = Uri.parse('$_baseUrl/v1/drones/stream');
    if (!chunked) return uri;
    final isNgrok = uri.host.contains('ngrok');
    return uri.replace(
      queryParameters: {
        'chunked': '1',
        if (isNgrok) 'limit': '1000',
      },
    );
  }

  Future<List<Position>> fetchDroneHistory(
    String droneId, {
    int maxPoints = 500,
  }) async {
    final encodedId = Uri.encodeComponent(droneId);
    final url = Uri.parse(
      '$_baseUrl/v1/drones/$encodedId/history',
    ).replace(queryParameters: {'max_points': '$maxPoints'});
    final response = await _client
        .get(url, headers: _researchHeaders)
        .timeout(const Duration(seconds: 8));
    final data = _decodeObjectResponse(response);
    final history = data['history'] as List? ?? const [];
    return history
        .whereType<Map>()
        .map(
          (item) => Position.fromJson(
            Map<String, dynamic>.from(item['position'] as Map? ?? const {}),
          ),
        )
        .toList(growable: false);
  }

  Future<SimulationControlResult> controlSimulation({
    required int targetDroneCount,
    required int updateIntervalMs,
    required bool active,
  }) async {
    final response = await _client
        .post(
          Uri.parse('$_baseUrl/v1/simulation/control'),
          headers: const {
            'Content-Type': 'application/json',
            'ngrok-skip-browser-warning': 'true',
          },
          body: jsonEncode({
            'target_drone_count': targetDroneCount,
            'update_interval_ms': updateIntervalMs,
            'active': active,
          }),
        )
        .timeout(const Duration(seconds: 8));
    final data = _decodeObjectResponse(response);
    return SimulationControlResult(
      success: data['success'] == true,
      activeDrones: (data['active_drones'] as num?)?.toInt() ?? 0,
      message: data['message']?.toString() ?? '',
    );
  }

  Future<Map<String, dynamic>> fetchHealth() async {
    final response = await _client
        .get(Uri.parse('$_baseUrl/healthz'), headers: _researchHeaders)
        .timeout(const Duration(seconds: 4));
    return _decodeObjectResponse(response);
  }

  Map<String, dynamic> _decodeObjectResponse(http.Response response) {
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ApiException(
        'Server returned HTTP ${response.statusCode}',
        statusCode: response.statusCode,
      );
    }
    final decoded = jsonDecode(utf8.decode(response.bodyBytes));
    if (decoded is! Map) {
      throw const ApiException('Server returned an invalid JSON response');
    }
    return Map<String, dynamic>.from(decoded);
  }
}
