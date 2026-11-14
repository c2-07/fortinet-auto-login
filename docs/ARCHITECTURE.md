# System Architecture: Fortinet Auto-Login

This document details the software architecture, subsystem boundaries, data flow, concurrency model, and inter-process synchronization mechanisms of **Fortinet Auto-Login** (`autologin`).

---

## 1. High-Level Subsystem Architecture

The application is structured into decoupled modules, separating CLI input handling, configuration management, network probing, captive portal interaction, background daemon lifecycle, and Cloudflare WARP rotation.

```mermaid
graph TD
    User([User / Shell / OS Scheduler]) --> CLI[CLI Entrypoint & Command Dispatcher: main.go]
    
    subgraph Core Subsystems
        CLI --> Config[Configuration Engine: config.go]
        CLI --> Lock[Process Locking & IPC: lock.go / lock_unix.go / lock_windows.go]
        CLI --> Detect[Network Detection & Probes: detect.go]
        CLI --> Net[Network Client & Failover: network.go]
        CLI --> Session[Session & Credentials: session.go / credentials.go]
        CLI --> Daemon[Daemon & Background Service: daemon.go / platform.go]
        CLI --> Warp[Cloudflare WARP Engine: warp.go]
    end

    subgraph External Interfaces
        Config -.-> ConfigFile[(~/.fortinet-autologin/config.json)]
        Lock -.-> LockFile[(/tmp/*.lock or %TEMP%/*.lock)]
        Session -.-> CredStore[(~/.fortinet-autologin/credentials.json)]
        Session -.-> SessionStore[(~/.fortinet-autologin/session.json)]
        Net --> Probes{{Cloudflare 204 & Direct Gateway IP:1000}}
        Net --> FortiGate{{Fortinet FortiGate Gateway :1003}}
        Warp --> WarpCLI{{Cloudflare WARP CLI / Daemon}}
    end
```

---

## 2. Module Responsibilities & Source Map

| Source File | Responsibility |
| :--- | :--- |
| `src/main.go` | CLI argument parsing, sub-command routing, colored terminal logging, interactive prompts, top-level error reporting. |
| `src/config.go` | Configuration schema (`appConfig`), persistent JSON storage (`config.json`), CLI `-config` overrides, factory defaults. |
| `src/detect.go` | Network environment inspection, Fortinet gateway detection, DNS/DHCP inspection, Wi-Fi SSID discovery. |
| `src/network.go` | HTTP probe engine, direct gateway IP fallback (`:1000`), fast retry backoff, TLS configuration, captive portal redirect extraction, `/fgtauth` login & `/logout`. |
| `src/session.go` | Session state persistence (`session.json`), magic token lifecycle, keepalive countdown parsing, session clearing. |
| `src/credentials.go` | Secure credential storage (`credentials.json`), terminal prompt fallback, `-clear-creds` wiping. |
| `src/lock.go` | Cross-platform advisory file locking (`flock` on Unix, exclusive file sharing on Windows), preventing race conditions. |
| `src/daemon.go` | Foreground polling daemon loop with context-aware cancellation (`SIGINT`/`SIGTERM`). |
| `src/platform.go` | OS service management (`launchd` on macOS, `systemd` on Linux, Task Scheduler on Windows), `-uninstall` engine. |
| `src/warp.go` | Cloudflare WARP Anycast IP and port matrix generator, UDP reachability testing, automated interface reset. |
| `src/upgrade.go` | Self-updater, GitHub Releases tag query, binary asset download, atomic binary replacement. |

---

## 3. Core Workflows & Sequence Diagrams

### 3.1 Authentication Workflow (Interactive & CLI)

