import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../core/app_config.dart';

class SettingsProvider extends ChangeNotifier {
  SettingsProvider._(this._preferences, this._savedApiBaseUrl);

  static const _apiBaseUrlKey = 'api_base_url';

  final SharedPreferencesAsync _preferences;
  String? _savedApiBaseUrl;

  static Future<SettingsProvider> create() async {
    final preferences = SharedPreferencesAsync();
    final saved = await preferences.getString(_apiBaseUrlKey);
    return SettingsProvider._(preferences, saved);
  }

  String get apiBaseUrl =>
      _savedApiBaseUrl ?? AppConfig.platformDefaultApiBaseUrl;

  bool get hasOverride => _savedApiBaseUrl != null;

  Future<void> saveApiBaseUrl(String value) async {
    final normalized = AppConfig.normalizeApiBaseUrl(value);
    await _preferences.setString(_apiBaseUrlKey, normalized);
    _savedApiBaseUrl = normalized;
    notifyListeners();
  }

  Future<void> resetApiBaseUrl() async {
    await _preferences.remove(_apiBaseUrlKey);
    _savedApiBaseUrl = null;
    notifyListeners();
  }
}
