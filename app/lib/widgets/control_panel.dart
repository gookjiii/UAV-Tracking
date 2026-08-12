import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../models/drone.dart';
import '../providers/drone_provider.dart';
import '../providers/settings_provider.dart';
import '../theme/tactical_theme.dart';

class TacticalControlPanel extends StatefulWidget {
  const TacticalControlPanel({
    this.scrollController,
    this.mobile = false,
    super.key,
  });

  final ScrollController? scrollController;
  final bool mobile;

  @override
  State<TacticalControlPanel> createState() => _TacticalControlPanelState();
}

class _TacticalControlPanelState extends State<TacticalControlPanel> {
  final TextEditingController _searchController = TextEditingController();

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      elevation: widget.mobile ? 12 : 6,
      color: TacticalTheme.bgPanel.withValues(alpha: 0.97),
      borderRadius:
          widget.mobile
              ? const BorderRadius.vertical(top: Radius.circular(18))
              : null,
      clipBehavior: Clip.antiAlias,
      child: Column(
        children: [
          if (widget.mobile)
            Container(
              margin: const EdgeInsets.only(top: 8),
              width: 42,
              height: 4,
              decoration: BoxDecoration(
                color: TacticalTheme.borderCard,
                borderRadius: BorderRadius.circular(4),
              ),
            ),
          _Header(onSettings: () => _showSettings(context)),
          Expanded(
            child: ListView(
              controller: widget.scrollController,
              padding: const EdgeInsets.all(14),
              children: [
                const _ErrorBanner(),
                const _MetricsCard(),
                const SizedBox(height: 12),
                TextField(
                  controller: _searchController,
                  onChanged: context.read<DroneProvider>().setSearchQuery,
                  decoration: const InputDecoration(
                    hintText: 'Search UAV ID…',
                    prefixIcon: Icon(Icons.search_rounded, size: 19),
                    filled: true,
                    fillColor: TacticalTheme.bgCard,
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                ),
                const SizedBox(height: 10),
                const _FilterRow(),
                const SizedBox(height: 12),
                const _SimulationCard(),
                const SizedBox(height: 12),
                const _TargetInspector(),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _showSettings(BuildContext context) async {
    final settings = context.read<SettingsProvider>();
    final drones = context.read<DroneProvider>();
    final controller = TextEditingController(text: settings.apiBaseUrl);
    String? validationError;
    await showDialog<void>(
      context: context,
      builder:
          (dialogContext) => StatefulBuilder(
            builder:
                (context, setDialogState) => AlertDialog(
                  title: const Text('Backend connection'),
                  content: SizedBox(
                    width: 440,
                    child: TextField(
                      controller: controller,
                      autofocus: true,
                      decoration: InputDecoration(
                        labelText: 'Server URL',
                        hintText: 'http://192.168.1.10:8080',
                        errorText: validationError,
                      ),
                    ),
                  ),
                  actions: [
                    TextButton(
                      onPressed: () async {
                        await settings.resetApiBaseUrl();
                        drones.configureBaseUrl(settings.apiBaseUrl);
                        if (dialogContext.mounted) Navigator.pop(dialogContext);
                      },
                      child: const Text('Reset'),
                    ),
                    FilledButton(
                      onPressed: () async {
                        try {
                          await settings.saveApiBaseUrl(controller.text);
                          drones.configureBaseUrl(settings.apiBaseUrl);
                          if (dialogContext.mounted) {
                            Navigator.pop(dialogContext);
                          }
                        } on FormatException catch (error) {
                          setDialogState(() => validationError = error.message);
                        }
                      },
                      child: const Text('Connect'),
                    ),
                  ],
                ),
          ),
    );
    controller.dispose();
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.onSettings});

  final VoidCallback onSettings;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 10, 8, 10),
      decoration: const BoxDecoration(
        color: TacticalTheme.bgCard,
        border: Border(bottom: BorderSide(color: TacticalTheme.borderCard)),
      ),
      child: Row(
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'UAV TRACKING LAB',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 1.1,
                  ),
                ),
                Text(
                  'Research telemetry dashboard',
                  style: TextStyle(
                    fontSize: 10,
                    color: TacticalTheme.textMuted,
                  ),
                ),
              ],
            ),
          ),
          IconButton(
            onPressed: onSettings,
            tooltip: 'Backend settings',
            icon: const Icon(Icons.settings_ethernet_rounded),
          ),
        ],
      ),
    );
  }
}

