use std::net::UdpSocket;
use std::process::Command;
use std::thread;
use std::time::Duration;

use crate::detect::detect_fortinet_network;
use crate::logf;
use crate::network::login;

pub const WARP_ANYCAST_IPS: [&str; 12] = [
    "162.159.192.1",
    "162.159.192.2",
    "162.159.192.3",
    "162.159.193.1",
    "162.159.193.2",
    "162.159.195.1",
    "188.114.96.1",
    "188.114.96.2",
    "188.114.97.1",
    "188.114.97.2",
    "188.114.98.1",
    "188.114.99.1",
];

pub const WARP_PORTS: [&str; 6] = ["500", "4500", "1701", "854", "880", "890"];

pub fn generate_candidate_endpoints() -> Vec<String> {
    let mut candidates = vec![
        "162.159.192.1:500".to_string(),
        "162.159.192.1:4500".to_string(),
        "162.159.193.1:500".to_string(),
        "162.159.193.1:4500".to_string(),
        "188.114.96.1:500".to_string(),
        "188.114.97.1:4500".to_string(),
    ];

    let mut pool = Vec::new();
    for ip in &WARP_ANYCAST_IPS {
        for port in &WARP_PORTS {
            let ep = format!("{}:{}", ip, port);
            if !candidates.contains(&ep) {
                pool.push(ep);
            }
        }
    }

    // Pseudo-random shuffle based on timestamp
    let seed = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let mut state = seed as u64;
    for i in (1..pool.len()).rev() {
        state = state.wrapping_mul(6364136223846793005).wrapping_add(1);
        let j = (state as usize) % (i + 1);
        pool.swap(i, j);
    }

    candidates.extend(pool);
    candidates
}

pub fn preflight_probe(endpoint: &str) -> bool {
    let socket = match UdpSocket::bind("0.0.0.0:0") {
        Ok(s) => s,
        Err(_) => return false,
    };
    let _ = socket.set_read_timeout(Some(Duration::from_millis(400)));
    let _ = socket.set_write_timeout(Some(Duration::from_millis(400)));
    socket.connect(endpoint).is_ok()
}

pub fn test_internet_connectivity(timeout: Duration) -> bool {
    use ureq::config::Config;
    let config = Config::builder()
        .timeout_global(Some(timeout))
        .max_redirects(0)
        .build();
    let client: ureq::Agent = config.into();

    // Probe 1: Direct Cloudflare trace IP query
    if let Ok(res) = client.get("https://1.1.1.1/cdn-cgi/trace").call() {
        if res.status() == 200 {
            return true;
        }
    }

    // Probe 2: Standard Firefox captive portal probe
    if let Ok(res) = client
        .get("http://detectportal.firefox.com/success.txt")
        .call()
    {
        if res.status() == 200 {
            return true;
        }
    }

    // Probe 3: Cloudflare 204
    if let Ok(res) = client.get("https://cp.cloudflare.com/generate_204").call() {
        if res.status() == 200 || res.status() == 204 {
            return true;
        }
    }

    false
}

pub fn cycle_wifi() -> bool {
    #[cfg(target_os = "macos")]
    {
        cycle_wifi_darwin()
    }

    #[cfg(target_os = "linux")]
    {
        cycle_wifi_linux()
    }

    #[cfg(target_os = "windows")]
    {
        cycle_wifi_windows()
    }

    #[cfg(not(any(target_os = "macos", target_os = "linux", target_os = "windows")))]
    {
        false
    }
}

