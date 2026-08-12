import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:uav_tracking_app/providers/drone_provider.dart';
import 'package:uav_tracking_app/widgets/control_panel.dart';

void main() {
  testWidgets('control panel renders research dashboard controls', (
    tester,
  ) async {
    final provider = DroneProvider();
    await tester.pumpWidget(
      ChangeNotifierProvider.value(
        value: provider,
        child: const MaterialApp(home: Scaffold(body: TacticalControlPanel())),
      ),
    );

    expect(find.text('UAV TRACKING LAB'), findsOneWidget);
    expect(find.text('SIMULATION'), findsOneWidget);
    expect(find.text('ALL'), findsOneWidget);

    await tester.pumpWidget(const SizedBox.shrink());
    provider.dispose();
  });
}
