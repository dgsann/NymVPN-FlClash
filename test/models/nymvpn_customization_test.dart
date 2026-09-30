import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('old layout keeps its order and gains the previously fixed profile once', () {
    final old = {'dashboardWidgets': ['trafficUsage', 'networkSpeed']};
    final settings = AppSettingProps.fromJson(old);
    expect(settings.dashboardWidgets, [DashboardWidget.nymvpnAccount, DashboardWidget.trafficUsage, DashboardWidget.networkSpeed]);
    expect(AppSettingProps.fromJson(settings.toJson()), settings);
    expect(old.containsKey('nymDashboardVersion'), isFalse);
  });
  test('hidden profile and an entirely empty layout survive reopening', () {
    for (final widgets in [<DashboardWidget>[], [DashboardWidget.trafficUsage, DashboardWidget.networkSpeed]]) {
      final settings = AppSettingProps(dashboardWidgets: widgets, nymAutoStopMinutes: 30, nymStopBatteryPercent: 15);
      expect(AppSettingProps.fromJson(settings.toJson()), settings);
    }
  });
  test('old and fresh installations never enable automatic disconnect', () {
    for (final settings in [const AppSettingProps(), AppSettingProps.fromJson({})]) {
      expect(settings.nymAutoStopMinutes, 0);
      expect(settings.nymStopBatteryPercent, 0);
    }
  });
}
