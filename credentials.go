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
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
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

func saveCredentials(c credentials) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(credentialsFilePath(), data, 0o600)
	os.Remove(credentialsFilePath() + ".notified")
}

func deleteCredentials() {
	_ = os.Remove(credentialsFilePath())
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
	pass := strings.TrimSpace(string(passBytes))

	c := credentials{Username: user, Password: pass}
	saveCredentials(c)
	return c
}

func initCredentials(usernameFlag, passwordFlag string) {
	if usernameFlag != "" && passwordFlag != "" {
		username = usernameFlag
		password = passwordFlag
		saveCredentials(credentials{Username: username, Password: password})
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
