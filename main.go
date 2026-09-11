package main

import (
	"flag"
	"fmt"
	"time"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var usernameFlag, passwordFlag string
	flag.StringVar(&usernameFlag, "username", "", "portal username")
	flag.StringVar(&usernameFlag, "u", "", "shorthand for -username")
	flag.StringVar(&passwordFlag, "password", "", "portal password")
	flag.StringVar(&passwordFlag, "p", "", "shorthand for -password")

	var logoutFlag, keepaliveFlag, daemonFlag bool
	flag.BoolVar(&logoutFlag, "logout", false, "log out of the captive portal instead of logging in")
	flag.BoolVar(&logoutFlag, "l", false, "shorthand for -logout")
	flag.BoolVar(&keepaliveFlag, "keepalive", false, "keep the current session alive instead of logging in")
	flag.BoolVar(&keepaliveFlag, "k", false, "shorthand for -keepalive")
	flag.BoolVar(&daemonFlag, "daemon", false, "run forever, auto re-logging in whenever the session drops")
	flag.BoolVar(&daemonFlag, "d", false, "shorthand for -daemon")

	var intervalFlag time.Duration
	flag.DurationVar(&intervalFlag, "interval", 45*time.Second, "polling interval for -daemon")
	flag.DurationVar(&intervalFlag, "i", 45*time.Second, "shorthand for -interval")

	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "print version information and exit")
	flag.BoolVar(&versionFlag, "v", false, "shorthand for -version")

	var autoFlag bool
	flag.BoolVar(&autoFlag, "auto", false, "run once automatically (for background jobs, checks SSID)")

	var installFlag bool
	flag.BoolVar(&installFlag, "install", false, "install macOS LaunchAgent for event-driven background execution")

	var uninstallFlag bool
	flag.BoolVar(&uninstallFlag, "uninstall", false, "uninstall macOS LaunchAgent")

	var logsFlag bool
	flag.BoolVar(&logsFlag, "logs", false, "view background task logs")

	flag.Parse()

	if versionFlag {
		fmt.Printf("fortinet-auto-login %s, commit %s, built at %s\n", version, commit, date)
		return
	}

	if installFlag {
		installAgent()
		return
	}

	if uninstallFlag {
		uninstallAgent()
		return
	}

	if logsFlag {
		viewLogs()
		return
	}

	if autoFlag {
		runAutoMode()
		return
	}

	initCredentials(usernameFlag, passwordFlag)

	if logoutFlag {
		if !logout() {
			logf("\033[31m[FAIL]\033[0m   Logout failed")
		}
		return
	}

	if keepaliveFlag {
		keepalive()
		return
	}

	if daemonFlag {
		daemon(intervalFlag)
		return
	}

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
