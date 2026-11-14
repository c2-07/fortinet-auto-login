use serde::{Deserialize, Serialize};

#[derive(Clone, Serialize, Deserialize)]
pub struct Credentials {
    pub username: String,
    pub password: String,
    #[serde(default)]
    pub ssid: String,
}

impl std::fmt::Debug for Credentials {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Credentials")
            .field("username", &self.username)
            .field("password", &"[REDACTED]")
            .field("ssid", &self.ssid)
            .finish()
    }
}

impl Drop for Credentials {
    fn drop(&mut self) {
        // Zeroize password memory to prevent plaintext retention in RAM
        unsafe {
            let bytes = self.password.as_bytes_mut();
            for b in bytes.iter_mut() {
                std::ptr::write_volatile(b, 0);
            }
        }
    }
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
