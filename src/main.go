package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func printCustomUsage() {
	fmt.Println("Usage: autologin [options]")
	fmt.Println("\nAuthentication & Session:")
	fmt.Println("  autologin                    Log in to captive portal (default, prompts on first run)")
	fmt.Println("  autologin -u <user> -p <pwd> Update saved credentials and authenticate")
	fmt.Println("  autologin -clear-creds       Delete saved credentials and active session cache")
	fmt.Println("  autologin -logout            Log out of current captive portal session")
	fmt.Println("\nNetwork & Diagnostics:")
	fmt.Println("  autologin -status            Inspect network, Fortinet detection & active session")
	fmt.Println("\nBackground Service & Daemon:")
	fmt.Println("  autologin -service install   Install event-driven background service (macOS / Linux / Windows)")
	fmt.Println("  autologin -service uninstall Remove background service")
	fmt.Println("  autologin -daemon [-i 45s]   Run continuous polling daemon in foreground")
	fmt.Println("\nCloudflare WARP Circumvention:")
	fmt.Println("  autologin -warp              Intelligently rotate Anycast IPs & ports to connect Cloudflare WARP")
	fmt.Println("  autologin -warp-disconnect   Disconnect WARP and restore normal network routing")
	fmt.Println("\nLogs & Maintenance:")
	fmt.Println("  autologin -logs [-n 40]      View background service logs (default: last 40 lines)")
	fmt.Println("  autologin -logs -f           Live stream / follow logs (like tail -f)")
	fmt.Println("  autologin -logs -clear       Clear background log file")
	fmt.Println("\nConfiguration & Campus Settings:")
	fmt.Println("  autologin -config            View active configuration and probe endpoints")
	fmt.Println("  autologin -config set-host <h> Update campus gateway host (e.g. 192.168.55.253:1003)")
	fmt.Println("  autologin -config reset      Reset configuration to factory defaults")
	fmt.Println("  autologin -host <host:port>  Temporarily override campus gateway host for this run")
	fmt.Println("  autologin -upgrade           Check for and install the latest release from GitHub")
	fmt.Println("  autologin -uninstall         Completely uninstall autologin and remove all data")
	fmt.Println("  autologin -version           Print version information")
	fmt.Println("")
}

