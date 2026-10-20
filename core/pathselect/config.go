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
	Version    int      `json:"version,omitempty"`
	UDP        bool     `json:"udp,omitempty"`
	Candidates []string `json:"candidates"`
	ProbeURL   string   `json:"probe-url"`
}

func Parse(node map[string]any) (*Config, error) {
	value, exists := node[Field]
	if !exists {
		return nil, nil
	}
	if node["type"] != "socks5" || node["server"] != "127.0.0.1" ||
		(node["port"] != 1 && node["port"] != float64(1)) ||
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
	limit := 4
	if cfg.Version == 2 {
		limit = 12
	} else if cfg.Version != 0 || cfg.UDP {
		return nil, errors.New("unsupported automatic route version")
	}
	if node["udp"] != cfg.UDP {
		return nil, errors.New("automatic route UDP capability mismatch")
	}
	if len(cfg.Candidates) < 1 || len(cfg.Candidates) > limit {
		return nil, errors.New("invalid automatic route candidate count")
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
	if cfg.Version == 2 && !strings.HasPrefix(u.Path, "/probe/v2/") {
		return nil, errors.New("verified routing requires a v2 transfer probe")
	}
	return &cfg, nil
}
