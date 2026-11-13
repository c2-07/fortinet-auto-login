mod credentials;
mod daemon;
mod detect;
mod error;
mod lock;
mod logger;
mod network;
mod platform;
mod session;
mod types;
mod upgrade;
mod warp;

use std::env;
use std::process;
use std::time::Duration;

use credentials::{load_credentials, prompt_credentials, save_credentials};
use daemon::{run_auto_mode, run_daemon};
use detect::print_status;
use network::{login, logout};
use platform::{clear_logs, install_service, uninstall_service, view_logs};
use types::Credentials;
use upgrade::check_upgrade;
use warp::{disconnect_warp, fix_and_connect_warp};

const VERSION: &str = env!("CARGO_PKG_VERSION");

fn print_custom_usage() {
    println!("Usage: autologin [options]");
    println!("\nAuthentication & Session:");
    println!("  autologin                    Log in to captive portal (default, prompts on first run)");
    println!("  autologin -u <user> -p <pwd> Update saved credentials and authenticate");
    println!("  autologin -logout            Log out of current captive portal session");
    println!("\nNetwork & Diagnostics:");
    println!("  autologin -status            Inspect network, Fortinet detection & active session");
    println!("\nBackground Service & Daemon:");
    println!("  autologin -service install   Install event-driven background service (macOS / Linux)");
    println!("  autologin -service uninstall Remove background service");
    println!("  autologin -daemon [-i 45s]   Run continuous polling daemon in foreground");
    println!("\nCloudflare WARP Circumvention:");
    println!("  autologin -warp              Intelligently rotate Anycast IPs & ports to connect Cloudflare WARP");
    println!("  autologin -warp-disconnect   Disconnect WARP and restore normal network routing");
    println!("\nLogs & Maintenance:");
    println!("  autologin -logs [-n 40]      View background service logs (default: last 40 lines)");
    println!("  autologin -logs -f           Live stream / follow logs (like tail -f)");
    println!("  autologin -logs -clear       Clear background log file");
    println!("  autologin -upgrade           Check for and install the latest release from GitHub");
    println!("  autologin -version           Print version information\n");
}

fn parse_duration(s: &str) -> Duration {
    let s = s.trim();
    if let Some(sec_str) = s.strip_suffix('s') {
        if let Ok(sec) = sec_str.parse::<u64>() {
            return Duration::from_secs(sec);
        }
    } else if let Some(min_str) = s.strip_suffix('m') {
        if let Ok(min) = min_str.parse::<u64>() {
            return Duration::from_secs(min * 60);
        }
    } else if let Ok(sec) = s.parse::<u64>() {
        return Duration::from_secs(sec);
    }
    Duration::from_secs(45)
}

