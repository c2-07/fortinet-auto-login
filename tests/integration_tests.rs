use std::io::{Read, Write};
use std::net::TcpListener;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use fortinet_lib::credentials::{
    credentials_file_path, delete_credentials, load_credentials, save_credentials,
};
use fortinet_lib::lock::ProcessLock;
use fortinet_lib::network::{
    do_with_retry, extract_portal_url, make_client, parse_host_from_url, parse_magic_from_url,
};
use fortinet_lib::session::{
    clear_session, extract_countdown, is_session_expired, load_session, random_magic, save_session,
};
use fortinet_lib::types::{Credentials, Session};

/// Helper: spawns a mock HTTP server on an ephemeral loopback port
fn spawn_mock_server<F>(handler: F) -> (String, Arc<AtomicBool>)
where
    F: Fn(&str) -> String + Send + Sync + 'static,
{
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind ephemeral port");
    let addr = listener.local_addr().unwrap();
    let port = addr.port();
    let base_url = format!("http://127.0.0.1:{}", port);

    let stop_signal = Arc::new(AtomicBool::new(false));
    let stop_clone = Arc::clone(&stop_signal);
    let handler = Arc::new(handler);

    thread::spawn(move || {
        listener.set_nonblocking(true).unwrap();
        while !stop_clone.load(Ordering::Relaxed) {
            match listener.accept() {
                Ok((mut stream, _)) => {
                    let mut buf = [0u8; 4096];
                    let _ = stream.set_read_timeout(Some(Duration::from_millis(500)));
                    let n = stream.read(&mut buf).unwrap_or(0);
                    let req_str = String::from_utf8_lossy(&buf[..n]);

                    let response = handler(&req_str);
                    let _ = stream.write_all(response.as_bytes());
                    let _ = stream.flush();
                }
                Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                    thread::sleep(Duration::from_millis(10));
                }
                Err(_) => break,
            }
        }
    });

    (base_url, stop_signal)
}

static TEST_ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

fn setup_test_env() -> std::sync::MutexGuard<'static, ()> {
    let guard = TEST_ENV_LOCK.lock().unwrap();
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let tmp = std::env::temp_dir().join(format!("fortinet_test_{}", nanos));
    let _ = std::fs::create_dir_all(&tmp);
    std::env::set_var("FORTINET_CONFIG_DIR", tmp);
    guard
}

#[test]
fn test_integration_already_authenticated() {
    let (url, stop) = spawn_mock_server(|_req| {
        "HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nsuccess\n".to_string()
    });

    let client = make_client();
    let mut resp = client.get(&url).call().expect("http request");
    let body = resp.body_mut().read_to_string().unwrap_or_default();
    assert_eq!(body.trim(), "success");

    stop.store(true, Ordering::Relaxed);
}

#[test]
fn test_integration_captive_portal_redirect_detection() {
    let (url, stop) = spawn_mock_server(|_req| {
        let portal_html = r#"<HTML><HEAD><TITLE>Firewall Authentication</TITLE></HEAD>
<BODY><script>window.location="https://192.168.55.253:1003/fgtauth?magic=079c4a9e6abca729";</script>
</BODY></HTML>"#;
        format!(
            "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            portal_html.len(),
            portal_html
        )
    });

    let client = make_client();
    let mut resp = client.get(&url).call().expect("http request");
    let body = resp.body_mut().read_to_string().unwrap_or_default();

    let portal_url = extract_portal_url(&body);
    assert_eq!(
        portal_url,
        Some("https://192.168.55.253:1003/fgtauth?magic=079c4a9e6abca729".to_string())
    );

    let magic = parse_magic_from_url(&portal_url.unwrap());
    assert_eq!(magic, Some("079c4a9e6abca729".to_string()));

    stop.store(true, Ordering::Relaxed);
}

#[test]
fn test_integration_login_flow_valid_credentials() {
    let (url, stop) = spawn_mock_server(|req| {
        if req.starts_with("GET") {
            // Preflight login page
            let body = "<html><title>Fortinet Login</title></html>";
            format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body
            )
        } else if req.starts_with("POST") {
            // Auth success response with countdown
            let body = r#"<html><script>var countdown = 14400;</script><body>Authentication Success</body></html>"#;
            format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body
            )
        } else {
            "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n".to_string()
        }
    });

    let client = make_client();

    // 1. Preflight
    let preflight_res = client.get(&url).call();
    assert!(preflight_res.is_ok());

    // 2. Submit form
    let post_res = client.post(&format!("{}/fgtauth", url)).send_form([
        ("4Tredir", "http://google.com/"),
        ("magic", "079c4a9e6abca729"),
        ("answer", "1"),
        ("username", "testuser"),
        ("password", "testpass"),
    ]);
    assert!(post_res.is_ok());

    let mut res = post_res.unwrap();
    let body = res.body_mut().read_to_string().unwrap_or_default();
    let timeout = extract_countdown(&body);
    assert_eq!(timeout, Some(14400));

    stop.store(true, Ordering::Relaxed);
}

