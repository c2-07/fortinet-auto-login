use std::sync::LazyLock;
use std::thread;
use std::time::{Duration, SystemTime};

use regex::Regex;
use ureq::config::Config;
use ureq::tls::TlsConfig;

use crate::config::{load_config, DEFAULT_PORTAL_HOST};
use crate::credentials::delete_credentials;
use crate::error::AppError;
use crate::logf;
use crate::session::{clear_session, extract_countdown, load_session, random_magic, save_session};
use crate::types::Session;

pub const CHECK_URL: &str = "http://detectportal.firefox.com/";
pub const FALLBACK_HOST: &str = DEFAULT_PORTAL_HOST;

pub fn get_fallback_host() -> String {
    load_config().portal_host
}
const USER_AGENT: &str = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0 Safari/537.36";

static PORTAL_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    vec![
        Regex::new(r#"window\.location="([^"]+)""#).unwrap(),
        Regex::new(r#"location\.href="([^"]+)""#).unwrap(),
        Regex::new(r#"location\.replace\("([^"]+)"\)"#).unwrap(),
        Regex::new(r#"<meta\s+http-equiv="refresh"\s+content="[^"]*url=([^"]+)""#).unwrap(),
    ]
});

static MAGIC_RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"magic=([a-fA-F0-9]+)").unwrap());

static HOST_RE: LazyLock<Regex> = LazyLock::new(|| Regex::new(r"https?://([^/]+)").unwrap());

pub fn make_client() -> ureq::Agent {
    let timeout_secs = crate::config::load_config().request_timeout_secs;
    let tls_config = TlsConfig::builder().disable_verification(true).build();
    let config = Config::builder()
        .timeout_global(Some(Duration::from_secs(timeout_secs)))
        .max_redirects(0)
        .tls_config(tls_config)
        .build();
    config.into()
}

pub fn do_with_retry<F, T>(retries: usize, mut op: F) -> Result<T, AppError>
where
    F: FnMut() -> Result<T, ureq::Error>,
{
    let mut last_err = String::new();
    for attempt in 0..=retries {
        match op() {
            Ok(val) => return Ok(val),
            Err(e) => {
                last_err = format!("{}", e);
                if attempt < retries {
                    let backoff = Duration::from_millis(500 * (attempt as u64 + 1));
                    thread::sleep(backoff);
                }
            }
        }
    }
    Err(AppError::NetworkUnreachable(last_err))
}

pub fn get_check_url() -> String {
    let cfg = load_config();
    cfg.check_urls
        .first()
        .cloned()
        .unwrap_or_else(|| CHECK_URL.to_string())
}

pub fn get_portal(client: &ureq::Agent) -> Result<(Option<String>, bool), AppError> {
    let config = load_config();
    let mut last_err = None;

    for url in &config.check_urls {
        match client.get(url).header("Connection", "close").call() {
            Ok(mut res) => {
                let status = res.status();
                let body = res.body_mut().read_to_string().unwrap_or_default();
                let trimmed = body.trim();

                let is_online = (status == 204)
                    || (status == 200
                        && (trimmed == "success"
                            || body.contains("<TITLE>Success</TITLE>")
                            || body.contains("Microsoft Connect Test")));

                if is_online {
                    return Ok((None, true));
                }

                if let Some(portal) = extract_portal_url(&body) {
                    return Ok((Some(portal), false));
                }

                if let Some(loc) = res.headers().get("location") {
                    if let Ok(loc_str) = loc.to_str() {
                        return Ok((Some(loc_str.to_string()), false));
                    }
                }

                if body.to_lowercase().contains("fortigate")
                    || body.to_lowercase().contains("fgtauth")
                {
                    return Ok((
                        Some(format!("https://{}/fgtauth", config.portal_host)),
                        false,
                    ));
                }
            }
            Err(e) => {
                last_err = Some(e);
            }
        }
    }

    // Direct gateway probe: when logged out, external DNS can be blackholed.
    // Querying the gateway IP on port 1000 bypasses DNS and returns the real captive portal URL in <20ms.
    let gw_ip = config
        .portal_host
        .split(':')
        .next()
        .unwrap_or("192.168.55.253");
    let direct_url = format!("http://{}:1000/", gw_ip);
    if let Ok(mut res) = client.get(&direct_url).call() {
        let body = res.body_mut().read_to_string().unwrap_or_default();
        if let Some(portal) = extract_portal_url(&body) {
            return Ok((Some(portal), false));
        }
    }

    if let Some(err) = last_err {
        Err(AppError::NetworkUnreachable(err.to_string()))
    } else {
        Ok((None, false))
    }
}

pub fn extract_portal_url(text: &str) -> Option<String> {
    for re in PORTAL_PATTERNS.iter() {
        if let Some(caps) = re.captures(text) {
            if let Some(m) = caps.get(1) {
                return Some(m.as_str().to_string());
            }
        }
    }
    None
}

pub fn parse_magic_from_url(url_str: &str) -> Option<String> {
    if let Some(caps) = MAGIC_RE.captures(url_str) {
        return caps.get(1).map(|m| m.as_str().to_string());
    }
    if let Some(pos) = url_str.find('?') {
        let query = &url_str[pos + 1..];
        let token = query.strip_prefix("magic=").unwrap_or(query);
        let token = token.split('&').next().unwrap_or(token);
        if !token.is_empty() && token.chars().all(|c| c.is_ascii_hexdigit()) {
            return Some(token.to_string());
        }
    }
    None
}

