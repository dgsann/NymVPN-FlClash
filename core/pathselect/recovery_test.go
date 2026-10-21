package pathselect

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func TestRecoveryDoesNotWaitForStalledPreferredRoute(t *testing.T) {
	old, backup := fake("old"), fake("backup")
	a := New("verified", Config{Version: 2}, []C.Proxy{old, backup})
	a.check = func(ctx context.Context, p C.Proxy, _ string) error {
		if p == old {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	a.SetActive(true)
	defer a.Close()
	a.mu.Lock()
	a.selected = old
	a.mu.Unlock()
	if choose(t, a) != backup {
		t.Fatal("working backup was not selected")
	}
}

func TestRecoveryWarmsRouteWithoutWaitingForUserTraffic(t *testing.T) {
	a := New("verified", Config{Version: 2}, []C.Proxy{fake("ready")})
	checked := make(chan struct{}, 1)
	a.check = func(context.Context, C.Proxy, string) error { checked <- struct{}{}; return nil }
	a.SetActive(true)
	defer a.Close()
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("VPN did not prepare a route after start")
	}
}

func TestRecoveryRetriesAfterOfflineStartWithoutAnotherRequest(t *testing.T) {
	a := New("verified", Config{Version: 2}, []C.Proxy{fake("restored")})
	var calls atomic.Int32
	recovered := make(chan struct{}, 1)
	a.check = func(context.Context, C.Proxy, string) error {
		if calls.Add(1) == 1 {
			return errors.New("network not ready")
		}
		recovered <- struct{}{}
		return nil
	}
	a.SetActive(true)
	defer a.Close()
	select {
	case <-recovered:
	case <-time.After(4 * time.Second):
		t.Fatal("no automatic recovery after network returned")
	}
}

func TestRecoveryRetriesFailedDestinationDialBeforeReturningError(t *testing.T) {
	old, backup := fake("old"), fake("backup")
	a := New("verified", Config{Version: 2}, []C.Proxy{old, backup})
	var failed atomic.Bool
	a.check = func(_ context.Context, p C.Proxy, _ string) error {
		if p == old && !failed.Load() || p == backup && failed.Load() {
			return nil
		}
		return errors.New("unreachable")
	}
	old.Adapter().(*fakeAdapter).dial = func(context.Context, *C.Metadata) (C.Conn, error) {
		failed.Store(true)
		return nil, errors.New("route disconnected")
	}
	a.SetActive(true)
	defer a.Close()
	conn, err := a.DialContext(context.Background(), &C.Metadata{})
	if err != nil {
		t.Fatal("available backup was not retried:", err)
	}
	conn.Close()
	if choose(t, a) != backup {
		t.Fatal("backup was not retained")
	}
}
