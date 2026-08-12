import 'package:flutter_test/flutter_test.dart';
import 'package:uav_tracking_app/widgets/drone_spatial_index.dart';

void main() {
  test('spatial index returns the nearest UAV and ignores distant taps', () {
    final index = DroneSpatialIndex();
    expect(index.resetIfChanged(1), isTrue);
    index.add('UAV-0001', const Offset(10, 10));
    index.add('UAV-0002', const Offset(42, 10));

    expect(index.findNearest(const Offset(39, 11)), 'UAV-0002');
    expect(index.findNearest(const Offset(200, 200)), isNull);
  });

  test('spatial index is retained until its render signature changes', () {
    final index = DroneSpatialIndex();
    index.resetIfChanged(7);
    index.add('UAV-0001', const Offset(10, 10));

    expect(index.resetIfChanged(7), isFalse);
    expect(index.findNearest(const Offset(10, 10)), 'UAV-0001');
    expect(index.resetIfChanged(8), isTrue);
    expect(index.findNearest(const Offset(10, 10)), isNull);
  });
}
