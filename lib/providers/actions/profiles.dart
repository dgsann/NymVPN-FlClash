part of '../action.dart';

@Riverpod(keepAlive: true)
class ProfilesAction extends _$ProfilesAction {
  CoreController get _core => ref.read(coreHandlerProvider);

  @override
  void build() {}

  final _refreshQueue = ProfileRefreshQueue();
  bool _importing = false;

  void updateCurrentSelectedMap(String groupName, String proxyName) {
    final currentProfile = ref.read(currentProfileProvider);
    if (currentProfile != null &&
        currentProfile.selectedMap[groupName] != proxyName) {
      final selectedMap = Map<String, String>.from(currentProfile.selectedMap)
        ..[groupName] = proxyName;
      ref
          .read(profilesProvider.notifier)
          .put(currentProfile.copyWith(selectedMap: selectedMap));
    }
  }

  Future<void> deleteProfile(int id) async {
    await ref.read(profilesProvider.notifier).del(id);
    await clearEffect(id);
    final currentProfileId = ref.read(currentProfileIdProvider);
    if (currentProfileId == id) {
      final profiles = ref.read(profilesProvider);
      if (profiles.isNotEmpty) {
        final updateId = profiles.first.id;
        ref.read(currentProfileIdProvider.notifier).value = updateId;
      } else {
        ref.read(currentProfileIdProvider.notifier).value = null;
        unawaited(ref.read(setupActionProvider.notifier).setRunning(false));
      }
    }
  }

  Future<String> validateConfigWithData(String data) async {
    return _core.validateConfigWithData(data);
  }

  Future<void> autoUpdateProfiles() async {
    for (final profile in ref.read(profilesProvider)) {
      if (!profile.autoUpdate || !_refreshQueue.canRetry(profile.id)) continue;
      final managed = isNymvpnSubscription(profile.url);
      if (managed && !ref.read(isStartProvider)) continue;
      final interval = managed && profile.autoUpdateDuration > const Duration(hours: 1)
          ? const Duration(hours: 1) : profile.autoUpdateDuration;
      final isNotNeedUpdate = profile.lastUpdateDate
          ?.add(interval)
          .isBeforeNow;
      if (isNotNeedUpdate == false || profile.type == ProfileType.file) {
        continue;
      }
      try {
        await updateProfile(profile);
      } catch (e) {
        commonPrint.log('Subscription refresh failed (${e.runtimeType}); keeping saved profile', logLevel: LogLevel.warning);
      }
    }
  }

  void putProfile(Profile profile) {
    ref.read(profilesProvider.notifier).put(profile);
    if (ref.read(currentProfileIdProvider) != null) return;
    ref.read(currentProfileIdProvider.notifier).value = profile.id;
  }

  Future<void> updateProfiles() async {
    for (final profile in ref.read(profilesProvider)) {
      if (profile.type == ProfileType.file) continue;
      await updateProfile(profile);
    }
  }

  Future<void> updateProfile(
    Profile profile, {
    bool showLoading = false,
  }) => _refreshQueue.run(profile.id, () async {
    final operation = showLoading
        ? ref.read(updatingKeysProvider.notifier).start(profile.updatingKey)
        : null;
    try {
      ref.read(profilesProvider.notifier).put(profile);
      List<int>? oldBytes;
      final newProfile = await profile.update(
        validate: (path) async {
          final latest = ref.read(profilesProvider).getProfile(profile.id);
          if (latest == null || latest.url != profile.url) {
            throw MessageException('Profile changed during refresh');
          }
          final savedFile = File(await appPath.getProfilePath(profile.id.toString()));
          if (await savedFile.exists()) oldBytes = await savedFile.readAsBytes();
          return _core.validateConfig(path);
        },
      );
      final savedFile = await profile.file;
      final latest = ref.read(profilesProvider).getProfile(profile.id);
      if (latest == null || latest.url != profile.url) return;
      ref.read(profilesProvider.notifier).put(latest.copyWith(
        lastUpdateDate: newProfile.lastUpdateDate,
        subscriptionInfo: newProfile.subscriptionInfo,
        autoUpdateDuration: newProfile.autoUpdateDuration,
      ));
      final changed = !listEquals(oldBytes, await savedFile.readAsBytes());
      if (changed && profile.id == ref.read(currentProfileIdProvider)) {
        ref
            .read(setupActionProvider.notifier)
            .applyProfileDebounce(silence: true);
      }
    } finally {
      if (operation != null) {
        ref
            .read(updatingKeysProvider.notifier)
            .stop(profile.updatingKey, operation);
      }
    }
  });

