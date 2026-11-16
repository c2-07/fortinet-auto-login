//go:build !windows

package main

// ensureAdmin is a no-op on non-Windows platforms.
// macOS LaunchAgents and Linux systemd --user services do not require
// elevated privileges, so no elevation prompt is needed.
func ensureAdmin(action string) bool {
	return false
}
