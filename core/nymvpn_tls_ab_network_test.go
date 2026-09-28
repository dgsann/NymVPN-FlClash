package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/transport/vmess"
)

type abCountConn struct {
	net.Conn
	readBytes, writeBytes atomic.Int64
}

func (c *abCountConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.readBytes.Add(int64(n))
	return n, err
}

func (c *abCountConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.writeBytes.Add(int64(n))
	return n, err
}

func TestTLSABOptInNetwork(t *testing.T) {
	mode := os.Getenv("NYMVPN_TLS_AB_MODE")
	if mode != "https" && mode != "tls-only" {
		t.Skip("requires explicit HTTPS/TLS diagnostic opt-in")
	}
	data, err := os.ReadFile(os.Getenv("NYMVPN_PROTOCOL_TEST_NODES"))
	if err != nil {
		t.Fatal("private node file unavailable")
	}
	var nodes []map[string]any
	if json.Unmarshal(data, &nodes) != nil || len(nodes) == 0 {
		t.Fatal("invalid node file")
	}
	node := nodes[0]
	if node["type"] != "anytls" {
		t.Fatal("AnyTLS node required")
	}
	iface := os.Getenv("NYMVPN_PROTOCOL_TEST_INTERFACE")
	if iface == "" {
		t.Fatal("explicit interface required")
	}
	previous := dialer.DefaultInterface.Load()
	if iface == "loopback-control" {
		dialer.DefaultInterface.Store("")
	} else {
		dialer.DefaultInterface.Store(iface)
	}
	defer dialer.DefaultInterface.Store(previous)
	for _, fingerprint := range []string{"chrome", "chrome120"} {
		t.Run(fingerprint, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			address := fmt.Sprintf("%s:%d", node["server"], int(node["port"].(float64)))
			conn, err := dialer.DialContext(ctx, "tcp", address)
			if err != nil {
				t.Fatalf("NYMVPN_AB TCP connect failed: %v", err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(12 * time.Second))
			counted := &abCountConn{Conn: conn}
			defer func() {
				t.Logf("NYMVPN_AB mode=%s hello=%s socket_written=%d socket_read=%d", mode, fingerprint, counted.writeBytes.Load(), counted.readBytes.Load())
			}()
			tlsConn, err := vmess.StreamTLSConn(ctx, counted, &vmess.TLSConfig{Host: node["sni"].(string), FingerPrint: node["fingerprint"].(string), ClientFingerprint: fingerprint})
			if err != nil {
				t.Fatalf("NYMVPN_AB TLS handshake failed: %v", err)
			}
			t.Log("NYMVPN_AB TLS handshake passed with certificate pin")
			if mode == "tls-only" {
				return
			}
			if _, err = fmt.Fprintf(tlsConn, "GET /probe HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", node["sni"]); err != nil {
				t.Fatal("NYMVPN_AB HTTP write failed")
			}
			response, err := http.ReadResponse(bufio.NewReader(tlsConn), nil)
			if err != nil {
				t.Fatalf("NYMVPN_AB HTTP headers failed: %v", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
			want := sha256.Sum256(bytes.Repeat([]byte("NymVPN-HTTPS-AB\n"), 65536))
			if err != nil || response.StatusCode != 200 || len(body) != 1048576 || sha256.Sum256(body) != want {
				t.Fatalf("NYMVPN_AB HTTP incomplete status=%d bytes=%d err=%v", response.StatusCode, len(body), err)
			}
			t.Logf("NYMVPN_AB HTTPS full body hash passed bytes=%d", len(body))
		})
	}
}
