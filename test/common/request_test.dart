import 'package:dio/dio.dart';
import 'package:fl_clash/common/exception.dart';
import 'package:fl_clash/common/request.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:fl_clash/state.dart';
import 'package:package_info_plus/package_info_plus.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  globalState.packageInfo = PackageInfo(
    appName: 'NymVPN',
    packageName: 'com.follow.clash.nymvpn',
    version: '0.8.98-nymvpn.11',
    buildNumber: '2026102011',
  );

  for (final tag in [
    'v0.8.98-nymvpn.12',
    'v0.8.98-nymvpn.11',
    'v0.8.98-nymvpn.9',
    'invalid',
  ]) {
    test('checks the NymVPN update channel for $tag', () async {
      final client = Request();
      addTearDown(() => client.dio.close(force: true));
      String? requestedUrl;
      client.dio.interceptors.add(
        InterceptorsWrapper(
          onRequest: (options, handler) {
            requestedUrl = options.uri.toString();
            handler.resolve(
              Response(
                requestOptions: options,
                statusCode: 200,
                data: {'tag_name': tag},
              ),
            );
          },
        ),
      );
      final release = await client.checkForUpdate();
      expect(
        requestedUrl,
        'https://api.github.com/repos/dgsann/NymVPN-FlClash/releases/latest',
      );
      if (tag == 'v0.8.98-nymvpn.12') {
        expect(release?['tag_name'], tag);
      } else {
        expect(release, isNull);
      }
    });
  }


  test('getTextResponseForUrl propagates the typed DioException', () async {
    // flutter_test's mocked HttpClient answers every request with HTTP 400,
    // which Dio surfaces as a badResponse DioException.
    await expectLater(
      request.getTextResponseForUrl('http://127.0.0.1/anything'),
      throwsA(
        isA<DioException>().having(
          (e) => e.type,
          'type',
          DioExceptionType.badResponse,
        ),
      ),
    );
  });

  test(
    'subscription failures show a safe message without the private URL',
    () async {
      await expectLater(
        request.getFileResponseForUrl('http://127.0.0.1/private-token'),
        throwsA(
          isA<MessageException>().having(
            (e) => e.message,
            'message',
            isNot(contains('private-token')),
          ),
        ),
      );
    },
  );
}
