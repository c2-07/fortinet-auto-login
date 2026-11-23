# fortinet-auto-login

**Never type your campus or office Fortinet Wi-Fi login page again.**

`autologin` is a fast, lightweight tool for **macOS, Linux, and Windows** that automatically signs you into Fortinet (FortiGate) captive portals, keeps your session alive in the background, and can even get Cloudflare WARP working on restrictive networks.

<img width="1462" height="919" alt="fortinet-auto-login CLI screenshot" src="https://github.com/user-attachments/assets/87e70954-728b-4462-aa9d-c99ab4e67793" />

---

## Main Features

- **One-Command Login:** Enter your username and password once; `autologin` saves them privately on your computer and signs you in automatically from then on.
- **Set-and-Forget Background Service:** Automatically detects when you connect to Wi-Fi, wake your laptop from sleep, or get timed out by the firewall — and logs you right back in.
- **Session Keepalive:** Proactively refreshes your Fortinet session before it expires so downloads, calls, and games don't drop.
- **Wi-Fi Network Filter (SSID Allow-List):** Optionally limit auto-login to specific Wi-Fi names (e.g. your campus or hostel Wi-Fi) so it stays idle on home/mobile networks.
- **Cloudflare WARP Bypass:** Automatically tests Cloudflare endpoints and firewall-friendly ports to establish a working WARP tunnel when standard ports are blocked.
- **Works on Any Fortinet Campus:** Configure your own portal gateway address in one command.
- **Easy Updates & Clean Removal:** Built-in self-updater (`-upgrade`) and a full uninstaller (`-uninstall`) that leaves zero files behind.

---

## Installation (Takes ~10 Seconds)

Open your terminal and run the command for your operating system:

### macOS & Linux
```bash
curl -fsSL https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.ps1 | iex
```

