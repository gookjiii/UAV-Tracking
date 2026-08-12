import 'package:flutter/widgets.dart';

/// A compact 32 px grid used for pointer hit-testing on the UAV canvas.
///
/// The map layer resets this index only when its telemetry/filter revision or
/// camera signature changes. Animation-only repaints reuse the existing grid.
class DroneSpatialIndex {
  DroneSpatialIndex({this.cellSize = 32});

  final double cellSize;
  final Map<int, List<DroneHitTarget>> _cells = {};
  int? _signature;

  bool resetIfChanged(int signature) {
    if (_signature == signature) return false;
    _signature = signature;
    _cells.clear();
    return true;
  }

  void add(String droneId, Offset offset) {
    _cells
        .putIfAbsent(_key(_cell(offset.dx), _cell(offset.dy)), () => [])
        .add(DroneHitTarget(droneId, offset));
  }

  String? findNearest(Offset point, {double radius = 18}) {
    final centerX = _cell(point.dx);
    final centerY = _cell(point.dy);
    final cellRadius = (radius / cellSize).ceil();
    DroneHitTarget? nearest;
    var nearestDistance = radius;
    for (var x = centerX - cellRadius; x <= centerX + cellRadius; x++) {
      for (var y = centerY - cellRadius; y <= centerY + cellRadius; y++) {
        for (final target in _cells[_key(x, y)] ?? const <DroneHitTarget>[]) {
          final distance = (target.offset - point).distance;
          if (distance < nearestDistance) {
            nearest = target;
            nearestDistance = distance;
          }
        }
      }
    }
    return nearest?.droneId;
  }

  int _cell(double coordinate) => (coordinate / cellSize).floor();

  int _key(int x, int y) => Object.hash(x, y);
}

class DroneHitTarget {
  const DroneHitTarget(this.droneId, this.offset);

  final String droneId;
  final Offset offset;
}
