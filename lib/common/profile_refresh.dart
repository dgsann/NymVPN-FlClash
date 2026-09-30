import 'dart:async';

class ProfileRefreshQueue {
  final DateTime Function() now;
  final Map<int, Future<void>> _pending = {};
  final Map<int, DateTime> _retryAfter = {};

  ProfileRefreshQueue({DateTime Function()? now}) : now = now ?? DateTime.now;

  bool canRetry(int id) => !(_retryAfter[id]?.isAfter(now()) ?? false);

  Future<void> run(int id, Future<void> Function() action) {
    final pending = _pending[id];
    if (pending != null) return pending;
    final completer = Completer<void>();
    _pending[id] = completer.future;
    unawaited(() async {
      try {
        await action();
        _retryAfter.remove(id);
        completer.complete();
      } catch (error, stack) {
        _retryAfter[id] = now().add(const Duration(minutes: 2));
        completer.completeError(error, stack);
      } finally {
        _pending.remove(id);
      }
    }());
    return completer.future;
  }
}
