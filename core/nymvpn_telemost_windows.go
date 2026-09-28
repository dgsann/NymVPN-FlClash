//go:build windows

package main

import (
	"math/bits"
	"net"
	"syscall"

	"github.com/metacubex/mihomo/component/dialer"
)

func protectTelemostSocket(fd int) bool {
	name := dialer.DefaultInterface.Load()
	if name == "" {
		return true
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	socket := syscall.Handle(fd)
	if syscall.SetsockoptInt(socket, syscall.IPPROTO_IP, 31, int(bits.ReverseBytes32(uint32(iface.Index)))) == nil {
		return true
	}
	return syscall.SetsockoptInt(socket, syscall.IPPROTO_IPV6, 31, iface.Index) == nil
}
