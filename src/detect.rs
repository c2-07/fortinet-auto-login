use std::net::{TcpStream, ToSocketAddrs};
use std::process::Command;
use std::sync::{LazyLock, Mutex};
use std::time::{Duration, Instant};

use regex::Regex;

use crate::network::{get_portal, make_client, parse_host_from_url, parse_magic_from_url};
use crate::session::load_session;
use crate::types::FortinetDetectionResult;

static CACHED_DETECTION: Mutex<Option<(FortinetDetectionResult, Instant)>> = Mutex::new(None);
static CACHED_GATEWAY: Mutex<Option<(String, Instant)>> = Mutex::new(None);
const CACHE_TTL: Duration = Duration::from_secs(20);

static SERIAL_RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"(FG[0-9A-Z]{14})").unwrap());

#[allow(dead_code)]
pub fn invalidate_detection_cache() {
    let mut lock = CACHED_DETECTION.lock().unwrap();
    *lock = None;
    let mut gw_lock = CACHED_GATEWAY.lock().unwrap();
    *gw_lock = None;
}

pub fn get_default_gateway_hint() -> Option<String> {
    {
        let lock = CACHED_GATEWAY.lock().unwrap();
        if let Some((ref gw, instant)) = *lock {
            if instant.elapsed() < CACHE_TTL {
                return Some(gw.clone());
            }
        }
    }

    let gw = resolve_gateway_from_system();
    if let Some(ref val) = gw {
        let mut lock = CACHED_GATEWAY.lock().unwrap();
        *lock = Some((val.clone(), Instant::now()));
    }
    gw
}

fn resolve_gateway_from_system() -> Option<String> {
    #[cfg(target_os = "macos")]
    {
        if let Ok(output) = Command::new("route")
            .args(["-n", "get", "default"])
            .output()
        {
            let text = String::from_utf8_lossy(&output.stdout);
            for line in text.lines() {
                let trimmed = line.trim();
                if trimmed.starts_with("gateway:") {
                    let parts: Vec<&str> = trimmed.split_whitespace().collect();
                    if parts.len() >= 2 {
                        return Some(parts[1].to_string());
                    }
                }
            }
        }
    }

    #[cfg(target_os = "linux")]
    {
        if let Ok(output) = Command::new("ip")
            .args(["route", "show", "default"])
            .output()
        {
            let text = String::from_utf8_lossy(&output.stdout);
            let parts: Vec<&str> = text.split_whitespace().collect();
            for i in 0..parts.len() {
                if parts[i] == "via" && i + 1 < parts.len() {
                    return Some(parts[i + 1].to_string());
                }
            }
        }
    }

    #[cfg(target_os = "windows")]
    {
        if let Ok(output) = Command::new("route").args(["print", "0.0.0.0"]).output() {
            let text = String::from_utf8_lossy(&output.stdout);
            for line in text.lines() {
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 3 && parts[0] == "0.0.0.0" && parts[1] == "0.0.0.0" {
                    return Some(parts[2].to_string());
                }
            }
        }
    }

    None
}

