use std::net::TcpStream;
use std::process::Command;
use std::sync::Mutex;
use std::time::{Duration, Instant};

use regex::Regex;

use crate::network::{get_portal, make_client, parse_host_from_url, parse_magic_from_url, FALLBACK_HOST};
use crate::session::load_session;
use crate::types::FortinetDetectionResult;

static CACHED_DETECTION: Mutex<Option<(FortinetDetectionResult, Instant)>> = Mutex::new(None);
const CACHE_TTL: Duration = Duration::from_secs(20);

#[allow(dead_code)]
pub fn invalidate_detection_cache() {
    let mut lock = CACHED_DETECTION.lock().unwrap();
    *lock = None;
}

pub fn get_default_gateway_hint() -> Option<String> {
    #[cfg(target_os = "macos")]
    {
        if let Ok(output) = Command::new("route").args(["-n", "get", "default"]).output() {
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
        if let Ok(output) = Command::new("ip").args(["route", "show", "default"]).output() {
            let text = String::from_utf8_lossy(&output.stdout);
            let parts: Vec<&str> = text.split_whitespace().collect();
            for i in 0..parts.len() {
                if parts[i] == "via" && i + 1 < parts.len() {
                    return Some(parts[i + 1].to_string());
                }
            }
        }
    }

    None
}

pub fn detect_fortinet_network() -> FortinetDetectionResult {
    {
        let lock = CACHED_DETECTION.lock().unwrap();
        if let Some((ref res, instant)) = *lock {
            if instant.elapsed() < CACHE_TTL && res.is_fortinet_network {
                return res.clone();
            }
        }
    }

    let mut result = FortinetDetectionResult::default();
    let client = make_client();

    // Probe 1: Check captive portal redirect and general connectivity
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
    }

    // Probe 2: Gateway port & TLS probe
    let mut gateway_candidates = vec![FALLBACK_HOST.to_string()];
    if let Some(gw) = get_default_gateway_hint() {
        gateway_candidates.insert(0, format!("{}:1000", gw));
        gateway_candidates.insert(0, format!("{}:1003", gw));
    }

    for target in &gateway_candidates {
        // Quick TCP probe
        if let Ok(addr) = target.parse() {
            if TcpStream::connect_timeout(&addr, Duration::from_millis(500)).is_err() {
                continue;
            }
        }

        // Test HTTP handshake on the candidate port
        let probe_url = format!("https://{}/fgtauth", target);
        if let Ok(mut resp) = client.get(&probe_url).call() {
            let body = resp.body_mut().read_to_string().unwrap_or_default();
            let body_lower = body.to_lowercase();
            if body_lower.contains("fortigate") || body_lower.contains("fortinet") || body.contains("FG6H0") {
                result.is_fortinet_network = true;
                result.gateway_ip = Some(target.clone());
                result.detection_method = Some("Gateway TLS Service Signature".to_string());

                // Extract serial number if present
                if let Ok(re) = Regex::new(r"(FG[0-9A-Z]{14})") {
                    if let Some(caps) = re.captures(&body) {
                        if let Some(m) = caps.get(1) {
                            result.device_serial_number = Some(m.as_str().to_string());
                        }
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