  Future<void> addProfileFormFile() async {
    final platformFile = await globalState.safeRun(picker.pickerFile);
    if (platformFile == null) return;
    final bytes = await platformFile.readBytes();
    globalState.navigatorKey.currentState?.popUntil((route) => route.isFirst);
    ref.read(currentPageLabelProvider.notifier).toProfiles();
    final profile = await globalState.loadingRun(
      tag: LoadingTag.profiles,
      () async {
        return Profile.normal(
          label: platformFile.name,
        ).saveFile(bytes, validate: (path) => _core.validateConfig(path));
      },
      title: currentAppLocalizations.addProfile,
    );
    if (profile != null) {
      putProfile(profile);
    }
  }

  Future<void> addProfileFormURL(String url) async {
    if (_importing) return;
    _importing = true;
    try {
      url = url.trim();
      final managed = isNymvpnSubscription(url);
      globalState.navigatorKey.currentState?.popUntil((route) => route.isFirst);
      ref.read(currentPageLabelProvider.notifier).value = PageLabel.profiles;
      final profile = await globalState.loadingRun(
        tag: LoadingTag.profiles,
        () async {
          final matches = ref.read(profilesProvider).where((p) => p.url == url);
          Profile updated;
          if (matches.isNotEmpty) {
            final existing = matches.first;
            await updateProfile(existing);
            final latest = ref.read(profilesProvider).getProfile(existing.id);
            if (latest == null) return null;
            updated = latest;
          } else {
            updated = await Profile.normal(url: url).update(
              validate: (path) => _core.validateConfig(path),
            );
          }
          if (!managed) return updated;
          final yaml = await (await updated.file).readAsString();
          return updated.copyWith(
            label: 'NymVPN',
            autoUpdate: true,
            autoUpdateDuration: const Duration(hours: 1),
            currentGroupName: null,
            selectedMap: nymvpnInitialSelections(yaml),
          );
        },
        title: currentAppLocalizations.addProfile,
      );
      if (profile == null) return;
      final sameProfile = ref.read(currentProfileIdProvider) == profile.id;
      if (managed) {
        ref.read(patchClashConfigProvider.notifier).update(
          (state) => state.copyWith(mode: Mode.rule, tun: state.tun.copyWith(enable: true)),
        );
        ref.read(vpnSettingProvider.notifier).update((state) => state.copyWith(enable: true));
        ref.read(overrideDnsProvider.notifier).value = false;
      }
      putProfile(profile);
      ref.read(currentProfileIdProvider.notifier).value = profile.id;
      if (sameProfile) {
        ref.read(setupActionProvider.notifier).applyProfileDebounce(silence: true);
      }
      if (managed) ref.read(currentPageLabelProvider.notifier).value = PageLabel.dashboard;
    } finally {
      _importing = false;
    }
  }

  void setProfileAndAutoApply(Profile profile) {
    ref.read(profilesProvider.notifier).put(profile);
    if (profile.id == ref.read(currentProfileIdProvider)) {
      ref.read(setupActionProvider.notifier).applyProfileDebounce();
    }
  }

  Future<void> addProfileFormQrCode() async {
    final url = await globalState.safeRun(picker.pickerConfigQRCode);
    if (url == null) return;
    unawaited(addProfileFormURL(url));
  }

  void reorder(List<Profile> profiles) {
    ref.read(profilesProvider.notifier).reorder(profiles);
  }

  Future<void> clearEffect(int profileId) async {
    final profilePath = await appPath.getProfilePath(profileId.toString());
    final profileFile = File(profilePath);
    final isExists = await profileFile.exists();
    if (isExists) {
      await profileFile.safeDelete(recursive: true);
    }
    try {
      final error = await _core.clearEffect(profileId);
      if (error.isNotEmpty) {
        commonPrint.log(error, logLevel: LogLevel.warning);
      }
    } catch (error) {
      commonPrint.log(
        'clearEffect($profileId) failed: $error',
        logLevel: coreFailureLogLevel(error),
      );
    }
  }
}
