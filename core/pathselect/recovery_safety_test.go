package pathselect

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func TestRecoveryKeepsHealthyPreferredWithoutProbingBackups(t *testing.T) {
	preferred, backup := fake("preferred"), fake("backup")
	a := New("verified", Config{Version: 2}, []C.Proxy{preferred, backup})
	var backups atomic.Int32
	a.check = func(_ context.Context, p C.Proxy, _ string) error {
		if p == backup {
			backups.Add(1)
		}
		return nil
	}
	a.SetActive(true)
	defer a.Close()
	a.mu.Lock()
	a.selected = preferred
	a.mu.Unlock()
	if choose(t, a) != preferred || backups.Load() != 0 {
		t.Fatal("healthy preferred route was replaced")
	}
}

func TestRecoveryConfirmsFailureBeforeRetiringExistingConnections(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		t.Run(map[bool]string{true: "healthy", false: "failed"}[healthy], func(t *testing.T) {
			preferred, backup := fake("preferred"), fake("backup")
			a := New("verified", Config{Version: 2}, []C.Proxy{preferred, backup})
			release := make(chan struct{})
			checked := make(chan struct{})
			a.check = func(ctx context.Context, p C.Proxy, _ string) error {
				if p == backup {
					return nil
				}
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				defer close(checked)
				if healthy {
					return nil
				}
				return errors.New("old route stopped transferring")
			}
			a.SetActive(true)
			defer a.Close()
			a.mu.Lock()
			a.selected = preferred
			life := a.life
			a.mu.Unlock()
			conn, _ := preferred.DialContext(context.Background(), &C.Metadata{})
			raw := &countedConn{Conn: conn}
			tracked := a.track(raw, preferred, life)
			defer tracked.Close()
			if choose(t, a) != backup {
				t.Fatal("backup was not available during slow probe")
			}
			if raw.closed.Load() {
				t.Fatal("unconfirmed failure interrupted existing connection")
			}
			close(release)
			<-checked
			if healthy {
				time.Sleep(30 * time.Millisecond)
				if raw.closed.Load() {
					t.Fatal("healthy connection was retired")
				}
			} else {
				deadline := time.Now().Add(time.Second)
				for !raw.closed.Load() && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if !raw.closed.Load() {
					t.Fatal("confirmed dead connection was not retired")
				}
			}
		})
	}
}

func TestRecoveryStopCancelsWarmupAndDoesNotReconnect(t *testing.T) {
	a := New("verified", Config{Version: 2}, []C.Proxy{fake("blocked")})
	started, canceled := make(chan struct{}), make(chan struct{})
	a.check = func(ctx context.Context, _ C.Proxy, _ string) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}
	a.SetActive(true)
	defer a.Close()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("warmup did not start")
	}
	a.SetActive(false)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel warmup")
	}
	if _, _, err := a.choose(context.Background()); err == nil {
		t.Fatal("stopped VPN selected a path")
	}
}

func TestRecoveryDoesNotRetryCallerCanceledDial(t *testing.T) {
	route := fake("route")
	a := New("verified", Config{Version: 2}, []C.Proxy{route})
	a.check = func(context.Context, C.Proxy, string) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	route.Adapter().(*fakeAdapter).dial = func(context.Context, *C.Metadata) (C.Conn, error) {
		calls.Add(1)
		cancel()
		return nil, context.Canceled
	}
	a.SetActive(true)
	defer a.Close()
	if _, err := a.DialContext(ctx, &C.Metadata{}); err == nil {
		t.Fatal("canceled call succeeded")
	}
	if calls.Load() != 1 {
		t.Fatal("canceled request was retried")
	}
}

func TestRecoveryBackoffIsBoundedAndNetworkChangeResetsIt(t *testing.T) {
	a := New("verified", Config{Version: 2}, []C.Proxy{fake("blocked")})
	a.check = func(context.Context, C.Proxy, string) error { return errors.New("offline") }
	a.SetActive(true)
	defer a.Close()
	var previous time.Duration
	for range 7 {
		a.mu.Lock()
		a.retryAfter = time.Time{}
		a.mu.Unlock()
		if _, _, err := a.choose(context.Background()); err == nil {
			t.Fatal("offline path accepted")
		}
		a.mu.Lock()
		delay := time.Until(a.retryAfter)
		a.mu.Unlock()
		if delay < previous-100*time.Millisecond || delay > 30*time.Second {
			t.Fatal("unbounded or regressing backoff")
		}
		previous = delay
	}
	a.Reset()
	if _, _, err := a.choose(context.Background()); err == nil {
		t.Fatal("offline path accepted after reset")
	}
	a.mu.Lock()
	delay := time.Until(a.retryAfter)
	a.mu.Unlock()
	if delay > 2*time.Second {
		t.Fatal("network change retained old backoff")
	}
}