#[cfg(target_os = "macos")]
fn cycle_wifi_darwin() -> bool {
    let mut wifi_device = "en0".to_string();
    if let Ok(output) = Command::new("networksetup")
        .arg("-listallhardwareports")
        .output()
    {
        let text = String::from_utf8_lossy(&output.stdout);
        let lines: Vec<&str> = text.lines().collect();
        for i in 0..lines.len() {
            if (lines[i].contains("Wi-Fi") || lines[i].contains("AirPort"))
                && i + 1 < lines.len()
                && lines[i + 1].contains("Device:")
            {
                let parts: Vec<&str> = lines[i + 1].split_whitespace().collect();
                if parts.len() >= 2 {
                    wifi_device = parts[1].to_string();
                    break;
                }
            }
        }
    }

    let off = Command::new("networksetup")
        .args(["-setairportpower", &wifi_device, "off"])
        .status();
    if off.is_err() {
        return false;
    }

    thread::sleep(Duration::from_secs(2));

    let on = Command::new("networksetup")
        .args(["-setairportpower", &wifi_device, "on"])
        .status();
    matches!(on, Ok(s) if s.success())
}

#[cfg(target_os = "linux")]
fn cycle_wifi_linux() -> bool {
    if let Ok(s) = Command::new("nmcli")
        .args(["radio", "wifi", "off"])
        .status()
    {
        if s.success() {
            thread::sleep(Duration::from_secs(2));
            return Command::new("nmcli")
                .args(["radio", "wifi", "on"])
                .status()
                .map(|s| s.success())
                .unwrap_or(false);
        }
    }

    if let Ok(s) = Command::new("rfkill").args(["block", "wifi"]).status() {
        if s.success() {
            thread::sleep(Duration::from_secs(2));
            return Command::new("rfkill")
                .args(["unblock", "wifi"])
                .status()
                .map(|s| s.success())
                .unwrap_or(false);
        }
    }

    false
}

#[cfg(target_os = "windows")]
fn cycle_wifi_windows() -> bool {
    let ps_script = "Get-NetAdapter -Name *Wi-Fi* | Disable-NetAdapter -Confirm:$false; Start-Sleep -Seconds 2; Get-NetAdapter -Name *Wi-Fi* | Enable-NetAdapter -Confirm:$false";
    let status = Command::new("powershell")
        .args(["-NoProfile", "-NonInteractive", "-Command", ps_script])
        .status();
    if let Ok(s) = status {
        if s.success() {
            return true;
        }
    }

    let _ = Command::new("netsh")
        .args(["interface", "set", "interface", "Wi-Fi", "disable"])
        .status();
    thread::sleep(Duration::from_secs(2));
    Command::new("netsh")
        .args(["interface", "set", "interface", "Wi-Fi", "enable"])
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}

fn recover_via_wifi_cycle(username: &str, password: &str) -> bool {
    logf!("\x1b[33m[WARN]\x1b[0m   WARP tunnel connected but internet traffic is blackholed by FortiGate.");
    logf!("\x1b[36m[ACTION]\x1b[0m Automatically restarting Wi-Fi to renew DHCP lease and reset firewall session table...");

    let _ = Command::new("warp-cli").arg("disconnect").status();
    thread::sleep(Duration::from_millis(500));

    if !cycle_wifi() {
        logf!("\x1b[33m[INFO]\x1b[0m   Could not toggle Wi-Fi automatically (system permission required).");
        logf!(
            "         Please toggle Wi-Fi OFF and ON in system settings to refresh your DHCP IP,"
        );
        logf!("         then re-run './autologin' and './autologin -warp'.");
        return false;
    }

    logf!("\x1b[32m[OK]\x1b[0m     Wi-Fi restarted. Waiting 6s for network association & DHCP lease...");
    thread::sleep(Duration::from_secs(6));

    let det = detect_fortinet_network();
    if det.is_fortinet_network && !det.internet_connected {
        logf!("\x1b[34m[AUTH]\x1b[0m   Re-authenticating with Fortinet captive portal...");
        if !login(false, false, username, password) {
            logf!("\x1b[33m[WARN]\x1b[0m   Portal login pending. Retrying in 2s...");
            thread::sleep(Duration::from_secs(2));
            let _ = login(false, false, username, password);
        }
    }

    let recovery_endpoints = [
        "162.159.192.1:500",
        "162.159.192.1:4500",
        "162.159.193.1:500",
    ];
    logf!(
        "\x1b[36m[WARP]\x1b[0m   Retrying Cloudflare WARP with fresh IP and clean session table..."
    );
    for ep in &recovery_endpoints {
        let _ = Command::new("warp-cli")
            .args(["tunnel", "endpoint", "set", ep])
            .status();
        let _ = Command::new("warp-cli").arg("connect").status();

        for _ in 0..5 {
            thread::sleep(Duration::from_millis(800));
            if let Ok(output) = Command::new("warp-cli").arg("status").output() {
                let status_str = String::from_utf8_lossy(&output.stdout);
                if status_str.contains("Connected")
                    && test_internet_connectivity(Duration::from_secs(3))
                {
                    logf!("\x1b[32m[OK]\x1b[0m     Success! Reconnected to Cloudflare WARP via {} (Internet verified active)", ep);
                    return true;
                }
            }
        }
        let _ = Command::new("warp-cli").arg("disconnect").status();
    }

    logf!("\x1b[31m[FAIL]\x1b[0m   Unable to restore WARP connection. Tunnel safely disconnected.");
    false
}

