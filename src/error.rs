use std::fmt;
use std::io;

#[allow(dead_code)]
#[derive(Debug)]
pub enum AppError {
    Io(io::Error),
    Http(String),
    Json(serde_json::Error),
    PortalNotFound,
    AuthFailed {
        reason: String,
        bad_credentials: bool,
    },
    LockAcquisitionFailed(String),
    NetworkUnreachable(String),
}

impl fmt::Display for AppError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            AppError::Io(e) => write!(f, "I/O error: {}", e),
            AppError::Http(msg) => write!(f, "HTTP error: {}", msg),
            AppError::Json(e) => write!(f, "JSON error: {}", e),
            AppError::PortalNotFound => write!(f, "Captive portal redirect URL not found"),
            AppError::AuthFailed { reason, .. } => write!(f, "Authentication failed: {}", reason),
            AppError::LockAcquisitionFailed(msg) => write!(f, "Process lock failed: {}", msg),
            AppError::NetworkUnreachable(msg) => write!(f, "Network unreachable: {}", msg),
        }
    }
}

impl std::error::Error for AppError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            AppError::Io(e) => Some(e),
            AppError::Json(e) => Some(e),
            _ => None,
        }
    }
}

impl From<io::Error> for AppError {
    fn from(err: io::Error) -> Self {
        AppError::Io(err)
    }
}

impl From<serde_json::Error> for AppError {
    fn from(err: serde_json::Error) -> Self {
        AppError::Json(err)
    }
}
