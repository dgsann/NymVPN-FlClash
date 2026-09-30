import 'dart:async';

import 'package:flutter/services.dart';

class DesktopBattery {
  final int? percent;
  final bool onBattery;

  const DesktopBattery({this.percent, this.onBattery = false});

  static const _channel = MethodChannel('com.follow.clash/power');

  static Future<DesktopBattery> read() async {
    try {
      final value = await _channel.invokeMapMethod<String, Object?>(
        'readBattery',
      );
      final level = value?['percent'];
      return DesktopBattery(
        percent: level is int && level >= 0 && level <= 100 ? level : null,
        onBattery: value?['onBattery'] == true,
      );
    } catch (_) {
      return const DesktopBattery();
    }
  }
}

class DesktopPowerMonitor {
  final Future<DesktopBattery> Function() readBattery;
  final Future<void> Function(Object token) stop;
  final int Function() now;
  final void Function(Object error)? onError;
  Timer? _timer;
  Object? _token;
  int _minutes = 0;
  int _batteryPercent = 0;
  int _started = 0;
  int _generation = 0;
  bool _checking = false;

  DesktopPowerMonitor({
    required this.readBattery,
    required this.stop,
    required this.now,
    this.onError,
  });

  void configure({
    required Object? token,
    required int minutes,
    required int batteryPercent,
  }) {
    if (identical(token, _token) &&
        minutes == _minutes &&
        batteryPercent == _batteryPercent) {
      return;
    }
    _generation++;
    _timer?.cancel();
    _token = token;
    _minutes = minutes;
    _batteryPercent = batteryPercent;
    _started = now();
    if (token == null || (!_hasTimer && !_hasBattery)) {
      return;
    }
    _timer = Timer.periodic(
      const Duration(seconds: 30),
      (_) => unawaited(check()),
    );
  }

  bool get _hasTimer => _minutes >= 1 && _minutes <= 1440;
  bool get _hasBattery => _batteryPercent >= 1 && _batteryPercent <= 50;

  Future<void> check() async {
    final token = _token;
    if (_checking || token == null || (!_hasTimer && !_hasBattery)) {
      return;
    }
    _checking = true;
    final generation = _generation;
    try {
      final timerDue = _hasTimer && now() - _started >= _minutes * 60000;
      final battery = _hasBattery && !timerDue
          ? await readBattery().timeout(const Duration(seconds: 2))
          : const DesktopBattery();
      if (generation != _generation || !identical(token, _token)) {
        return;
      }
      final level = battery.percent;
      if (timerDue ||
          (_hasBattery &&
              battery.onBattery &&
              level != null &&
              level >= 0 &&
              level <= _batteryPercent)) {
        await stop(token);
        if (generation == _generation) {
          configure(
            token: null,
            minutes: _minutes,
            batteryPercent: _batteryPercent,
          );
        }
      }
    } catch (error) {
      onError?.call(error);
    } finally {
      _checking = false;
    }
  }

  void dispose() {
    _generation++;
    _timer?.cancel();
    _token = null;
  }
}
