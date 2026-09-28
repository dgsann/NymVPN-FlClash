package main

import (
	"core/telemost"
	"errors"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/config"
	"github.com/openlibrecommunity/olcrtc/mobile"
)

type telemostProtector struct{}

func (telemostProtector) Protect(fd int) bool { return protectTelemostSocket(fd) }

func initTelemostProtection() {
	(&mobile.Runtime{}).SetProtector(telemostProtector{})
}

func validateTelemost(raw *config.RawConfig) error {
	count := 0
	for _, node := range raw.Proxy {
		cfg, err := telemost.Parse(node)
		if err != nil {
			return err
		}
		if cfg != nil {
			count++
		}
	}
	if count > 1 {
		return errors.New("only one Telemost tunnel per profile is supported")
	}
	return nil
}

func attachTelemost(raw *config.RawConfig, cfg *config.Config) error {
	for _, node := range raw.Proxy {
		options, err := telemost.Parse(node)
		if err != nil {
			return err
		}
		if options == nil {
			continue
		}
		name, _ := node["name"].(string)
		wrapped, ok := cfg.Proxies[name].(*adapter.Proxy)
		if !ok {
			return errors.New("Telemost proxy is missing from parsed profile")
		}
		wrapped.ProxyAdapter = telemost.New(name, *options)
	}
	return nil
}

func setTelemostActive(cfg *config.Config, active bool) {
	if cfg == nil {
		return
	}
	for _, proxy := range cfg.Proxies {
		if transport, ok := proxy.Adapter().(*telemost.Adapter); ok {
			transport.SetActive(active)
		}
	}
}

func closeTelemost(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, proxy := range cfg.Proxies {
		if transport, ok := proxy.Adapter().(*telemost.Adapter); ok {
			_ = transport.Close()
		}
	}
}
