use std::fs::{File, OpenOptions};
use std::path::PathBuf;

use crate::error::AppError;

pub fn get_lock_dir() -> PathBuf {
    let dir = std::env::temp_dir().join("fortinet-autologin-locks");
    let _ = std::fs::create_dir_all(&dir);
    dir
}

#[cfg(unix)]
extern "C" {
    fn flock(fd: std::os::raw::c_int, operation: std::os::raw::c_int) -> std::os::raw::c_int;
}

#[cfg(unix)]
const LOCK_EX: std::os::raw::c_int = 2;
#[cfg(unix)]
const LOCK_NB: std::os::raw::c_int = 4;
#[cfg(unix)]
const LOCK_UN: std::os::raw::c_int = 8;

pub struct ProcessLock {
    file: Option<File>,
    path: PathBuf,
}

impl ProcessLock {
    pub fn try_acquire(name: &str) -> Result<Self, AppError> {
        let lock_path = get_lock_dir().join(format!("{}.lock", name));

        #[cfg(unix)]
        let file = {
            let file = OpenOptions::new()
                .read(true)
                .write(true)
                .create(true)
                .truncate(true)
                .open(&lock_path)?;

            use std::os::unix::io::AsRawFd;
            let fd = file.as_raw_fd();
            let ret = unsafe { flock(fd, LOCK_EX | LOCK_NB) };
            if ret != 0 {
                return Err(AppError::LockAcquisitionFailed(format!(
                    "Another autologin instance is currently running ({})",
                    lock_path.display()
                )));
            }
            file
        };

        #[cfg(windows)]
        let file = {
            use std::os::windows::fs::OpenOptionsExt;
            OpenOptions::new()
                .read(true)
                .write(true)
                .create(true)
                .share_mode(0) // Exclusive lock: sharing denied
                .open(&lock_path)
                .map_err(|_| {
                    AppError::LockAcquisitionFailed(format!(
                        "Another autologin instance is currently running ({})",
                        lock_path.display()
                    ))
                })?
        };

        #[cfg(not(any(unix, windows)))]
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(true)
            .open(&lock_path)?;

        Ok(Self {
            file: Some(file),
            path: lock_path,
        })
    }
}

impl Drop for ProcessLock {
    fn drop(&mut self) {
        #[cfg(unix)]
        {
            if let Some(ref f) = self.file {
                use std::os::unix::io::AsRawFd;
                let fd = f.as_raw_fd();
                let _ = unsafe { flock(fd, LOCK_UN) };
            }
        }
        self.file.take();
        let _ = std::fs::remove_file(&self.path);
    }
}
