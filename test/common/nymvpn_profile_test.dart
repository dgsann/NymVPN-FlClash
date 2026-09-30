import 'dart:async';
import 'package:fl_clash/common/nymvpn_profile.dart';
import 'package:fl_clash/common/profile_refresh.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('automatic setup only accepts the exact HTTPS subscription origin', () {
    expect(isNymvpnSubscription('https://sub.pixel-node.online/sub/123/test-token'), isTrue);
    expect(isNymvpnSubscription('https://sub.pixel-node.online/sub/123/token/p/game/key'), isTrue);
    for (final url in [
      'http://sub.pixel-node.online/sub/123/token',
      'https://sub.pixel-node.online.evil.test/sub/123/token',
      'https://sub.pixel-node.online@evil.test/sub/123/token',
      'https://user@sub.pixel-node.online/sub/123/token',
      'https://sub.pixel-node.online:8443/sub/123/token',
      'https://sub.pixel-node.online/health',
      'not a url',
    ]) {
      expect(isNymvpnSubscription(url), isFalse, reason: url);
    }
  });

  test('selects automatic VPN and game rules without selecting DIRECT for VPN', () {
    const yaml = """
proxy-groups:
  - name: 🧠 NymVPN Adaptive
    type: select
    proxies: [DIRECT, RU-AMS, NymVPN-AutoTCP]
  - name: 🎮 Игры
    type: select
    proxies: [RU-AMS, DIRECT]
""";
    expect(nymvpnInitialSelections(yaml), {
      '🧠 NymVPN Adaptive': 'NymVPN-AutoTCP',
      '🎮 Игры': 'DIRECT',
    });
    expect(nymvpnInitialSelections(yaml.replaceAll('NymVPN-AutoTCP', 'NymVPN'))['🧠 NymVPN Adaptive'], 'NymVPN');
    expect(nymvpnInitialSelections('proxy-groups: []'), isEmpty);
  });

  test('server interval is bounded and malformed headers preserve settings', () {
    const fallback = Duration(hours: 3);
    expect(subscriptionUpdateInterval('1', fallback), const Duration(hours: 1));
    expect(subscriptionUpdateInterval('9999', fallback), const Duration(hours: 24));
    for (final header in [null, 'bad', '0', '-1']) {
      expect(subscriptionUpdateInterval(header, fallback), fallback);
    }
  });

  test('overlapping refreshes download once and retain failure backoff', () async {
    var now = DateTime(2026);
    final queue = ProfileRefreshQueue(now: () => now);
    final download = Completer<void>();
    var calls = 0;
    final first = queue.run(1, () { calls++; return download.future; });
    final second = queue.run(1, () async { calls++; });
    expect(identical(first, second), isTrue);
    expect(calls, 1);
    final failure = expectLater(first, throwsStateError);
    download.completeError(StateError('offline'));
    await failure;
    expect(queue.canRetry(1), isFalse);
    expect(queue.canRetry(2), isTrue);
    now = now.add(const Duration(minutes: 2));
    expect(queue.canRetry(1), isTrue);
    await queue.run(1, () async { calls++; });
    expect(calls, 2);
  });
}
