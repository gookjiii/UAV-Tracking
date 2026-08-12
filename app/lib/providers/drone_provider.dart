import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../models/drone.dart';
import '../services/drone_api_service.dart';
import '../services/sse_client.dart';

enum TelemetryConnectionState { disconnected, connecting, connected, stale }

class DroneProvider extends ChangeNotifier {
  DroneProvider({http.Client? client}) : _client = client ?? http.Client() {
    _api = DroneApiService(_client, 'http://127.0.0.1:8080');
    _sse = SseClient(_client);
    _metricsTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      _positionsPerSecond = _positionsSinceLastMetric;
      _positionsSinceLastMetric = 0;
      if (_connectionState == TelemetryConnectionState.connected &&
          dataAge > const Duration(seconds: 2)) {
        _connectionState = TelemetryConnectionState.stale;
      }
      notifyListeners();
    });
  }

  final http.Client _client;
  late final DroneApiService _api;
  late final SseClient _sse;

  String _baseUrl = '';
  List<DronePositionUpdate> _targetDrones = const [];
  Map<String, DronePositionUpdate> _targetById = const {};
  Map<String, DronePositionUpdate> _previousById = const {};
  DateTime _transitionStartedAt = DateTime.now();
  Duration _transitionDuration = const Duration(milliseconds: 300);
  DateTime? _lastSnapshotAt;

  DroneType _activeFilter = DroneType.all;
  String _searchQuery = '';
  String? _selectedDroneId;
  List<Position> _historyTrajectory = const [];
  bool _isLoadingHistory = false;

  int _targetDroneCount = 10;
  int _confirmedDroneCount = 10;
  int _updateIntervalMs = 300;
  int _confirmedIntervalMs = 300;
  bool _isSimulationActive = true;
  bool _confirmedSimulationActive = true;
  bool _isApplyingConfig = false;

  TelemetryConnectionState _connectionState =
      TelemetryConnectionState.disconnected;
  String? _lastError;
  int _filteredCount = 0;
  int _positionsSinceLastMetric = 0;
  int _positionsPerSecond = 0;
  int _connectionGeneration = 0;
  int _reconnectAttempt = 0;
  int _renderRevision = 0;
  bool _disposed = false;

  StreamSubscription<List<DronePositionUpdate>>? _streamSubscription;
  Timer? _reconnectTimer;
  late final Timer _metricsTimer;

  List<DronePositionUpdate> get targetDrones => _targetDrones;
  Map<String, DronePositionUpdate> get previousById => _previousById;
  DroneType get activeFilter => _activeFilter;
  String get searchQuery => _searchQuery;
  String? get selectedDroneId => _selectedDroneId;
  List<Position> get historyTrajectory => _historyTrajectory;
  bool get isLoadingHistory => _isLoadingHistory;
  int get targetDroneCount => _targetDroneCount;
  int get updateIntervalMs => _updateIntervalMs;
  bool get isSimulationActive => _isSimulationActive;
  bool get isApplyingConfig => _isApplyingConfig;
  TelemetryConnectionState get connectionState => _connectionState;
  String? get lastError => _lastError;
  int get filteredCount => _filteredCount;
  int get positionsPerSecond => _positionsPerSecond;
  int get activeDroneCount => _targetDrones.length;
  String get baseUrl => _baseUrl;
  int get renderRevision => _renderRevision;

  Duration get dataAge =>
      _lastSnapshotAt == null
          ? const Duration(days: 365)
          : DateTime.now().difference(_lastSnapshotAt!);

  DronePositionUpdate? get selectedDrone =>
      _selectedDroneId == null ? null : _targetById[_selectedDroneId];

  void configureBaseUrl(String value) {
    if (_baseUrl == value) {
      return;
    }
    _baseUrl = value;
    _api.baseUrl = value;
    _connectionGeneration++;
    _reconnectAttempt = 0;
    _lastError = null;
    _clearTelemetry();
    unawaited(_restartStream(_connectionGeneration));
  }

  Future<void> _restartStream(int generation) async {
    _reconnectTimer?.cancel();
    await _streamSubscription?.cancel();
    if (_disposed || generation != _connectionGeneration || _baseUrl.isEmpty) {
      return;
    }
    _connectionState = TelemetryConnectionState.connecting;
    notifyListeners();

    final assembler = _SnapshotAssembler();
    final parsedStream = _sse
        .connect(_api.streamUri(chunked: true))
        .where(
          (event) =>
              event.event == 'snapshot' ||
              event.event.startsWith('snapshot-part-'),
        )
        .asyncMap<List<DronePositionUpdate>?>((event) async {
          final updates = await compute(parseDroneSnapshot, event.data);
          if (event.event == 'snapshot') {
            return updates;
          }
          final match = RegExp(
            r'^snapshot-part-(\d+)-(\d+)-(\d+)$',
          ).firstMatch(event.event);
          if (match == null) return null;
          return assembler.add(
            sequence: int.parse(match.group(1)!),
            part: int.parse(match.group(2)!),
            total: int.parse(match.group(3)!),
            updates: updates,
          );
        })
        .where((updates) => updates != null)
        .cast<List<DronePositionUpdate>>();
    _streamSubscription = parsedStream.listen(
      (updates) {
        if (generation == _connectionGeneration) {
          _applySnapshot(updates);
        }
      },
      onError: (Object error, StackTrace stackTrace) {
        if (generation == _connectionGeneration) {
          _handleStreamFailure(error);
        }
      },
      onDone: () {
        if (generation == _connectionGeneration && !_disposed) {
          _handleStreamFailure(const ApiException('Telemetry stream closed'));
        }
      },
      cancelOnError: true,
    );
  }

  void _applySnapshot(List<DronePositionUpdate> updates) {
    final now = DateTime.now();
    if (_lastSnapshotAt != null) {
      final observed = now.difference(_lastSnapshotAt!);
      _transitionDuration = Duration(
        milliseconds: observed.inMilliseconds.clamp(100, 2000),
      );
    }
    _previousById = _targetById;
    _targetDrones = updates;
    _targetById = {for (final update in updates) update.droneId: update};
    _renderRevision++;
    _transitionStartedAt = now;
    _lastSnapshotAt = now;
    _positionsSinceLastMetric += updates.length;
    _connectionState = TelemetryConnectionState.connected;
    _reconnectAttempt = 0;
    _lastError = null;
    _recalculateFilteredCount();
    notifyListeners();
  }

  void _handleStreamFailure(Object error) {
    _connectionState = TelemetryConnectionState.disconnected;
    _lastError = error.toString();
    notifyListeners();
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    _reconnectTimer?.cancel();
    final seconds = (1 << _reconnectAttempt.clamp(0, 4)).clamp(1, 15);
    _reconnectAttempt++;
    final generation = _connectionGeneration;
    _reconnectTimer = Timer(Duration(seconds: seconds), () {
      unawaited(_restartStream(generation));
    });
  }

  bool matchesFilter(DronePositionUpdate drone) {
    if (_activeFilter != DroneType.all && drone.type != _activeFilter) {
      return false;
    }
    return _searchQuery.isEmpty ||
        drone.droneId.toLowerCase().contains(_searchQuery);
  }

  double transitionProgress(DateTime now) {
    final elapsed = now.difference(_transitionStartedAt).inMicroseconds;
    final total = _transitionDuration.inMicroseconds;
    if (total <= 0) {
      return 1;
    }
    final raw = (elapsed / total).clamp(0.0, 1.0);
    return 1 - (1 - raw) * (1 - raw) * (1 - raw);
  }

  double interpolatedHeading(
    DronePositionUpdate? previous,
    DronePositionUpdate target,
    double t,
  ) {
    if (previous == null) {
      return target.headingDeg;
    }
    final difference =
        (target.headingDeg - previous.headingDeg + 540) % 360 - 180;
    return (previous.headingDeg + difference * t) % 360;
  }

  void setFilter(DroneType filter) {
    if (_activeFilter == filter) return;
    _activeFilter = filter;
    _renderRevision++;
    _recalculateFilteredCount();
    notifyListeners();
  }

  void setSearchQuery(String query) {
    final normalized = query.trim().toLowerCase();
    if (_searchQuery == normalized) return;
    _searchQuery = normalized;
    _renderRevision++;
    _recalculateFilteredCount();
    notifyListeners();
  }

  void selectDrone(String droneId) {
    _selectedDroneId = droneId;
    _renderRevision++;
    _historyTrajectory = const [];
    notifyListeners();
  }

  void clearSelection() {
    _selectedDroneId = null;
    _renderRevision++;
    _historyTrajectory = const [];
    notifyListeners();
  }

  Future<void> fetchHistoryForSelected() async {
    final droneId = _selectedDroneId;
    if (droneId == null || _isLoadingHistory) return;
    _isLoadingHistory = true;
    _lastError = null;
    notifyListeners();
    try {
      final history = await _api.fetchDroneHistory(droneId, maxPoints: 500);
      if (_selectedDroneId == droneId) {
        _historyTrajectory = history;
      }
    } on Object catch (error) {
      _lastError = error.toString();
    } finally {
      _isLoadingHistory = false;
      notifyListeners();
    }
  }

  void setDraftDroneCount(int value) {
    _targetDroneCount = value.clamp(10, 10000);
    notifyListeners();
  }

  void setDraftInterval(int value) {
    _updateIntervalMs = value.clamp(100, 2000);
    notifyListeners();
  }

  Future<void> applySimulationConfig({bool? active}) async {
    if (_isApplyingConfig) return;
    if (active != null) {
      _isSimulationActive = active;
    }
    _isApplyingConfig = true;
    _lastError = null;
    notifyListeners();
    try {
      final result = await _api.controlSimulation(
        targetDroneCount: _targetDroneCount,
        updateIntervalMs: _updateIntervalMs,
        active: _isSimulationActive,
      );
      if (!result.success) {
        throw ApiException(
          result.message.isEmpty
              ? 'Simulation update was rejected'
              : result.message,
        );
      }
      _confirmedDroneCount = _targetDroneCount;
      _confirmedIntervalMs = _updateIntervalMs;
      _confirmedSimulationActive = _isSimulationActive;
    } on Object catch (error) {
      _targetDroneCount = _confirmedDroneCount;
      _updateIntervalMs = _confirmedIntervalMs;
      _isSimulationActive = _confirmedSimulationActive;
      _lastError = error.toString();
    } finally {
      _isApplyingConfig = false;
      notifyListeners();
    }
  }

  Future<void> toggleSimulationState() =>
      applySimulationConfig(active: !_isSimulationActive);

  void clearError() {
    if (_lastError == null) return;
    _lastError = null;
    notifyListeners();
  }

  void _recalculateFilteredCount() {
    _filteredCount = _targetDrones.where(matchesFilter).length;
  }

  void _clearTelemetry() {
    _targetDrones = const [];
    _targetById = const {};
    _previousById = const {};
    _historyTrajectory = const [];
    _lastSnapshotAt = null;
    _connectionState = TelemetryConnectionState.disconnected;
    _filteredCount = 0;
    _renderRevision++;
    notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _connectionGeneration++;
    _reconnectTimer?.cancel();
    unawaited(_streamSubscription?.cancel());
    _metricsTimer.cancel();
    _client.close();
    super.dispose();
  }
}

class _SnapshotAssembler {
  final Map<int, _PendingSnapshot> _pending = {};

  List<DronePositionUpdate>? add({
    required int sequence,
    required int part,
    required int total,
    required List<DronePositionUpdate> updates,
  }) {
    if (total < 1 || part < 0 || part >= total) return null;
    // Keep at most the current and one previous sequence when a connection is
    // slow, preventing a stalled browser from growing this map unboundedly.
    _pending.removeWhere((key, _) => key < sequence - 1);
    final snapshot = _pending.putIfAbsent(
      sequence,
      () => _PendingSnapshot(total),
    );
    if (snapshot.total != total) {
      _pending.remove(sequence);
      return null;
    }
    snapshot.parts[part] = updates;
    if (snapshot.parts.length != total) return null;
    final result = <DronePositionUpdate>[];
    for (var index = 0; index < total; index++) {
      final chunk = snapshot.parts[index];
      if (chunk == null) {
        _pending.remove(sequence);
        return null;
      }
      result.addAll(chunk);
    }
    _pending.remove(sequence);
    return result;
  }
}

class _PendingSnapshot {
  _PendingSnapshot(this.total);

  final int total;
  final Map<int, List<DronePositionUpdate>> parts = {};
}