pub fn parse_host_from_url(url_str: &str) -> Option<String> {
    let caps = HOST_RE.captures(url_str)?;
    caps.get(1).map(|m| m.as_str().to_string())
}

pub fn login(quiet: bool, is_retry: bool, username: &str, password: &str) -> bool {
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
                logf!("\x1b[31m[FAIL]\x1b[0m   Could not obtain captive portal URL");
            }
            return false;
        }
    };

    if !quiet {
        logf!("\x1b[33m[PORTAL]\x1b[0m {}", portal_url);
        logf!("\x1b[34m[AUTH]\x1b[0m   User: {}", username);
    }

    // Pre-flight GET to the captive portal to initialize session/cookies (same as Go & browser)
    let mut page_loaded = false;
    for attempt in 0..=3 {
        match client.get(&portal_url).call() {
            Ok(mut res) if res.status() == 200 => {
                let _ = res.body_mut().read_to_string();
                page_loaded = true;
                break;
            }
            Ok(mut res) => {
                let _ = res.body_mut().read_to_string();
            }
            Err(_) => {}
        }
        if attempt < 3 {
            thread::sleep(Duration::from_millis(250 * (attempt as u64 + 1)));
        }
    }

    if !page_loaded && !quiet {
        logf!("\x1b[33m[WARN]\x1b[0m   Pre-flight portal probe was non-200, proceeding with POST...");
    }
    thread::sleep(Duration::from_millis(200));

    let magic = match parse_magic_from_url(&portal_url) {
        Some(m) => m,
        None => {
            if !quiet {
                logf!("\x1b[31m[FAIL]\x1b[0m   Failed to extract magic token from portal URL");
            }
            return false;
        }
    };
    let host = parse_host_from_url(&portal_url).unwrap_or_else(get_fallback_host);

    let post_url = if let Some(pos) = portal_url.find('?') {
        portal_url[..pos].to_string()
    } else {
        format!("https://{}/fgtauth", host)
    };
    let origin = format!("https://{}", host);

    // Form parameter order matching Go's url.Values.Encode() alphabetical sort:
    // 4Tredir, magic, password, username
    let form_params = [
        ("4Tredir", "http://google.com/"),
        ("magic", &magic),
        ("password", password),
        ("username", username),
    ];

    let post_res = do_with_retry(3, || {
        client
            .post(&post_url)
            .header("Content-Type", "application/x-www-form-urlencoded")
            .header("Referer", &portal_url)
            .header("Origin", &origin)
            .header("User-Agent", USER_AGENT)
            .send_form(form_params)
    });

    match post_res {
        Ok(mut res) => {
            let body = res.body_mut().read_to_string().unwrap_or_default();
            let body_lower = body.to_lowercase();

            if body_lower.contains("failed")
                || body_lower.contains("invalid")
                || body_lower.contains("wrong")
            {
                if !quiet {
                    logf!("\x1b[31m[FAIL]\x1b[0m   Login rejected by Fortinet");
                }

                let is_bad_creds = body_lower.contains("invalid user")
                    || body_lower.contains("invalid pass")
                    || body_lower.contains("authentication failed")
                    || body_lower.contains("login failed")
                    || body_lower.contains("wrong");

                if is_bad_creds {
                    delete_credentials();
                    if !quiet {
                        logf!("\x1b[33m[WARN]\x1b[0m   Invalid credentials detected. Saved credentials removed.");
                    }
                    if !is_retry && !quiet {
                        let (new_u, new_p) = crate::credentials::prompt_credentials();
                        let creds = crate::types::Credentials {
                            username: new_u.clone(),
                            password: new_p.clone(),
                            ssid: String::new(),
                        };
                        let _ = crate::credentials::save_credentials(&creds);
                        return login(quiet, true, &new_u, &new_p);
                    }
                }
                return false;
            }

            let timeout = extract_countdown(&body).unwrap_or(14400);
            let session = Session {
                host: host.clone(),
                magic: magic.clone(),
                timeout,
                created_at: SystemTime::now()
                    .duration_since(SystemTime::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs()
                    .to_string(),
                login_url: portal_url,
            };
            let _ = save_session(&session);

            if !quiet {
                logf!(
                    "\x1b[32m[OK]\x1b[0m     Auth success (host={} magic={} timeout={}s)",
                    session.host,
                    session.magic,
                    timeout
                );
            }
            true
        }
        Err(e) => {
            if !quiet {
                logf!(
                    "\x1b[31m[FAIL]\x1b[0m   Authentication request failed: {}",
                    e
                );
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
            logf!(
                "\x1b[33m[NOTE]\x1b[0m   No session found. Using random magic on fallback gateway."
            );
            (FALLBACK_HOST.to_string(), random_magic())
        }
    };

    let logout_url = format!("https://{}/logout?{}", host, magic);
    logf!(
        "\x1b[34m[NET]\x1b[0m    Sending logout request to: {}",
        logout_url
    );

    let res = do_with_retry(3, || {
        client.get(&logout_url).header("Connection", "close").call()
    });
    clear_session();

    match res {
        Ok(_) => {
            logf!(
                "\x1b[32m[OK]\x1b[0m     Logged out successfully (host={} magic={})",
                host,
                magic
            );
            true
        }
        Err(e) => {
            logf!(
                "\x1b[33m[WARN]\x1b[0m   Logout request returned: {} (session cleared locally)",
                e
            );
            true
        }
    }
}
