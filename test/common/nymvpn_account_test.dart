import 'package:fl_clash/common/nymvpn_account.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  const token = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
  test(
    'account credentials stay on the pinned API, never in Telegram links',
    () {
      final access = NymvpnAccountAccess.fromSubscription(
        'https://sub.pixel-node.online/sub/7/$token/p/android/signed',
      )!;
      expect(
        access.endpoint.toString(),
        'https://sub.pixel-node.online/app/account/7',
      );
      expect(
        access.telegramLink(game: true).toString(),
        'https://t.me/nym_vpnbot?start=app_game_7',
      );
      expect(
        access.telegramLink(game: false).toString(),
        isNot(contains(token)),
      );
      for (final url in [
        'http://sub.pixel-node.online/sub/7/$token',
        'https://sub.pixel-node.online.evil.test/sub/7/$token',
        'https://user@sub.pixel-node.online/sub/7/$token',
        'https://sub.pixel-node.online:444/sub/7/$token',
        'https://sub.pixel-node.online/sub/0/$token',
        'https://sub.pixel-node.online/sub/7/bad',
      ]) {
        expect(NymvpnAccountAccess.fromSubscription(url), isNull);
      }
    },
  );
  test('server data must belong to the selected subscription', () {
    final data = <String, dynamic>{
      'version': 1,
      'tg_id': 7,
      'name': 'Player',
      'username': 'player',
      'level': 3,
      'xp': 310,
      'xp_left': 390,
      'coins': 22,
      'level_xp': 10,
      'level_size': 400,
      'subscription_active': true,
    };
    expect(NymvpnAccount.fromJson(data, 7).progress, 0.025);
    expect(() => NymvpnAccount.fromJson(data, 8), throwsFormatException);
    expect(
      () => NymvpnAccount.fromJson({...data, 'coins': -1}, 7),
      throwsFormatException,
    );
    expect(
      () => NymvpnAccount.fromJson({...data, 'level_size': 0}, 7),
      throwsFormatException,
    );
    expect(
      NymvpnAccount.fromJson({...data, 'level_size': null}, 7).progress,
      1,
    );
  });
}
