import 'package:yaml/yaml.dart';

bool isNymvpnSubscription(String value) {
  final uri = Uri.tryParse(value);
  if (uri == null ||
      uri.scheme != 'https' ||
      uri.host != 'sub.pixel-node.online' ||
      uri.port != 443 ||
      uri.userInfo.isNotEmpty) {
    return false;
  }
  return RegExp(
    r'^/sub/[0-9]+/[a-zA-Z0-9_-]+(?:/p/[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)?$',
  ).hasMatch(uri.path);
}

Duration subscriptionUpdateInterval(String? header, Duration fallback) {
  final hours = int.tryParse(header ?? '');
  if (hours == null || hours <= 0) return fallback;
  return Duration(hours: hours.clamp(1, 24));
}

Map<String, String> nymvpnInitialSelections(String yaml) {
  final config = loadYaml(yaml);
  if (config is! Map) return {};
  final groups = config['proxy-groups'];
  if (groups is! List) return {};
  final result = <String, String>{};
  for (final group in groups) {
    if (group is! Map || group['type'] != 'select') continue;
    final name = group['name'];
    final proxies = group['proxies'];
    if (name is! String || proxies is! List || proxies.isEmpty) continue;
    if (name == '🧠 NymVPN Adaptive') {
      for (final candidate in [
        'NymVPN',
        'NymVPN-AutoTCP',
        'RU-AMS',
        'RU-Finland',
      ]) {
        if (proxies.contains(candidate)) {
          result[name] = candidate;
          break;
        }
      }
    } else if (name == '🎮 Игры' && proxies.contains('DIRECT')) {
      result[name] = 'DIRECT';
    }
  }
  return result;
}
