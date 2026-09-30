import 'dart:io';
import 'dart:ui' as ui;

import 'package:fl_clash/common/nymvpn_account.dart';
import 'package:fl_clash/common/shape.dart';
import 'package:fl_clash/views/dashboard/widgets/nymvpn_account_card.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';

import '../helpers/test_app.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUpAll(() async {
    final root = Platform.environment['FLUTTER_ROOT'];
    if (root == null) return;
    final font = File(
      '$root/bin/cache/artifacts/material_fonts/Roboto-Regular.ttf',
    );
    if (!font.existsSync()) return;
    final loader = FontLoader('Roboto');
    loader.addFont(
      Future.value(ByteData.sublistView(await font.readAsBytes())),
    );
    await loader.load();
    final icons = File('$root/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf');
    if (icons.existsSync()) {
      final iconLoader = FontLoader('MaterialIcons');
      iconLoader.addFont(Future.value(ByteData.sublistView(await icons.readAsBytes())));
      await iconLoader.load();
    }
  });

  for (final size in [(320.0, 2.0), (390.0, 1.0), (840.0, 1.0)]) {
    testWidgets('Russian account fits ${size.$1} wide at text scale ${size.$2}', (
      tester,
    ) async {
      tester.view.physicalSize = Size(size.$1, 1500);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final capture = GlobalKey();
      await tester.pumpWidget(
        TestApp(
          includeNavigatorKey: false,
          setTheme: false,
          locale: const Locale('ru'),
          homeBuilder: (child) =>
              Scaffold(body: SingleChildScrollView(child: child)),
          child: MediaQuery.withClampedTextScaling(
            minScaleFactor: size.$2,
            maxScaleFactor: size.$2,
            child: Theme(
              data: ThemeData(
                fontFamily: 'Roboto',
                colorScheme: ColorScheme.fromSeed(
                  seedColor: const Color(0xFFBA91FF),
                  brightness: Brightness.dark,
                ),
              ).withAppShapes,
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: RepaintBoundary(
                  key: capture,
                  child: NymvpnAccountCard(
                    subscription:
                        'https://sub.pixel-node.online/sub/7/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
                    load: (_) async => const NymvpnAccount(
                      userId: 7,
                      name: 'Игрок NymVPN',
                      username: 'nymvpn_player',
                      level: 3,
                      xp: 310,
                      xpLeft: 390,
                      coins: 22,
                      levelXp: 10,
                      levelSize: 400,
                      subscriptionActive: true,
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      expect(find.text('Уровень 3'), findsOneWidget);
      expect(find.text('Играть в Telegram'), findsOneWidget);
      if (Platform.environment['NYMVPN_UI_CAPTURE'] == '1') {
        final boundary =
            capture.currentContext!.findRenderObject()!
                as RenderRepaintBoundary;
        await tester.runAsync(() async {
          final image = await boundary.toImage(pixelRatio: 2);
          final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
          final output = File(
            'build/profile-preview/account-${size.$1.toInt()}-${size.$2}.png',
          );
          await output.parent.create(recursive: true);
          await output.writeAsBytes(bytes!.buffer.asUint8List());
          image.dispose();
        });
      }
    });
  }
}