When a user or the background service initiates login, the system first verifies network connectivity to determine if authentication is required:

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant CLI as autologin (CLI)
    participant Detect as Network Engine
    participant Probes as Probe Endpoints (Firefox/Google/Apple)
    participant FortiGate as FortiGate Gateway
    participant Store as Local Storage

    User->>CLI: autologin
    CLI->>Detect: Check internet & portal presence
    Detect->>Probes: GET /success.txt (Firefox)
    alt Internet Reachable (200 OK 'success')
        Probes-->>Detect: 200 OK (Internet alive)
        Detect-->>CLI: Already authenticated
        CLI->>User: [OK] Internet connection is active. No login required.
    else Captive Portal Intercepted (302 Redirect to /fgtauth)
        Probes-->>Detect: 302 Found (https://192.168.55.253:1003/fgtauth?magic=abc)
        Detect-->>CLI: Portal detected with magic token
        CLI->>Store: Load saved credentials
        alt Credentials Missing
            CLI->>User: Prompt Username & Password
            User-->>CLI: Input credentials
            CLI->>Store: Save credentials (0600 mode)
        end
        CLI->>FortiGate: POST /fgtauth (magic, username, password, 4Tredir)
        FortiGate-->>CLI: 200 OK (Keepalive HTML session page)
        CLI->>Store: Save active session (magic, portal_host, expiry)
        CLI->>User: [OK] Authentication successful! Session established.
    end
```

---

### 3.2 Daemon Execution & Unexpected Logout Recovery

The daemon continuously monitors the connection. If the firewall drops the session (due to idle timeouts, DHCP renewals, or firewall failovers), the daemon instantly recovers:

```mermaid
sequenceDiagram
    autonumber
    participant Daemon as Daemon Loop
    participant Lock as Lock File
    participant Net as Probe Engine
    participant Auth as Auth Engine
    participant FortiGate as FortiGate Gateway

    Note over Daemon: Background Polling (Default: 30s)
    Daemon->>Lock: Acquire exclusive daemon lock
    loop Every Check Interval
        Daemon->>Net: Probe connectivity (Firefox / Google / Apple)
        alt Probe succeeds (Status 200)
            Net-->>Daemon: Internet verified OK
            Note over Daemon: Sleep until next tick
        else Probe redirected or fails (Unexpected Logout)
            Net-->>Daemon: Redirected to /fgtauth?magic=xyz
            Note over Daemon: [WARN] Unexpected logout detected!
            Daemon->>Auth: Attempt auto-relogin using saved credentials
            Auth->>FortiGate: POST /fgtauth with fresh magic token
            FortiGate-->>Auth: 200 OK (Session Restored)
            Auth-->>Daemon: Relogin Successful
            Note over Daemon: [OK] Reconnected to Fortinet network
        end
    end
```

---

## 4. Concurrency & Process Locking Architecture

To guarantee that multiple instances (e.g., a background `systemd` daemon and a manual `autologin -status` CLI command) do not collide or corrupt session files, the application implements fine-grained advisory file locking.

### Lock Hierarchy

1. **`daemon.lock`**: Acquired exclusively by the running daemon process. Prevents multiple daemon processes from running concurrently.
2. **`auth.lock`**: Acquired during the login/logout handshake (short duration: ~100–300 ms). If the daemon is in the middle of re-authenticating, a manual CLI login waits or detects the in-flight operation.
3. **`warp.lock`**: Acquired while WARP Anycast endpoints are being rotated to prevent simultaneous network interface resets.

### Cross-Platform Locking Mechanics

- **Unix (macOS / Linux)**: Implemented using POSIX advisory record locking via `libc::flock(fd, LOCK_EX | LOCK_NB)`. Locks are automatically released by the kernel if a process terminates or crashes unexpectedly.
- **Windows**: Implemented using the Win32 `CreateFileW` API with sharing flag set to `0` (`FILE_SHARE_NONE`). Any competing process attempting to open the file receives `ERROR_SHARING_VIOLATION`.

---

## 5. Memory Management & Performance Guarantees

1. **Zero Dynamic Runtime Overhead**: Pure compiled Rust code without any garbage collection or runtime VM pauses.
2. **Deterministic Resource Deallocation (RAII)**: Sockets, file descriptors, and locks are tied to scoped guards and released immediately upon exiting execution blocks.
3. **No External Shared Library Dependencies**: Compiles to a statically linked executable that runs out-of-the-box on clean OS installations without requiring Python, Node.js, or OpenSSL shared libraries.
