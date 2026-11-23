package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPortalHost         = "192.168.55.253:1003"
	DefaultCheckURL           = "http://cp.cloudflare.com/generate_204"
	DefaultDaemonIntervalSecs = 45
	DefaultRequestTimeoutSecs = 4
	DefaultMaxRetries         = 2
)

var runtimeHostOverride string

type appConfig struct {
	PortalHost         string   `json:"portal_host"`
	CheckURLs          []string `json:"check_urls"`
	DaemonIntervalSecs int      `json:"daemon_interval_secs"`
	RequestTimeoutSecs int      `json:"request_timeout_secs"`
	MaxRetries         int      `json:"max_retries"`
}

func defaultConfig() appConfig {
	return appConfig{
		PortalHost:         DefaultPortalHost,
		CheckURLs:          []string{DefaultCheckURL},
		DaemonIntervalSecs: DefaultDaemonIntervalSecs,
		RequestTimeoutSecs: DefaultRequestTimeoutSecs,
		MaxRetries:         DefaultMaxRetries,
	}
}

func configFilePath() string {
	return filepath.Join(getAppDir(), "config.json")
}

func loadConfig() appConfig {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)

	// Validate & ensure non-empty defaults
	if cfg.PortalHost == "" {
		cfg.PortalHost = DefaultPortalHost
	}
	if len(cfg.CheckURLs) == 0 {
		cfg.CheckURLs = []string{DefaultCheckURL}
	}
	if cfg.DaemonIntervalSecs <= 0 {
		cfg.DaemonIntervalSecs = DefaultDaemonIntervalSecs
	}
	if cfg.RequestTimeoutSecs <= 0 {
		cfg.RequestTimeoutSecs = DefaultRequestTimeoutSecs
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	return cfg
}

// effectiveConfig returns the saved configuration with environment variable
// overrides applied (FORTINET_CHECK_URL, FORTINET_INTERVAL). These overrides are
// never written back to config.json.
func effectiveConfig() appConfig {
	cfg := loadConfig()
	if v := strings.TrimSpace(os.Getenv("FORTINET_CHECK_URL")); v != "" {
		cfg.CheckURLs = []string{v}
	}
	if v := strings.TrimSpace(os.Getenv("FORTINET_INTERVAL")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.DaemonIntervalSecs = secs
		}
	}
	return cfg
}

func (c appConfig) requestTimeout() time.Duration {
	return time.Duration(c.RequestTimeoutSecs) * time.Second
}

func (c appConfig) daemonInterval() time.Duration {
	return time.Duration(c.DaemonIntervalSecs) * time.Second
}

func saveConfig(cfg appConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath(), data, 0o600)
}

func resetConfig() error {
	cfg := defaultConfig()
	return saveConfig(cfg)
}

func setPortalHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	cfg := loadConfig()
	cfg.PortalHost = host
	return saveConfig(cfg)
}

func printConfig() {
	cfg := effectiveConfig()
	path := configFilePath()

	fmt.Println("=== Campus & Engine Configuration ===")
	fmt.Printf("Config File:          %s\n", path)
	fmt.Printf("Campus Gateway Host:  %s\n", getEffectivePortalHost())
	fmt.Printf("Daemon Interval:      %ds (used when -interval is not given)\n", cfg.DaemonIntervalSecs)
	fmt.Printf("Request Timeout:      %ds\n", cfg.RequestTimeoutSecs)
	fmt.Printf("Max HTTP Retries:     %d\n", cfg.MaxRetries)

	ssids := "Any network"
	if c, _ := loadCredentials(); len(parseSSIDList(c.SSID)) > 0 {
		ssids = strings.Join(parseSSIDList(c.SSID), ", ")
	}
	fmt.Printf("Allowed Wi-Fi SSIDs:  %s (change with: autologin -ssid \"A,B\")\n", ssids)

	fmt.Println("\nConnectivity Check Endpoints (Primary & Fallbacks):")
	for idx, url := range cfg.CheckURLs {
		if idx == 0 {
			fmt.Printf("  [1] %s (Primary)\n", url)
		} else {
			fmt.Printf("  [%d] %s (Fallback)\n", idx+1, url)
		}
	}
	fmt.Println("\nEdit the config file directly to change timeouts, retries, interval or check URLs.")
	fmt.Println()
}

func handleConfigCommand(cmd, val string) {
	switch strings.ToLower(cmd) {
	case "":
		printConfig()
	case "set-host":
		if val == "" {
			fmt.Println("\033[31m[FAIL]\033[0m   Usage: autologin -config set-host <host:port>")
			os.Exit(1)
		}
		if err := setPortalHost(val); err != nil {
			fmt.Printf("\033[31m[FAIL]\033[0m   Failed to save config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\033[32m[OK]\033[0m     Updated campus gateway host to: %s\n", val)
	case "reset":
		if err := resetConfig(); err != nil {
			fmt.Printf("\033[31m[FAIL]\033[0m   Failed to reset config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\033[32m[OK]\033[0m     Configuration reset to factory defaults")
	default:
		fmt.Printf("\033[31m[FAIL]\033[0m   Unknown config command: %q. Use 'set-host <h>' or 'reset'.\n", cmd)
		os.Exit(1)
	}
}

func getEffectivePortalHost() string {
	if runtimeHostOverride != "" {
		return runtimeHostOverride
	}
	if envHost := os.Getenv("FORTINET_PORTAL_HOST"); envHost != "" {
		return envHost
	}
	return loadConfig().PortalHost
}
