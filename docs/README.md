# Fortinet Auto-Login Documentation Index

Welcome to the technical documentation for **Fortinet Auto-Login** (`autologin`), a high-performance, resilient, cross-platform CLI and background daemon written in Rust.

---

## Documentation Structure

| Document | Description |
| :--- | :--- |
| **[TECHNICAL_SPEC.md](TECHNICAL_SPEC.md)** | Complete technical specifications, CLI interface, system requirements, and performance benchmarks. |
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | Subsystem breakdown, concurrency model, module boundaries, and Mermaid architectural diagrams. |
| **[FORTINET_PROTOCOL.md](FORTINET_PROTOCOL.md)** | Reverse engineering findings on FortiGate captive portal mechanics, magic tokens, session lifecycle, and keepalive probes. |
| **[CAMPUS_CUSTOMIZATION.md](CAMPUS_CUSTOMIZATION.md)** | Multi-campus configuration guide, custom portal gateways/ports, and primary/fallback connectivity endpoints. |
| **[CROSS_PLATFORM_DAEMON.md](CROSS_PLATFORM_DAEMON.md)** | Daemon mode and OS background service integration on macOS (`launchd`), Linux (`systemd`), and Windows (`Task Scheduler`). |
| **[WARP_AUTO_ROTATION.md](WARP_AUTO_ROTATION.md)** | Intelligent Cloudflare WARP circumvention engine, UDP blackhole detection, and Anycast rotation algorithms. |
| **[TESTING_AND_QA.md](TESTING_AND_QA.md)** | Test discovery, automated test suites (31 tests), Docker container verification, and quality gates. |

---

## Quick Reference Commands

```bash
# Authenticate with captive portal (default, prompts on first run)
autologin

# Inspect network, Fortinet detection & active session
autologin -status

# View & update campus configuration
autologin -config
autologin -config set-host 192.168.55.253:1003

# Install OS background service (macOS / Linux / Windows)
autologin -service install

# Run continuous polling daemon in foreground
autologin -daemon -interval 30s

# Connect Cloudflare WARP with auto-rotation
autologin -warp

# View / follow background service logs
autologin -logs -f
```
