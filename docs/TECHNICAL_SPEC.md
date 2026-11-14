# Technical Specifications: Fortinet Auto-Login

## 1. System Overview

**Fortinet Auto-Login** (`autologin`) is a self-contained, high-performance binary engineered to automate authentication, session keepalive, unexpected logout recovery, and VPN circumvention on enterprise networks protected by Fortinet FortiGate firewalls.

### Key Metrics
- **Language**: Go (Standard Library `net/http`, `crypto/tls`, pure Go)
- **Binary Footprint**: ~6.8 MB (stripped release binary, zero external C dependencies)
- **Startup Latency**: < 5 ms
- **Discovery Latency**: < 20 ms (direct gateway IP fallback on port 1000)
- **Memory Footprint (RSS)**: < 10 MB
- **Dependencies**: Minimal standard library with pure Go multi-platform release builds.

---

## 2. Supported Platforms & Targets

The application is compiled and tested across 6 production targets:

| OS | Architecture | Go GOOS / GOARCH | Release Artifact Name |
| :--- | :--- | :--- | :--- |
| **macOS** | ARM64 (Apple Silicon) | `darwin` / `arm64` | `fortinet-auto-login-darwin-arm64` |
| **macOS** | x86_64 (Intel) | `darwin` / `amd64` | `fortinet-auto-login-darwin-amd64` |
| **Linux** | x86_64 (Intel/AMD) | `linux` / `amd64` | `fortinet-auto-login-linux-amd64` |
| **Linux** | ARM64 | `linux` / `arm64` | `fortinet-auto-login-linux-arm64` |
| **Windows** | x86_64 | `windows` / `amd64` | `fortinet-auto-login-windows-amd64.exe` |
| **Windows** | ARM64 | `windows` / `arm64` | `fortinet-auto-login-windows-arm64.exe` |

---

## 3. Command Line Interface (CLI) Specification

```
Usage: autologin [options]

Authentication & Session:
  autologin                    Log in to captive portal (default, prompts on first run)
  autologin -u <user> -p <pwd> Update saved credentials and authenticate
  autologin -logout            Log out of current captive portal session

Network & Diagnostics:
  autologin -status            Inspect network, Fortinet detection & active session

Background Service & Daemon:
  autologin -service install   Install background service (macOS / Linux / Windows)
  autologin -service uninstall Remove background service
  autologin -daemon [-i 45s]   Run continuous polling daemon in foreground

Cloudflare WARP Circumvention:
  autologin -warp              Intelligently rotate Anycast IPs & ports to connect Cloudflare WARP
  autologin -warp-disconnect   Disconnect WARP and restore normal network routing

Logs & Maintenance:
  autologin -logs [-n 40]      View background service logs (default: last 40 lines)
  autologin -logs -f           Live stream / follow logs (like tail -f)
  autologin -logs -clear       Clear background log file

Configuration & Campus Settings:
  autologin -config            View active configuration and probe endpoints
  autologin -config set-host <h> Update campus gateway host (e.g. 192.168.55.253:1003)
  autologin -config reset      Reset configuration to factory defaults
  autologin -host <host:port>  Temporarily override campus gateway host for this run
  autologin -upgrade           Check for and install the latest release from GitHub
  autologin -uninstall         Completely uninstall autologin and remove all data
  autologin -version           Print version information
```

### Exit Codes

| Exit Code | Meaning |
| :---: | :--- |
| `0` | Success / Operation completed normally / Clean shutdown |
| `1` | Operational failure (authentication rejected, network unreachable, invalid CLI argument) |
| `101` | Rust panic / Unhandled assertion (guarded against across all production code paths) |

---

## 4. File System Paths & Security Permissions

All user configuration, credentials, active sessions, and lock files are isolated under platform-specific standard directories:

| Component | macOS / Linux Path | Windows Path | Permissions |
| :--- | :--- | :--- | :---: |
| **Credentials Store** | `~/.fortinet-autologin/credentials.json` | `%USERPROFILE%\.fortinet-autologin\credentials.json` | `0600` (User only) |
| **Configuration** | `~/.fortinet-autologin/config.json` | `%USERPROFILE%\.fortinet-autologin\config.json` | `0644` |
| **Active Session Cache**| `~/.fortinet-autologin/session.json` | `%USERPROFILE%\.fortinet-autologin\session.json` | `0600` |
| **Process Locks** | `/tmp/fortinet-autologin-locks/*.lock` | `%TEMP%\fortinet-autologin-locks\*.lock` | Exclusive Lock |
| **Daemon Logs** | `/tmp/autologin.log` | `%TEMP%\autologin.log` | Standard append |

---

## 5. Security & Isolation Considerations

1. **Credential Masking**:
   - Terminal echo is disabled during interactive password prompts (`stty -echo` on Unix).
   - Saved credentials are saved with restricted filesystem permissions (`0600`).
   - Passwords and auth tokens are stripped from log outputs and transcripts.
2. **TLS Certificate Validation**:
   - Captive portals typically utilize self-signed or internal enterprise PKI root certificates for captive portal interception.
   - The HTTP agent disables certificate hostname validation specifically for captive portal interaction while using standard HTTPS encryption.
3. **Log Sanitization**:
   - ANSI terminal color codes are stripped before persisting to log files.
   - Automatic log file rotation enforces a 2 MB ceiling (`autologin.log` -> `autologin.log.old`).
