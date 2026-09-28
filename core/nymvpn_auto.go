package main

import (
	"core/pathselect"
	"errors"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
)

func validateAuto(raw *config.RawConfig) error {
	nodes := map[string]map[string]any{}
	for _, node := range raw.Proxy {
		name, _ := node["name"].(string)
		nodes[name] = node
	}
	count := 0
	for _, node := range raw.Proxy {
		options, err := pathselect.Parse(node)
		if err != nil {
			return err
		}
		if options == nil {
			continue
		}
		count++
		for _, name := range options.Candidates {
			child := nodes[name]
			if child == nil || child[pathselect.Field] != nil || child["dialer-proxy"] != nil {
				return errors.New("AutoTCP candidates must be unchained inline transports")
			}
			switch child["type"] {
			case "vless", "hysteria2", "tuic":
			case "socks5":
				if child["x-nymvpn-telemost"] == nil {
					return errors.New("AutoTCP SOCKS5 candidate must be Telemost")
				}
			default:
				return errors.New("unsupported AutoTCP candidate transport")
			}
		}
	}
	if count > 1 {
		return errors.New("only one AutoTCP selector per profile is supported")
	}
	return nil
}

func attachAuto(raw *config.RawConfig, cfg *config.Config) error {
	for _, node := range raw.Proxy {
		options, err := pathselect.Parse(node)
		if err != nil {
			return err
		}
		if options == nil {
			continue
		}
		name, _ := node["name"].(string)
		wrapped, ok := cfg.Proxies[name].(*adapter.Proxy)
		if !ok {
			return errors.New("AutoTCP placeholder is missing")
		}
		candidates := make([]C.Proxy, 0, len(options.Candidates))
		for _, name := range options.Candidates {
			p := cfg.Proxies[name]
			if p == nil {
				return errors.New("AutoTCP candidate is missing")
			}
			candidates = append(candidates, p)
		}
		wrapped.ProxyAdapter = pathselect.New(name, *options, candidates)
	}
	return nil
}

func eachAuto(cfg *config.Config, fn func(*pathselect.Adapter)) {
	if cfg == nil {
		return
	}
	for _, proxy := range cfg.Proxies {
		if a, ok := proxy.Adapter().(*pathselect.Adapter); ok {
			fn(a)
		}
	}
}

func setAutoActive(cfg *config.Config, active bool) {
	eachAuto(cfg, func(a *pathselect.Adapter) { a.SetActive(active) })
}

func closeAuto(cfg *config.Config) {
	eachAuto(cfg, func(a *pathselect.Adapter) { _ = a.Close() })
}

func resetAuto(cfg *config.Config) {
	eachAuto(cfg, func(a *pathselect.Adapter) { a.Reset() })
}
