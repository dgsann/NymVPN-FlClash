import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:fl_clash/views/nymvpn_customization.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../helpers/test_app.dart';

void main() {
  testWidgets('profile visibility and presets preserve connection preferences', (tester) async {
    await tester.pumpWidget(TestApp(
      includeNavigatorKey: false,
      setTheme: false,
      locale: const Locale('en'),
      overrides: [appSettingProvider.overrideWithBuild((_, _) => const AppSettingProps(nymAutoStopMinutes: 30, nymStopBatteryPercent: 15))],
      child: const NymvpnCustomizationView(),
    ));
    await tester.pumpAndSettle();
    final container = ProviderScope.containerOf(tester.element(find.byType(NymvpnCustomizationView)));
    final original = container.read(appSettingProvider);
    await tester.tap(find.byType(SwitchListTile));
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).dashboardWidgets, original.dashboardWidgets.where((item) => item != DashboardWidget.nymvpnAccount).toList());
    await tester.tap(find.byType(SwitchListTile));
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).dashboardWidgets.last, DashboardWidget.nymvpnAccount);
    await tester.tap(find.text('Minimal'));
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).dashboardWidgets, [DashboardWidget.networkSpeed, DashboardWidget.outboundModeV2]);
    expect(container.read(appSettingProvider).nymAutoStopMinutes, 30);
    expect(container.read(appSettingProvider).nymStopBatteryPercent, 15);
    expect(tester.takeException(), isNull);
  });
}
