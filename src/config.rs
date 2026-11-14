use std::env;
use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::credentials::get_app_dir;

pub const DEFAULT_PORTAL_HOST: &str = "192.168.55.253:1003";

pub const DEFAULT_CHECK_URLS: &[&str] = &[
    "http://detectportal.firefox.com/success.txt",
    "http://connectivitycheck.gstatic.com/generate_204",
    "http://captive.apple.com/hotspot-detect.html",
    "http://www.msftconnecttest.com/connecttest.txt",
    "http://cp.cloudflare.com/generate_204",
];

pub const DEFAULT_GATEWAY_PORTS: &[u16] = &[1003, 1000];
pub const DEFAULT_DAEMON_INTERVAL_SECS: u64 = 45;
pub const DEFAULT_REQUEST_TIMEOUT_SECS: u64 = 10;
pub const DEFAULT_MAX_RETRIES: usize = 3;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AppConfig {
    #[serde(default = "default_portal_host")]
    pub portal_host: String,

    #[serde(default = "default_check_urls")]
    pub check_urls: Vec<String>,

    #[serde(default = "default_gateway_ports")]
    pub gateway_ports: Vec<u16>,

    #[serde(default = "default_daemon_interval")]
    pub daemon_interval_secs: u64,

    #[serde(default = "default_request_timeout")]
    pub request_timeout_secs: u64,

    #[serde(default = "default_max_retries")]
    pub max_retries: usize,
}

fn default_portal_host() -> String {
    DEFAULT_PORTAL_HOST.to_string()
}

fn default_check_urls() -> Vec<String> {
    DEFAULT_CHECK_URLS.iter().map(|s| s.to_string()).collect()
}

fn default_gateway_ports() -> Vec<u16> {
    DEFAULT_GATEWAY_PORTS.to_vec()
}

fn default_daemon_interval() -> u64 {
    DEFAULT_DAEMON_INTERVAL_SECS
}

fn default_request_timeout() -> u64 {
    DEFAULT_REQUEST_TIMEOUT_SECS
}

fn default_max_retries() -> usize {
    DEFAULT_MAX_RETRIES
}

impl Default for AppConfig {
    fn default() -> Self {
        Self {
            portal_host: default_portal_host(),
            check_urls: default_check_urls(),
            gateway_ports: default_gateway_ports(),
            daemon_interval_secs: default_daemon_interval(),
            request_timeout_secs: default_request_timeout(),
            max_retries: default_max_retries(),
        }
    }
}

pub fn config_file_path() -> PathBuf {
    get_app_dir().join("config.json")
}

pub fn load_config() -> AppConfig {
    let mut config = if let Ok(data) = fs::read_to_string(config_file_path()) {
        serde_json::from_str::<AppConfig>(&data).unwrap_or_default()
    } else {
        AppConfig::default()
    };

    // Environment overrides take precedence for container/script flexibility
    if let Ok(host) = env::var("FORTINET_PORTAL_HOST") {
        if !host.trim().is_empty() {
            config.portal_host = host.trim().to_string();
        }
    }

    if let Ok(custom_check) = env::var("FORTINET_CHECK_URL") {
        let trimmed = custom_check.trim().to_string();
        if !trimmed.is_empty() {
            // Prioritize custom probe URL as first candidate
            if !config.check_urls.contains(&trimmed) {
                config.check_urls.insert(0, trimmed);
            } else {
                config.check_urls.retain(|u| u != &trimmed);
                config.check_urls.insert(0, trimmed);
            }
        }
    }

    if let Ok(interval_str) = env::var("FORTINET_INTERVAL") {
        if let Ok(secs) = interval_str.trim().parse::<u64>() {
            config.daemon_interval_secs = secs;
        }
    }

    config
}

pub fn save_config(config: &AppConfig) -> std::io::Result<()> {
    let path = config_file_path();
    let data = serde_json::to_string_pretty(config)
        .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))?;
    fs::write(path, data)
}

pub fn reset_config() -> std::io::Result<()> {
    let default = AppConfig::default();
    save_config(&default)
}

pub fn set_portal_host(host: &str) -> std::io::Result<()> {
    let mut config = load_config();
    config.portal_host = host.trim().to_string();
    save_config(&config)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config_values() {
        let config = AppConfig::default();
        assert_eq!(config.portal_host, DEFAULT_PORTAL_HOST);
        assert_eq!(config.check_urls.len(), 5);
        assert_eq!(
            config.check_urls[0],
            "http://detectportal.firefox.com/success.txt"
        );
        assert!(config
            .check_urls
            .contains(&"http://connectivitycheck.gstatic.com/generate_204".to_string()));
        assert!(config
            .check_urls
            .contains(&"http://captive.apple.com/hotspot-detect.html".to_string()));
        assert_eq!(config.gateway_ports, vec![1003, 1000]);
        assert_eq!(config.daemon_interval_secs, 45);
    }

    #[test]
    fn test_config_serialization() {
        let config = AppConfig::default();
        let json = serde_json::to_string(&config).unwrap();
        let deserialized: AppConfig = serde_json::from_str(&json).unwrap();
        assert_eq!(config, deserialized);
    }
}