#[test]
fn test_integration_login_flow_invalid_credentials() {
    let (url, stop) = spawn_mock_server(|req| {
        if req.starts_with("POST") {
            let body =
                "<html><body>Authentication failed: invalid username or password.</body></html>";
            format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body
            )
        } else {
            "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n".to_string()
        }
    });

    let client = make_client();
    let mut res = client
        .post(&url)
        .send_form([("username", "baduser"), ("password", "badpass")])
        .unwrap();

    let body = res.body_mut().read_to_string().unwrap_or_default();
    let body_lower = body.to_lowercase();
    assert!(body_lower.contains("invalid") || body_lower.contains("failed"));

    stop.store(true, Ordering::Relaxed);
}

#[test]
fn test_integration_retry_policy_on_server_error() {
    let attempts = Arc::new(AtomicUsize::new(0));
    let attempts_clone = Arc::clone(&attempts);

    let (url, stop) = spawn_mock_server(move |_req| {
        let count = attempts_clone.fetch_add(1, Ordering::SeqCst);
        if count < 2 {
            "HTTP/1.1 500 Internal Server Error\r\nContent-Length: 0\r\n\r\n".to_string()
        } else {
            "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok".to_string()
        }
    });

    let client = make_client();
    let result = do_with_retry(3, || client.get(&url).call());
    assert!(result.is_ok());
    assert!(attempts.load(Ordering::SeqCst) >= 2);

    stop.store(true, Ordering::Relaxed);
}

#[test]
fn test_integration_process_lock_concurrency() {
    let lock_name = "integration_test_lock";
    let lock1 = ProcessLock::try_acquire(lock_name);
    assert!(lock1.is_ok(), "First lock acquisition must succeed");

    let lock2 = ProcessLock::try_acquire(lock_name);
    assert!(
        lock2.is_err(),
        "Concurrent lock acquisition must be rejected"
    );

    drop(lock1);

    let lock3 = ProcessLock::try_acquire(lock_name);
    assert!(lock3.is_ok(), "Lock acquisition after release must succeed");
}

#[test]
fn test_integration_credentials_persistence_and_permissions() {
    let _env_guard = setup_test_env();
    let test_creds = Credentials {
        username: "qa_test_user".to_string(),
        password: "qa_test_secret_password".to_string(),
        ssid: "Campus-WiFi".to_string(),
    };

    let save_res = save_credentials(&test_creds);
    assert!(save_res.is_ok());

    // Verify Debug representation redacts sensitive password bytes
    let debug_repr = format!("{:?}", test_creds);
    assert!(debug_repr.contains("[REDACTED]"));
    assert!(!debug_repr.contains("qa_test_secret_password"));

    let loaded = load_credentials().expect("credentials should load");
    assert_eq!(loaded.username, "qa_test_user");
    assert_eq!(loaded.password, "qa_test_secret_password");
    assert_eq!(loaded.ssid, "Campus-WiFi");

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let metadata = std::fs::metadata(credentials_file_path()).unwrap();
        let mode = metadata.permissions().mode() & 0o777;
        assert_eq!(mode, 0o600, "credentials.json must have 0600 permissions");
    }

    delete_credentials();
    assert!(load_credentials().is_none());
}

#[test]
fn test_integration_session_lifecycle() {
    let _env_guard = setup_test_env();
    let session = Session {
        host: "192.168.55.253:1003".to_string(),
        magic: random_magic(),
        timeout: 14400,
        created_at: "1700000000".to_string(),
        login_url: "https://192.168.55.253:1003/fgtauth".to_string(),
    };

    assert!(save_session(&session).is_ok());

    let loaded = load_session().expect("session should load");
    assert_eq!(loaded.host, "192.168.55.253:1003");
    assert_eq!(loaded.timeout, 14400);
    assert!(is_session_expired(&loaded));

    clear_session();
    assert!(load_session().is_none());
}

#[test]
fn test_integration_url_parsers() {
    let url = "https://192.168.55.253:1003/fgtauth?magic=079c4a9e6abca729&extra=true";
    assert_eq!(
        parse_host_from_url(url),
        Some("192.168.55.253:1003".to_string())
    );
    assert_eq!(
        parse_magic_from_url(url),
        Some("079c4a9e6abca729".to_string())
    );

    let invalid_url = "not_a_valid_url";
    assert_eq!(parse_host_from_url(invalid_url), None);
    assert_eq!(parse_magic_from_url(invalid_url), None);
}
