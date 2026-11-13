# fortinet-auto-login

A lightning-fast, zero-dependency CLI written in Go that automates login, logout, network detection, and background keepalive for Fortinet FortiGate captive portals.

---

## Quick Start (30 Seconds)

### 1. Build and Run
```bash
# Build from source
go build -o autologin .

# Run login (prompts for username & password on first run)
./autologin
```

### 2. Enable Automatic Background Login
Install the native event-driven background service (macOS / Linux):
```bash
./autologin -service install
```
* **Instant Login:** Automatically logs in the split-second you join campus Wi-Fi or switch access points.
* **Zero Battery Drain:** Completely idles (< 5ms exit) when on home Wi-Fi or personal hotspots.

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
| `-service install` | `-install` | Install automatic background service |
| `-service uninstall`| `-uninstall`| Remove background service |
| `-daemon` | `-d` | Run continuous monitoring daemon in foreground |
| `-interval <duration>`| `-i 30s` | Set custom polling interval for daemon (default `45s`) |
| `-warp` | | Intelligently rotate Anycast IPs & ports to connect Cloudflare WARP |
| `-warp-disconnect` | | Disconnect Cloudflare WARP and restore normal network routing |
| `-logs` | | View recent background service logs (last 40 lines) |
| `-logs -f` | `-follow` | Stream / follow background logs live (like `tail -f`) |
| `-logs -clear` | `-clean` | Clear background log history |
| `-upgrade` | | Check for and install latest release from GitHub |
| `-version` | `-v` | Display version information |

---

## Examples

### Check Network & Fortinet Status
Inspect whether you are on a Fortinet network and view active session details:
```bash
./autologin -status
```
**Sample Output:**
```text
--- Network & Fortinet Status ---
Internet:         Connected
Fortinet Network: Yes (Method: Gateway TLS Certificate Signature)
Gateway IP:       192.168.55.253:1003
Device ID:        FG6H0FTB22905105

--- Active Session Cache ---
Host:             https://192.168.55.253:1003
Magic:            079c4a9e6abca729
Timeout:          14400s

Saved User:       bt25cse138
```

### Stream Background Logs
Follow real-time background authentication events:
```bash
./autologin -logs -f
```

### Log Out Cleanly
Terminate your active captive portal session on the gateway:
```bash
./autologin -logout
```

### Cloudflare WARP Auto-Rotation & Connect
Evade FortiGate UDP session blackholing with automated network pre-detection, Anycast pool rotation across whitelisted VPN ports (500, 4500), real outbound internet verification, and automatic Wi-Fi reset recovery:
```bash
./autologin -warp
```

To disconnect and restore normal routing:
```bash
./autologin -warp-disconnect
```

> **Automatic Recovery:** If FortiGate blackholes all candidate UDP sessions, `autologin -warp` automatically cycles Wi-Fi off and on (macOS, Linux, Windows), acquires a clean DHCP lease, re-authenticates with the captive portal, and re-establishes the tunnel.

---

## Security & Storage
* Credentials are saved locally to `~/.fortinet-autologin/credentials.json` with strict `0600` user-only permissions.
* Session tokens and countdowns are saved to `~/.fortinet-autologin/session.json`.
* Log outputs are capped and rotated automatically at 2MB to protect disk space.
