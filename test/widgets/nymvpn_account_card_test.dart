import 'dart:async';
import 'package:fl_clash/common/nymvpn_account.dart';
import 'package:fl_clash/views/dashboard/widgets/nymvpn_account_card.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';

import '../helpers/test_app.dart';

String subscription(int id) =>
    'https://sub.pixel-node.online/sub/$id/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
NymvpnAccount account(int id) => NymvpnAccount(
  userId: id,
  name: 'Player $id',
  username: '',
  level: 3,
  xp: 310,
  xpLeft: 390,
  coins: 22,
  levelXp: 10,
  levelSize: 400,
  subscriptionActive: true,
);
Widget app({
  required int id,
  required AccountLoader load,
  AccountLinkOpener? open,
}) => TestApp(
  includeNavigatorKey: false,
  setTheme: false,
  locale: const Locale('en'),
  homeBuilder: (child) => Scaffold(body: SingleChildScrollView(child: child)),
  child: NymvpnAccountCard(
    subscription: subscription(id),
    load: load,
    openLink: open,
  ),
);

void main() {
  testWidgets(
    'late response from an old subscription cannot replace new account',
    (tester) async {
      final first = Completer<NymvpnAccount>();
      final second = Completer<NymvpnAccount>();
      Future<NymvpnAccount> load(String url) =>
          url == subscription(7) ? first.future : second.future;
      await tester.pumpWidget(app(id: 7, load: load));
      await tester.pumpWidget(app(id: 8, load: load));
      second.complete(account(8));
      await tester.pumpAndSettle();
      first.complete(account(7));
      await tester.pumpAndSettle();
      expect(find.text('Player 8'), findsOneWidget);
      expect(find.text('Player 7'), findsNothing);
    },
  );
  testWidgets(
    'failure ends loading, retry succeeds and game opens without credentials',
    (tester) async {
      var calls = 0;
      Uri? opened;
      Future<NymvpnAccount> load(String _) async {
        if (++calls == 1) throw StateError('offline');
        return account(7);
      }

      await tester.pumpWidget(
        app(
          id: 7,
          load: load,
          open: (uri) async {
            opened = uri;
            return true;
          },
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byType(LinearProgressIndicator), findsNothing);
      await tester.tap(find.byTooltip('Refresh profile'));
      await tester.pumpAndSettle();
      expect(find.text('Player 7'), findsOneWidget);
      await tester.tap(find.text('Play in Telegram'));
      await tester.pumpAndSettle();
      expect(opened.toString(), 'https://t.me/nym_vpnbot?start=app_game_7');
    },
  );
  testWidgets('returning from Telegram refreshes progress', (tester) async {
    var calls = 0;
    await tester.pumpWidget(
      app(
        id: 7,
        load: (_) async {
          calls++;
          return account(7);
        },
      ),
    );
    await tester.pumpAndSettle();
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();
    expect(calls, 2);
  });

  testWidgets('failed refresh keeps the last account with a stale notice', (
    tester,
  ) async {
    var calls = 0;
    await tester.pumpWidget(
      app(
        id: 7,
        load: (_) async {
          if (++calls > 1) throw StateError('offline');
          return account(7);
        },
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Refresh profile'));
    await tester.pumpAndSettle();
    expect(find.text('Player 7'), findsOneWidget);
    expect(
      find.text('Could not refresh. Showing your last loaded progress.'),
      findsOneWidget,
    );
    expect(
      tester.widget<IconButton>(find.byType(IconButton)).onPressed,
      isNotNull,
    );
  });
  testWidgets('closing the card safely ignores a pending response', (
    tester,
  ) async {
    final pending = Completer<NymvpnAccount>();
    await tester.pumpWidget(app(id: 7, load: (_) => pending.future));
    await tester.pumpWidget(const SizedBox.shrink());
    pending.complete(account(7));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });
}
