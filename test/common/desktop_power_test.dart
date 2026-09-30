import 'dart:async';

import 'package:fl_clash/common/desktop_power.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('timer is opt in, fires once and unrelated updates preserve deadline', () async {
    var time = 0;
    var stops = 0;
    var reads = 0;
    final token = Object();
    final monitor = DesktopPowerMonitor(now: () => time, readBattery: () async { reads++; return const DesktopBattery(); }, stop: (_) async { stops++; });
    addTearDown(monitor.dispose);
    monitor.configure(token: token, minutes: 0, batteryPercent: 0);
    time = 90000;
    await monitor.check();
    expect(stops, 0);
    monitor.configure(token: token, minutes: 1, batteryPercent: 0);
    time += 30000;
    monitor.configure(token: token, minutes: 1, batteryPercent: 0);
    time += 29999;
    await monitor.check();
    expect(stops, 0);
    time++;
    await monitor.check();
    await monitor.check();
    expect(stops, 1);
    expect(reads, 0);
  });
  test('battery needs a valid reading while unplugged', () async {
    var battery = const DesktopBattery();
    var stops = 0;
    final monitor = DesktopPowerMonitor(now: () => 0, readBattery: () async => battery, stop: (_) async { stops++; });
    addTearDown(monitor.dispose);
    monitor.configure(token: Object(), minutes: 0, batteryPercent: 15);
    for (final sample in [const DesktopBattery(), const DesktopBattery(percent: 10), const DesktopBattery(percent: -1, onBattery: true), const DesktopBattery(percent: 16, onBattery: true)]) {
      battery = sample;
      await monitor.check();
      expect(stops, 0);
    }
    battery = const DesktopBattery(percent: 15, onBattery: true);
    await monitor.check();
    expect(stops, 1);
  });
  for (final change in ['restart', 'disable', 'dispose', 'disable-enable']) {
    test('late battery result cannot stop after $change', () async {
      final reading = Completer<DesktopBattery>();
      var stops = 0;
      final token = Object();
      final monitor = DesktopPowerMonitor(now: () => 0, readBattery: () => reading.future, stop: (_) async { stops++; });
      addTearDown(monitor.dispose);
      monitor.configure(token: token, minutes: 0, batteryPercent: 15);
      final pending = monitor.check();
      if (change == 'dispose') {
        monitor.dispose();
      } else {
        monitor.configure(token: change == 'restart' ? Object() : token, minutes: 0, batteryPercent: change == 'restart' ? 15 : 0);
        if (change == 'disable-enable') {
          monitor.configure(token: token, minutes: 0, batteryPercent: 15);
        }
      }
      reading.complete(const DesktopBattery(percent: 1, onBattery: true));
      await pending;
      expect(stops, 0);
    });
  }
}
