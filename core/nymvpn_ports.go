package main

import (
	"core/portprofile"

	"github.com/metacubex/mihomo/config"
)

func unmarshalProfile(buf []byte) (*config.RawConfig, error) {
	raw, err := config.UnmarshalRawConfig(buf)
	if err != nil {
		return nil, err
	}
	raw.Proxy, raw.ProxyGroup, err = portprofile.Expand(raw.Proxy, raw.ProxyGroup)
	if err != nil {
		return nil, err
	}
	if err := validateTelemost(raw); err != nil {
		return nil, err
	}
	if err := validateAuto(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
