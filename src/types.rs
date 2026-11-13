use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Credentials {
    pub username: String,
    pub password: String,
    #[serde(default)]
    pub ssid: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    #[serde(default)]
    pub host: String,
    #[serde(default)]
    pub magic: String,
    #[serde(default)]
    pub timeout: u64,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub login_url: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct FortinetDetectionResult {
    pub is_fortinet_network: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub captive_portal_host: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub magic_token: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gateway_ip: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub device_serial_number: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detection_method: Option<String>,
    pub internet_connected: bool,
}
