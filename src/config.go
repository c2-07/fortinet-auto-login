package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	cfg := loadConfig()
	path := configFilePath()

	fmt.Println("=== Campus & Engine Configuration ===")
	fmt.Printf("Config File:          %s\n", path)
	fmt.Printf("Campus Gateway Host:  %s\n", cfg.PortalHost)
	fmt.Printf("Gateway Probe Port:   1003 (HTTPS)\n")
	fmt.Printf("Daemon Interval:      %ds\n", cfg.DaemonIntervalSecs)
	fmt.Printf("Request Timeout:      %ds\n", cfg.RequestTimeoutSecs)
	fmt.Printf("Max HTTP Retries:     %d\n", cfg.MaxRetries)
	fmt.Println("\nConnectivity Check Endpoints (Primary & Fallbacks):")
	for idx, url := range cfg.CheckURLs {
		if idx == 0 {
			fmt.Printf("  [1] %s (Primary)\n", url)
		} else {
			fmt.Printf("  [%d] %s (Fallback)\n", idx+1, url)
		}
	}
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
