use std::process::Command;

fn get_bin_path() -> String {
    // Determine path to built binary (debug or release)
    let debug_bin = "./target/debug/autologin";
    let release_bin = "./target/release/autologin";
    if std::path::Path::new(debug_bin).exists() {
        debug_bin.to_string()
    } else {
        release_bin.to_string()
    }
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
    let output = Command::new(&bin)
        .arg("-status")
        .output()
        .expect("execute autologin -status");

    assert!(output.status.success(), "Exit code must be 0 for -status");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("--- Network & Fortinet Status ---"));
    assert!(stdout.contains("Internet:"));
    assert!(stdout.contains("Fortinet Network:"));
    assert!(stdout.contains("--- Active Session Cache ---"));
}
