use std::thread;
use std::time::Duration;

use crate::detect::detect_fortinet_network;
use crate::lock::ProcessLock;
use crate::logf;
use crate::network::login;

pub fn run_daemon(interval: Duration, username: &str, password: &str) {
    let _lock = match ProcessLock::try_acquire("daemon") {
        Ok(l) => l,
        Err(e) => {
            logf!("\x1b[31m[FAIL]\x1b[0m   {}", e);
            return;
        }
    };

    logf!(
        "\x1b[36m[DAEMON]\x1b[0m Starting background monitoring daemon (interval: {:?})",
        interval
    );

    let mut last_connected = false;
    let mut initial_run = true;
    let mut consecutive_failures = 0;

    loop {
        let det = detect_fortinet_network();

        if det.internet_connected {
            consecutive_failures = 0;
            if !last_connected {
                logf!("\x1b[32m[OK]\x1b[0m     Internet is active and healthy");
                last_connected = true;
            }
        } else {
            if last_connected || initial_run {
                logf!("\x1b[33m[WARN]\x1b[0m   Internet connection lost or captive portal intercepted");
                last_connected = false;
            }

            if det.is_fortinet_network {
                logf!("\x1b[34m[AUTH]\x1b[0m   Fortinet network detected. Authenticating...");
                if login(false, false, username, password) {
                    last_connected = true;
                    consecutive_failures = 0;
                } else {
                    consecutive_failures += 1;
                    logf!("\x1b[31m[FAIL]\x1b[0m   Login attempt failed (failures: {}). Will retry in {:?}", consecutive_failures, interval);
                }
            } else {
                logf!("\x1b[33m[NOTE]\x1b[0m   Not on a Fortinet network. Idling...");
            }
        }

        initial_run = false;

        // Exponential backoff if network is persistently down to avoid battery drain
        let sleep_duration = if consecutive_failures > 3 {
            interval * 2
        } else {
            interval
        };

        thread::sleep(sleep_duration);
    }
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