fn main() {
    let args: Vec<String> = env::args().collect();

    let mut username_flag = String::new();
    let mut password_flag = String::new();
    let mut logout_flag = false;
    let mut status_flag = false;
    let mut service_cmd = String::new();
    let mut daemon_flag = false;
    let mut interval = Duration::from_secs(45);
    let mut logs_flag = false;
    let mut follow_logs = false;
    let mut clear_logs_flag = false;
    let mut log_lines = 40;
    let mut warp_flag = false;
    let mut warp_disconnect_flag = false;
    let mut upgrade_flag = false;
    let mut version_flag = false;
    let mut auto_flag = false;

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "-h" | "--help" | "-help" => {
                print_custom_usage();
                return;
            }
            "-v" | "-version" | "--version" => {
                version_flag = true;
            }
            "-u" | "-username" | "--username" => {
                if i + 1 < args.len() {
                    username_flag = args[i + 1].clone();
                    i += 1;
                }
            }
            "-p" | "-password" | "--password" => {
                if i + 1 < args.len() {
                    password_flag = args[i + 1].clone();
                    i += 1;
                }
            }
            "-logout" | "-l" | "--logout" => {
                logout_flag = true;
            }
            "-status" | "-detect" | "--status" | "--detect" => {
                status_flag = true;
            }
            "-service" | "--service" => {
                if i + 1 < args.len() {
                    service_cmd = args[i + 1].to_lowercase();
                    i += 1;
                }
            }
            "-install" | "--install" => {
                service_cmd = "install".to_string();
            }
            "-uninstall" | "--uninstall" => {
                service_cmd = "uninstall".to_string();
            }
            "-daemon" | "-d" | "--daemon" | "-keepalive" | "-k" | "--keepalive" => {
                daemon_flag = true;
            }
            "-i" | "-interval" | "--interval" => {
                if i + 1 < args.len() {
                    interval = parse_duration(&args[i + 1]);
                    i += 1;
                }
            }
            "-logs" | "--logs" => {
                logs_flag = true;
            }
            "-f" | "-follow" | "--follow" => {
                follow_logs = true;
                logs_flag = true;
            }
            "-clear" | "-clean" | "--clear" | "--clean" => {
                clear_logs_flag = true;
            }
            "-n" => {
                if i + 1 < args.len() {
                    if let Ok(n) = args[i + 1].parse::<usize>() {
                        log_lines = n;
                    }
                    i += 1;
                }
            }
            "-warp" | "--warp" => {
                warp_flag = true;
            }
            "-warp-disconnect" | "--warp-disconnect" => {
                warp_disconnect_flag = true;
            }
            "-upgrade" | "--upgrade" => {
                upgrade_flag = true;
            }
            "-auto" | "--auto" => {
                auto_flag = true;
            }
            _ => {
                println!("\x1b[31m[FAIL]\x1b[0m   Unknown option: {}", args[i]);
                print_custom_usage();
                process::exit(1);
            }
        }
        i += 1;
    }

    // 1. Version
    if version_flag {
        println!("fortinet-auto-login v{} (rust)", VERSION);
        return;
    }

    // 2. Upgrade
    if upgrade_flag {
        if !check_upgrade() {
            process::exit(1);
        }
        return;
    }

    // 3. Status & Diagnostics
    if status_flag {
        print_status();
        return;
    }

    // 4. Service Management
    if !service_cmd.is_empty() {
        match service_cmd.as_str() {
            "install" => {
                install_service();
                return;
            }
            "uninstall" | "remove" => {
                uninstall_service();
                return;
            }
            _ => {
                println!("\x1b[31m[FAIL]\x1b[0m   Unknown service command: '{}'. Use 'install' or 'uninstall'.", service_cmd);
                process::exit(1);
            }
        }
    }

    // 5. Logs Management
    if logs_flag || clear_logs_flag {
        if clear_logs_flag {
            clear_logs();
            return;
        }
        view_logs(follow_logs, log_lines);
        return;
    }

    // Credentials resolution
    let (username, password) = if !username_flag.is_empty() && !password_flag.is_empty() {
        let creds = Credentials {
            username: username_flag.clone(),
            password: password_flag.clone(),
            ssid: String::new(),
        };
        let _ = save_credentials(&creds);
        (username_flag, password_flag)
    } else if let Some(creds) = load_credentials() {
        (creds.username, creds.password)
    } else if !logout_flag {
        let (u, p) = prompt_credentials();
        let creds = Credentials {
            username: u.clone(),
            password: p.clone(),
            ssid: String::new(),
        };
        let _ = save_credentials(&creds);
        (u, p)
    } else {
        (String::new(), String::new())
    };

    // 6. Internal Background Trigger
    if auto_flag {
        run_auto_mode(&username, &password);
        return;
    }

    // 7. Cloudflare WARP Circumvention
    if warp_flag {
        if !fix_and_connect_warp(&username, &password) {
            process::exit(1);
        }
        return;
    }

    if warp_disconnect_flag {
        if !disconnect_warp() {
            process::exit(1);
        }
        return;
    }

    // 8. Logout
    if logout_flag {
        if !logout() {
            process::exit(1);
        }
        return;
    }

    // 9. Daemon Mode
    if daemon_flag {
        run_daemon(interval, &username, &password);
        return;
    }

    // 10. Default Login
    let _lock = match lock::ProcessLock::try_acquire("login") {
        Ok(l) => Some(l),
        Err(_) => {
            logf!("\x1b[33m[NOTE]\x1b[0m   Another autologin process is actively running. Exiting cleanly.");
            return;
        }
    };

    let mut success = false;
    for attempt in 1..=5 {
        if attempt > 1 {
            logf!("\x1b[36m[RETRY]\x1b[0m  {}/5", attempt);
        }
        if login(false, false, &username, &password) {
            success = true;
            break;
        }
        std::thread::sleep(Duration::from_secs(2));
    }

    if !success {
        logf!("\x1b[31m[FAIL]\x1b[0m   Auth failed");
        process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_duration() {
        assert_eq!(parse_duration("30s"), Duration::from_secs(30));
        assert_eq!(parse_duration("2m"), Duration::from_secs(120));
        assert_eq!(parse_duration("60"), Duration::from_secs(60));
    }

    #[test]
    fn test_warp_candidate_generation() {
        let candidates = warp::generate_candidate_endpoints();
        assert_eq!(candidates.len(), 72);
        assert!(candidates.contains(&"162.159.192.1:500".to_string()));
        assert!(candidates.contains(&"162.159.192.1:4500".to_string()));
    }

    #[test]
    fn test_extract_countdown() {
        let body = r#"<script>var countdown = 14400;</script>"#;
        assert_eq!(session::extract_countdown(body), Some(14400));
    }

    #[test]
    fn test_random_magic() {
        let m1 = session::random_magic();
        let m2 = session::random_magic();
        assert_eq!(m1.len(), 16);
        assert_ne!(m1, m2);
    }

    #[test]
    fn test_extract_portal_url() {
        let html = r#"<html><script>window.location="https://192.168.55.253:1003/fgtauth?magic=079c4a9e6abca729";</script></html>"#;
        let url = network::extract_portal_url(html);
        assert_eq!(
            url,
            Some("https://192.168.55.253:1003/fgtauth?magic=079c4a9e6abca729".to_string())
        );

        let magic = network::parse_magic_from_url(&url.unwrap());
        assert_eq!(magic, Some("079c4a9e6abca729".to_string()));
    }

    #[test]
    fn test_parse_host_from_url() {
        let url = "https://192.168.55.253:1003/fgtauth?magic=abc";
        assert_eq!(network::parse_host_from_url(url), Some("192.168.55.253:1003".to_string()));
    }

    #[test]
    fn test_process_lock() {
        let lock1 = lock::ProcessLock::try_acquire("unit_test");
        if let Err(ref e) = lock1 {
            eprintln!("lock1 error: {:?}", e);
        }
        assert!(lock1.is_ok());

        // Second lock with same name should fail while lock1 is held
        let lock2 = lock::ProcessLock::try_acquire("unit_test");
        assert!(lock2.is_err());

        drop(lock1);

        // After dropping lock1, can acquire again
        let lock3 = lock::ProcessLock::try_acquire("unit_test");
        assert!(lock3.is_ok());
    }

    #[test]
    fn test_session_expiry() {
        let expired_session = types::Session {
            host: "test".to_string(),
            magic: "123".to_string(),
            timeout: 100,
            created_at: "1000000000".to_string(), // In the past
            login_url: "".to_string(),
        };
        assert!(session::is_session_expired(&expired_session));
    }
}
