package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNymVPNPortsSurviveProfileImportAndJSONRoundTrip(t *testing.T) {
	input := []byte(`proxies:
  - name: FI-Reality
    type: vless
    server: example.invalid
    port: 443
    uuid: 00000000-0000-4000-8000-000000000001
    x-nymvpn-ports: [2022, 65535]
proxy-groups:
  - name: VPN
    type: select
    proxies: [FI-Reality]
rules: ['MATCH,VPN']
`)
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	if result := handleValidateConfig(path); result != "" {
		t.Fatal(result)
	}
	raw, err := handleGetConfig(path)
	if err != nil || len(raw.Proxy) != 3 {
		t.Fatalf("import failed: %v", err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := handleGetConfig(path)
	if err != nil || len(imported.Proxy) != 3 {
		t.Fatalf("JSON round trip failed: %v", err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("expanded profile failed to load: %v", err)
	}
	if cfg.Proxies["FI-Reality @2022"] == nil || cfg.Proxies["FI-Reality @65535"] == nil {
		t.Fatal("port candidates missing from parsed core config")
	}
}

func TestNymVPNInvalidPortsAreRejectedDuringValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte("proxies:\n  - {name: FI, type: vless, port: 443, x-nymvpn-ports: [0]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := handleValidateConfig(path); result == "" {
		t.Fatal("validation accepted invalid port")
	}
}
