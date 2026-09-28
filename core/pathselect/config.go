package pathselect

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

const Field = "x-nymvpn-auto"

type Config struct {
	Candidates []string `json:"candidates"`
	ProbeURL   string   `json:"probe-url"`
}

func Parse(node map[string]any) (*Config, error) {
	value, exists := node[Field]
	if !exists {
		return nil, nil
	}
	if node["type"] != "socks5" || node["server"] != "127.0.0.1" ||
		(node["port"] != 1 && node["port"] != float64(1)) || node["udp"] != false ||
		node["dialer-proxy"] != nil || node["x-nymvpn-telemost"] != nil {
		return nil, errors.New("AutoTCP requires an exclusive TCP-only loopback placeholder")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid AutoTCP options")
	}
	var cfg Config
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil {
		return nil, errors.New("invalid AutoTCP options")
	}
	if len(cfg.Candidates) < 1 || len(cfg.Candidates) > 4 {
		return nil, errors.New("AutoTCP requires 1 to 4 candidates")
	}
	seen := map[string]bool{}
	for _, name := range cfg.Candidates {
		if name == "" || name == node["name"] || seen[name] {
			return nil, errors.New("invalid AutoTCP candidates")
		}
		seen[name] = true
	}
	u, err := url.Parse(cfg.ProbeURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || !strings.HasPrefix(u.Path, "/probe/") {
		return nil, errors.New("AutoTCP requires an authenticated HTTPS probe endpoint")
	}
	return &cfg, nil
}
