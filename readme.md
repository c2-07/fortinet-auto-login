# fortinet-auto-login

A small CLI that automates login, logout, and session keepalive for FortiGate
captive portals (the kind that show a browser popup asking for username/password
and a "keepalive" window that has to stay open). Written in Go, no external
dependencies, cross-compiles for macOS/Linux/Windows.

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
| `-install`   |           | `false` | Install macOS LaunchAgent for background execution    |
| `-uninstall` |           | `false` | Uninstall macOS LaunchAgent                           |
| `-auto`      |           | `false` | Run once automatically (used by background jobs)      |
| `-version`   | `-v`      | `false` | Print version information and exit                    |

The tool uses a secure persistent credential cache. When you run it for the first time without flags, it will interactively prompt you for your username and password, which are then saved securely in `~/.fortinet-autologin/credentials.json` with secure permissions (`0600`). You can also supply `-u` and `-p` flags explicitly to provide or update these credentials without editing the source.

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

Run as a persistent background daemon:

```bash
./autologin -d -i 30s
```

## Running automatically in the background

Rather than reacting to Wi-Fi connect/disconnect events (fragile on both
OSes), the daemon just polls every `-interval` and no-ops instantly if
already connected, re-authenticating only when the session actually drops.

### macOS & Linux

We provide a built-in command to easily install a background service (`LaunchAgent` on macOS, `systemd` user service on Linux) that automatically logs you in whenever you connect to Wi-Fi.

Simply run:
```bash
./autologin -install
```
This will set up the agent to automatically re-authenticate in the background! Logs are written to `/tmp/autologin.log`.

To remove the background service at any time:
```bash
./autologin -uninstall
```

### Windows — Scheduled Task

```powershell
$action = New-ScheduledTaskAction -Execute "C:\Tools\autologin.exe" -Argument "-daemon"
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -Hidden -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit 0
Register-ScheduledTask -TaskName "AutoLogin-WiFi" -Action $action -Trigger $trigger -Settings $settings -RunLevel Limited
```

## Notes

- Session state (host, magic, countdown) and credentials are automatically cached in `~/.fortinet-autologin/`.
- `-logout` falls back to a randomly generated magic against a default
  gateway if no cached session exists — the portal doesn't validate the
  magic against the actual session, so this still works.
- All output is timestamped and only logs meaningful events (fresh logins,
  failures, session refreshes) — the daemon stays quiet while connected.
