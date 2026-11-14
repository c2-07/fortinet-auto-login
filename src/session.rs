use std::fs::{self, File};
use std::io::{self, Write};
use std::path::PathBuf;
use std::sync::LazyLock;
use std::time::SystemTime;

use regex::Regex;

use crate::credentials::get_app_dir;
use crate::types::Session;

static COUNTDOWN_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    vec![
        Regex::new(r#"var\s+countdown\s*=\s*(\d+)"#).unwrap(),
        Regex::new(r#"name="countdown"\s+value="(\d+)""#).unwrap(),
        Regex::new(r#"keepalive\((\d+)\)"#).unwrap(),
        Regex::new(r#"id="countdown"[^>]*>(\d+)<"#).unwrap(),
    ]
});

pub fn session_file_path() -> PathBuf {
    get_app_dir().join("session.json")
}

pub fn load_session() -> Option<Session> {
    let path = session_file_path();
    let data = fs::read_to_string(path).ok()?;
    serde_json::from_str(&data).ok()
}

pub fn save_session(session: &Session) -> io::Result<()> {
    let path = session_file_path();
    let data = serde_json::to_string_pretty(session)
        .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;

    let mut file = File::create(&path)?;
    file.write_all(data.as_bytes())?;
    Ok(())
}

pub fn clear_session() {
    let path = session_file_path();
    let _ = fs::remove_file(path);
}

#[allow(dead_code)]
pub fn is_session_expired(session: &Session) -> bool {
    let created_at_sec: u64 = session.created_at.parse().unwrap_or(0);
    if created_at_sec == 0 || session.timeout == 0 {
        return false;
    }
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();

    // Consider expired 30s before actual timeout
    now >= created_at_sec + session.timeout.saturating_sub(30)
}

pub fn extract_countdown(body: &str) -> Option<u64> {
    for re in COUNTDOWN_PATTERNS.iter() {
        if let Some(caps) = re.captures(body) {
            if let Some(m) = caps.get(1) {
                if let Ok(val) = m.as_str().parse::<u64>() {
                    return Some(val);
                }
            }
        }
    }

    None
}

use std::sync::atomic::{AtomicU64, Ordering};

static MAGIC_COUNTER: AtomicU64 = AtomicU64::new(1);

pub fn random_magic() -> String {
    let nanos = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;
    let count = MAGIC_COUNTER.fetch_add(1, Ordering::Relaxed);
    let mut z = nanos.wrapping_add(count.wrapping_mul(0x9E3779B97F4A7C15));
    z = (z ^ (z >> 30)).wrapping_mul(0xBF58476D1CE4E5B9);
    z = (z ^ (z >> 27)).wrapping_mul(0x94D049BB133111EB);
    z ^= z >> 31;
    format!("{:016x}", z)
}
