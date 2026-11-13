use std::env;
use std::fs::{self, File};
use std::io::{self, BufRead, Write};
use std::path::PathBuf;
use std::process::Command;

#[cfg(unix)]
use std::os::unix::fs::PermissionsExt;

use crate::types::Credentials;

pub fn get_app_dir() -> PathBuf {
    let home = env::var("HOME")
        .or_else(|_| env::var("USERPROFILE"))
        .unwrap_or_else(|_| env::temp_dir().to_string_lossy().to_string());
    let dir = PathBuf::from(home).join(".fortinet-autologin");
    let _ = fs::create_dir_all(&dir);

    #[cfg(unix)]
    {
        let _ = fs::set_permissions(&dir, fs::Permissions::from_mode(0o700));
    }

    dir
}

pub fn credentials_file_path() -> PathBuf {
    get_app_dir().join("credentials.json")
}

pub fn load_credentials() -> Option<Credentials> {
    let path = credentials_file_path();
    let data = fs::read_to_string(path).ok()?;
    let creds: Credentials = serde_json::from_str(&data).ok()?;
    if !creds.username.is_empty() && !creds.password.is_empty() {
        Some(creds)
    } else {
        None
    }
}

pub fn save_credentials(creds: &Credentials) -> io::Result<()> {
    let path = credentials_file_path();
    let data = serde_json::to_string_pretty(creds)
        .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;

    let mut file = File::create(&path)?;
    file.write_all(data.as_bytes())?;

    #[cfg(unix)]
    {
        let _ = fs::set_permissions(&path, fs::Permissions::from_mode(0o600));
    }

    Ok(())
}

pub fn prompt_credentials() -> (String, String) {
    let stdin = io::stdin();
    let mut stdout = io::stdout();

    print!("Enter Fortinet Username: ");
    let _ = stdout.flush();
    let mut username = String::new();
    let _ = stdin.lock().read_line(&mut username);
    let username = username.trim().to_string();

    print!("Enter Fortinet Password: ");
    let _ = stdout.flush();

    // Mask password using stty on Unix
    #[cfg(unix)]
    {
        let _ = Command::new("stty").arg("-echo").status();
    }

    let mut password = String::new();
    let _ = stdin.lock().read_line(&mut password);

    #[cfg(unix)]
    {
        let _ = Command::new("stty").arg("echo").status();
        println!();
    }

    let password = password.trim().to_string();
    (username, password)
}
