import 'nymvpn_profile.dart';

class NymvpnAccountAccess {
  final int userId;
  final String token;

  const NymvpnAccountAccess(this.userId, this.token);

  static NymvpnAccountAccess? fromSubscription(String value) {
    if (!isNymvpnSubscription(value)) return null;
    final parts = Uri.parse(value).pathSegments;
    final id = int.tryParse(parts[1]);
    if (id == null ||
        id <= 0 ||
        !RegExp(r'^[a-f0-9]{32}$').hasMatch(parts[2])) {
      return null;
    }
    return NymvpnAccountAccess(id, parts[2]);
  }

  Uri get endpoint =>
      Uri.https('sub.pixel-node.online', '/app/account/$userId');

  Uri telegramLink({required bool game}) => Uri.https('t.me', '/nym_vpnbot', {
    'start': 'app_${game ? 'game' : 'profile'}_$userId',
  });
}

class NymvpnAccount {
  final int userId;
  final String name;
  final String username;
  final int level;
  final int xp;
  final int xpLeft;
  final int coins;
  final int levelXp;
  final int? levelSize;
  final bool subscriptionActive;

  const NymvpnAccount({
    required this.userId,
    required this.name,
    required this.username,
    required this.level,
    required this.xp,
    required this.xpLeft,
    required this.coins,
    required this.levelXp,
    required this.levelSize,
    required this.subscriptionActive,
  });

  factory NymvpnAccount.fromJson(
    Map<String, dynamic> data,
    int expectedUserId,
  ) {
    if (data['version'] != 1 || data['tg_id'] != expectedUserId) {
      throw const FormatException('Account mismatch');
    }
    int count(String key) {
      final value = data[key];
      if (value is! int || value < 0) {
        throw const FormatException('Invalid account data');
      }
      return value;
    }

    final name = data['name'];
    final username = data['username'];
    final active = data['subscription_active'];
    if (name is! String || username is! String || active is! bool) {
      throw const FormatException('Invalid account data');
    }
    final level = count('level');
    final size = data['level_size'] == null ? null : count('level_size');
    if (level == 0 || size == 0) {
      throw const FormatException('Invalid account data');
    }
    return NymvpnAccount(
      userId: expectedUserId,
      name: name,
      username: username,
      level: level,
      xp: count('xp'),
      xpLeft: count('xp_left'),
      coins: count('coins'),
      levelXp: count('level_xp'),
      levelSize: size,
      subscriptionActive: active,
    );
  }

  double get progress =>
      levelSize == null ? 1 : (levelXp / levelSize!).clamp(0, 1);
}
