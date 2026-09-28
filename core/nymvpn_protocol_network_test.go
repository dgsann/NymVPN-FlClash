package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/miekg/dns"
)

func TestProtocolTrialsOptInNetwork(t *testing.T) {
	path := os.Getenv("NYMVPN_PROTOCOL_TEST_NODES")
	if path == "" {
		t.Skip("requires explicit private protocol trial nodes")
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("private nodes unavailable")
	}
	var nodes []map[string]any
	if json.Unmarshal(data, &nodes) != nil {
		t.Fatal("invalid private nodes")
	}
	for _, node := range nodes {
		t.Run(node["name"].(string), func(t *testing.T) {
			proxy, err := adapter.ParseProxy(node)
			if err != nil {
				t.Fatal("adapter rejected client config")
			}
			defer proxy.Close()
			transport := &http.Transport{DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
				host, portText, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				port, err := strconv.ParseUint(portText, 10, 16)
				if err != nil {
					return nil, err
				}
				return proxy.DialContext(ctx, &C.Metadata{NetWork: C.TCP, Host: host, DstPort: uint16(port)})
			}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 25 * time.Second}
			for _, probe := range []struct {
				name, url string
				size      int
			}{
				{"egress", "https://api.ipify.org", 0},
				{"download_5MiB", "https://speed.cloudflare.com/__down?bytes=5242880", 5242880},
				{"telegram_after", "https://telegram.org", 0},
			} {
				start := time.Now()
				response, err := client.Get(probe.url)
				if err != nil {
					t.Fatalf("NYMVPN_TRIAL %s request failed: %v", probe.name, err)
				}
				body, err := io.ReadAll(io.LimitReader(response.Body, 6*1024*1024))
				response.Body.Close()
				if err != nil || response.StatusCode != 200 || (probe.size > 0 && len(body) != probe.size) {
					t.Fatalf("NYMVPN_TRIAL %s incomplete status=%d bytes=%d", probe.name, response.StatusCode, len(body))
				}
				if probe.name == "egress" && strings.TrimSpace(string(body)) != "45.88.14.192" {
					t.Fatal("unexpected egress")
				}
				t.Logf("NYMVPN_TRIAL %s bytes=%d seconds=%.3f", probe.name, len(body), time.Since(start).Seconds())
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			pc, err := proxy.ListenPacketContext(ctx, &C.Metadata{NetWork: C.UDP, DstIP: netip.MustParseAddr("1.1.1.1"), DstPort: 53})
			if err != nil {
				t.Fatalf("NYMVPN_TRIAL UDP open failed: %v", err)
			}
			defer pc.Close()
			pc.SetDeadline(time.Now().Add(10 * time.Second))
			question := new(dns.Msg)
			question.SetQuestion("example.com.", dns.TypeA)
			wire, _ := question.Pack()
			if _, err = pc.WriteTo(wire, &net.UDPAddr{IP: net.ParseIP("1.1.1.1"), Port: 53}); err != nil {
				t.Fatal("NYMVPN_TRIAL UDP write failed")
			}
			buf := make([]byte, 4096)
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				t.Fatalf("NYMVPN_TRIAL UDP read failed: %v", err)
			}
			reply := new(dns.Msg)
			if reply.Unpack(buf[:n]) != nil || reply.Id != question.Id || reply.Rcode != dns.RcodeSuccess || len(reply.Answer) == 0 {
				t.Fatal("NYMVPN_TRIAL invalid DNS reply")
			}
			t.Log("NYMVPN_TRIAL UDP DNS passed")
		})
	}
}