func main() {
	// Custom usage output
	flag.Usage = printCustomUsage

	// Check manual -config subcommand before flag.Parse() to support sub-arguments
	for i, arg := range os.Args[1:] {
		if arg == "-config" || arg == "--config" {
			subCmd := ""
			subVal := ""
			if i+2 < len(os.Args) && !strings.HasPrefix(os.Args[i+2], "-") {
				subCmd = os.Args[i+2]
				if i+3 < len(os.Args) && !strings.HasPrefix(os.Args[i+3], "-") {
					subVal = os.Args[i+3]
				}
			}
			handleConfigCommand(subCmd, subVal)
			return
		}
	}

	// Host override
	flag.StringVar(&runtimeHostOverride, "host", "", "temporary campus gateway host override")
	flag.StringVar(&runtimeHostOverride, "portal", "", "alias for -host")

	// Credentials
	var usernameFlag, passwordFlag string
	flag.StringVar(&usernameFlag, "username", "", "portal username")
	flag.StringVar(&usernameFlag, "u", "", "shorthand for -username")
	flag.StringVar(&passwordFlag, "password", "", "portal password")
	flag.StringVar(&passwordFlag, "p", "", "shorthand for -password")

	var clearCredsFlag bool
	flag.BoolVar(&clearCredsFlag, "clear-creds", false, "delete saved credentials and active session cache")
	flag.BoolVar(&clearCredsFlag, "reset-creds", false, "alias for -clear-creds")
	flag.BoolVar(&clearCredsFlag, "delete-creds", false, "alias for -clear-creds")

	// Session & Action flags
	var logoutFlag, keepaliveFlag bool
	flag.BoolVar(&logoutFlag, "logout", false, "log out of the captive portal")
	flag.BoolVar(&logoutFlag, "l", false, "shorthand for -logout")
	flag.BoolVar(&keepaliveFlag, "keepalive", false, "deprecated: runs daemon mode")
	flag.BoolVar(&keepaliveFlag, "k", false, "shorthand for -keepalive")

	// Diagnostics & Status
	var statusFlag, detectFlag bool
	flag.BoolVar(&statusFlag, "status", false, "inspect network, Fortinet gateway detection & active session")
	flag.BoolVar(&detectFlag, "detect", false, "alias for -status")

	// Service & Daemon
	var serviceFlag string
	flag.StringVar(&serviceFlag, "service", "", "manage background service: install | uninstall")

	var installFlag, serviceUninstallFlag, fullUninstallFlag bool
	flag.BoolVar(&installFlag, "install", false, "alias for -service install")
	flag.BoolVar(&serviceUninstallFlag, "service-uninstall", false, "remove background service")
	flag.BoolVar(&fullUninstallFlag, "uninstall", false, "completely uninstall autologin and remove all data")

	var daemonFlag bool
	flag.BoolVar(&daemonFlag, "daemon", false, "run forever, auto re-logging in whenever the session drops")
	flag.BoolVar(&daemonFlag, "d", false, "shorthand for -daemon")

	var intervalFlag time.Duration
	flag.DurationVar(&intervalFlag, "interval", 45*time.Second, "polling interval for -daemon")
	flag.DurationVar(&intervalFlag, "i", 45*time.Second, "shorthand for -interval")

	// Cloudflare WARP Bypass
	var warpFlag, warpDisconnectFlag bool
	flag.BoolVar(&warpFlag, "warp", false, "intelligent Cloudflare WARP Anycast rotation & auto-connection")
	flag.BoolVar(&warpDisconnectFlag, "warp-disconnect", false, "disconnect Cloudflare WARP cleanly")

	// Logs
	var logsFlag, followLogsFlag, clearLogsFlag bool
	flag.BoolVar(&logsFlag, "logs", false, "view background task logs")
	flag.BoolVar(&followLogsFlag, "f", false, "stream/follow background logs live (use with -logs)")
	flag.BoolVar(&followLogsFlag, "follow", false, "shorthand for -f")
	flag.BoolVar(&clearLogsFlag, "clear", false, "clear background logs (use with -logs)")
	flag.BoolVar(&clearLogsFlag, "clean", false, "shorthand for -clear")

	var linesFlag int
	flag.IntVar(&linesFlag, "n", 40, "number of log lines to show with -logs (default 40, 0 for all)")

	// Maintenance
	var upgradeFlag bool
	flag.BoolVar(&upgradeFlag, "upgrade", false, "check for and install latest release from GitHub")

	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "print version information and exit")
	flag.BoolVar(&versionFlag, "v", false, "shorthand for -version")

	// Internal flag (used exclusively by LaunchAgent/systemd)
	var autoFlag bool
	flag.BoolVar(&autoFlag, "auto", false, "internal: run once automatically for background triggers")

	flag.Parse()

	// 1. Maintenance & Version
	if versionFlag {
		fmt.Printf("fortinet-auto-login %s, commit %s, built at %s\n", version, commit, date)
		return
	}

	if upgradeFlag {
		if !upgradeCLI() {
			os.Exit(1)
		}
		return
	}

	// 1b. Full Uninstall
	if fullUninstallFlag {
		fullUninstall()
		return
	}

	// 1c. Clear Credentials
	if clearCredsFlag {
		clearCredentials()
		return
	}

	// 2. Status & Diagnostics
	if statusFlag || detectFlag {
		PrintStatus()
		return
	}

	// 3. Cloudflare WARP Circumvention
	if warpFlag {
		if !FixAndConnectWarp() {
			os.Exit(1)
		}
		return
	}
	if warpDisconnectFlag {
		if !DisconnectWarp() {
			os.Exit(1)
		}
		return
	}

	// 4. Service Management (consolidated)
	if serviceFlag != "" || installFlag || serviceUninstallFlag {
		action := strings.ToLower(serviceFlag)
		if installFlag || action == "install" {
			installAgent()
			return
		}
		if serviceUninstallFlag || action == "uninstall" || action == "remove" {
			uninstallAgent()
			return
		}
		fmt.Printf("\033[31m[FAIL]\033[0m   Unknown service command: %q. Use 'install' or 'uninstall'.\n", serviceFlag)
		os.Exit(1)
	}

	// 5. Logs Management
	if logsFlag || followLogsFlag || clearLogsFlag {
		if clearLogsFlag {
			clearLogs()
			return
		}
		viewLogs(followLogsFlag, linesFlag)
		return
	}

	// 6. Internal Background Trigger
	if autoFlag {
		runAutoMode()
		return
	}

	// 7. Credentials initialization for auth actions
	initCredentials(usernameFlag, passwordFlag)

	// 8. Logout
	if logoutFlag {
		if !logout() {
			logf("\033[31m[FAIL]\033[0m   Logout failed")
		}
		return
	}

	// 9. Daemon / Keepalive (merged)
	if keepaliveFlag {
		logf("\033[33m[NOTE]\033[0m   -keepalive has been consolidated into -daemon (monitoring & transparent re-auth)")
		daemonFlag = true
	}

	if daemonFlag {
		procLock, err := acquireProcessLock("autologin")
		if err != nil {
			logf("\033[33m[WARN]\033[0m   Another autologin process is already active. Exiting.")
			return
		}
		defer procLock.Release()

		if runtime.GOOS == "windows" {
			logPath := filepath.Join(os.TempDir(), "autologin.log")
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				logOut = f
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		daemon(ctx, intervalFlag)
		return
	}

	// 10. Standard Login (Default Action) with Process Lock
	procLock, err := acquireProcessLock("autologin")
	if err != nil {
		logf("\033[33m[WARN]\033[0m   Another autologin process is running. Waiting 1s...")
		time.Sleep(1 * time.Second)
		procLock, err = acquireProcessLock("autologin")
		if err != nil {
			logf("\033[31m[FAIL]\033[0m   Could not acquire lock: %v", err)
			return
		}
	}
	defer procLock.Release()

	success := false
	for attempt := 1; attempt <= 5; attempt++ {
		if attempt > 1 {
			logf("\033[36m[RETRY]\033[0m  %d/5", attempt)
		}
		if login(false, false) {
			success = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !success {
		logf("\033[31m[FAIL]\033[0m   Auth failed")
	}
}
