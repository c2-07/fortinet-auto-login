use std::fs::{self, File};
use std::io::{self, Write};
use std::path::PathBuf;
use std::time::SystemTime;

use regex::Regex;

use crate::credentials::get_app_dir;
use crate::types::Session;

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

pub fn extract_countdown(body: &str) -> Option<u64> {
    let patterns = [
        r#"var\s+countdown\s*=\s*(\d+)"#,
        r#"name="countdown"\s+value="(\d+)""#,
        r#"keepalive\((\d+)\)"#,
        r#"id="countdown"[^>]*>(\d+)<"#,
    ];

    for pat in &patterns {
        if let Ok(re) = Regex::new(pat) {
            if let Some(caps) = re.captures(body) {
                if let Some(m) = caps.get(1) {
                    if let Ok(val) = m.as_str().parse::<u64>() {
                        return Some(val);
                    }
                }
            }
        }
    }

    None
}

pub fn random_magic() -> String {
    let nanos = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    format!("{:016x}", nanos)
}
