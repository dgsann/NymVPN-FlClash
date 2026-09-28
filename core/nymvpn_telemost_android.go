//go:build android && cgo

package main

import "core/platform"

func protectTelemostSocket(fd int) bool {
	if platform.ShouldBlockConnection() {
		return false
	}
	handler := activeTunHandler.Load()
	if handler == nil {
		return true
	}
	return handler.handleProtect(fd) == nil
}
