import 'package:flutter_test/flutter_test.dart';
import 'package:uav_tracking_app/core/app_config.dart';
import 'package:uav_tracking_app/models/drone.dart';

void main() {
  test('parses protobuf JSON enum names and numeric values', () {
    final snapshot = parseDroneSnapshot('''
      [{
        "drone_id":"UAV-0001",
        "type":"DRONE_TYPE_ENEMY",
        "orbit_type":3,
        "position":{"latitude":21.1,"longitude":105.8,"altitude":500},
        "speed_m_s":20,
        "heading_deg":90,
        "timestamp":"2026-01-01T00:00:00Z"
      }]
    ''');

    expect(snapshot, hasLength(1));
    expect(snapshot.single.type, DroneType.enemy);
    expect(snapshot.single.orbitType, OrbitType.zigzag);
    expect(snapshot.single.speedKmh, 72);
  });

  test('normalizes backend URL and rejects unsupported schemes', () {
    expect(
      AppConfig.normalizeApiBaseUrl(' http://192.168.1.10:8080/ '),
      'http://192.168.1.10:8080',
    );
    expect(
      () => AppConfig.normalizeApiBaseUrl('ftp://example.com'),
      throwsFormatException,
    );
  });
}
