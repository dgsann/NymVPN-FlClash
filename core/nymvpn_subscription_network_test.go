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

func TestSubscriptionOptInNetwork(t *testing.T) {
	urlFile := os.Getenv("NYMVPN_SUBSCRIPTION_URL_FILE")
	if urlFile == "" {
		t.Skip("requires private subscription URL file")
	}
	urlBytes, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatal("subscription URL unavailable")
	}
	iface := os.Getenv("NYMVPN_TELEMOST_TEST_INTERFACE")
	if iface == "" {
		t.Fatal("physical interface required")
	}
	previous := dialer.DefaultInterface.Load()
	dialer.DefaultInterface.Store(iface)
	defer dialer.DefaultInterface.Store(previous)
	initTelemostProtection()
	cfg, err := loadConfig(os.Getenv("NYMVPN_TELEMOST_TEST_PROFILE"))
	if err != nil {
		t.Fatal("private profile failed to load")
	}
	defer closeTelemost(cfg)
	defer closeAuto(cfg)
	setTelemostActive(cfg, true)
	proxy := cfg.Proxies["NymVPN-Telemost"]
	if proxy == nil {
		t.Fatal("Telemost missing")
	}
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
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	start := time.Now()
	response, err := client.Get(strings.TrimSpace(string(urlBytes)))
	if err != nil {
		t.Fatal("subscription transfer failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil || response.StatusCode != 200 {
		t.Fatal("subscription response incomplete")
	}
	raw, err := unmarshalProfile(data)
	if err != nil {
		t.Fatal("subscription validation failed")
	}
	t.Logf("NYMVPN_SUBSCRIPTION bytes=%d nodes=%d seconds=%.3f", len(data), len(raw.Proxy), time.Since(start).Seconds())
}
