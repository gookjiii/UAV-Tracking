import 'dart:convert';

enum DroneType { all, enemy, ally, undefined }

enum OrbitType { unknown, circle, straight, zigzag }

class Position {
  const Position({
    required this.latitude,
    required this.longitude,
    required this.altitude,
  });

  final double latitude;
  final double longitude;
  final double altitude;

  factory Position.fromJson(Map<String, dynamic> json) => Position(
    latitude: (json['latitude'] as num?)?.toDouble() ?? 0,
    longitude: (json['longitude'] as num?)?.toDouble() ?? 0,
    altitude: (json['altitude'] as num?)?.toDouble() ?? 0,
  );

  static Position lerp(Position start, Position end, double t) => Position(
    latitude: start.latitude + (end.latitude - start.latitude) * t,
    longitude: start.longitude + (end.longitude - start.longitude) * t,
    altitude: start.altitude + (end.altitude - start.altitude) * t,
  );
}

class DronePositionUpdate {
  const DronePositionUpdate({
    required this.droneId,
    required this.type,
    required this.orbitType,
    required this.position,
    required this.speedMs,
    required this.headingDeg,
    required this.timestamp,
  });

  final String droneId;
  final DroneType type;
  final OrbitType orbitType;
  final Position position;
  final double speedMs;
  final double headingDeg;
  final DateTime timestamp;

  factory DronePositionUpdate.fromJson(Map<String, dynamic> json) =>
      DronePositionUpdate(
        droneId: json['drone_id']?.toString() ?? 'UAV-UNKNOWN',
        type: _parseDroneType(json['type']),
        orbitType: _parseOrbitType(json['orbit_type']),
        position: Position.fromJson(
          Map<String, dynamic>.from(json['position'] as Map? ?? const {}),
        ),
        speedMs: (json['speed_m_s'] as num?)?.toDouble() ?? 0,
        headingDeg: (json['heading_deg'] as num?)?.toDouble() ?? 0,
        timestamp:
            DateTime.tryParse(json['timestamp']?.toString() ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0, isUtc: true),
      );

  double get speedKmh => speedMs * 3.6;

  String get typeName => switch (type) {
    DroneType.enemy => 'ENEMY',
    DroneType.ally => 'ALLY',
    _ => 'UNDEFINED',
  };

  String get orbitName => switch (orbitType) {
    OrbitType.circle => 'CIRCLE',
    OrbitType.straight => 'STRAIGHT',
    OrbitType.zigzag => 'ZIGZAG',
    _ => 'UNKNOWN',
  };
}

List<DronePositionUpdate> parseDroneSnapshot(String rawJson) {
  final decoded = jsonDecode(rawJson);
  if (decoded is! List) {
    throw const FormatException('SSE snapshot must be a JSON array');
  }
  return decoded
      .whereType<Map>()
      .map(
        (item) => DronePositionUpdate.fromJson(Map<String, dynamic>.from(item)),
      )
      .toList(growable: false);
}

DroneType _parseDroneType(Object? value) {
  final normalized = value?.toString().toUpperCase() ?? '';
  if (normalized == '1' || normalized.contains('ENEMY')) {
    return DroneType.enemy;
  }
  if (normalized == '2' || normalized.contains('ALLY')) {
    return DroneType.ally;
  }
  return DroneType.undefined;
}

OrbitType _parseOrbitType(Object? value) {
  final normalized = value?.toString().toUpperCase() ?? '';
  if (normalized == '1' || normalized.contains('CIRCLE')) {
    return OrbitType.circle;
  }
  if (normalized == '3' || normalized.contains('ZIGZAG')) {
    return OrbitType.zigzag;
  }
  if (normalized == '2' || normalized.contains('STRAIGHT')) {
    return OrbitType.straight;
  }
  return OrbitType.unknown;
}
