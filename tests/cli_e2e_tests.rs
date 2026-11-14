use std::process::Command;

fn get_bin_path() -> String {
    env!("CARGO_BIN_EXE_autologin").to_string()
}

#[test]
fn test_cli_help_flag() {
    let bin = get_bin_path();
    let output = Command::new(&bin)
        .arg("-h")
        .output()
        .expect("execute autologin -h");

    assert!(output.status.success(), "Exit code must be 0 for -h");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("Usage: autologin [options]"));
    assert!(stdout.contains("Authentication & Session:"));
    assert!(stdout.contains("Network & Diagnostics:"));
    assert!(stdout.contains("Background Service & Daemon:"));
    assert!(stdout.contains("Cloudflare WARP Circumvention:"));
    assert!(stdout.contains("Logs & Maintenance:"));
}

#[test]
fn test_cli_version_flag() {
    let bin = get_bin_path();
    let output = Command::new(&bin)
        .arg("-version")
        .output()
        .expect("execute autologin -version");

    assert!(output.status.success(), "Exit code must be 0 for -version");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("fortinet-auto-login v1.0.0 (rust)"));
}

#[test]
fn test_cli_invalid_flag() {
    let bin = get_bin_path();
    let output = Command::new(&bin)
        .arg("-nonexistent-flag-xyz")
        .output()
        .expect("execute autologin with invalid flag");

    assert!(
        !output.status.success(),
        "Exit code must be non-zero for invalid flag"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("Unknown option: -nonexistent-flag-xyz"));
}

#[test]
fn test_cli_status_mode() {
    let bin = get_bin_path();
    let temp_dir =
        std::env::temp_dir().join(format!("test_fortinet_status_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&temp_dir);
    std::fs::create_dir_all(&temp_dir).expect("create temp dir");

    let output = Command::new(&bin)
        .arg("-status")
        .env("FORTINET_CONFIG_DIR", &temp_dir)
        .output()
        .expect("execute autologin -status");

    let _ = std::fs::remove_dir_all(&temp_dir);

    assert!(output.status.success(), "Exit code must be 0 for -status");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("--- Network & Fortinet Status ---"));
    assert!(stdout.contains("Internet:"));
    assert!(stdout.contains("Fortinet Network:"));
    assert!(stdout.contains("--- Active Session Cache ---"));
}

#[test]
fn test_cli_config_commands() {
    let bin = get_bin_path();
    let temp_dir = std::env::temp_dir().join(format!("test_fortinet_cfg_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&temp_dir);
    std::fs::create_dir_all(&temp_dir).expect("create temp dir");

    // 1. Show config
    let output = Command::new(&bin)
        .arg("-config")
        .env("FORTINET_CONFIG_DIR", &temp_dir)
        .output()
        .expect("execute autologin -config");
    assert!(output.status.success());
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("=== Campus & Engine Configuration ==="));
    assert!(stdout.contains("Campus Gateway Host:"));
    assert!(stdout.contains("Connectivity Check Endpoints"));

    // 2. Set custom host
    let set_out = Command::new(&bin)
        .args(["-config", "set-host", "10.0.0.1:1003"])
        .env("FORTINET_CONFIG_DIR", &temp_dir)
        .output()
        .expect("set host");
    assert!(set_out.status.success());
    let set_str = String::from_utf8_lossy(&set_out.stdout);
    assert!(set_str.contains("Updated campus gateway host to: 10.0.0.1:1003"));

    // Verify change took effect in show
    let show_out = Command::new(&bin)
        .arg("-config")
        .env("FORTINET_CONFIG_DIR", &temp_dir)
        .output()
        .expect("show");
    let show_str = String::from_utf8_lossy(&show_out.stdout);
    assert!(show_str.contains("10.0.0.1:1003"));

    // 3. Reset config
    let reset_out = Command::new(&bin)
        .args(["-config", "reset"])
        .env("FORTINET_CONFIG_DIR", &temp_dir)
        .output()
        .expect("reset");
    assert!(reset_out.status.success());
    let reset_str = String::from_utf8_lossy(&reset_out.stdout);
    assert!(reset_str.contains("Configuration reset to factory defaults"));

    let _ = std::fs::remove_dir_all(&temp_dir);
}
