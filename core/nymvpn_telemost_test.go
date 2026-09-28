package main

import (
	"context"
	"core/telemost"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

const telemostFixture = `proxies:
  - name: Telemost
    type: socks5
    server: 127.0.0.1
    port: 1
    udp: false
    x-nymvpn-telemost:
      room: test-room
      key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy-groups:
  - name: VPN
    type: select
    proxies: [Telemost]
rules: ['MATCH,VPN']
`

func TestTelemostImportRoundTripAndGroupReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(telemostFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if result := handleValidateConfig(path); result != "" {
		t.Fatal(result)
	}
	raw, err := handleGetConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTelemost(cfg) })
	if _, ok := cfg.Proxies["Telemost"].Adapter().(*telemost.Adapter); !ok {
		t.Fatal("custom adapter missing after JSON roundtrip")
	}
	selected := cfg.Proxies["VPN"].Adapter().Unwrap(&C.Metadata{}, false)
	if selected != cfg.Proxies["Telemost"] {
		t.Fatal("group retained old placeholder")
	}
	_, err = cfg.Proxies["VPN"].DialContext(context.Background(), &C.Metadata{Host: "example.com", DstPort: 443})
	if err == nil || !strings.Contains(err.Error(), "Telemost transport is stopped") {
		t.Fatalf("group bypassed inactive transport: %v", err)
	}
	setTelemostActive(cfg, true)
	setTelemostActive(cfg, false)
	_, err = selected.DialContext(context.Background(), &C.Metadata{Host: "example.com", DstPort: 443})
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("service stop failed: %v", err)
	}
}

func TestTelemostRejectsMultipleSessionsPerProfile(t *testing.T) {
	raw, err := unmarshalProfile([]byte(telemostFixture))
	if err != nil {
		t.Fatal(err)
	}
	raw.Proxy = append(raw.Proxy, raw.Proxy[0])
	if validateTelemost(raw) == nil {
		t.Fatal("multiple Telemost sessions accepted")
	}
}
