package anet

import (
	"net"

	mihomo "github.com/metacubex/mihomo/component/iface/anet"
)

// Pion's Android fallback must not access private Go net.zoneCache symbols.
func Interfaces() ([]net.Interface, error) {
	return mihomo.Interfaces()
}

func InterfaceAddrs() ([]net.Addr, error) {
	return mihomo.InterfaceAddrs()
}

func InterfaceAddrsByInterface(iface *net.Interface) ([]net.Addr, error) {
	return mihomo.InterfaceAddrsByInterface(iface)
}
