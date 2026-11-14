use std::env;
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use fortinet_lib::config::*;
use fortinet_lib::network::get_portal;

static TEST_MUTEX: Mutex<()> = Mutex::new(());

fn setup_test_config() -> (PathBuf, std::sync::MutexGuard<'static, ()>) {
    let guard = TEST_MUTEX.lock().unwrap();
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let tmp = env::temp_dir().join(format!("fortinet_cfg_test_{}", nanos));
    let _ = fs::create_dir_all(&tmp);
    env::set_var("FORTINET_CONFIG_DIR", &tmp);
    env::remove_var("FORTINET_PORTAL_HOST");
    env::remove_var("FORTINET_CHECK_URL");
    env::remove_var("FORTINET_INTERVAL");
    (tmp, guard)
}

#[test]
fn test_config_defaults() {
    let (_tmp, _guard) = setup_test_config();
    let config = load_config();
    assert_eq!(config.portal_host, "192.168.55.253:1003");
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
    assert_eq!(config.request_timeout_secs, 5);
}

#[test]
fn test_config_set_and_save() {
    let (_tmp, _guard) = setup_test_config();
    let new_host = "10.200.1.1:1003";
    set_portal_host(new_host).expect("save host");

    let loaded = load_config();
    assert_eq!(loaded.portal_host, new_host);

    reset_config().expect("reset");
    let reset = load_config();
    assert_eq!(reset.portal_host, DEFAULT_PORTAL_HOST);
}

#[test]
fn test_config_env_overrides() {
    let (_tmp, _guard) = setup_test_config();
    env::set_var("FORTINET_PORTAL_HOST", "172.16.50.1:8090");
    env::set_var("FORTINET_INTERVAL", "60");

    let cfg = load_config();
    assert_eq!(cfg.portal_host, "172.16.50.1:8090");
    assert_eq!(cfg.daemon_interval_secs, 60);

    env::remove_var("FORTINET_PORTAL_HOST");
    env::remove_var("FORTINET_INTERVAL");
}

#[test]
fn test_fallback_probe_when_primary_fails() {
    let (_tmp, _guard) = setup_test_config();

    // Spawn mock server that will act as the working fallback endpoint (Apple style)
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let stop_signal = Arc::new(AtomicBool::new(false));
    let stop_clone = stop_signal.clone();

    thread::spawn(move || {
        listener.set_nonblocking(true).unwrap();
        while !stop_clone.load(Ordering::Relaxed) {
            match listener.accept() {
                Ok((mut stream, _)) => {
                    use std::io::{Read, Write};
                    let mut buf = [0u8; 1024];
                    let _ = stream.read(&mut buf);
                    let resp = "HTTP/1.1 200 OK\r\nContent-Length: 45\r\nConnection: close\r\n\r\n<HTML><HEAD><TITLE>Success</TITLE></HEAD></HTML>";
                    let _ = stream.write_all(resp.as_bytes());
                    let _ = stream.flush();
                }
                Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                    thread::sleep(Duration::from_millis(10));
                }
                Err(_) => break,
            }
        }
    });

    let working_fallback_url = format!("http://127.0.0.1:{}/hotspot-detect.html", port);

    // Configure primary to point to a dead port on localhost (guaranteed to fail quickly)
    let mut cfg = load_config();
    cfg.check_urls = vec![
        "http://127.0.0.1:1/nonexistent".to_string(), // Dead primary
        working_fallback_url,                         // Working fallback
    ];
    save_config(&cfg).expect("save test config");

    let client = fortinet_lib::network::make_client();
    let res = get_portal(&client);

    stop_signal.store(true, Ordering::Relaxed);

    assert!(res.is_ok(), "get_portal must succeed via fallback URL");
    let (portal, is_online) = res.unwrap();
    assert!(is_online, "Fallback URL should report internet is online");
    assert_eq!(portal, None);
}
