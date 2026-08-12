import 'package:flutter/material.dart';

import '../models/drone.dart';

class TacticalTheme {
  static const bgLight = Color(0xFFF8FAFC);
  static const bgPanel = Color(0xFFFFFFFF);
  static const bgCard = Color(0xFFF1F5F9);
  static const borderCard = Color(0xFFE2E8F0);
  static const colorEnemy = Color(0xFFDC2626);
  static const colorAlly = Color(0xFF16A34A);
  static const colorUndefined = Color(0xFFD97706);
  static const colorCyan = Color(0xFF0284C7);
  static const textMain = Color(0xFF0F172A);
  static const textMuted = Color(0xFF64748B);

  static ThemeData get lightTheme => ThemeData(
    useMaterial3: true,
    brightness: Brightness.light,
    scaffoldBackgroundColor: bgLight,
    colorScheme: const ColorScheme.light(
      primary: colorCyan,
      surface: bgPanel,
      error: colorEnemy,
    ),
    textTheme: const TextTheme(
      bodyMedium: TextStyle(color: textMain),
      bodySmall: TextStyle(color: textMuted),
    ),
  );

  static Color getDroneColor(DroneType type) => switch (type) {
    DroneType.enemy => colorEnemy,
    DroneType.ally => colorAlly,
    _ => colorUndefined,
  };
}
