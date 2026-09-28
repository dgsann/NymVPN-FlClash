package main

import (
	"context"
	"core/pathselect"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

var autoFixture = strings.Replace(strings.Replace(telemostFixture, "proxy-groups:", `  - name: AutoTCP
    type: socks5
    server: 127.0.0.1
    port: 1
    udp: false
    x-nymvpn-auto:
      candidates: [Telemost]
      probe-url: https://example.test/probe/private
proxy-groups:`, 1), "proxies: [Telemost]", "proxies: [AutoTCP]", 1)

func TestAutoImportRoundTripAndLifecycle(t *testing.T) {
	raw, err := unmarshalProfile([]byte(autoFixture))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAuto(cfg)
	defer closeTelemost(cfg)
	if _, ok := cfg.Proxies["AutoTCP"].Adapter().(*pathselect.Adapter); !ok {
		t.Fatal("adapter not replaced")
	}
	selected := cfg.Proxies["VPN"].Adapter().Unwrap(&C.Metadata{}, false)
	if selected != cfg.Proxies["AutoTCP"] {
		t.Fatal("selector retained placeholder")
	}
	setAutoActive(cfg, true)
	resetAuto(cfg)
	setAutoActive(cfg, false)
	_, err = selected.DialContext(context.Background(), &C.Metadata{Host: "example.test", DstPort: 443})
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatal("stop not propagated")
	}
}

func TestAutoRejectsUnsafeConfiguration(t *testing.T) {
	for _, change := range [][2]string{
		{"candidates: [Telemost]", "candidates: [DIRECT]"},
		{"candidates: [Telemost]", "candidates: [VPN]"},
		{"candidates: [Telemost]", "candidates: [AutoTCP]"},
		{"candidates: [Telemost]", "candidates: [Telemost, Telemost]"},
		{"https://example.test/probe/private", "http://example.test/probe/private"},
		{"https://example.test/probe/private", "https://user:password@example.test/probe/private"},
		{"probe-url:", "unexpected:"},
		{"udp: false", "udp: true"},
		{"name: Telemost", "name: Telemost\n    dialer-proxy: AutoTCP"},
	} {
		if _, err := unmarshalProfile([]byte(strings.Replace(autoFixture, change[0], change[1], 1))); err == nil {
			t.Fatalf("unsafe config accepted: %s", change[1])
		}
	}
}
