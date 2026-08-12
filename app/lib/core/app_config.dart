import 'package:flutter/foundation.dart';

class AppConfig {
  static const compileTimeApiBaseUrl = String.fromEnvironment('API_BASE_URL');

  static String get platformDefaultApiBaseUrl {
    if (compileTimeApiBaseUrl.trim().isNotEmpty) {
      return normalizeApiBaseUrl(compileTimeApiBaseUrl);
    }
    if (kIsWeb) {
      return Uri.base.origin;
    }
    return 'http://127.0.0.1:8080';
  }

  static String normalizeApiBaseUrl(String input) {
    final trimmed = input.trim();
    final uri = Uri.tryParse(trimmed);
    if (uri == null ||
        !uri.hasScheme ||
        (uri.scheme != 'http' && uri.scheme != 'https') ||
        uri.host.isEmpty) {
      throw const FormatException(
        'Enter a valid http:// or https:// server URL',
      );
    }
    final normalized = uri.replace(query: null, fragment: null).toString();
    return normalized.endsWith('/')
        ? normalized.substring(0, normalized.length - 1)
        : normalized;
  }
}
