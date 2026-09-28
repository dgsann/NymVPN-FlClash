package pathselect

import (
	"context"
	"fmt"
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

func TestProbeRequiresEntireCorrectBodyThroughCandidate(t *testing.T) {
	for _, mode := range []string{"valid", "truncated", "wrong", "oversize", "redirect", "status", "encoded", "stall", "untrusted-tls"} {
		t.Run(mode, func(t *testing.T) {
			var dialed atomic.Int32
			p := fake("candidate")
			a := p.Adapter().(*fakeAdapter)
			a.dial = func(ctx context.Context, m *C.Metadata) (C.Conn, error) {
				dialed.Add(1)
				conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", m.RemoteAddress())
				if err != nil {
					return nil, err
				}
				return outbound.NewConn(conn, a), nil
			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept-Encoding") != "identity" {
					t.Error("compression requested")
				}
				data := make([]byte, ProbeSize)
				for i := range data {
					data[i] = byte(i)
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
					w.Header().Set("Location", "/other")
					w.WriteHeader(302)
					return
				case "status":
					w.WriteHeader(503)
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
			var server *httptest.Server
			if mode == "untrusted-tls" {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			err := probe(ctx, p, server.URL+"/probe/secret")
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected probe result: %v", err)
			}
			if dialed.Load() != 1 {
				t.Fatal("candidate bypassed or redirect followed")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("credential exposed")
			}
		})
	}
}
