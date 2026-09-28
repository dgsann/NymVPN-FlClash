package telemost

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
)

const Field = "x-nymvpn-telemost"

type Config struct {
	Room string `json:"room"`
	Key  string `json:"key"`
	DNS  string `json:"dns,omitempty"`
}

func Parse(node map[string]any) (*Config, error) {
	value, exists := node[Field]
	if !exists {
		return nil, nil
	}
	if node["type"] != "socks5" || node["server"] != "127.0.0.1" ||
		(node["port"] != 1 && node["port"] != float64(1)) || node["udp"] == true ||
		node["dialer-proxy"] != nil {
		return nil, errors.New("Telemost requires a TCP-only SOCKS5 placeholder at 127.0.0.1:1 without a dialer proxy")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid Telemost options")
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		return nil, errors.New("invalid Telemost options")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{6,128}$`).MatchString(cfg.Room) {
		return nil, errors.New("invalid Telemost room identifier")
	}
	key, err := hex.DecodeString(cfg.Key)
	if err != nil || len(key) != 32 {
		return nil, errors.New("Telemost requires a 32-byte hexadecimal key")
	}
	if cfg.DNS == "" {
		cfg.DNS = "77.88.8.8:53"
	}
	host, portText, err := net.SplitHostPort(cfg.DNS)
	port, portErr := strconv.Atoi(portText)
	if err != nil || net.ParseIP(host) == nil || portErr != nil || port < 1 || port > 65535 {
		return nil, errors.New("Telemost bootstrap DNS must be an IP address with a valid port")
	}
	return &cfg, nil
}
