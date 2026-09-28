package portprofile

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func fixture(ports any) map[string]any {
	return map[string]any{
		"name": "FI-HY2", "type": "hysteria2", "server": "example.invalid",
		"port": 443, "ports": "443,8443", "password": "test-secret",
		"skip-cert-verify": false, "fingerprint": "test-pin", Field: ports,
		"obfs": map[string]any{"type": "salamander", "password": "test-obfs"},
	}
}

func TestCandidatesPreserveCredentialsAndManualSelection(t *testing.T) {
	base := fixture([]any{443, 2022, 2022, 65535})
	groups := []map[string]any{
		{"name": "VPN", "type": "select", "proxies": []any{"AUTO", "FI-HY2", "DIRECT"}},
		{"name": "AUTO", "type": "url-test", "proxies": []any{"FI-HY2"}},
	}
	proxies, expanded, err := Expand([]map[string]any{base}, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 3 || proxies[1]["port"] != 2022 || proxies[2]["port"] != 65535 {
		t.Fatalf("unexpected candidates: %d", len(proxies))
	}
	for _, node := range proxies {
		if node["password"] != base["password"] || node["fingerprint"] != base["fingerprint"] || node["skip-cert-verify"] != false {
			t.Fatal("authentication or certificate verification changed")
		}
		if _, ok := node[Field]; ok {
			t.Fatal("extension leaked into generated profile")
		}
	}
	if proxies[0]["ports"] != "443,8443" || proxies[1]["ports"] != nil {
		t.Fatal("port hopping must survive only on the original node")
	}
	want := []any{"AUTO", "FI-HY2", "FI-HY2 @2022", "FI-HY2 @65535", "DIRECT"}
	if !reflect.DeepEqual(expanded[0]["proxies"], want) || !reflect.DeepEqual(expanded[1], groups[1]) {
		t.Fatal("manual candidates or original auto group changed")
	}
	if base[Field] == nil || len(groups[0]["proxies"].([]any)) != 3 {
		t.Fatal("input was mutated")
	}
	proxies[1]["obfs"].(map[string]any)["password"] = "changed"
	if base["obfs"].(map[string]any)["password"] != "test-obfs" {
		t.Fatal("candidate shares nested credentials with the input")
	}
	again, againGroups, err := Expand(proxies, expanded)
	if err != nil || !reflect.DeepEqual(again, proxies) || !reflect.DeepEqual(againGroups, expanded) {
		t.Fatal("expansion is not idempotent")
	}
}

func TestInvalidPortsFailWithoutPrintingSecrets(t *testing.T) {
	for _, value := range []any{nil, "443,8443", []any{0}, []any{-1}, []any{65536}, []any{true}, []any{"secret-value"}, []any{443.5}, []any{math.NaN()}} {
		_, _, err := Expand([]map[string]any{fixture(value)}, nil)
		if err == nil {
			t.Fatalf("accepted invalid input of type %T", value)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("error contains input contents")
		}
	}
}

func TestAllPortNumbersAreAllowedIndividually(t *testing.T) {
	for _, value := range []any{1, 80, 443, 2022, 8448, 65535, float64(8443)} {
		_, _, err := Expand([]map[string]any{fixture([]any{value})}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRejectsCollisionsAndUnsupportedProtocols(t *testing.T) {
	base := fixture([]any{2022})
	_, _, err := Expand([]map[string]any{base, {"name": "FI-HY2 @2022"}}, nil)
	if err == nil {
		t.Fatal("accepted conflicting proxy name")
	}
	_, _, err = Expand([]map[string]any{base}, []map[string]any{{"name": "FI-HY2 @2022"}})
	if err == nil {
		t.Fatal("accepted conflicting group name")
	}
	base["type"] = "wireguard"
	if _, _, err := Expand([]map[string]any{base}, nil); err == nil {
		t.Fatal("expanded unsupported protocol")
	}
}

func TestExpansionLimits(t *testing.T) {
	ports := make([]any, 17)
	for i := range ports {
		ports[i] = 1000 + i
	}
	if _, _, err := Expand([]map[string]any{fixture(ports)}, nil); err == nil {
		t.Fatal("accepted too many ports per node")
	}
	nodes := make([]map[string]any, 9)
	for i := range nodes {
		nodes[i] = fixture(ports[:16])
		nodes[i]["name"] = fmt.Sprintf("node-%d", i)
	}
	if _, _, err := Expand(nodes, nil); err == nil {
		t.Fatal("accepted more than 128 candidates")
	}
}
