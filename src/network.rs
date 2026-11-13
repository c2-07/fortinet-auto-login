use std::error::Error;
use std::time::Duration;

use regex::Regex;
use ureq::config::Config;
use ureq::tls::TlsConfig;

use crate::logf;
use crate::session::{clear_session, extract_countdown, load_session, random_magic, save_session};
use crate::types::Session;

pub const CHECK_URL: &str = "http://detectportal.firefox.com/success.txt";
pub const FALLBACK_HOST: &str = "192.168.55.253:1003";

pub fn make_client() -> ureq::Agent {
    let tls_config = TlsConfig::builder().disable_verification(true).build();
    let config = Config::builder()
        .timeout_global(Some(Duration::from_millis(2500)))
        .max_redirects(0)
        .tls_config(tls_config)
        .build();
    config.into()
}

pub fn get_portal(client: &ureq::Agent) -> Result<(Option<String>, bool), Box<dyn Error>> {
    let response = client.get(CHECK_URL).call();
    match response {
        Ok(mut res) => {
            let body = res.body_mut().read_to_string().unwrap_or_default();
            if body.trim() == "success" {
                return Ok((None, true));
            }

            // Check if body has JavaScript window.location redirect
            if let Some(url) = extract_portal_url(&body) {
                return Ok((Some(url), false));
            }

            Ok((None, false))
        }
        Err(ureq::Error::StatusCode(code)) if code == 301 || code == 302 || code == 303 || code == 307 => {
            // Some captive portals return direct HTTP redirect header
            Ok((None, false))
        }
        Err(e) => Err(Box::new(e)),
    }
}

pub fn extract_portal_url(text: &str) -> Option<String> {
    let patterns = [
        r#"window\.location="([^"]+)""#,
        r#"location\.href="([^"]+)""#,
        r#"location\.replace\("([^"]+)"\)"#,
        r#"<meta\s+http-equiv="refresh"\s+content="[^"]*url=([^"]+)""#,
    ];

    for pat in &patterns {
        if let Ok(re) = Regex::new(pat) {
            if let Some(caps) = re.captures(text) {
                if let Some(m) = caps.get(1) {
                    return Some(m.as_str().to_string());
                }
            }
        }
    }
    None
}

pub fn parse_magic_from_url(url_str: &str) -> Option<String> {
    let re = Regex::new(r"magic=([a-fA-F0-9]+)").ok()?;
    let caps = re.captures(url_str)?;
    caps.get(1).map(|m| m.as_str().to_string())
}

pub fn parse_host_from_url(url_str: &str) -> Option<String> {
    let re = Regex::new(r"https?://([^/]+)").ok()?;
    let caps = re.captures(url_str)?;
    caps.get(1).map(|m| m.as_str().to_string())
}

pub fn login(quiet: bool, _is_retry: bool, username: &str, password: &str) -> bool {
    let client = make_client();

    if !quiet {
        logf!("\x1b[34m[NET]\x1b[0m    Checking internet (Mozilla method)...");
    }

    let (portal, already_connected) = match get_portal(&client) {
        Ok(res) => res,
        Err(e) => {
            if !quiet {
                logf!("\x1b[31m[FAIL]\x1b[0m   Network unreachable: {}", e);
            }
            return false;
        }
    };

    if already_connected {
        if !quiet {
            logf!("\x1b[32m[OK]\x1b[0m     Internet connected");
        }
        return true;
    }

    let portal_url = match portal {
        Some(u) => u,
        None => {
            if !quiet {
                logf!("\x1b[31m[FAIL]\x1b[0m   Captive portal redirect URL not found");
            }
            return false;
        }
    };

    if !quiet {
        logf!("\x1b[33m[PORTAL]\x1b[0m {}", portal_url);
    }

    let magic = parse_magic_from_url(&portal_url).unwrap_or_else(random_magic);
    let host = parse_host_from_url(&portal_url).unwrap_or_else(|| FALLBACK_HOST.to_string());

    let post_url = if portal_url.contains("/fgtauth") {
        portal_url.clone()
    } else {
        format!("https://{}/fgtauth", host)
    };

    let form_params = [
        ("4Tredir", "http://detectportal.firefox.com/success.txt"),
        ("magic", &magic),
        ("answer", "1"),
        ("username", username),
        ("password", password),
    ];

    let post_res = client
        .post(&post_url)
        .header("Content-Type", "application/x-www-form-urlencoded")
        .header("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:132.0) Gecko/20100101 Firefox/132.0")
        .send_form(form_params);

    match post_res {
        Ok(mut res) => {
            let body = res.body_mut().read_to_string().unwrap_or_default();
            let timeout = extract_countdown(&body).unwrap_or(14400);

            let session = Session {
                host: host.clone(),
                magic: magic.clone(),
                timeout,
                created_at: chrono_now_string(),
                login_url: portal_url,
            };
            let _ = save_session(&session);

            if !quiet {
                logf!("\x1b[32m[OK]\x1b[0m     Logged in successfully! (Session Timeout: {}s)", timeout);
            }
            true
        }
        Err(e) => {
            if !quiet {
                logf!("\x1b[31m[FAIL]\x1b[0m   Authentication request failed: {}", e);
            }
            false
        }
    }
}

pub fn logout() -> bool {
    let client = make_client();
    let (host, magic) = match load_session() {
        Some(s) => (s.host, s.magic),
        None => {
            logf!("\x1b[33m[NOTE]\x1b[0m   No session found. Using random magic on default gateway.");
            (FALLBACK_HOST.to_string(), random_magic())
        }
    };

    let logout_url = format!("https://{}/logout?{}", host, magic);
    logf!("\x1b[34m[NET]\x1b[0m    Sending logout to: {}", logout_url);

    let res = client.get(&logout_url).call();
    clear_session();

    match res {
        Ok(_) => {
            logf!("\x1b[32m[OK]\x1b[0m     Logged out successfully");
            true
        }
        Err(e) => {
            logf!("\x1b[33m[WARN]\x1b[0m   Logout request returned: {} (session cleared locally)", e);
            true
        }
    }
}

fn chrono_now_string() -> String {
    use std::time::SystemTime;
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    format!("{}", now)
}
