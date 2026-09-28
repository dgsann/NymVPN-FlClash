package telemost

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/client"
)

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for transport")
	}
}

func TestTransportIsLazyAndSharedAndStops(t *testing.T) {
	a := New("test", Config{})
	t.Cleanup(func() { _ = a.Close() })
	var calls atomic.Int32
	started := make(chan struct{})
	stopped := make(chan struct{})
	release := make(chan struct{})
	a.run = func(ctx context.Context, cfg client.Config, ready func(string)) error {
		calls.Add(1)
		if cfg.LocalAddr != "127.0.0.1:0" || cfg.SOCKSUser == "" || len(cfg.SOCKSPass) != 64 {
			t.Error("local listener must use ephemeral loopback port and random authentication")
		}
		close(started)
		<-ctx.Done()
		close(stopped)
		<-release
		return ctx.Err()
	}
	if _, err := a.session(); err == nil {
		t.Fatal("inactive adapter started")
	}
	a.SetActive(true)
	if calls.Load() != 0 {
		t.Fatal("activation must not connect until selected")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, err := a.DialContext(ctx, &C.Metadata{Host: "example.com", DstPort: 443})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("caller deadline: %v", err)
			}
		}()
	}
	wg.Wait()
	await(t, started)
	if calls.Load() != 1 {
		t.Fatal("concurrent requests opened more than one transport")
	}
	g, err := a.session()
	if err != nil || g.ctx.Err() != nil {
		t.Fatal("caller timeout canceled the shared transport")
	}
	a.SetActive(false)
	await(t, stopped)
	if _, err := a.session(); err == nil {
		t.Fatal("stop allowed reconnection")
	}
	a.SetActive(true)
	if _, err := a.session(); err == nil {
		t.Fatal("restart overlapped previous transport cleanup")
	}
	close(release)
	await(t, g.done)
	if _, err := a.session(); err == nil {
		t.Fatal("failed transport skipped reconnect backoff")
	}
	_ = a.Close()
	a.SetActive(true)
	if _, err := a.session(); err == nil {
		t.Fatal("terminal close was undone")
	}
	if a.SupportUDP() {
		t.Fatal("TCP transport advertises UDP")
	}
}

func TestFailureAndPanicReleaseWaiters(t *testing.T) {
	for _, panics := range []bool{false, true} {
		a := New("test", Config{})
		a.run = func(context.Context, client.Config, func(string)) error {
			if panics {
				panic("private upstream details")
			}
			return errors.New("private upstream details")
		}
		a.SetActive(true)
		g, err := a.session()
		if err != nil {
			t.Fatal(err)
		}
		await(t, g.done)
		if g.ctx.Err() == nil {
			t.Fatal("failed generation did not cancel waiting requests")
		}
		if _, err := a.session(); err == nil {
			t.Fatal("failure retries without backoff")
		}
		_ = a.Close()
	}
}

func TestInvalidListenerAddressCancelsGeneration(t *testing.T) {
	a := New("test", Config{})
	a.run = func(ctx context.Context, _ client.Config, ready func(string)) error {
		ready("0.0.0.0:1080")
		<-ctx.Done()
		return ctx.Err()
	}
	a.SetActive(true)
	g, err := a.session()
	if err != nil {
		t.Fatal(err)
	}
	await(t, g.done)
	select {
	case <-g.ready:
		t.Fatal("non-loopback listener accepted")
	default:
	}
	_ = a.Close()
}
