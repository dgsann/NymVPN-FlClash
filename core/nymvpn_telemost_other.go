//go:build !windows && !(android && cgo)

package main

// The first canary targets Android; other platforms must not run an unprotected tunnel.
func protectTelemostSocket(fd int) bool { return false }
