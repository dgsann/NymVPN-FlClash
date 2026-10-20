package pathselect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
)

func TestTransferProbeRequiresBothCompleteDirections(t *testing.T) {
	for _, mode := range []string{"valid", "truncated", "wrong", "oversize", "redirect", "encoded", "stall", "untrusted-tls"} {
		t.Run(mode, func(t *testing.T) {
			p := fake("candidate")
			a := p.Adapter().(*fakeAdapter)
			a.dial = func(ctx context.Context, m *C.Metadata) (C.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", m.RemoteAddress())
				if err != nil {
					return nil, err
				}
				return outbound.NewConn(conn, a), nil
			}
			var upload atomic.Int64
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				upload.Store(int64(len(data)))
				if err != nil || len(data) != ProbeSize || r.Method != http.MethodPost {
					t.Error("upload was not complete")
					return
				}
				switch mode {
				case "truncated":
					w.Header().Set("Content-Length", fmt.Sprint(ProbeSize))
					data = data[:11000]
				case "wrong":
					data[15000]++
				case "oversize":
					data = append(data, 1)
				case "redirect":
					w.Header().Set("Location", "/elsewhere")
					w.WriteHeader(302)
					return
				case "encoded":
					w.Header().Set("Content-Encoding", "gzip")
				case "stall":
					w.Write(data[:11000])
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				w.Write(data)
			})
			server := httptest.NewUnstartedServer(handler)
			if mode == "untrusted-tls" {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			err := probeTransfer(ctx, p, server.URL+"/probe/v2/private")
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected result: %v", err)
			}
			if mode != "untrusted-tls" && upload.Load() != ProbeSize {
				t.Fatal("upload bypassed")
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("endpoint exposed")
			}
		})
	}
}

type countedConn struct {
	C.Conn
	closed atomic.Bool
}

func (c *countedConn) Close() error { c.closed.Store(true); return c.Conn.Close() }

func TestBackgroundRecheckRetiresOnlyFailedPathSessions(t *testing.T) {
	one, two := fake("one"), fake("two")
	a := New("verified", Config{Version: 2}, []C.Proxy{one, two})
	a.SetActive(true)
	defer a.Close()
	var phase atomic.Int32
	a.check = func(_ context.Context, p C.Proxy, _ string) error {
		if phase.Load() == 0 && p == one || phase.Load() == 1 && p == two {
			return nil
		}
		return errors.New("blocked")
	}
	var raw *countedConn
	one.Adapter().(*fakeAdapter).dial = func(context.Context, *C.Metadata) (C.Conn, error) {
		left, right := net.Pipe()
		t.Cleanup(func() { right.Close() })
		raw = &countedConn{Conn: outbound.NewConn(left, one.Adapter().(*fakeAdapter))}
		return raw, nil
	}
	conn, err := a.DialContext(context.Background(), &C.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	a.mu.Lock()
	life := a.life
	a.validUntil = time.Time{}
	a.mu.Unlock()
	a.recheck(life)
	if raw.closed.Load() {
		t.Fatal("healthy session was interrupted")
	}
	phase.Store(1)
	a.mu.Lock()
	a.validUntil = time.Time{}
	a.mu.Unlock()
	a.recheck(life)
	deadline := time.Now().Add(time.Second)
	for !raw.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !raw.closed.Load() || choose(t, a) != two {
		t.Fatal("failed path was not replaced")
	}
}

func TestUDPChoiceIsIndependentAndFailsClosed(t *testing.T) {
	tcp, udp := fake("tcp"), fake("udp")
	udp.Adapter().(*fakeAdapter).Base = outbound.NewBase(outbound.BaseOption{Name: "udp", Type: C.Hysteria2, UDP: true})
	a := New("verified", Config{Version: 2, UDP: true}, []C.Proxy{tcp, udp})
	a.check = func(_ context.Context, p C.Proxy, _ string) error {
		if p == tcp {
			return nil
		}
		return errors.New("blocked TCP")
	}
	a.udpRoute.check = func(_ context.Context, p C.Proxy, _ string) error {
		if p == udp {
			return nil
		}
		return errors.New("blocked UDP")
	}
	a.SetActive(true)
	defer a.Close()
	if !a.SupportUDP() || choose(t, a) != tcp || choose(t, a.udpRoute) != udp {
		t.Fatal("TCP and UDP choices were coupled")
	}
	a.Reset()
	a.udpRoute.check = func(context.Context, C.Proxy, string) error { return errors.New("blocked") }
	if _, _, err := a.udpRoute.choose(context.Background()); err == nil {
		t.Fatal("unverified UDP route selected")
	}
	if choose(t, a) != tcp {
		t.Fatal("UDP failure broke TCP")
	}
}

func TestVerifiedCandidateConcurrencyIsBounded(t *testing.T) {
	var candidates []C.Proxy
	for i := range 12 {
		candidates = append(candidates, fake(fmt.Sprint(i)))
	}
	a := New("verified", Config{Version: 2}, candidates)
	var running, peak atomic.Int32
	a.check = func(ctx context.Context, _ C.Proxy, _ string) error {
		n := running.Add(1)
		defer running.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			return errors.New("blocked")
		}
	}
	a.SetActive(true)
	defer a.Close()
	if _, _, err := a.choose(context.Background()); err == nil {
		t.Fatal("failed probes accepted")
	}
	if peak.Load() > 4 {
		t.Fatal("probe fanout exceeded budget")
	}
}

func TestVersionTwoConfigAndLegacyCompatibility(t *testing.T) {
	node := map[string]any{"name": "auto", "type": "socks5", "server": "127.0.0.1", "port": 1, "udp": true,
		Field: map[string]any{"version": 2, "udp": true, "candidates": []string{"one"}, "probe-url": "https://example.test/probe/v2/1/token"}}
	cfg, err := Parse(node)
	if err != nil || !cfg.UDP || cfg.Version != 2 {
		t.Fatalf("valid v2 refused: %v", err)
	}
	node["udp"] = false
	if _, err := Parse(node); err == nil {
		t.Fatal("capability mismatch accepted")
	}
}
