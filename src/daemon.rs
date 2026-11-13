use std::thread;
use std::time::Duration;

use crate::detect::detect_fortinet_network;
use crate::logf;
use crate::network::login;

pub fn run_daemon(interval: Duration, username: &str, password: &str) {
    logf!("\x1b[36m[DAEMON]\x1b[0m Starting background monitoring daemon (interval: {:?})", interval);

    let mut last_connected = false;
    let mut initial_run = true;

    loop {
        let det = detect_fortinet_network();

        if det.internet_connected {
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
                } else {
                    logf!("\x1b[31m[FAIL]\x1b[0m   Login attempt failed. Will retry in {:?}", interval);
                }
            } else {
                logf!("\x1b[33m[NOTE]\x1b[0m   Not on a Fortinet network. Waiting for network reconnection...");
            }
        }

        initial_run = false;
        thread::sleep(interval);
    }
}

pub fn run_auto_mode(username: &str, password: &str) {
    let det = detect_fortinet_network();
    if det.is_fortinet_network && !det.internet_connected {
        logf!("\x1b[36m[AUTO]\x1b[0m   Background trigger: Captive portal detected. Logging in...");
        let _ = login(true, false, username, password);
    }
}
