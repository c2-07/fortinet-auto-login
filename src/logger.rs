use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::SystemTime;

pub const MAX_LOG_SIZE: u64 = 2 * 1024 * 1024; // 2 MB

static LOG_FILE_PATH: Mutex<Option<PathBuf>> = Mutex::new(None);

#[allow(dead_code)]
pub fn set_log_file(path: PathBuf) {
    let mut lock = LOG_FILE_PATH.lock().unwrap();
    *lock = Some(path);
}

pub fn rotate_log_if_needed(path: &Path, max_bytes: u64) {
    if let Ok(metadata) = fs::metadata(path) {
        if metadata.len() >= max_bytes {
            let mut old_path = path.to_path_buf();
            old_path.set_extension("log.old");
            let _ = fs::remove_file(&old_path);
            let _ = fs::rename(path, old_path);
        }
    }
}

pub fn get_timestamp() -> String {
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    let hours = (now / 3600) % 24;
    let minutes = (now / 60) % 60;
    let seconds = now % 60;
    format!("{:02}:{:02}:{:02}", hours, minutes, seconds)
}

pub fn log_raw(msg: &str) {
    let ts = get_timestamp();
    let formatted = format!("[{}] {}", ts, msg);
    println!("{}", formatted);

    if let Ok(lock) = LOG_FILE_PATH.lock() {
        if let Some(ref path) = *lock {
            rotate_log_if_needed(path, MAX_LOG_SIZE);
            if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
                // Strip ANSI escape codes for clean file logs
                let clean = strip_ansi_escapes(&formatted);
                let _ = writeln!(file, "{}", clean);
            }
        }
    }
}

fn strip_ansi_escapes(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    let mut in_escape = false;
    for c in s.chars() {
        if c == '\x1b' {
            in_escape = true;
        } else if in_escape {
            if c == 'm' {
                in_escape = false;
            }
        } else {
            result.push(c);
        }
    }
    result
}

#[macro_export]
macro_rules! logf {
    ($($arg:tt)*) => {
        $crate::logger::log_raw(&format!($($arg)*))
    };
}
