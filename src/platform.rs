use std::env;
#[cfg(any(target_os = "macos", target_os = "linux"))]
use std::fs;
use std::fs::File;
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Command, Stdio};

pub fn get_log_path() -> PathBuf {
    #[cfg(windows)]
    {
        env::var("TEMP")
            .or_else(|_| env::var("TMP"))
            .map(PathBuf::from)
            .unwrap_or_else(|_| env::temp_dir())
            .join("autologin.log")
    }
    #[cfg(not(windows))]
    {
        PathBuf::from("/tmp/autologin.log")
    }
}

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

    #[cfg(target_os = "windows")]
    {
        install_windows_task();
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        println!("\x1b[31m[FAIL]\x1b[0m   Automatic background service installation is supported on macOS, Linux, and Windows.");
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

    #[cfg(target_os = "windows")]
    {
        uninstall_windows_task();
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        println!("\x1b[31m[FAIL]\x1b[0m   Automatic background service removal is supported on macOS, Linux, and Windows.");
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
    let log_path = get_log_path();
    let log_str = log_path.to_string_lossy();

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
        exe_str, log_str, log_str
    );

    if let Err(e) = fs::write(&plist_path, plist_content) {
        println!(
            "\x1b[31m[FAIL]\x1b[0m   Failed to write LaunchAgent plist: {}",
            e
        );
        return;
    }

    let plist_str = plist_path.to_string_lossy();
    let _ = Command::new("launchctl")
        .args(["unload", &plist_str])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    let status = Command::new("launchctl")
        .args(["load", &plist_str])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    match status {
        Ok(s) if s.success() => {
            println!(
                "\x1b[32m[OK]\x1b[0m     macOS LaunchAgent installed and loaded successfully!"
            );
            println!("\x1b[36m[INFO]\x1b[0m   Hybrid service active (event-driven Wi-Fi triggers + 30s session watchdog)");
            println!("\x1b[36m[INFO]\x1b[0m   Logs: {}", log_str);
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
    let _ = Command::new("launchctl")
        .args(["unload", &plist_str])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
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
    let log_path = get_log_path();
    let log_str = log_path.to_string_lossy();

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
        exe_str, log_str, log_str
    );

    if let Err(e) = fs::write(&service_path, unit_content) {
        println!(
            "\x1b[31m[FAIL]\x1b[0m   Failed to write systemd unit: {}",
            e
        );
        return;
    }

    let _ = Command::new("systemctl")
        .args(["--user", "daemon-reload"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    let status = Command::new("systemctl")
        .args(["--user", "enable", "--now", "fortinet-autologin.service"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    match status {
        Ok(s) if s.success() => {
            println!(
                "\x1b[32m[OK]\x1b[0m     Linux systemd service installed and started successfully!"
            );
            println!("\x1b[36m[INFO]\x1b[0m   Logs: {}", log_str);
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
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    let service_path = get_systemd_service_path();
    let _ = fs::remove_file(service_path);
    let _ = Command::new("systemctl")
        .args(["--user", "daemon-reload"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    println!("\x1b[32m[OK]\x1b[0m     Linux systemd service uninstalled successfully");
}

#[cfg(target_os = "windows")]
fn install_windows_task() {
    let exe = get_exe_path();
    let exe_str = exe.to_string_lossy();
    let tr_arg = format!("\"{}\" -daemon -interval 30s", exe_str);

    let status = Command::new("schtasks")
        .args([
            "/create",
            "/tn",
            "FortinetAutoLogin",
            "/tr",
            &tr_arg,
            "/sc",
            "onlogon",
            "/f",
        ])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    match status {
        Ok(s) if s.success() => {
            let _ = Command::new("schtasks")
                .args(["/run", "/tn", "FortinetAutoLogin"])
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .status();

            println!(
                "\x1b[32m[OK]\x1b[0m     Windows Task Scheduler background service installed and started!"
            );
            println!(
                "\x1b[36m[INFO]\x1b[0m   Runs automatically at logon with 30s session watchdog"
            );
            println!("\x1b[36m[INFO]\x1b[0m   Logs: {}", get_log_path().display());
        }
        _ => {
            println!("\x1b[31m[FAIL]\x1b[0m   Failed to create scheduled task via schtasks");
        }
    }
}

#[cfg(target_os = "windows")]
fn uninstall_windows_task() {
    let _ = Command::new("schtasks")
        .args(["/end", "/tn", "FortinetAutoLogin"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    let status = Command::new("schtasks")
        .args(["/delete", "/tn", "FortinetAutoLogin", "/f"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    match status {
        Ok(s) if s.success() => {
            println!("\x1b[32m[OK]\x1b[0m     Windows background service uninstalled successfully");
        }
        _ => {
            println!("\x1b[31m[FAIL]\x1b[0m   Failed to remove scheduled task via schtasks");
        }
    }
}

pub fn view_logs(follow: bool, lines: usize) {
    let log_path = get_log_path();
    if !log_path.exists() {
        println!(
            "\x1b[33m[INFO]\x1b[0m   No logs found yet at {}",
            log_path.display()
        );
        return;
    }

    if follow {
        println!(
            "\x1b[36m[LOGS]\x1b[0m   Streaming {} live (Ctrl+C to exit)...",
            log_path.display()
        );
        #[cfg(not(target_os = "windows"))]
        {
            let _ = Command::new("tail")
                .args(["-f", "-n", &lines.to_string(), &log_path.to_string_lossy()])
                .status();
        }
        #[cfg(target_os = "windows")]
        {
            let cmd = format!(
                "Get-Content -Path '{}' -Tail {} -Wait",
                log_path.to_string_lossy(),
                lines
            );
            let _ = Command::new("powershell")
                .args(["-NoProfile", "-Command", &cmd])
                .status();
        }
    } else {
        if let Ok(file) = File::open(&log_path) {
            let reader = BufReader::new(file);
            let all_lines: Vec<String> = reader.lines().map_while(Result::ok).collect();
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
    let log_path = get_log_path();
    if let Ok(file) = File::create(&log_path) {
        drop(file);
        println!(
            "\x1b[32m[OK]\x1b[0m     Background logs cleared ({})",
            log_path.display()
        );
    } else {
        println!("\x1b[31m[FAIL]\x1b[0m   Failed to clear log file");
    }
}
