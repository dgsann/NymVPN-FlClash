package pathselect

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
)

type fakeAdapter struct {
	*outbound.Base
	dial func(context.Context, *C.Metadata) (C.Conn, error)
}

func (a *fakeAdapter) DialContext(ctx context.Context, m *C.Metadata) (C.Conn, error) {
	return a.dial(ctx, m)
}

func fake(name string) C.Proxy {
	a := &fakeAdapter{Base: outbound.NewBase(outbound.BaseOption{Name: name, Type: C.Socks5})}
	a.dial = func(context.Context, *C.Metadata) (C.Conn, error) {
		left, right := net.Pipe()
		right.Close()
		return outbound.NewConn(left, a), nil
	}
	return adapter.NewProxy(a)
}

func fixture(t *testing.T, candidates ...C.Proxy) *Adapter {
	t.Helper()
	a := New("AutoTCP", Config{ProbeURL: "https://example.test/probe/private"}, candidates)
	t.Cleanup(func() { a.Close() })
	a.SetActive(true)
	return a
}

func choose(t *testing.T, a *Adapter) C.Proxy {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, _, err := a.choose(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompleteProbeWinsAndCancelsStalledCandidate(t *testing.T) {
	bad, good := fake("bad"), fake("good")
	a := fixture(t, bad, good)
	started, stopped := make(chan struct{}), make(chan struct{})
	a.check = func(ctx context.Context, p C.Proxy, _ string) error {
		if p == bad {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}
		<-started
		return nil
	}
	if choose(t, a) != good {
		t.Fatal("wrong winner")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("loser not canceled")
	}
	if a.SupportUDP() {
		t.Fatal("TCP selector advertises UDP")
	}
}

func TestConcurrentWaitersShareProbeAndCallerCancellation(t *testing.T) {
	good := fake("good")
	a := fixture(t, good)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.check = func(ctx context.Context, p C.Proxy, _ string) error {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, _, err := a.choose(ctx); first <- err }()
	<-started
	cancel()
	if !errors.Is(<-first, context.Canceled) {
		t.Fatal("caller cancellation lost")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { choose(t, a) })
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate probes: %d", calls.Load())
	}
	choose(t, a)
	if calls.Load() != 1 {
		t.Fatal("fresh healthy path re-probed")
	}
}

func TestStopResetAndCloseRejectStaleResults(t *testing.T) {
	for _, action := range []string{"stop", "reset", "close"} {
		t.Run(action, func(t *testing.T) {
			a := fixture(t, fake("good"))
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			a.check = func(context.Context, C.Proxy, string) error { close(started); <-release; return nil }
			go func() { _, _, err := a.choose(context.Background()); finished <- err }()
			<-started
			a.mu.Lock()
			old := a.current
			a.mu.Unlock()
			switch action {
			case "stop":
				a.SetActive(false)
			case "reset":
				a.Reset()
			case "close":
				a.Close()
			}
			close(release)
			if <-finished == nil {
				t.Fatal("stale result accepted")
			}
			<-old.done
			a.mu.Lock()
			selected := a.selected
			a.mu.Unlock()
			if selected != nil {
				t.Fatal("old generation resurrected")
			}
			if action == "close" {
				a.SetActive(true)
				if _, _, err := a.choose(context.Background()); err == nil {
					t.Fatal("close is not terminal")
				}
			}
		})
	}
}

func TestAllFailuresBackOffWithoutDirectFallback(t *testing.T) {
	a := fixture(t, fake("one"), fake("two"))
	var calls atomic.Int32
	a.check = func(context.Context, C.Proxy, string) error { calls.Add(1); return errors.New("failed") }
	if _, _, err := a.choose(context.Background()); err == nil {
		t.Fatal("all failed but accepted")
	}
	if _, _, err := a.choose(context.Background()); err == nil || !strings.Contains(err.Error(), "waiting") {
		t.Fatal("missing backoff")
	}
	if calls.Load() != 2 {
		t.Fatal("unbounded retries")
	}
}

func TestStaleHealthyPathRetainedAndFailedPathReplaced(t *testing.T) {
	one, two := fake("one"), fake("two")
	a := fixture(t, one, two)
	var phase atomic.Int32
	var calls atomic.Int32
	a.check = func(_ context.Context, p C.Proxy, _ string) error {
		calls.Add(1)
		if (phase.Load() < 2 && p == one) || (phase.Load() == 2 && p == two) {
			return nil
		}
		return errors.New("failed")
	}
	a.mu.Lock()
	a.selected = one
	a.mu.Unlock()
	phase.Store(1)
	calls.Store(0)
	a.mu.Lock()
	a.validUntil = time.Time{}
	a.mu.Unlock()
	if choose(t, a) != one || calls.Load() != 1 {
		t.Fatal("healthy preferred path was not retained")
	}
	phase.Store(2)
	a.mu.Lock()
	a.validUntil = time.Time{}
	a.mu.Unlock()
	if choose(t, a) != two {
		t.Fatal("failed path was not replaced")
	}
}

func TestDialFailureInvalidatesAndDiagnosticsHideEndpoint(t *testing.T) {
	p := fake("one")
	a := fixture(t, p)
	a.check = func(context.Context, C.Proxy, string) error { return nil }
	p.Adapter().(*fakeAdapter).dial = func(context.Context, *C.Metadata) (C.Conn, error) { return nil, errors.New("secret") }
	_, err := a.DialContext(context.Background(), &C.Metadata{Host: "example.test", DstPort: 443})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe dial error")
	}
	a.mu.Lock()
	expired := a.validUntil.IsZero()
	a.mu.Unlock()
	if !expired {
		t.Fatal("failed dial retained healthy cache")
	}
	data, err := a.MarshalJSON()
	if err != nil || strings.Contains(string(data), "private") {
		t.Fatal("probe URL exposed")
	}
}
