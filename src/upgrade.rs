use std::env;
use std::fs;
use std::time::Duration;
use ureq::config::Config;

use crate::logf;

const CURRENT_VERSION: &str = env!("CARGO_PKG_VERSION");
const REPO: &str = "c2-07/fortinet-auto-login";

#[cfg(all(target_os = "macos", target_arch = "aarch64"))]
const ASSET_NAME: &str = "fortinet-auto-login-darwin-arm64";

#[cfg(all(target_os = "macos", target_arch = "x86_64"))]
const ASSET_NAME: &str = "fortinet-auto-login-darwin-amd64";

#[cfg(all(target_os = "linux", target_arch = "x86_64"))]
const ASSET_NAME: &str = "fortinet-auto-login-linux-amd64";

#[cfg(all(target_os = "linux", target_arch = "aarch64"))]
const ASSET_NAME: &str = "fortinet-auto-login-linux-arm64";

#[cfg(all(target_os = "windows", target_arch = "x86_64"))]
const ASSET_NAME: &str = "fortinet-auto-login-windows-amd64.exe";

#[cfg(all(target_os = "windows", target_arch = "aarch64"))]
const ASSET_NAME: &str = "fortinet-auto-login-windows-arm64.exe";

#[cfg(not(any(
    all(
        target_os = "macos",
        any(target_arch = "aarch64", target_arch = "x86_64")
    ),
    all(
        target_os = "linux",
        any(target_arch = "x86_64", target_arch = "aarch64")
    ),
    all(
        target_os = "windows",
        any(target_arch = "x86_64", target_arch = "aarch64")
    )
)))]
const ASSET_NAME: &str = "unknown";

#[derive(serde::Deserialize)]
struct GitHubRelease {
    tag_name: String,
    html_url: String,
    #[serde(default)]
    assets: Vec<GitHubAsset>,
}

#[derive(serde::Deserialize)]
struct GitHubAsset {
    name: String,
    browser_download_url: String,
}

pub fn check_upgrade() -> bool {
    logf!(
        "\x1b[36m[UPDATE]\x1b[0m Checking for updates (current version: v{})...",
        CURRENT_VERSION
    );

    let config = Config::builder()
        .timeout_global(Some(Duration::from_secs(10)))
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
                    logf!(
                        "\x1b[31m[FAIL]\x1b[0m   Failed to parse GitHub release info: {}",
                        e
                    );
                    return false;
                }
            };

            let remote_version = release.tag_name.trim_start_matches('v');
            if remote_version == CURRENT_VERSION {
                logf!(
                    "\x1b[32m[OK]\x1b[0m     You are on the latest version (v{})!",
                    CURRENT_VERSION
                );
                return true;
            }

            logf!(
                "\x1b[33m[UPDATE]\x1b[0m A new version is available: {} (current: v{})",
                release.tag_name,
                CURRENT_VERSION
            );

            // Find matching asset for this architecture
            let matching_asset = release.assets.iter().find(|a| a.name == ASSET_NAME);

            if let Some(asset) = matching_asset {
                logf!(
                    "\x1b[36m[UPDATE]\x1b[0m Downloading {} from GitHub Releases...",
                    asset.name
                );

                match client.get(&asset.browser_download_url).call() {
                    Ok(mut download_resp) => {
                        use std::io::Read;
                        let mut bytes = Vec::new();
                        if let Err(e) = download_resp.body_mut().as_reader().read_to_end(&mut bytes)
                        {
                            logf!(
                                "\x1b[31m[FAIL]\x1b[0m   Failed to download update bytes: {}",
                                e
                            );
                            return false;
                        }

                        if bytes.is_empty() {
                            logf!("\x1b[31m[FAIL]\x1b[0m   Downloaded update file is empty.");
                            return false;
                        }

                        match apply_binary_update(&bytes) {
                            Ok(path) => {
                                logf!(
                                    "\x1b[32m[OK]\x1b[0m     Successfully upgraded autologin to {}!",
                                    release.tag_name
                                );
                                logf!(
                                    "\x1b[36m[INFO]\x1b[0m   Executable updated: {}",
                                    path.display()
                                );
                                true
                            }
                            Err(e) => {
                                logf!(
                                    "\x1b[31m[FAIL]\x1b[0m   Failed to replace current executable: {}",
                                    e
                                );
                                logf!(
                                    "\x1b[36m[INFO]\x1b[0m   You can manually download the release at: {}",
                                    release.html_url
                                );
                                false
                            }
                        }
                    }
                    Err(e) => {
                        logf!(
                            "\x1b[31m[FAIL]\x1b[0m   Failed to download update asset: {}",
                            e
                        );
                        logf!(
                            "\x1b[36m[INFO]\x1b[0m   Manual download URL: {}",
                            release.html_url
                        );
                        false
                    }
                }
            } else {
                logf!(
                    "\x1b[33m[NOTE]\x1b[0m   Pre-built binary '{}' not found in release assets.",
                    ASSET_NAME
                );
                logf!(
                    "\x1b[36m[INFO]\x1b[0m   Download release from: {}",
                    release.html_url
                );
                true
            }
        }
        Err(e) => {
            logf!("\x1b[31m[FAIL]\x1b[0m   Failed to check for updates: {}", e);
            false
        }
    }
}

fn apply_binary_update(new_bytes: &[u8]) -> std::io::Result<std::path::PathBuf> {
    let current_exe = env::current_exe()?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let tmp_path = current_exe.with_extension("tmp_upgrade");
        fs::write(&tmp_path, new_bytes)?;
        let _ = fs::set_permissions(&tmp_path, fs::Permissions::from_mode(0o755));
        fs::rename(&tmp_path, &current_exe)?;
    }

    #[cfg(windows)]
    {
        let old_path = current_exe.with_extension("old");
        let _ = fs::remove_file(&old_path);
        fs::rename(&current_exe, &old_path)?;
        if let Err(e) = fs::write(&current_exe, new_bytes) {
            // Restore original on write failure
            let _ = fs::rename(&old_path, &current_exe);
            return Err(e);
        }
    }

    #[cfg(not(any(unix, windows)))]
    {
        fs::write(&current_exe, new_bytes)?;
    }

    Ok(current_exe)
}
