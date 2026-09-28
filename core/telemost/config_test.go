package telemost

import (
	"strings"
	"testing"
)

func validNode() map[string]any {
	return map[string]any{"name": "test", "type": "socks5", "server": "127.0.0.1", "port": 1, "udp": false,
		Field: map[string]any{"room": "test-room", "key": strings.Repeat("ab", 32)}}
}

func TestParseStrictOptions(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"remote placeholder": func(n map[string]any) { n["server"] = "example.com" },
		"UDP":                func(n map[string]any) { n["udp"] = true },
		"proxy chain":        func(n map[string]any) { n["dialer-proxy"] = "other" },
		"wrong port":         func(n map[string]any) { n["port"] = 1080 },
		"URL room":           func(n map[string]any) { n[Field].(map[string]any)["room"] = "https://example.com/private" },
		"short key":          func(n map[string]any) { n[Field].(map[string]any)["key"] = "private-key" },
		"DNS hostname":       func(n map[string]any) { n[Field].(map[string]any)["dns"] = "example.com:53" },
		"DNS port":           func(n map[string]any) { n[Field].(map[string]any)["dns"] = "127.0.0.1:65536" },
		"unknown option":     func(n map[string]any) { n[Field].(map[string]any)["private-option"] = true },
		"null options":       func(n map[string]any) { n[Field] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			node := validNode()
			mutate(node)
			_, err := Parse(node)
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("error exposes supplied options")
			}
		})
	}
	cfg, err := Parse(validNode())
	if err != nil || cfg.DNS != "77.88.8.8:53" {
		t.Fatalf("valid config: %v", err)
	}
	if cfg, err := Parse(map[string]any{"type": "direct"}); err != nil || cfg != nil {
		t.Fatal("ordinary node changed")
	}
}
