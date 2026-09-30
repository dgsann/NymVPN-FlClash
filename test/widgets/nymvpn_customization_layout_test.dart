import 'dart:io';
import 'dart:ui' as ui;

import 'package:fl_clash/common/shape.dart';
import 'package:fl_clash/views/nymvpn_customization.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../helpers/test_app.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() async {
    final root = Platform.environment['FLUTTER_ROOT'];
    if (root == null) {
      return;
    }
    for (final item in [
      ('Roboto', 'Roboto-Regular.ttf'),
      ('MaterialIcons', 'MaterialIcons-Regular.otf'),
    ]) {
      final font = File('$root/bin/cache/artifacts/material_fonts/${item.$2}');
      if (font.existsSync()) {
        final loader = FontLoader(item.$1);
        loader.addFont(
          Future.value(ByteData.sublistView(await font.readAsBytes())),
        );
        await loader.load();
      }
    }
  });
  for (final size in [(320.0, 2.0), (390.0, 1.0), (840.0, 1.0)]) {
    testWidgets('customization fits ${size.$1} at text scale ${size.$2}', (
      tester,
    ) async {
      tester.view.physicalSize = Size(size.$1, 1300);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final capture = GlobalKey();
      await tester.pumpWidget(
        TestApp(
          includeNavigatorKey: false,
          setTheme: true,
          wrapInProviderScope: true,
          locale: const Locale('ru'),
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
              child: RepaintBoundary(
                key: capture,
                child: const NymvpnCustomizationView(isAndroid: true),
              ),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      if (Platform.environment['NYMVPN_UI_CAPTURE'] == '1') {
        final boundary =
            capture.currentContext!.findRenderObject()!
                as RenderRepaintBoundary;
        await tester.runAsync(() async {
          final image = await boundary.toImage(pixelRatio: 2);
          final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
          final output = File(
            'build/profile-preview/customize-${size.$1.toInt()}-${size.$2}.png',
          );
          await output.parent.create(recursive: true);
          await output.writeAsBytes(bytes!.buffer.asUint8List());
          image.dispose();
        });
      }
      await tester.drag(find.byType(ListView), const Offset(0, -1000));
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
    });
  }
}
