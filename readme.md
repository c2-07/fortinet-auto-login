# fortinet-auto-login (Rust Edition)

A lightning-fast, highly optimized CLI and background daemon written in Rust that automates login, logout, network detection, keepalive, and Cloudflare WARP circumvention for Fortinet FortiGate captive portals.

---

## ⚡ 1-Second Install

### macOS & Linux (curl | bash)
```bash
curl -fsSL https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.sh | bash
```

### Windows (PowerShell irm | iex)
```powershell
irm https://raw.githubusercontent.com/c2-07/fortinet-auto-login/main/install.ps1 | iex
```

* Automatically detects your operating system and architecture (`x86_64` / `ARM64`).
* Downloads the latest pre-compiled binary and places it in your user binary directory.
* Permanently adds the tool to your shell (`.zshrc`, `.bashrc`, `config.fish`) or Windows User `PATH` environment variable.

---

## Quick Start

### 1. Authenticate with Captive Portal
```bash
autologin
```
Prompts for your campus username and password on the first run, saves credentials with strict user-only permissions (`0600`), and connects.

### 2. Install Background Service
Install the native event-driven background service for your OS:
```bash
autologin -service install
```
* **macOS:** Native `launchd` LaunchAgent triggered instantly on Wi-Fi roaming or interface state changes.
* **Linux:** Unprivileged `systemd --user` service with automated restart watchdog.
* **Windows:** Windows Task Scheduler (`schtasks.exe`) logon integration.

### 3. Check Network & Hardware Status
```bash
autologin -status
```

---

## Command Reference

```bash
autologin [options]
```

| Command / Flag | Shorthand | Description |
| :--- | :--- | :--- |
| *(no arguments)* | | Log in to captive portal (default action) |
| `-u <user> -p <pass>` | `-username`, `-password` | Update saved credentials and authenticate |
| `-logout` | `-l` | Log out of current captive portal session |
| `-status` | `-detect` | Inspect internet, Fortinet hardware presence & active session |
| `-service install` | `-install` | Install automatic background service (macOS / Linux / Windows) |
| `-service uninstall`| | Remove background service |
| `-daemon` | `-d` | Run continuous monitoring daemon in foreground |
| `-interval <duration>`| `-i 30s` | Set custom polling interval for daemon (default `45s`) |
| `-warp` | | Intelligently rotate Anycast IPs & ports to connect Cloudflare WARP |
| `-warp-disconnect` | | Disconnect Cloudflare WARP and restore normal network routing |
| `-logs` | | View recent background service logs (last 40 lines) |
| `-logs -f` | `-follow` | Stream / follow background logs live (like `tail -f`) |
| `-logs -clear` | `-clean` | Clear background log history |
| `-config` | | View campus gateway and probe endpoint configuration |
| `-config set-host <h>`| | Update campus gateway host (e.g. `10.0.0.1:1003`) |
| `-config reset` | | Reset campus settings to factory defaults |
| `-host <host:port>` | | Temporarily override campus gateway host for this run |
| `-upgrade` | | Check for and install the latest release binary from GitHub |
| `-uninstall` | | Completely uninstall autologin, background services, and remove all data |
| `-version` | `-v` | Display version information |

---

## Self-Upgrade & Clean Uninstallation

### In-Place Upgrade
```bash
autologin -upgrade
```
Queries the GitHub API, checks your version against the latest release, downloads the matching pre-compiled binary for your architecture, and replaces the running executable in-place.

### Complete Uninstallation
```bash
autologin -uninstall
```
1. Stops and removes any registered background service (`launchd`, `systemd`, or `schtasks`).
2. Disconnects Cloudflare WARP if active.
3. Completely deletes `~/.fortinet-autologin` (`credentials.json`, `config.json`, `session.json`).
4. Purges background log files (`autologin.log`).
5. Cleans PATH additions from your shell profile files (`.zshrc`, `.bashrc`, `.profile`) or Windows User environment variables.
6. Removes the `autologin` binary itself from your disk.

---

## Documentation

Full architectural specifications, protocol reverse engineering findings, and testing reports are available in the **[`docs/`](docs/)** directory:

- [**Technical Specification**](docs/TECHNICAL_SPEC.md): Platform matrix, CLI spec, security permissions.
- [**System Architecture**](docs/ARCHITECTURE.md): Subsystems, Mermaid diagrams, advisory file locking.
- [**Fortinet Protocol Reverse Engineering**](docs/FORTINET_PROTOCOL.md): `fgtauth` POST handshake, magic tokens, session keepalive.
- [**Campus Customization**](docs/CAMPUS_CUSTOMIZATION.md): Multi-campus setup and prioritized probe failover.
- [**Cross-Platform Daemon**](docs/CROSS_PLATFORM_DAEMON.md): Deep dive into macOS, Linux, and Windows services.
- [**Cloudflare WARP Auto-Rotation**](docs/WARP_AUTO_ROTATION.md): UDP blackhole detection, 72 Anycast combinations.
- [**Testing & QA Engineering**](docs/TESTING_AND_QA.md): Test harness, 31 automated tests, Docker verification.
