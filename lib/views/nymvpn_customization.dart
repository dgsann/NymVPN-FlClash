import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:fl_clash/views/access.dart';
import 'package:fl_clash/views/config/on_demand.dart';
import 'package:fl_clash/views/theme.dart';
import 'package:fl_clash/widgets/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

class NymvpnCustomizationView extends ConsumerWidget {
  const NymvpnCustomizationView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final strings = context.appLocalizations;
    final settings = ref.watch(appSettingProvider);
    void layout(List<DashboardWidget> widgets) {
      ref.read(appSettingProvider.notifier).update(
        (state) => state.copyWith(dashboardWidgets: widgets),
      );
    }

    String minutes(int value) => value == 0 ? strings.nymDisabled : strings.nymMinutes(value);
    String battery(int value) => value == 0 ? strings.nymDisabled : '$value%';

    return BaseScaffold(
      title: strings.nymCustomize,
      body: ListView(
        padding: const EdgeInsets.only(bottom: 24),
        children: [
          ListHeader(title: strings.nymHomeLayout),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Text(strings.nymLayoutHint),
          ),
          Padding(
            padding: const EdgeInsets.all(16),
            child: Wrap(spacing: 8, runSpacing: 8, children: [
              OutlinedButton(
                onPressed: () => layout(const [DashboardWidget.networkSpeed, DashboardWidget.outboundModeV2]),
                child: Text(strings.nymMinimal),
              ),
              OutlinedButton(
                onPressed: () => layout(defaultDashboardWidgets),
                child: Text(strings.nymStandard),
              ),
              OutlinedButton(
                onPressed: () => layout(const [DashboardWidget.networkSpeed, DashboardWidget.networkDetection, DashboardWidget.trafficUsage, DashboardWidget.memoryInfo]),
                child: Text(strings.nymConnectionStats),
              ),
            ]),
          ),
          SwitchListTile(
            title: Text(strings.nymAccountTitle),
            value: settings.dashboardWidgets.contains(DashboardWidget.nymvpnAccount),
            onChanged: (value) => layout(value
                ? [...settings.dashboardWidgets, DashboardWidget.nymvpnAccount]
                : settings.dashboardWidgets.where((item) => item != DashboardWidget.nymvpnAccount).toList()),
          ),
          if (system.isAndroid) ...[
            ListHeader(title: strings.nymPowerTitle),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Text(strings.nymPowerHint),
            ),
            ListItem<int>.options(
              leading: const Icon(Icons.timer_outlined),
              title: Text(strings.nymStopTimer),
              subtitle: Text(minutes(settings.nymAutoStopMinutes)),
              dialogTitle: strings.nymStopTimer,
              options: const [0, 15, 30, 60, 120, 240],
              value: settings.nymAutoStopMinutes,
              textBuilder: minutes,
              onChanged: (int? value) {
                if (value == null) return;
                ref.read(appSettingProvider.notifier).update((state) => state.copyWith(nymAutoStopMinutes: value));
              },
            ),
            ListItem<int>.options(
              leading: const Icon(Icons.battery_saver_outlined),
              title: Text(strings.nymLowBattery),
              subtitle: Text(battery(settings.nymStopBatteryPercent)),
              dialogTitle: strings.nymLowBattery,
              options: const [0, 5, 10, 15, 20],
              value: settings.nymStopBatteryPercent,
              textBuilder: battery,
              onChanged: (int? value) {
                if (value == null) return;
                ref.read(appSettingProvider.notifier).update((state) => state.copyWith(nymStopBatteryPercent: value));
              },
            ),
          ],
          ListHeader(title: strings.nymMoreCustomization),
          ListItem.open(
            leading: const Icon(Icons.palette_outlined),
            title: Text(strings.theme),
            subtitle: Text(strings.themeDesc),
            widget: const ThemeView(),
          ),
          if (system.isAndroid)
            ListItem.open(
              leading: const Icon(Icons.apps),
              title: Text(strings.nymApps),
              subtitle: Text(strings.nymAppsHint),
              widget: const AccessView(),
            ),
          ListItem.open(
            leading: const Icon(Icons.wifi),
            title: Text(strings.onDemand),
            subtitle: Text(strings.onDemandDesc),
            widget: const OnDemandView(),
          ),
        ],
      ),
    );
  }
}