class _MetricsCard extends StatelessWidget {
  const _MetricsCard();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, _MetricsView>(
      selector:
          (_, provider) => _MetricsView(
            provider.connectionState,
            provider.activeDroneCount,
            provider.filteredCount,
            provider.positionsPerSecond,
          ),
      builder:
          (_, metrics, _) => _Card(
            child: Column(
              children: [
                Row(
                  children: [
                    _ConnectionDot(state: metrics.connection),
                    const SizedBox(width: 7),
                    Text(
                      metrics.connection.name.toUpperCase(),
                      style: const TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 10),
                Row(
                  children: [
                    Expanded(
                      child: _Metric(
                        label: 'ACTIVE',
                        value: '${metrics.active}',
                      ),
                    ),
                    Expanded(
                      child: _Metric(
                        label: 'VISIBLE',
                        value: '${metrics.filtered}',
                      ),
                    ),
                    Expanded(
                      child: _Metric(
                        label: 'POSITIONS/S',
                        value: _compact(metrics.positionsPerSecond),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
    );
  }

  static String _compact(int value) =>
      value >= 1000 ? '${(value / 1000).toStringAsFixed(1)}k' : '$value';
}

class _ConnectionDot extends StatelessWidget {
  const _ConnectionDot({required this.state});

  final TelemetryConnectionState state;

  @override
  Widget build(BuildContext context) {
    final color = switch (state) {
      TelemetryConnectionState.connected => TacticalTheme.colorAlly,
      TelemetryConnectionState.connecting => TacticalTheme.colorCyan,
      TelemetryConnectionState.stale => TacticalTheme.colorUndefined,
      _ => TacticalTheme.colorEnemy,
    };
    return Container(
      width: 9,
      height: 9,
      decoration: BoxDecoration(color: color, shape: BoxShape.circle),
    );
  }
}

class _Metric extends StatelessWidget {
  const _Metric({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        label,
        style: const TextStyle(fontSize: 8, color: TacticalTheme.textMuted),
      ),
      Text(
        value,
        style: const TextStyle(
          fontSize: 17,
          fontWeight: FontWeight.bold,
          color: TacticalTheme.colorCyan,
        ),
      ),
    ],
  );
}

class _FilterRow extends StatelessWidget {
  const _FilterRow();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, DroneType>(
      selector: (_, provider) => provider.activeFilter,
      builder:
          (_, selected, _) => Wrap(
            spacing: 6,
            runSpacing: 6,
            children: DroneType.values
                .map((type) {
                  final label =
                      type == DroneType.all ? 'ALL' : type.name.toUpperCase();
                  return ChoiceChip(
                    label: Text(label, style: const TextStyle(fontSize: 10)),
                    selected: selected == type,
                    onSelected:
                        (_) => context.read<DroneProvider>().setFilter(type),
                  );
                })
                .toList(growable: false),
          ),
    );
  }
}

class _SimulationCard extends StatelessWidget {
  const _SimulationCard();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, _SimulationView>(
      selector:
          (_, provider) => _SimulationView(
            provider.targetDroneCount,
            provider.updateIntervalMs,
            provider.isSimulationActive,
            provider.isApplyingConfig,
          ),
      builder: (_, view, _) {
        final provider = context.read<DroneProvider>();
        return _Card(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Expanded(
                    child: Text(
                      'SIMULATION',
                      style: TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  ),
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    onPressed:
                        view.busy ? null : provider.toggleSimulationState,
                    icon: Icon(
                      view.active ? Icons.pause_circle : Icons.play_circle,
                      color:
                          view.active
                              ? TacticalTheme.colorAlly
                              : TacticalTheme.colorEnemy,
                    ),
                  ),
                ],
              ),
              _SliderLabel(label: 'DRONES', value: '${view.count}'),
              Slider(
                value: view.count.toDouble(),
                min: 10,
                max: 10000,
                divisions: 999,
                onChanged:
                    view.busy
                        ? null
                        : (value) => provider.setDraftDroneCount(value.round()),
                onChangeEnd:
                    view.busy ? null : (_) => provider.applySimulationConfig(),
              ),
              Wrap(
                spacing: 5,
                children: [10, 100, 1000, 5000, 10000]
                    .map(
                      (count) => ActionChip(
                        label: Text(
                          count >= 1000 ? '${count ~/ 1000}k' : '$count',
                        ),
                        onPressed:
                            view.busy
                                ? null
                                : () {
                                  provider.setDraftDroneCount(count);
                                  provider.applySimulationConfig();
                                },
                      ),
                    )
                    .toList(growable: false),
              ),
              const SizedBox(height: 8),
              _SliderLabel(
                label: 'UPDATE INTERVAL',
                value: '${view.interval} ms',
              ),
              Slider(
                value: view.interval.toDouble(),
                min: 100,
                max: 2000,
                divisions: 38,
                onChanged:
                    view.busy
                        ? null
                        : (value) => provider.setDraftInterval(value.round()),
                onChangeEnd:
                    view.busy ? null : (_) => provider.applySimulationConfig(),
              ),
              if (view.busy) const LinearProgressIndicator(minHeight: 2),
            ],
          ),
        );
      },
    );
  }
}

class _SliderLabel extends StatelessWidget {
  const _SliderLabel({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => Row(
    mainAxisAlignment: MainAxisAlignment.spaceBetween,
    children: [
      Text(
        label,
        style: const TextStyle(fontSize: 9, color: TacticalTheme.textMuted),
      ),
      Text(
        value,
        style: const TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.bold,
          color: TacticalTheme.colorCyan,
        ),
      ),
    ],
  );
}

class _TargetInspector extends StatelessWidget {
  const _TargetInspector();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, _TargetView>(
      selector:
          (_, provider) =>
              _TargetView(provider.selectedDrone, provider.isLoadingHistory),
      builder: (_, view, _) {
        final drone = view.drone;
        if (drone == null) {
          return const _Card(
            child: Text(
              'Select an individual UAV after zooming in.',
              style: TextStyle(fontSize: 11, color: TacticalTheme.textMuted),
            ),
          );
        }
        final provider = context.read<DroneProvider>();
        final color = TacticalTheme.getDroneColor(drone.type);
        return _Card(
          borderColor: color,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      drone.droneId,
                      style: TextStyle(
                        fontWeight: FontWeight.bold,
                        color: color,
                      ),
                    ),
                  ),
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    onPressed: provider.clearSelection,
                    icon: const Icon(Icons.close, size: 17),
                  ),
                ],
              ),
              Wrap(
                runSpacing: 8,
                children: [
                  _Info(label: 'TYPE', value: drone.typeName),
                  _Info(label: 'ORBIT', value: drone.orbitName),
                  _Info(
                    label: 'POSITION',
                    value:
                        '${drone.position.latitude.toStringAsFixed(4)}, ${drone.position.longitude.toStringAsFixed(4)}',
                  ),
                  _Info(
                    label: 'ALT / SPEED',
                    value:
                        '${drone.position.altitude.toStringAsFixed(0)} m / ${drone.speedKmh.toStringAsFixed(0)} km/h',
                  ),
                ],
              ),
              const SizedBox(height: 10),
              SizedBox(
                width: double.infinity,
                child: OutlinedButton.icon(
                  onPressed:
                      view.loading ? null : provider.fetchHistoryForSelected,
                  icon:
                      view.loading
                          ? const SizedBox.square(
                            dimension: 14,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                          : const Icon(Icons.route_rounded, size: 16),
                  label: const Text('LOAD 7-DAY TRAJECTORY'),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _Info extends StatelessWidget {
  const _Info({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) => SizedBox(
    width: 150,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(fontSize: 8, color: TacticalTheme.textMuted),
        ),
        Text(
          value,
          style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w600),
        ),
      ],
    ),
  );
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner();

  @override
  Widget build(BuildContext context) {
    return Selector<DroneProvider, String?>(
      selector: (_, provider) => provider.lastError,
      builder: (_, error, _) {
        if (error == null) return const SizedBox.shrink();
        return Container(
          margin: const EdgeInsets.only(bottom: 10),
          padding: const EdgeInsets.all(9),
          decoration: BoxDecoration(
            color: TacticalTheme.colorEnemy.withValues(alpha: 0.08),
            border: Border.all(color: TacticalTheme.colorEnemy),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            children: [
              const Icon(
                Icons.error_outline,
                color: TacticalTheme.colorEnemy,
                size: 17,
              ),
              const SizedBox(width: 7),
              Expanded(
                child: Text(error, style: const TextStyle(fontSize: 10)),
              ),
              IconButton(
                visualDensity: VisualDensity.compact,
                onPressed: context.read<DroneProvider>().clearError,
                icon: const Icon(Icons.close, size: 15),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _Card extends StatelessWidget {
  const _Card({required this.child, this.borderColor});

  final Widget child;
  final Color? borderColor;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(12),
    decoration: BoxDecoration(
      color: TacticalTheme.bgCard,
      borderRadius: BorderRadius.circular(8),
      border: Border.all(color: borderColor ?? TacticalTheme.borderCard),
    ),
    child: child,
  );
}

class _MetricsView {
  const _MetricsView(
    this.connection,
    this.active,
    this.filtered,
    this.positionsPerSecond,
  );
  final TelemetryConnectionState connection;
  final int active;
  final int filtered;
  final int positionsPerSecond;

  @override
  bool operator ==(Object other) =>
      other is _MetricsView &&
      connection == other.connection &&
      active == other.active &&
      filtered == other.filtered &&
      positionsPerSecond == other.positionsPerSecond;

  @override
  int get hashCode =>
      Object.hash(connection, active, filtered, positionsPerSecond);
}

class _SimulationView {
  const _SimulationView(this.count, this.interval, this.active, this.busy);
  final int count;
  final int interval;
  final bool active;
  final bool busy;

  @override
  bool operator ==(Object other) =>
      other is _SimulationView &&
      count == other.count &&
      interval == other.interval &&
      active == other.active &&
      busy == other.busy;

  @override
  int get hashCode => Object.hash(count, interval, active, busy);
}

class _TargetView {
  const _TargetView(this.drone, this.loading);
  final DronePositionUpdate? drone;
  final bool loading;

  @override
  bool operator ==(Object other) =>
      other is _TargetView && drone == other.drone && loading == other.loading;

  @override
  int get hashCode => Object.hash(drone, loading);
}
