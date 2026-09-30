import 'dart:async';

import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/common/nymvpn_account.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:url_launcher/url_launcher.dart';

typedef AccountLoader = Future<NymvpnAccount> Function(String subscription);
typedef AccountLinkOpener = Future<bool> Function(Uri uri);

class NymvpnAccountPanel extends ConsumerWidget {
  const NymvpnAccountPanel({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profile = ref.watch(currentProfileProvider);
    if (profile == null ||
        NymvpnAccountAccess.fromSubscription(profile.url) == null) {
      return const SizedBox.shrink();
    }
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: NymvpnAccountCard(
        subscription: profile.url,
        connected: ref.watch(isStartProvider),
        load: request.getNymvpnAccount,
      ),
    );
  }
}

class NymvpnAccountCard extends StatefulWidget {
  final String subscription;
  final bool connected;
  final AccountLoader load;
  final AccountLinkOpener? openLink;

  const NymvpnAccountCard({
    super.key,
    required this.subscription,
    required this.load,
    this.connected = false,
    this.openLink,
  });

  @override
  State<NymvpnAccountCard> createState() => _NymvpnAccountCardState();
}

class _NymvpnAccountCardState extends State<NymvpnAccountCard>
    with WidgetsBindingObserver {
  NymvpnAccount? _account;
  bool _loading = false;
  bool _failed = false;
  bool _opening = false;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    unawaited(_refresh());
  }

  @override
  void didUpdateWidget(covariant NymvpnAccountCard oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.subscription != widget.subscription) {
      _generation++;
      _account = null;
      _loading = false;
      _failed = false;
      unawaited(_refresh());
    } else if (!oldWidget.connected && widget.connected) {
      unawaited(_refresh());
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) unawaited(_refresh());
  }

  @override
  void dispose() {
    _generation++;
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  Future<void> _refresh() async {
    if (_loading) return;
    final generation = ++_generation;
    setState(() {
      _loading = true;
      _failed = false;
    });
    try {
      final account = await widget.load(widget.subscription);
      if (!mounted || generation != _generation) return;
      setState(() => _account = account);
    } catch (_) {
      if (!mounted || generation != _generation) return;
      setState(() => _failed = true);
    } finally {
      if (mounted && generation == _generation) {
        setState(() => _loading = false);
      }
    }
  }

  Future<void> _open(bool game) async {
    final access = NymvpnAccountAccess.fromSubscription(widget.subscription);
    if (access == null || _opening) return;
    setState(() => _opening = true);
    try {
      final uri = access.telegramLink(game: game);
      final opened =
          await (widget.openLink?.call(uri) ??
              launchUrl(uri, mode: LaunchMode.externalApplication));
      if (!opened) throw StateError('Telegram unavailable');
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.appLocalizations.nymTelegramError)),
        );
      }
    } finally {
      if (mounted) setState(() => _opening = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final strings = context.appLocalizations;
    final account = _account;
    return Card(
      margin: EdgeInsets.zero,
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.account_circle_outlined),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    strings.nymAccountTitle,
                    style: context.textTheme.titleMedium,
                  ),
                ),
                IconButton(
                  tooltip: strings.nymAccountRefresh,
                  onPressed: _loading ? null : _refresh,
                  icon: const Icon(Icons.refresh),
                ),
              ],
            ),
            if (_loading) ...[
              const LinearProgressIndicator(),
              const SizedBox(height: 12),
            ],
            if (account != null) ...[
              Text(account.name, style: context.textTheme.titleLarge),
              Text(
                account.username.isEmpty
                    ? 'Telegram · ${account.userId}'
                    : '@${account.username} · ${account.userId}',
              ),
              const SizedBox(height: 12),
              Wrap(
                spacing: 16,
                runSpacing: 8,
                children: [
                  Text(strings.nymAccountLevel(account.level)),
                  Text(strings.nymAccountCoins(account.coins)),
                  Text('${account.xp} XP'),
                ],
              ),
              const SizedBox(height: 10),
              LinearProgressIndicator(value: account.progress),
              const SizedBox(height: 8),
              Text(
                account.levelSize == null
                    ? strings.nymAccountMaxLevel
                    : strings.nymAccountNextLevel(account.xpLeft),
              ),
              const SizedBox(height: 8),
              Text(
                account.subscriptionActive
                    ? strings.nymAccountActive
                    : strings.nymAccountInactive,
              ),
            ] else if (!_failed) ...[
              Text(strings.nymAccountLoading),
            ],
            if (_failed) ...[
              const SizedBox(height: 8),
              Text(
                account == null
                    ? strings.nymAccountError
                    : strings.nymAccountStale,
              ),
            ],
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                FilledButton.icon(
                  onPressed: _opening ? null : () => _open(true),
                  icon: const Icon(Icons.sports_esports_outlined),
                  label: Text(strings.nymAccountPlay),
                ),
                OutlinedButton.icon(
                  onPressed: _opening ? null : () => _open(false),
                  icon: const Icon(Icons.telegram),
                  label: Text(strings.nymAccountTelegram),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(strings.nymAccountHint, style: context.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }
}