pub fn fix_and_connect_warp(username: &str, password: &str) -> bool {
    // Step 1: Detect Fortinet network presence
    logf!("\x1b[36m[DETECT]\x1b[0m Checking network environment and Fortinet presence...");
    let det = detect_fortinet_network();
    if det.is_fortinet_network {
        let mut info = String::new();
        if let Some(ref dev) = det.device_serial_number {
            info = format!(
                " (Device ID: {}, Gateway: {})",
                dev,
                det.gateway_ip.as_deref().unwrap_or("")
            );
        } else if let Some(ref gw) = det.gateway_ip {
            info = format!(" (Gateway: {})", gw);
        }
        logf!("\x1b[32m[OK]\x1b[0m     Fortinet network detected{}", info);

        if !det.internet_connected {
            logf!("\x1b[33m[AUTH]\x1b[0m   Captive portal is not authenticated yet. Logging in...");
            if !login(false, false, username, password) {
                logf!("\x1b[31m[FAIL]\x1b[0m   Could not authenticate with Fortinet captive portal. Run './autologin' first.");
                return false;
            }
            logf!("\x1b[32m[OK]\x1b[0m     Captive portal authenticated successfully.");
        }
    } else {
        logf!("\x1b[33m[WARN]\x1b[0m   No Fortinet gateway signature detected on this network.");
        logf!("         (WARP auto-rotation is specifically optimized for evading Fortinet firewall restrictions).");
    }

    // Step 2: Check warp-cli presence
    let status = Command::new("warp-cli").arg("--version").output();
    if status.is_err() {
        logf!("\x1b[31m[FAIL]\x1b[0m   warp-cli not found in system PATH. Please install Cloudflare WARP.");
        return false;
    }

    let _ = Command::new("warp-cli")
        .args(["debug", "connectivity-check", "disable"])
        .status();
    let _ = Command::new("warp-cli")
        .args(["tunnel", "protocol", "set", "WireGuard"])
        .status();

    // Step 3: Check if already connected and verify live internet
    if let Ok(output) = Command::new("warp-cli").arg("status").output() {
        let status_str = String::from_utf8_lossy(&output.stdout);
        if status_str.contains("Connected") {
            logf!("  Checking existing WARP connection health...");
            if test_internet_connectivity(Duration::from_secs(3)) {
                logf!("\x1b[32m[OK]\x1b[0m     WARP is already connected and internet is fully reachable!");
                return true;
            }
            logf!("\x1b[33m[WARN]\x1b[0m   WARP is connected but internet traffic is stalled. Disconnecting to rotate...");
            let _ = Command::new("warp-cli").arg("disconnect").status();
            thread::sleep(Duration::from_millis(200));
        }
    }

    logf!("\x1b[36m[WARP]\x1b[0m   Initializing intelligent Cloudflare WARP auto-rotation...");
    let endpoints = generate_candidate_endpoints();
    logf!(
        "  Generated {} candidate endpoint combinations across {} Anycast IP pools",
        endpoints.len(),
        WARP_ANYCAST_IPS.len()
    );

    // Step 4: Rotate candidates
    for (i, ep) in endpoints.iter().enumerate() {
        if i >= 8 {
            break;
        }

        if !preflight_probe(ep) {
            continue;
        }

        logf!(
            "\x1b[34m[WARP]\x1b[0m   Testing endpoint [{}/8]: {}...",
            i + 1,
            ep
        );
        let _ = Command::new("warp-cli").arg("disconnect").status();
        thread::sleep(Duration::from_millis(100));

        let _ = Command::new("warp-cli")
            .args(["tunnel", "endpoint", "set", ep])
            .status();
        let _ = Command::new("warp-cli").arg("connect").status();

        let mut connected = false;
        for attempt in 0..6 {
            thread::sleep(Duration::from_millis(500));
            if let Ok(output) = Command::new("warp-cli").arg("status").output() {
                let status_str = String::from_utf8_lossy(&output.stdout);
                if status_str.contains("Connected") {
                    if let Ok(stats_out) =
                        Command::new("warp-cli").args(["tunnel", "stats"]).output()
                    {
                        let stats_str = String::from_utf8_lossy(&stats_out.stdout);
                        if !stats_str.contains("Received: 0B") || attempt >= 3 {
                            connected = true;
                            break;
                        }
                    }
                }
            }
        }

        if connected {
            logf!("  Verifying internet reachability through {}...", ep);
            if test_internet_connectivity(Duration::from_secs(3)) {
                logf!("\x1b[32m[OK]\x1b[0m     Success! Connected to Cloudflare WARP via {} (Internet verified active)", ep);
                return true;
            }
            logf!("\x1b[33m[WARN]\x1b[0m   Endpoint {} connected to tunnel but internet is unreachable (UDP blackholed).", ep);
        } else {
            logf!(
                "\x1b[33m[WARN]\x1b[0m   Endpoint {} timed out. Trying next candidate...",
                ep
            );
        }
    }

    // Step 5: Tunnel key rotation attempt
    logf!("\x1b[34m[WARP]\x1b[0m   Rotating tunnel cryptographic keys...");
    let _ = Command::new("warp-cli")
        .args(["tunnel", "rotate-keys"])
        .status();
    let _ = Command::new("warp-cli")
        .args(["tunnel", "endpoint", "set", "162.159.192.1:500"])
        .status();
    let _ = Command::new("warp-cli").arg("connect").status();

    thread::sleep(Duration::from_secs(2));
    if let Ok(output) = Command::new("warp-cli").arg("status").output() {
        let status_str = String::from_utf8_lossy(&output.stdout);
        if status_str.contains("Connected") && test_internet_connectivity(Duration::from_secs(3)) {
            logf!("\x1b[32m[OK]\x1b[0m     Connected after tunnel key rotation (Internet verified active)!");
            return true;
        }
    }

    // Step 6: If all endpoints fail or internet is unreachable, trigger Wi-Fi toggle recovery
    recover_via_wifi_cycle(username, password)
}

pub fn disconnect_warp() -> bool {
    logf!("\x1b[34m[WARP]\x1b[0m   Disconnecting Cloudflare WARP...");
    let out = Command::new("warp-cli").arg("disconnect").output();
    match out {
        Ok(res) if res.status.success() => {
            logf!("\x1b[32m[OK]\x1b[0m     Cloudflare WARP disconnected. Normal routing restored.");
            true
        }
        Ok(res) => {
            logf!(
                "\x1b[31m[FAIL]\x1b[0m   Failed to disconnect: {}",
                String::from_utf8_lossy(&res.stderr)
            );
            false
        }
        Err(e) => {
            logf!("\x1b[31m[FAIL]\x1b[0m   Error executing warp-cli: {}", e);
            false
        }
    }
}
