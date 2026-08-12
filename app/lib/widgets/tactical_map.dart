import 'dart:async';
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';
import 'package:provider/provider.dart';

import '../models/drone.dart';
import '../providers/drone_provider.dart';
import '../theme/tactical_theme.dart';
import 'drone_spatial_index.dart';

class TacticalMapWidget extends StatelessWidget {
  const TacticalMapWidget({super.key});

  @override
  Widget build(BuildContext context) {
    final provider = context.read<DroneProvider>();
    return FlutterMap(
      options: const MapOptions(
        initialCenter: LatLng(21.0285, 105.8542),
        initialZoom: 2.5,
        minZoom: 1,
        maxZoom: 18,
      ),
      children: [
        TileLayer(
          urlTemplate:
              'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png',
          subdomains: const ['a', 'b', 'c', 'd'],
          userAgentPackageName: 'dev.uavtracking.uav_tracking_app',
        ),
        const _HistoryTrajectoryLayer(),
        DroneCanvasLayer(provider: provider),
        const RichAttributionWidget(
          showFlutterMapAttribution: false,
          attributions: [
            TextSourceAttribution('OpenStreetMap contributors'),
            TextSourceAttribution('CARTO'),
          ],
        ),
      ],
    );
  }
}

class _HistoryTrajectoryLayer extends StatelessWidget {
  const _HistoryTrajectoryLayer();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, List<Position>>(
      selector: (_, provider) => provider.historyTrajectory,
      builder: (_, history, _) {
        if (history.isEmpty) {
          return const SizedBox.shrink();
        }
        return PolylineLayer(
          polylines: [
            Polyline(
              points: history
                  .map((point) => LatLng(point.latitude, point.longitude))
                  .toList(growable: false),
              strokeWidth: 3.5,
              color: TacticalTheme.colorCyan,
            ),
          ],
        );
      },
    );
  }
}

class DroneCanvasLayer extends StatefulWidget {
  const DroneCanvasLayer({required this.provider, super.key});

  final DroneProvider provider;

  @override
  State<DroneCanvasLayer> createState() => _DroneCanvasLayerState();
}

class _DroneCanvasLayerState extends State<DroneCanvasLayer>
    with WidgetsBindingObserver {
  final ValueNotifier<int> _renderTick = ValueNotifier(0);
  final DroneSpatialIndex _hitIndex = DroneSpatialIndex();
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _startRendering();
  }

  void _startRendering() {
    _timer ??= Timer.periodic(const Duration(milliseconds: 50), (_) {
      _renderTick.value++;
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      _startRendering();
    } else {
      _timer?.cancel();
      _timer = null;
    }
  }

  @override
  Widget build(BuildContext context) {
    final camera = MapCamera.of(context);
    return MobileLayerTransformer(
      child: GestureDetector(
        behavior: HitTestBehavior.translucent,
        onTapUp: (details) {
          final droneId = _hitIndex.findNearest(details.localPosition);
          if (droneId != null) {
            widget.provider.selectDrone(droneId);
          }
        },
        child: CustomPaint(
          size: Size.infinite,
          painter: _DronePainter(
            provider: widget.provider,
            camera: camera,
            hitIndex: _hitIndex,
            repaint: Listenable.merge([widget.provider, _renderTick]),
          ),
        ),
      ),
    );
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _timer?.cancel();
    _renderTick.dispose();
    super.dispose();
  }
}

class _DronePainter extends CustomPainter {
  _DronePainter({
    required this.provider,
    required this.camera,
    required this.hitIndex,
    required super.repaint,
  });

  final DroneProvider provider;
  final MapCamera camera;
  final DroneSpatialIndex hitIndex;

