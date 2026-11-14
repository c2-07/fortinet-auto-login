use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::{Duration, Instant};

use crate::detect::detect_fortinet_network;
use crate::lock::ProcessLock;
use crate::logf;
use crate::network::login;

static RUNNING: AtomicBool = AtomicBool::new(true);

#[cfg(unix)]
extern "C" fn handle_signal(_sig: std::os::raw::c_int) {
    RUNNING.store(false, Ordering::SeqCst);
}

#[cfg(windows)]
extern "system" fn handle_ctrl(_event: u32) -> i32 {
    RUNNING.store(false, Ordering::SeqCst);
    1 // Handled
}

fn register_shutdown_handler() {
    RUNNING.store(true, Ordering::SeqCst);
    #[cfg(unix)]
    unsafe {
        extern "C" {
            fn signal(
                sig: std::os::raw::c_int,
                handler: extern "C" fn(std::os::raw::c_int),
            ) -> std::os::raw::c_int;
        }
        signal(2, handle_signal); // SIGINT
        signal(15, handle_signal); // SIGTERM
    }
    #[cfg(windows)]
    unsafe {
        extern "system" {
            fn SetConsoleCtrlHandler(
                handler: Option<extern "system" fn(u32) -> i32>,
                add: i32,
            ) -> i32;
        }
        SetConsoleCtrlHandler(Some(handle_ctrl), 1);
    }
}

fn interruptible_sleep(duration: Duration) {
    let start = Instant::now();
    while start.elapsed() < duration && RUNNING.load(Ordering::SeqCst) {
        let remaining = duration.saturating_sub(start.elapsed());
        let chunk = remaining.min(Duration::from_millis(250));
        thread::sleep(chunk);
    }
}

#[derive(Debug, PartialEq, Eq)]
pub enum DaemonTickResult {
    Connected,
    Reauthenticated,
    AuthFailed,
    NotFortinet,
}

pub fn daemon_tick(
    username: &str,
    password: &str,
    last_connected: &mut bool,
    initial_run: bool,
) -> DaemonTickResult {
    let det = detect_fortinet_network();

    if det.internet_connected {
        if !*last_connected {
            logf!("\x1b[32m[OK]\x1b[0m     Internet is active and healthy");
            *last_connected = true;
        }
        DaemonTickResult::Connected
    } else {
        if *last_connected || initial_run {
            logf!("\x1b[33m[WARN]\x1b[0m   Internet connection lost or captive portal intercepted");
            *last_connected = false;
        }

        if det.is_fortinet_network {
            logf!("\x1b[34m[AUTH]\x1b[0m   Fortinet network detected. Authenticating...");
            if login(false, false, username, password) {
                *last_connected = true;
                DaemonTickResult::Reauthenticated
            } else {
                DaemonTickResult::AuthFailed
            }
        } else {
            logf!("\x1b[33m[NOTE]\x1b[0m   Not on a Fortinet network. Idling...");
            DaemonTickResult::NotFortinet
        }
    }
}

pub fn run_daemon(interval: Duration, username: &str, password: &str) {
    let _lock = match ProcessLock::try_acquire("daemon") {
        Ok(l) => l,
        Err(e) => {
            logf!("\x1b[31m[FAIL]\x1b[0m   {}", e);
            return;
        }
    };

    register_shutdown_handler();

    logf!(
        "\x1b[36m[DAEMON]\x1b[0m Starting background monitoring daemon (interval: {:?})",
        interval
    );

    let mut last_connected = false;
    let mut initial_run = true;
    let mut consecutive_failures = 0;

    while RUNNING.load(Ordering::SeqCst) {
        match daemon_tick(username, password, &mut last_connected, initial_run) {
            DaemonTickResult::Connected | DaemonTickResult::Reauthenticated => {
                consecutive_failures = 0;
            }
            DaemonTickResult::AuthFailed => {
                consecutive_failures += 1;
                logf!(
                    "\x1b[31m[FAIL]\x1b[0m   Login attempt failed (failures: {}). Will retry in {:?}",
                    consecutive_failures,
                    interval
                );
            }
            DaemonTickResult::NotFortinet => {
                consecutive_failures = 0;
            }
        }

        initial_run = false;

        if !RUNNING.load(Ordering::SeqCst) {
            break;
        }

        // Exponential backoff if network is persistently down to avoid battery drain
        let sleep_duration = if consecutive_failures > 3 {
            interval * 2
        } else {
            interval
        };

        interruptible_sleep(sleep_duration);
    }

    logf!("\x1b[36m[DAEMON]\x1b[0m Daemon received shutdown signal. Exiting cleanly...");
}

pub fn run_auto_mode(username: &str, password: &str) {
    // Acquire temporary single-instance lock for background triggers
    let _lock = match ProcessLock::try_acquire("auto") {
        Ok(l) => l,
        Err(_) => {
            // Another background instance is already handling the event
            return;
        }
    };

    let det = detect_fortinet_network();
    if det.is_fortinet_network && !det.internet_connected {
        logf!("\x1b[36m[AUTO]\x1b[0m   Background trigger: Captive portal detected. Logging in...");
        let _ = login(true, false, username, password);
    }
}