What the installer does:
- Detects your OS and CPU (`x86_64` / `ARM64`) automatically.
- Downloads the latest pre-built `autologin` binary.
- Adds `autologin` to your `PATH` (restart your terminal window if `autologin` isn't recognized right away).

---

## Getting Started

### Step 1: Log In for the First Time
```bash
autologin
```
On the first run, it will ask for your Fortinet portal username and password, save them locally (readable only by your user account), and connect you to the internet.

### Step 2: Turn On Automatic Background Login
```bash
autologin -service install
```
That's it! From now on, `autologin` runs quietly in the background whenever you log into your computer:
- **macOS:** Runs via native `launchd` (reacts instantly to Wi-Fi changes).
- **Linux:** Runs as a user `systemd` service with automatic restart.
- **Windows:** Runs via Windows Task Scheduler (works on battery, no 72-hour stop limit, no popup window).

> To stop the background service later, run `autologin -service uninstall`.

---

## Everyday Usage

### Check Connection & Session Status
```bash
autologin -status
```
Shows whether you have internet access, if a Fortinet gateway is detected, and whether you have an active session.

### Only Auto-Login on Specific Wi-Fi Networks
```bash
autologin -ssid "Campus-WiFi,Hostel-5G"
```
To clear the filter and allow any network again:
```bash
autologin -ssid ""
```

### Switch Accounts or Log Out
```bash
autologin -logout                    # Log out of the current session
autologin -u newuser -p newpass      # Update saved credentials and log in
autologin -clear-creds               # Delete saved credentials & session cache
```

### Connect Cloudflare WARP on Restricted Networks
> [!IMPORTANT]
> Always disconnect any existing WARP session **first**. A stale or blocked tunnel can hide the Fortinet login page from detection.

```bash
autologin -warp-disconnect   # 1. Cleanly disconnect WARP & restore normal routing
autologin -warp              # 2. Log in to Fortinet (if needed) and find a working WARP endpoint
```
`autologin -warp` rotates through Cloudflare Anycast IPs and firewall-friendly ports (`500`, `4500`, `1701`, etc.) until real internet traffic flows. When you're done, `autologin -warp-disconnect` disconnects WARP and restores your default WARP settings.

### Use a Different Campus / Portal Address
```bash
autologin -config set-host 10.0.0.1:1003   # Save a new default gateway
autologin -config                          # View current settings
autologin -config reset                    # Reset back to defaults
```

### View Background Logs
```bash
autologin -logs          # Show last 40 log lines
autologin -logs -f       # Watch logs live (press Ctrl+C to stop)
autologin -logs -clear   # Clear log history
```

### Update or Completely Uninstall
```bash
autologin -upgrade       # Download and install the latest release in-place
autologin -uninstall     # Remove background service, saved credentials, config, logs, PATH entry & binary
```
*(Pass `-y` with `-uninstall` to skip the confirmation prompt.)*

---

## Command Reference

```text
autologin [options]
```

| Command / Flag | Shorthand | Description |
| :--- | :--- | :--- |
| *(no arguments)* | | Log in to captive portal (prompts for credentials on first run) |
| `-u <user> -p <pass>` | `-username`, `-password` | Update saved credentials and log in |
| `-ssid "A,B"` | | Restrict background auto-login to specific Wi-Fi SSIDs (`""` to clear) |
| `-clear-creds` | `-reset-creds`, `-delete-creds` | Delete saved credentials and active session cache |
| `-logout` | `-l` | Log out of the current captive portal session |
| `-status` | `-detect` | Show internet status, Fortinet gateway detection & active session |
| `-service install` | `-install` | Install the OS background service (macOS / Linux / Windows) |
| `-service uninstall` | `-service-uninstall` | Remove the OS background service |
| `-service status` | `-service-status` | Check whether the background service is installed and running |
| `-daemon` | `-d` | Run the adaptive monitoring daemon in the foreground |
| `-interval <dur>` | `-i 30s` | Set base polling interval for `-daemon` (default: `45s` or `config.json`) |
| `-warp` | | Auto-rotate Cloudflare WARP Anycast IPs & ports until connected |
| `-warp-disconnect` | | Disconnect WARP and restore default WARP settings & routing |
| `-logs` | | View recent background service logs (use `-n <count>` for line count) |
| `-logs -f` | `-follow` | Stream background logs live (like `tail -f`) |
| `-logs -clear` | `-clean` | Clear the background log file |
| `-config` | | Display active gateway host, timeouts, retries, SSID filter & probe URLs |
| `-config set-host <h>` | | Permanently set campus gateway host (e.g. `192.168.55.253:1003`) |
| `-config reset` | | Reset configuration to factory defaults |
| `-host <host:port>` | `-portal` | Temporarily override campus gateway host for a single run |
| `-upgrade` | | Check GitHub Releases and upgrade the binary in-place |
| `-uninstall` | | Completely remove `autologin`, its service, data, logs, and PATH entry (`-y` to skip prompt) |
| `-version` | `-v` | Print version, commit, and build timestamp |

---

# Developer & Technical Guide

Everything below is intended for developers, contributors, and power users who want to customize, build, or inspect how `autologin` works internally.

## Architecture Overview

- **Single-Binary Go CLI:** Written in Go (`go 1.26+`) with a minimal dependency footprint (`golang.org/x/term` for secure password input).
- **Fast-Path Logout Detection & Adaptive Daemon (`src/daemon.go`):**
  - **Zero-Packet Local Watcher:** Snapshots active non-loopback network interface addresses every `2s` (`<0.1ms`, zero network I/O) and monitors wall-clock jumps (`>15s`) to detect Wi-Fi roaming, DHCP renewals, VPN/WARP toggles, and sleep/wake events immediately.
  - **Lightweight Outbound Probe:** Uses a reused HTTP keep-alive client against a `204` endpoint (`http://cp.cloudflare.com/generate_204` by default, `0`-byte body, ~25–55ms). When logged in, it skips heavy gateway TLS probes entirely.
  - **Exponential Backoff:** Healthy sessions back off polling intervals (`base → 2× → 4×`, capped at `5m`), automatically clamped so proactive `/keepalive?<magic>` refreshes are never missed. Any network event, wake, or auth drop resets the interval to `base`.
- **Fortinet Protocol Handling (`src/network.go`, `src/detect.go`, `src/session.go`):**
  - Parses captive portal redirects (`fgtauth` / `magic` tokens) and submits portal credentials directly.
  - Falls back to direct gateway probing when DNS is blackholed.
  - Tracks session `countdown` and `refreshed_at` to issue proactive keepalive requests before expiry.
- **Cross-Platform Service Integration (`src/platform.go`, `src/elevate_*.go`):**
  - **macOS:** `~/Library/LaunchAgents/com.fortinet.autologin.plist` watching SystemConfiguration `WatchPaths` + `StartInterval` fallback watchdog.
  - **Linux:** `~/.config/systemd/user/fortinet-autologin.service` running `autologin -daemon` with `Restart=always`.
  - **Windows:** Task Scheduler 1.2 XML encoded as **UTF-16LE with BOM** (`schtasks /create /xml`), configured with `DisallowStartIfOnBatteries=false`, `StopIfGoingOnBatteries=false`, `ExecutionTimeLimit=PT0S`, `Hidden=true`, and `RestartOnFailure`.
- **Concurrency & State (`src/lock_*.go`, `src/credentials.go`, `src/logger.go`):**
  - Advisory OS file locks (`flock` on Unix, `LockFileEx` on Windows) prevent overlapping `auth`, `daemon`, and `warp` operations.
  - All user state lives in `~/.fortinet-autologin/` with `0600` file permissions (`credentials.json`, `config.json`, `session.json`).
  - Background logs strip ANSI escape codes and automatically rotate when exceeding the size cap.
- **Cloudflare WARP Auto-Rotation (`src/warp.go`):**
  - Cycles through a 72-entry Anycast IP × UDP port matrix, verifies real end-to-end connectivity (detecting UDP blackholes), temporarily disables WARP's internal connectivity check when needed, and restores original WARP settings on `-warp-disconnect` or `-uninstall`.

---

## Configuration File & Environment Variables

Configuration is stored at `~/.fortinet-autologin/config.json` (created automatically on first write):

```json
{
  "portal_host": "192.168.55.253:1003",
  "check_urls": [
    "http://cp.cloudflare.com/generate_204"
  ],
  "daemon_interval_secs": 45,
  "request_timeout_secs": 4,
  "max_retries": 2
}
```

### Environment Variable Overrides
Overrides apply at runtime without modifying `config.json`:

| Variable | Effect |
| :--- | :--- |
| `FORTINET_PORTAL_HOST` | Overrides `portal_host` (lower precedence than `-host`) |
| `FORTINET_CHECK_URL` | Overrides `check_urls` with a single connectivity check URL |
| `FORTINET_INTERVAL` | Overrides `daemon_interval_secs` (integer seconds) |

---

## Building & Testing from Source

### Prerequisites
- Go `1.26` or newer (see [`go.mod`](go.mod))

### Common Commands
```bash
git clone https://github.com/c2-07/fortinet-auto-login.git
cd fortinet-auto-login

make build        # Build ./autologin with version ldflags
make install      # Build and install to ~/.local/bin/autologin
make test         # Run full test suite (go test -v ./...)
make test-race    # Run tests with Go race detector
make bench        # Run benchmarks
make vet          # Run go vet ./...
```

### Project Layout
```text
.
├── src/                # Main Go package (CLI, auth, daemon, detection, WARP, OS services, unit tests)
├── tests/              # Fixture-based protocol and HTML parsing tests
├── docs/               # In-depth architecture, protocol, and QA documentation
├── scripts/            # Helper & release scripts
├── install.sh          # macOS / Linux installer
├── install.ps1         # Windows PowerShell installer
└── Makefile            # Build, test, race, benchmark, and lint targets
```

---

## Deep-Dive Documentation

Detailed specifications, protocol reverse-engineering notes, and test reports live in [`docs/`](docs/):

- [**Technical Specification**](docs/TECHNICAL_SPEC.md) — Platform matrix, CLI spec, security permissions.
- [**System Architecture**](docs/ARCHITECTURE.md) — Subsystems, Mermaid diagrams, advisory file locking.
- [**Fortinet Protocol Reverse Engineering**](docs/FORTINET_PROTOCOL.md) — `fgtauth` POST handshake, magic tokens, session keepalive.
- [**Campus Customization**](docs/CAMPUS_CUSTOMIZATION.md) — Multi-campus setup and prioritized probe failover.
- [**Cross-Platform Daemon**](docs/CROSS_PLATFORM_DAEMON.md) — Deep dive into macOS, Linux, and Windows services.
- [**Cloudflare WARP Auto-Rotation**](docs/WARP_AUTO_ROTATION.md) — UDP blackhole detection and 72 Anycast combinations.
- [**Testing & QA Engineering**](docs/TESTING_AND_QA.md) — Test harness, automated test suite, and verification.
