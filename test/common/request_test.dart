import 'package:dio/dio.dart';
import 'package:fl_clash/common/exception.dart';
import 'package:fl_clash/common/request.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

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
