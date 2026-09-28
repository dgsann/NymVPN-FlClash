package main

import (
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/loopback"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

type guardedPacket struct{ port int }

func (p guardedPacket) Data() []byte                            { return []byte("test") }
func (p guardedPacket) WriteBack([]byte, net.Addr) (int, error) { return 0, nil }
func (p guardedPacket) Drop()                                   {}
func (p guardedPacket) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p.port}
}

type recordedUDP struct {
	*outbound.Base
	selected chan string
}

func (p *recordedUDP) ListenPacketContext(context.Context, *C.Metadata) (C.PacketConn, error) {
	p.selected <- p.Name()
	return nil, loopback.ErrReject
}

func TestSubscriptionGuardStopsUDPDirectFallthrough(t *testing.T) {
	oldProxies, oldProviders := tunnel.Proxies(), tunnel.Providers()
	oldRules, oldRuleProviders := tunnel.Rules(), tunnel.RuleProviders()
	oldMode, oldStatus := tunnel.Mode(), tunnel.Status()
	defer func() {
		tunnel.UpdateProxies(oldProxies, oldProviders)
		tunnel.UpdateRules(oldRules, nil, oldRuleProviders)
		tunnel.SetMode(oldMode)
		switch oldStatus {
		case tunnel.Running:
			tunnel.OnRunning()
		case tunnel.Inner:
			tunnel.OnInnerLoading()
		default:
			tunnel.OnSuspend()
		}
	}()
	for i, tc := range []struct {
		guard, udp bool
		want       string
	}{
		{false, false, "DIRECT"}, {true, false, "REJECT"}, {true, true, "AutoTCP"},
	} {
		text := autoFixture
		if tc.guard {
			text = strings.Replace(text, "rules: ['MATCH,VPN']", "rules: ['MATCH,VPN', 'NETWORK,udp,REJECT']", 1)
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		defer closeAuto(cfg)
		defer closeTelemost(cfg)
		selected := make(chan string, 4)
		for _, name := range []string{"DIRECT", "REJECT", "AutoTCP"} {
			if name == "AutoTCP" && !tc.udp {
				continue
			}
			recorder := &recordedUDP{Base: outbound.NewBase(outbound.BaseOption{Name: name, Type: C.Socks5, UDP: true}), selected: selected}
			cfg.Proxies[name].(*adapter.Proxy).ProxyAdapter = recorder
		}
		tunnel.UpdateProxies(cfg.Proxies, cfg.Providers)
		tunnel.UpdateRules(cfg.Rules, cfg.SubRules, cfg.RuleProviders)
		tunnel.SetMode(tunnel.Rule)
		tunnel.OnRunning()
		port := 38000 + i
		tunnel.Tunnel.HandleUDPPacket(guardedPacket{port: port}, &C.Metadata{
			NetWork: C.UDP, Type: C.INNER, SrcIP: netip.MustParseAddr("127.0.0.1"), SrcPort: uint16(port),
			DstIP: netip.MustParseAddr("198.51.100.1"), DstPort: 9999,
		})
		select {
		case got := <-selected:
			if got != tc.want {
				t.Fatalf("guard=%v udp=%v routed to %s, want %s", tc.guard, tc.udp, got, tc.want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("router did not dispatch UDP")
		}
	}
}
