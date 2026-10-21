import 'dart:io';

import 'package:package_info_plus/package_info_plus.dart';

import 'common.dart';

extension PackageInfoExtension on PackageInfo {
  String get ua => [
    '$appName/v$version',
    'clash-verge',
    'Platform/${Platform.operatingSystem}',
    'nymvpn-auto/2',
  ].join(' ');
}

int compareVersions(String version1, String version2) {
  final pattern = RegExp(
    r'^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-nymvpn\.(\d+))?(?:\+(\d+))?$',
  );
  final first = pattern.firstMatch(version1);
  final second = pattern.firstMatch(version2);
  if (first == null || second == null) {
    throw const FormatException('Unsupported app version');
  }
  for (var group = 1; group <= 5; group++) {
    final comparison = int.parse(
      first.group(group) ?? '0',
    ).compareTo(int.parse(second.group(group) ?? '0'));
    if (comparison != 0) return comparison;
  }
  return 0;
}

const releaseNotesBeginMarker = '<!-- flclash:changelog:begin -->';
const releaseNotesEndMarker = '<!-- flclash:changelog:end -->';

List<String> parseReleaseBody(String? body) {
  if (body == null) return [];
  final regex = RegExp(r'^[ \t]*-[ \t]+(.*)$', multiLine: true);
  return regex
      .allMatches(scopeReleaseNotes(body))
      .map((match) => match.group(1)?.trim() ?? '')
      .where((item) => item.isNotEmpty)
      .toList();
}

String scopeReleaseNotes(String body) {
  final begin = body.indexOf(releaseNotesBeginMarker);
  if (begin < 0) return body;
  final start = begin + releaseNotesBeginMarker.length;
  final end = body.indexOf(releaseNotesEndMarker, start);
  return end < 0 ? body.substring(start) : body.substring(start, end);
}
