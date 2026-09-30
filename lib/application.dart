import 'dart:async';
import 'dart:io';

import 'package:fl_clash/common/nymvpn_profile.dart';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/common/window.dart';
import 'package:fl_clash/bootstrap.dart';
import 'package:fl_clash/common/system_dns.dart';
import 'package:fl_clash/l10n/l10n.dart';
import 'package:fl_clash/manager/hotkey_manager.dart';
import 'package:fl_clash/manager/manager.dart';
import 'package:fl_clash/plugins/app.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:fl_clash/state.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'pages/pages.dart';

Widget buildManagerStack({
  required bool isDesktop,
  required Future<void> Function(List<ConnectivityResult> results)
  onConnectivityChanged,
  required Widget child,
}) {
  final platformApp = isDesktop
      ? WindowHeaderContainer(child: child)
      : VpnManager(child: child);
  final state = AppStateManager(
    child: CoreManager(
      child: ConnectivityManager(
        onConnectivityChanged: onConnectivityChanged,
        child: platformApp,
      ),
    ),
  );
  final platformState = isDesktop
      ? WindowManager(
          child: TrayManager(
            child: HotKeyManager(child: ProxyManager(child: state)),
          ),
        )
      : AndroidManager(child: TileManager(child: state));
  return AppEnvManager(
    child: LocaleManager(
      child: StatusManager(child: ThemeManager(child: platformState)),
    ),
  );
}

class Application extends ConsumerStatefulWidget {
  const Application({super.key});

  @override
  ConsumerState<Application> createState() => ApplicationState();
}

class ApplicationState extends ConsumerState<Application> with WidgetsBindingObserver {
  Timer? _autoUpdateProfilesTaskTimer;
  bool _preHasVpn = false;
  Timer? _connectionRefreshTimer;

  final _pageTransitionsTheme = const PageTransitionsTheme(
    builders: <TargetPlatform, PageTransitionsBuilder>{
      TargetPlatform.android: commonSharedXPageTransitions,
      TargetPlatform.windows: commonSharedXPageTransitions,
      TargetPlatform.linux: commonSharedXPageTransitions,
      TargetPlatform.macOS: commonSharedXPageTransitions,
    },
  );

  ColorScheme _getAppColorScheme({required Brightness brightness}) {
    return ref.read(genColorSchemeProvider(brightness));
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    ref.listenManual(isStartProvider, (previous, next) {
      if (next && previous != next) _scheduleConnectionRefresh();
    });
    SystemNavigator.setFrameworkHandlesBack(true);
    WidgetsBinding.instance.addPostFrameCallback((timeStamp) async {
      if (globalState.navigatorKey.currentContext != null) {
        await bootstrap.attach();
      } else {
        exit(0);
      }
      if (!mounted) return;
      _autoUpdateProfilesTask();
      _scheduleConnectionRefresh();
      _initLink();
      unawaited(app?.initShortcuts());
    });
  }

  void _initLink() {
    linkManager.initAppLinksListen((url) async {
      unawaited(window?.show());
      if (isNymvpnSubscription(url)) {
        await ref.read(profilesActionProvider.notifier).addProfileFormURL(url);
        return;
      }
      final message = currentAppLocalizations.createProfileFromUrlTip(url);
      final parts = message.split(url);
      final res = await dialogs.showMessage(
        title: currentAppLocalizations.addProfile,
        message: TextSpan(
          children: [
            TextSpan(text: parts.first),
            TextSpan(
              text: url,
              style: TextStyle(
                color: context.colorScheme.primary,
                decoration: TextDecoration.underline,
                decorationColor: context.colorScheme.primary,
              ),
            ),
            if (parts.length > 1) TextSpan(text: parts.last),
          ],
        ),
      );
      if (res != true) return;
      unawaited(
        ref.read(profilesActionProvider.notifier).addProfileFormURL(url),
      );
    });
  }

  Future<void> _refreshDueProfiles() async {
    if (!mounted) return;
    try {
      await ref.read(profilesActionProvider.notifier).autoUpdateProfiles();
    } catch (error) {
      commonPrint.log('Subscription check failed (${error.runtimeType})');
    }
  }

  void _autoUpdateProfilesTask() {
    _autoUpdateProfilesTaskTimer?.cancel();
    _autoUpdateProfilesTaskTimer = Timer(const Duration(minutes: 1), () async {
      await _refreshDueProfiles();
      if (mounted) _autoUpdateProfilesTask();
    });
  }

  void _scheduleConnectionRefresh() {
    _connectionRefreshTimer?.cancel();
    _connectionRefreshTimer = Timer(const Duration(seconds: 5), _refreshDueProfiles);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _scheduleConnectionRefresh();
  }

  Future<void> _handleConnectivityChanged(
    List<ConnectivityResult> results,
  ) async {
    commonPrint.log('connectivityChanged ${results.toString()}');
    unawaited(systemDnsCoordinator?.resync() ?? Future.value());
    unawaited(ref.read(systemActionProvider.notifier).updateLocalIp());
    final hasVpn = results.contains(ConnectivityResult.vpn);
    if (_preHasVpn == hasVpn) {
      ref.read(checkIpNumProvider.notifier).add();
    }
    _preHasVpn = hasVpn;
    if (results.any((result) => result != ConnectivityResult.none)) {
      _scheduleConnectionRefresh();
    }
  }

  @override
  Widget build(context) {
    return Consumer(
      builder: (_, ref, child) {
        final locale = ref.watch(
          appSettingProvider.select((state) => state.locale),
        );
        final themeProps = ref.watch(themeSettingProvider);
        return MaterialApp(
          debugShowCheckedModeBanner: false,
          navigatorKey: globalState.navigatorKey,
          onNavigationNotification: (_) => true,
          localizationsDelegates: const [
            AppLocalizations.delegate,
            ...GlobalMaterialLocalizations.delegates,
          ],
          builder: (context, child) {
            // The bridge's legacy Theme swaps in its own default IconTheme color,
            // which material_ui IconButton.filled reads as custom and loses onPrimary.
            // ignore: deprecated_member_use
            return MaterialUiCompatibilityBridge(
              child: IconTheme(
                data: Theme.of(context).iconTheme,
                child: buildManagerStack(
                  isDesktop: system.isDesktop,
                  onConnectivityChanged: _handleConnectivityChanged,
                  child: child!,
                ),
              ),
            );
          },
          scrollBehavior: const BaseScrollBehavior(),
          title: appName,
          locale: getLocaleForString(locale),
          supportedLocales: AppLocalizations.delegate.supportedLocales,
          themeMode: themeProps.themeMode,
          theme: ThemeData(
            useMaterial3: true,
            pageTransitionsTheme: _pageTransitionsTheme,
            colorScheme: _getAppColorScheme(brightness: Brightness.light),
          ).withAppShapes,
          darkTheme: ThemeData(
            useMaterial3: true,
            pageTransitionsTheme: _pageTransitionsTheme,
            colorScheme: _getAppColorScheme(
              brightness: Brightness.dark,
            ).toPureBlack(themeProps.pureBlack),
          ).withAppShapes,
          home: child!,
        );
      },
      child: const HomePage(),
    );
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _connectionRefreshTimer?.cancel();
    linkManager.destroy();
    _autoUpdateProfilesTaskTimer?.cancel();
    super.dispose();
  }
}
