use std::env;
use std::fs::{self, File};
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::Command;

pub const LOG_PATH: &str = "/tmp/autologin.log";

pub fn get_exe_path() -> PathBuf {
    env::current_exe().unwrap_or_else(|_| PathBuf::from("autologin"))
}

pub fn install_service() {
    #[cfg(target_os = "macos")]
    {
        install_macos_launch_agent();
    }

    #[cfg(target_os = "linux")]
    {
        install_linux_systemd_service();
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        println!("\x1b[31m[FAIL]\x1b[0m   Automatic background service installation is supported on macOS and Linux.");
    }
}

pub fn uninstall_service() {
    #[cfg(target_os = "macos")]
    {
        uninstall_macos_launch_agent();
    }

    #[cfg(target_os = "linux")]
    {
        uninstall_linux_systemd_service();
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        println!("\x1b[31m[FAIL]\x1b[0m   Automatic background service removal is supported on macOS and Linux.");
    }
}

#[cfg(target_os = "macos")]
fn get_launch_agent_path() -> PathBuf {
    let home = env::var("HOME").unwrap_or_else(|_| "/Users/Shared".to_string());
    PathBuf::from(home)
        .join("Library")
        .join("LaunchAgents")
        .join("com.fortinet.autologin.plist")
}

#[cfg(target_os = "macos")]
fn install_macos_launch_agent() {
    let plist_path = get_launch_agent_path();
    if let Some(parent) = plist_path.parent() {
        let _ = fs::create_dir_all(parent);
    }

    let exe = get_exe_path();
    let exe_str = exe.to_string_lossy();

    let plist_content = format!(
        r#"<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.fortinet.autologin</string>
    <key>ProgramArguments</key>
    <array>
        <string>{}</string>
        <string>-auto</string>
    </array>
    <key>WatchPaths</key>
    <array>
        <string>/Library/Preferences/SystemConfiguration/com.apple.airport.preferences.plist</string>
        <string>/Library/Preferences/SystemConfiguration/NetworkInterfaces.plist</string>
        <string>/etc/resolv.conf</string>
    </array>
    <key>StartInterval</key>
    <integer>30</integer>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{}</string>
    <key>StandardErrorPath</key>
    <string>{}</string>
</dict>
</plist>
"#,
        exe_str, LOG_PATH, LOG_PATH
    );

    if let Err(e) = fs::write(&plist_path, plist_content) {
        println!("\x1b[31m[FAIL]\x1b[0m   Failed to write LaunchAgent plist: {}", e);
        return;
    }

    let plist_str = plist_path.to_string_lossy();
    let _ = Command::new("launchctl").args(["unload", &plist_str]).status();
    let status = Command::new("launchctl").args(["load", &plist_str]).status();

    match status {
        Ok(s) if s.success() => {
            println!("\x1b[32m[OK]\x1b[0m     macOS LaunchAgent installed and loaded successfully!");
            println!("\x1b[36m[INFO]\x1b[0m   Hybrid service active (event-driven Wi-Fi triggers + 30s session watchdog)");
            println!("\x1b[36m[INFO]\x1b[0m   Logs: {}", LOG_PATH);
        }
        _ => {
            println!("\x1b[31m[FAIL]\x1b[0m   Failed to load LaunchAgent with launchctl");
        }
    }
}

#[cfg(target_os = "macos")]
fn uninstall_macos_launch_agent() {
    let plist_path = get_launch_agent_path();
    let plist_str = plist_path.to_string_lossy();
    let _ = Command::new("launchctl").args(["unload", &plist_str]).status();
    let _ = fs::remove_file(&plist_path);
    println!("\x1b[32m[OK]\x1b[0m     macOS LaunchAgent removed successfully");
}

#[cfg(target_os = "linux")]
fn get_systemd_service_path() -> PathBuf {
    let home = env::var("HOME").unwrap_or_else(|_| "/tmp".to_string());
    PathBuf::from(home)
        .join(".config")
        .join("systemd")
        .join("user")
        .join("fortinet-autologin.service")
}

#[cfg(target_os = "linux")]
fn install_linux_systemd_service() {
    let service_path = get_systemd_service_path();
    if let Some(parent) = service_path.parent() {
        let _ = fs::create_dir_all(parent);
    }

    let exe = get_exe_path();
    let exe_str = exe.to_string_lossy();

    let unit_content = format!(
        r#"[Unit]
Description=Fortinet Captive Portal Auto-Login Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={} -daemon -interval 30s
Restart=always
RestartSec=5
StandardOutput=append:{}
StandardError=append:{}

[Install]
WantedBy=default.target
"#,
        exe_str, LOG_PATH, LOG_PATH
    );

    if let Err(e) = fs::write(&service_path, unit_content) {
        println!("\x1b[31m[FAIL]\x1b[0m   Failed to write systemd unit: {}", e);
        return;
    }

    let _ = Command::new("systemctl").args(["--user", "daemon-reload"]).status();
    let status = Command::new("systemctl")
        .args(["--user", "enable", "--now", "fortinet-autologin.service"])
        .status();

    match status {
        Ok(s) if s.success() => {
            println!("\x1b[32m[OK]\x1b[0m     Linux systemd service installed and started successfully!");
            println!("\x1b[36m[INFO]\x1b[0m   Logs: {}", LOG_PATH);
        }
        _ => {
            println!("\x1b[31m[FAIL]\x1b[0m   Failed to enable systemd service");
        }
    }
}

#[cfg(target_os = "linux")]
fn uninstall_linux_systemd_service() {
    let _ = Command::new("systemctl")
        .args(["--user", "disable", "--now", "fortinet-autologin.service"])
        .status();
    let service_path = get_systemd_service_path();
    let _ = fs::remove_file(service_path);
    let _ = Command::new("systemctl").args(["--user", "daemon-reload"]).status();
    println!("\x1b[32m[OK]\x1b[0m     Linux systemd service uninstalled successfully");
}

pub fn view_logs(follow: bool, lines: usize) {
    let log_path = PathBuf::from(LOG_PATH);
    if !log_path.exists() {
        println!("\x1b[33m[INFO]\x1b[0m   No logs found yet at {}", LOG_PATH);
        return;
    }

    if follow {
        println!("\x1b[36m[LOGS]\x1b[0m   Streaming {} live (Ctrl+C to exit)...", LOG_PATH);
        let _ = Command::new("tail")
            .args(["-f", "-n", &lines.to_string(), LOG_PATH])
            .status();
    } else {
        if let Ok(file) = File::open(&log_path) {
            let reader = BufReader::new(file);
            let all_lines: Vec<String> = reader.lines().filter_map(|l| l.ok()).collect();
            let start = if all_lines.len() > lines && lines > 0 {
                all_lines.len() - lines
            } else {
                0
            };

            for line in &all_lines[start..] {
                println!("{}", line);
            }
        }
    }
}

pub fn clear_logs() {
    let log_path = PathBuf::from(LOG_PATH);
    if let Ok(file) = File::create(&log_path) {
        drop(file);
        println!("\x1b[32m[OK]\x1b[0m     Background logs cleared ({})", LOG_PATH);
    } else {
        println!("\x1b[31m[FAIL]\x1b[0m   Failed to clear log file");
    }
}
