package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
)

func TestTelemostOptInNetwork(t *testing.T) {
	path := os.Getenv("NYMVPN_TELEMOST_TEST_PROFILE")
	if path == "" {
		t.Skip("requires an explicitly supplied private network-test profile")
	}
	iface := os.Getenv("NYMVPN_TELEMOST_TEST_INTERFACE")
	if iface == "" {
		t.Fatal("explicit physical interface required")
	}
	previous := dialer.DefaultInterface.Load()
	dialer.DefaultInterface.Store(iface)
	defer dialer.DefaultInterface.Store(previous)
	initTelemostProtection()
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal("network-test profile failed to load")
	}
	defer closeTelemost(cfg)
	defer closeAuto(cfg)
	setTelemostActive(cfg, true)
	setAutoActive(cfg, true)
	group := cfg.Proxies["VPN"]
	if group == nil {
		t.Fatal("VPN group missing")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return nil, err
		}
		return group.DialContext(ctx, &C.Metadata{NetWork: C.TCP, Host: host, DstPort: uint16(port)})
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 65 * time.Second}
	for _, probe := range []struct {
		name, url string
		size      int64
	}{
		{"egress", "https://api.ipify.org", 0},
		{"telegram", "https://telegram.org", 0},
		{"download_5MiB", "https://speed.cloudflare.com/__down?bytes=5242880", 5242880},
		{"telegram_after", "https://telegram.org", 0},
	} {
		start := time.Now()
		response, err := client.Get(probe.url)
		if err != nil {
			t.Fatalf("probe %s failed: %v", probe.name, err)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 6*1024*1024))
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || (probe.size > 0 && int64(len(data)) != probe.size) {
			t.Fatalf("probe %s incomplete: status=%d bytes=%d error=%v", probe.name, response.StatusCode, len(data), err)
		}
		if probe.name == "egress" && strings.TrimSpace(string(data)) != os.Getenv("NYMVPN_TELEMOST_EXPECTED_EGRESS") {
			t.Fatal("unexpected egress address")
		}
		t.Logf("NYMVPN_PROBE %s bytes=%d seconds=%.3f", probe.name, len(data), time.Since(start).Seconds())
	}
}
