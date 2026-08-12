import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'providers/drone_provider.dart';
import 'providers/settings_provider.dart';
import 'theme/tactical_theme.dart';
import 'widgets/control_panel.dart';
import 'widgets/tactical_map.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final settings = await SettingsProvider.create();
  final drones = DroneProvider()..configureBaseUrl(settings.apiBaseUrl);
  runApp(UAVTrackingApp(settings: settings, drones: drones));
}

class UAVTrackingApp extends StatelessWidget {
  const UAVTrackingApp({
    required this.settings,
    required this.drones,
    super.key,
  });

  final SettingsProvider settings;
  final DroneProvider drones;

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        ChangeNotifierProvider.value(value: settings),
        ChangeNotifierProvider.value(value: drones),
      ],
      child: MaterialApp(
        title: 'UAV Tracking Lab',
        debugShowCheckedModeBanner: false,
        theme: TacticalTheme.lightTheme,
        home: const TacticalDashboardScreen(),
      ),
    );
  }
}

class TacticalDashboardScreen extends StatelessWidget {
  const TacticalDashboardScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: LayoutBuilder(
        builder: (context, constraints) {
          final desktop = constraints.maxWidth >= 840;
          if (desktop) {
            return const Stack(
              children: [
                Positioned.fill(child: TacticalMapWidget()),
                Positioned(
                  left: 0,
                  top: 0,
                  bottom: 0,
                  width: 360,
                  child: TacticalControlPanel(),
                ),
              ],
            );
          }
          return Stack(
            children: [
              const Positioned.fill(child: TacticalMapWidget()),
              DraggableScrollableSheet(
                initialChildSize: 0.32,
                minChildSize: 0.13,
                maxChildSize: 0.88,
                snap: true,
                builder:
                    (_, controller) => TacticalControlPanel(
                      scrollController: controller,
                      mobile: true,
                    ),
              ),
            ],
          );
        },
      ),
    );
  }
}
