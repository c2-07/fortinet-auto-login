package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// Set from CLI flags or credential cache/prompts
var username, password string

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	SSID     string `json:"ssid"`
}

func getAppDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
	}
	dir := filepath.Join(home, ".fortinet-autologin")
	os.MkdirAll(dir, 0700)
	return dir
}

func credentialsFilePath() string {
	return filepath.Join(getAppDir(), "credentials.json")
}

func loadCredentials() (credentials, bool) {
	var c credentials
	data, err := os.ReadFile(credentialsFilePath())
	if err != nil {
		return c, false
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, false
	}
	return c, c.Username != "" && c.Password != ""
}

func saveCredentials(c credentials) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := os.WriteFile(credentialsFilePath(), data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	_ = os.Remove(credentialsFilePath() + ".notified")
	return nil
}

func deleteCredentials() {
	_ = os.Remove(credentialsFilePath())
}

func clearCredentials() {
	deleteCredentials()
	clearSession()
	fmt.Println("\033[32m[OK]\033[0m     Saved credentials and active session cache deleted.")
	fmt.Println("\033[36m[INFO]\033[0m   You will be prompted for credentials on your next login.")
}

func loadExistingSSID() string {
	var c credentials
	data, err := os.ReadFile(credentialsFilePath())
	if err != nil {
		return ""
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return ""
	}
	return c.SSID
}

func parseSSIDList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func ssidMatches(allowedRaw, current string) bool {
	allowed := parseSSIDList(allowedRaw)
	if len(allowed) == 0 {
		return true
	}
	cur := strings.TrimSpace(current)
	for _, s := range allowed {
		if strings.EqualFold(cur, s) {
			return true
		}
	}
	return false
}

func setAllowedSSIDs(raw string) error {
	var c credentials
	if data, err := os.ReadFile(credentialsFilePath()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	normalized := strings.Join(parseSSIDList(raw), ", ")
	c.SSID = normalized
	return saveCredentials(c)
}

func promptCredentials() credentials {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println()
		logf("\033[31m[ERR]\033[0m    Password read failed: %v", err)
		os.Exit(1)
	}
	fmt.Println()
	pass := strings.TrimRight(string(passBytes), "\r\n")

	c := credentials{
		Username: user,
		Password: pass,
		SSID:     loadExistingSSID(),
	}
	_ = saveCredentials(c)
	return c
}

func initCredentials(usernameFlag, passwordFlag string) {
	if usernameFlag != "" && passwordFlag != "" {
		username = usernameFlag
		password = passwordFlag
		_ = saveCredentials(credentials{
			Username: username,
			Password: password,
			SSID:     loadExistingSSID(),
		})
		return
	}

	if usernameFlag != "" || passwordFlag != "" {
		fmt.Println("\033[31m[ERR]\033[0m    -username and -password require each other")
		os.Exit(1)
	}

	c, ok := loadCredentials()
	if !ok {
		c = promptCredentials()
	}
	username = c.Username
	password = c.Password
}
