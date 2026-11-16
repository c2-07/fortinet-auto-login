//go:build !windows

package main

// no-op on non-windows (mac and linux services run as user)
func ensureAdmin(action string) bool {
	return false
}