pub fn detect_fortinet_network() -> FortinetDetectionResult {
    let mut result = FortinetDetectionResult::default();
    let client = make_client();

    // Probe 1: Check live captive portal redirect and general connectivity
    if let Ok((portal_url, already_connected)) = get_portal(&client) {
        result.internet_connected = already_connected;
        if let Some(portal) = portal_url {
            result.is_fortinet_network = true;
            result.detection_method = Some("Captive Portal Redirect".to_string());
            result.captive_portal_host = parse_host_from_url(&portal);
            result.magic_token = parse_magic_from_url(&portal);

            update_cache(&result);
            return result;
        }

        // If internet is active, reuse cached Fortinet network identity to avoid repeated TCP probes
        if already_connected {
            let lock = CACHED_DETECTION.lock().unwrap();
            if let Some((ref cached, instant)) = *lock {
                if instant.elapsed() < CACHE_TTL && cached.is_fortinet_network {
                    let mut res = cached.clone();
                    res.internet_connected = true;
                    return res;
                }
            }
        }
    }

    // Probe 2: Gateway port & TLS probe
    let config = crate::config::load_config();
    let mut gateway_candidates = vec![config.portal_host.clone()];
    if let Some(gw) = get_default_gateway_hint() {
        for port in &config.gateway_ports {
            gateway_candidates.insert(0, format!("{}:{}", gw, port));
        }
    }

    for target in &gateway_candidates {
        // Quick TCP probe (handles both IP:port and host:port)
        if let Ok(mut addrs) = target.to_socket_addrs() {
            if let Some(addr) = addrs.next() {
                if TcpStream::connect_timeout(&addr, Duration::from_millis(500)).is_err() {
                    continue;
                }
            }
        }

        // Test 1: HTTP portal redirect probe on port 1000 (responds in <20ms)
        let gw_ip = target.split(':').next().unwrap_or(target);
        let http_probe_url = format!("http://{}:1000/", gw_ip);
        if let Ok(mut resp) = client.get(&http_probe_url).call() {
            let body = resp.body_mut().read_to_string().unwrap_or_default();
            if body.contains("fgtauth") || body.to_lowercase().contains("fortigate") {
                result.is_fortinet_network = true;
                result.gateway_ip = Some(target.clone());
                result.detection_method = Some("Gateway Captive Portal Signature".to_string());

                if let Some(caps) = SERIAL_RE.captures(&body) {
                    if let Some(m) = caps.get(1) {
                        result.device_serial_number = Some(m.as_str().to_string());
                    }
                }

                update_cache(&result);
                return result;
            }
        }

        // Test 2: HTTP handshake on the candidate port (for non-standard portals)
        let probe_url = format!("https://{}/fgtauth", target);
        if let Ok(mut resp) = client.get(&probe_url).call() {
            let body = resp.body_mut().read_to_string().unwrap_or_default();
            let body_lower = body.to_lowercase();
            if body_lower.contains("fortigate")
                || body_lower.contains("fortinet")
                || body.contains("FG6H0")
            {
                result.is_fortinet_network = true;
                result.gateway_ip = Some(target.clone());
                result.detection_method = Some("Gateway TLS Service Signature".to_string());

                // Extract serial number if present
                if let Some(caps) = SERIAL_RE.captures(&body) {
                    if let Some(m) = caps.get(1) {
                        result.device_serial_number = Some(m.as_str().to_string());
                    }
                }

                update_cache(&result);
                return result;
            }
        }
    }

    // Probe 3: Check active authenticated session cache
    if result.internet_connected {
        if let Some(s) = load_session() {
            if !s.host.is_empty() {
                result.is_fortinet_network = true;
                result.captive_portal_host = Some(s.host.clone());
                result.gateway_ip = Some(s.host);
                result.detection_method = Some("Active Authenticated Session Cache".to_string());
                update_cache(&result);
                return result;
            }
        }
    }

    update_cache(&result);
    result
}

fn update_cache(result: &FortinetDetectionResult) {
    let mut lock = CACHED_DETECTION.lock().unwrap();
    *lock = Some((result.clone(), Instant::now()));
}

pub fn print_status() {
    let res = detect_fortinet_network();

    println!("\x1b[36m--- Network & Fortinet Status ---\x1b[0m");
    if res.internet_connected {
        println!("Internet:         \x1b[32mConnected\x1b[0m");
    } else if res.captive_portal_host.is_some() {
        println!("Internet:         \x1b[33mCaptive Portal Intercepted\x1b[0m");
    } else {
        println!("Internet:         \x1b[31mDisconnected / Offline\x1b[0m");
    }

    if res.is_fortinet_network {
        println!(
            "Fortinet Network: \x1b[32mYes\x1b[0m (Method: {})",
            res.detection_method.as_deref().unwrap_or("Unknown")
        );
        if let Some(ref gw) = res.gateway_ip {
            println!("Gateway IP:       {}", gw);
        }
        if let Some(ref dev) = res.device_serial_number {
            println!("Device ID:        {}", dev);
        }
    } else {
        println!("Fortinet Network: \x1b[33mNo Fortinet gateway signature detected\x1b[0m");
    }

    println!("\n\x1b[36m--- Active Session Cache ---\x1b[0m");
    if let Some(session) = load_session() {
        println!("Host:             https://{}", session.host);
        println!("Magic:            {}", session.magic);
        println!("Timeout:          {}s", session.timeout);
    } else {
        println!("Active Session:   None (not currently logged in)");
    }

    if let Some(creds) = crate::credentials::load_credentials() {
        println!("\nSaved User:       {}", creds.username);
    } else {
        println!("\nSaved User:       None");
    }
}
