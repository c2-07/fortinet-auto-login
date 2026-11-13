use serde::Deserialize;
use ureq::config::Config;
use std::time::Duration;

use crate::logf;

const CURRENT_VERSION: &str = env!("CARGO_PKG_VERSION");
const REPO: &str = "c2-07/fortinet-auto-login";

#[derive(Deserialize)]
struct GitHubRelease {
    tag_name: String,
    html_url: String,
}

pub fn check_upgrade() -> bool {
    logf!("\x1b[36m[UPGRADE]\x1b[0m Checking for updates (current version: v{})...", CURRENT_VERSION);

    let config = Config::builder()
        .timeout_global(Some(Duration::from_secs(5)))
        .build();
    let client: ureq::Agent = config.into();

    let url = format!("https://api.github.com/repos/{}/releases/latest", REPO);
    let resp = client
        .get(&url)
        .header("User-Agent", "fortinet-autologin-rust")
        .call();

    match resp {
        Ok(mut res) => {
            let release: GitHubRelease = match res.body_mut().read_json() {
                Ok(r) => r,
                Err(e) => {
                    logf!("\x1b[31m[FAIL]\x1b[0m   Failed to parse GitHub release info: {}", e);
                    return false;
                }
            };

            let remote_version = release.tag_name.trim_start_matches('v');
            if remote_version == CURRENT_VERSION {
                logf!("\x1b[32m[OK]\x1b[0m     You are on the latest version (v{})!", CURRENT_VERSION);
            } else {
                logf!("\x1b[33m[UPDATE]\x1b[0m A new version is available: {} (current: v{})", release.tag_name, CURRENT_VERSION);
                logf!("         Download release from: {}", release.html_url);
            }
            true
        }
        Err(e) => {
            logf!("\x1b[31m[FAIL]\x1b[0m   Failed to check for updates: {}", e);
            false
        }
    }
}
