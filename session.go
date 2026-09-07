package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Magic is the token issued per session in the portal redirect — the same
// value is reused for login, logout, and keepalive. It changes each new
// session, so it's captured at login time and stashed here for later
// -logout / -keepalive calls rather than hardcoded.
type session struct {
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Magic     string `json:"magic"`
	Countdown int    `json:"countdown"` // seconds, from the keepalive page
}

var countdownRe = regexp.MustCompile(`id="countdown">(\d+)<`)

const defaultCountdown = 14400 // seconds; fallback if the page doesn't have one

// extractCountdown pulls the refresh interval out of the keepalive page's
// <b id="countdown">N</b>, falling back to the default if not found.
func extractCountdown(body string) int {
	if m := countdownRe.FindStringSubmatch(body); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			return n
		}
	}
	return defaultCountdown
}

func sessionFilePath() string {
	return filepath.Join(getAppDir(), "session.json")
}

func saveSession(s session) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(sessionFilePath(), data, 0o600)
}

func loadSession() (session, bool) {
	var s session
	data, err := os.ReadFile(sessionFilePath())
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false
	}
	return s, s.Host != "" && s.Magic != ""
}

// randomMagic generates a random hex string shaped like the real magic
// tokens (16 hex chars), for use when we have no saved session to logout
// with.
func randomMagic() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// Extremely unlikely, but fall back to something well-formed anyway.
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}
