# fortinet-auto-login

A small CLI that automates login, logout, and session keepalive for FortiGate
captive portals (the kind that show a browser popup asking for username/password
and a "keepalive" window that has to stay open). Written in Go, no external
dependencies, cross-compiles for macOS/Linux/Windows.

<!-- ![Demo](docs/screenshots/demo.gif) -->
<img width="3140" height="2046" alt="image" src="https://github.com/user-attachments/assets/c255bf52-f1a3-4ce1-899a-0fe378ac7311" />


## How it works

FortiGate captive portals redirect an unauthenticated HTTP request to a login
page carrying a one-time `magic` token in the query string. That same token is
reused for login, logout, and keepalive for the lifetime of the session. This
tool:

1. Detects the portal redirect via `http://detectportal.firefox.com/`.
2. Logs in with your credentials, capturing the session's `magic` token.
3. Saves that session (host + magic + countdown) to a local cache file so
   later `-logout` / `-keepalive` calls don't need to log in again.
4. Optionally runs forever as a background daemon, transparently
   re-authenticating whenever the session drops.

<!-- ![Login flow](docs/screenshots/login-flow.png) -->

## Installation

### Download from Releases
You can download the latest pre-compiled binaries for Linux, macOS, and Windows directly from the [Releases](../../releases/latest) page.

After downloading, make the binary executable and move it to your global shell environment (`PATH`):

```bash
# Make it executable (Linux/macOS)
chmod +x fortinet-auto-login-*

# Move it to a directory in your PATH (e.g. /usr/local/bin)
sudo mv fortinet-auto-login-* /usr/local/bin/autologin
```

Now you can run it from anywhere in your terminal using the `autologin` command.

### Build from source

```bash
go build -o autologin .
```

Cross-compile for another OS/arch:

```bash
# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o autologin-mac .

# Windows
GOOS=windows GOARCH=amd64 go build -o autologin.exe .
```

## Usage

```bash
./autologin [flags]
```

| Flag         | Shorthand | Default | Description                                           |
| ------------ | --------- | ------- | ----------------------------------------------------- |
| `-username`  | `-u`      |         | Portal username (saved to credential cache)           |
| `-password`  | `-p`      |         | Portal password (saved to credential cache)           |
| `-logout`    | `-l`      | `false` | Log out instead of logging in                         |
| `-keepalive` | `-k`      | `false` | Keep the current session alive (blocking loop)        |
| `-daemon`    | `-d`      | `false` | Run forever, auto re-login whenever the session drops |
| `-interval`  | `-i`      | `45s`   | Poll interval for `-daemon`                           |
| `-version`   | `-v`      | `false` | Print version information and exit                    |

The tool uses a secure persistent credential cache. When you run it for the first time without flags, it will interactively prompt you for your username and password, which are then saved in `$XDG_CACHE_HOME/captive-portal-credentials.json` with secure permissions (`0600`). You can also supply `-u` and `-p` flags explicitly to provide or update these credentials without editing the source.

### Examples

Log in once:

```bash
./autologin
```

Log in with different credentials:

```bash
./autologin -u your_username -p your_password
```

Log out of the current session:

```bash
./autologin -logout
```

Run as a persistent background daemon (recommended for day-to-day use — see
[Running automatically](#running-automatically-macoswindows) below):

```bash
./autologin -d -i 30s
```

Keep the current session alive without a daemon loop:

```bash
./autologin -k
```

<!-- ![CLI output](docs/screenshots/cli-output.png) -->

## Running automatically (macOS/Windows)

Rather than reacting to Wi-Fi connect/disconnect events (fragile on both
OSes), the daemon just polls every `-interval` and no-ops instantly if
already connected, re-authenticating only when the session actually drops.

### macOS — LaunchAgent

Save as `~/Library/LaunchAgents/com.yourname.autologin.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.yourname.autologin</string>
    <key>ProgramArguments</key>
    <array>
        <string>/path/to/autologin</string>
        <string>-daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/autologin.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/autologin.err.log</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.yourname.autologin.plist
```

### Windows — Scheduled Task

```powershell
$action = New-ScheduledTaskAction -Execute "C:\Tools\autologin.exe" -Argument "-daemon"
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -Hidden -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit 0
Register-ScheduledTask -TaskName "AutoLogin-WiFi" -Action $action -Trigger $trigger -Settings $settings -RunLevel Limited
```

<!-- ![LaunchAgent setup](docs/screenshots/launchagent.png) -->
<!-- ![Task Scheduler setup](docs/screenshots/task-scheduler.png) -->

## Notes

- Session state (host, magic, countdown) is cached at
  `$XDG_CACHE_HOME/captive-portal-session.json` (or the OS equivalent).
- `-logout` falls back to a randomly generated magic against a default
  gateway if no cached session exists — the portal doesn't validate the
  magic against the actual session, so this still works.
- All output is timestamped and only logs meaningful events (fresh logins,
  failures, session refreshes) — the daemon stays quiet while connected.