  @override
  void paint(Canvas canvas, Size size) {
    if (provider.targetDrones.isEmpty) {
      hitIndex.resetIfChanged(Object.hash(provider.renderRevision, size));
      return;
    }

    final now = DateTime.now();
    final progress = provider.transitionProgress(now);
    final shouldCluster = camera.zoom < 6 || _visibleCountExceeds(size, 2000);
    final clusters = shouldCluster ? <int, _Cluster>{} : null;
    final cameraSignature = Object.hash(
      provider.renderRevision,
      camera.zoom,
      camera.visibleBounds.toString(),
      size.width,
      size.height,
    );
    final rebuildHitIndex = hitIndex.resetIfChanged(cameraSignature);
    DronePositionUpdate? selected;
    Offset? selectedOffset;

    for (final target in provider.targetDrones) {
      if (!provider.matchesFilter(target)) continue;
      final previous = provider.previousById[target.droneId];
      final latitude =
          previous == null
              ? target.position.latitude
              : previous.position.latitude +
                  (target.position.latitude - previous.position.latitude) *
                      progress;
      final longitude =
          previous == null
              ? target.position.longitude
              : previous.position.longitude +
                  (target.position.longitude - previous.position.longitude) *
                      progress;
      final coordinate = LatLng(latitude, longitude);
      if (!camera.visibleBounds.contains(coordinate)) continue;
      final offset = camera.getOffsetFromOrigin(coordinate);
      if (offset.dx < -24 ||
          offset.dy < -24 ||
          offset.dx > size.width + 24 ||
          offset.dy > size.height + 24) {
        continue;
      }

      if (target.droneId == provider.selectedDroneId) {
        selected = target;
        selectedOffset = offset;
      }
      if (shouldCluster) {
        if (target.droneId != provider.selectedDroneId) {
          final cellX = (offset.dx / 32).floor();
          final cellY = (offset.dy / 32).floor();
          final key = Object.hash(cellX, cellY);
          clusters!
              .putIfAbsent(key, () => _Cluster(offset))
              .add(offset, target.type);
        }
        continue;
      }
      _drawDrone(
        canvas,
        target,
        previous,
        offset,
        progress,
        selected: target.droneId == provider.selectedDroneId,
      );
      if (rebuildHitIndex) {
        final targetOffset = camera.getOffsetFromOrigin(
          LatLng(target.position.latitude, target.position.longitude),
        );
        hitIndex.add(target.droneId, targetOffset);
      }
    }

    if (clusters != null) {
      for (final cluster in clusters.values) {
        cluster.paint(canvas);
      }
    }
    if (selected != null && selectedOffset != null && shouldCluster) {
      final previous = provider.previousById[selected.droneId];
      _drawDrone(
        canvas,
        selected,
        previous,
        selectedOffset,
        progress,
        selected: true,
      );
      if (rebuildHitIndex) {
        hitIndex.add(selected.droneId, selectedOffset);
      }
    }
  }

  bool _visibleCountExceeds(Size size, int limit) {
    var visible = 0;
    for (final target in provider.targetDrones) {
      if (!provider.matchesFilter(target)) continue;
      final coordinate = LatLng(
        target.position.latitude,
        target.position.longitude,
      );
      if (!camera.visibleBounds.contains(coordinate)) continue;
      final offset = camera.getOffsetFromOrigin(coordinate);
      if (offset.dx < -24 ||
          offset.dy < -24 ||
          offset.dx > size.width + 24 ||
          offset.dy > size.height + 24) {
        continue;
      }
      visible++;
      if (visible > limit) return true;
    }
    return false;
  }

  void _drawDrone(
    Canvas canvas,
    DronePositionUpdate target,
    DronePositionUpdate? previous,
    Offset offset,
    double progress, {
    required bool selected,
  }) {
    final color = TacticalTheme.getDroneColor(target.type);
    final heading =
        provider.interpolatedHeading(previous, target, progress) *
        math.pi /
        180;
    final radius = selected ? 11.0 : 7.0;
    if (selected) {
      canvas.drawCircle(
        offset,
        16,
        Paint()..color = color.withValues(alpha: 0.22),
      );
      canvas.drawCircle(
        offset,
        12,
        Paint()
          ..color = Colors.white
          ..style = PaintingStyle.stroke
          ..strokeWidth = 2,
      );
    }
    final path =
        ui.Path()
          ..moveTo(0, -radius)
          ..lineTo(radius * 0.72, radius)
          ..lineTo(0, radius * 0.55)
          ..lineTo(-radius * 0.72, radius)
          ..close();
    canvas.save();
    canvas.translate(offset.dx, offset.dy);
    canvas.rotate(heading);
    canvas.drawPath(path, Paint()..color = color);
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _DronePainter oldDelegate) =>
      oldDelegate.camera != camera || oldDelegate.provider != provider;
}

class _Cluster {
  _Cluster(this._sum);

  Offset _sum;
  int count = 0;
  int enemy = 0;
  int ally = 0;

  void add(Offset offset, DroneType type) {
    if (count > 0) _sum += offset;
    count++;
    if (type == DroneType.enemy) enemy++;
    if (type == DroneType.ally) ally++;
  }

  void paint(Canvas canvas) {
    final center = _sum / count.toDouble();
    final color =
        enemy > ally
            ? TacticalTheme.colorEnemy
            : ally > enemy
            ? TacticalTheme.colorAlly
            : TacticalTheme.colorUndefined;
    final radius = (5 + math.log(count + 1) * 2.2).clamp(6.0, 16.0);
    canvas.drawCircle(
      center,
      radius,
      Paint()..color = color.withValues(alpha: 0.72),
    );
    if (count >= 10) {
      final text = TextPainter(
        text: TextSpan(
          text:
              count > 999 ? '${(count / 1000).toStringAsFixed(1)}k' : '$count',
          style: const TextStyle(
            color: Colors.white,
            fontSize: 9,
            fontWeight: FontWeight.bold,
          ),
        ),
        textDirection: TextDirection.ltr,
      )..layout();
      text.paint(canvas, center - Offset(text.width / 2, text.height / 2));
    }
  }
}
