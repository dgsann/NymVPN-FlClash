package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
)

func TestGoogleOptInNetwork(t *testing.T) {
	if os.Getenv("NYMVPN_GOOGLE_NETWORK_TEST") != "1" {
		t.Skip("requires explicit Google network test opt-in")
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
	for _, url := range []string{
		"https://www.google.com/", "https://accounts.google.com/ServiceLogin",
		"https://mail.google.com/", "https://play.google.com/store",
		"https://www.youtube.com/", "https://www.gstatic.com/generate_204",
		"https://www.googleapis.com/discovery/v1/apis?name=drive",
	} {
		start := time.Now()
		response, err := client.Get(url)
		if err != nil {
			t.Errorf("NYMVPN_GOOGLE request failed: %s", url)
			continue
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
		response.Body.Close()
		if err != nil || len(data) > 4*1024*1024 || (response.StatusCode != 200 && response.StatusCode != 204) {
			t.Errorf("NYMVPN_GOOGLE incomplete: %s status=%d bytes=%d", url, response.StatusCode, len(data))
			continue
		}
		t.Logf("NYMVPN_GOOGLE %s status=%d bytes=%d seconds=%.3f", url, response.StatusCode, len(data), time.Since(start).Seconds())
	}
}
