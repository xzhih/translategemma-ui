use std::{
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    sync::Mutex,
    thread,
    time::{Duration, Instant},
};

use tauri::{AppHandle, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::{process::CommandChild, ShellExt};
use url::Url;
use uuid::Uuid;

const DESKTOP_TOKEN_ENV: &str = "TRANSLATEGEMMA_UI_DESKTOP_TOKEN";
const SHUTDOWN_TOKEN_HEADER: &str = "X-TranslateGemma-Desktop-Token";

#[derive(Default)]
struct DesktopState {
    sidecar: Mutex<Option<CommandChild>>,
    backend_url: Mutex<Option<String>>,
    shutdown_token: Mutex<Option<String>>,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            app.manage(DesktopState::default());
            create_splash_window(app.handle())?;

            let handle = app.handle().clone();
            thread::spawn(move || {
                if let Err(err) = launch_backend_and_open(handle.clone()) {
                    request_graceful_shutdown(&handle);
                    update_splash_error(&handle, &err);
                }
            });
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("failed to build desktop shell");

    app.run(|app_handle, event| {
        if matches!(event, RunEvent::ExitRequested { .. } | RunEvent::Exit) {
            request_graceful_shutdown(app_handle);
        }
    });
}

fn create_splash_window(app: &AppHandle) -> tauri::Result<()> {
    WebviewWindowBuilder::new(app, "splash", WebviewUrl::App("index.html".into()))
        .title("TranslateGemmaUI")
        .inner_size(520.0, 360.0)
        .min_inner_size(520.0, 360.0)
        .resizable(false)
        .center()
        .build()?;
    Ok(())
}

fn launch_backend_and_open(app: AppHandle) -> Result<(), String> {
    update_splash_status(
        &app,
        "Preparing desktop shell",
        "Reserving a local loopback port",
    );
    let listen_addr = reserve_loopback_listen()?;
    let backend_url = format!("http://{listen_addr}");
    let shutdown_token = format!(
        "{:032x}{:032x}",
        Uuid::new_v4().as_u128(),
        Uuid::new_v4().as_u128()
    );

    update_splash_status(
        &app,
        "Starting TranslateGemmaUI",
        &format!("Launching local service on {listen_addr}"),
    );
    let (rx, child) = app
        .shell()
        .sidecar("translategemma-ui")
        .map_err(|err| format!("unable to resolve bundled service: {err}"))?
        .args(["--webui", "--listen", &listen_addr])
        .env(DESKTOP_TOKEN_ENV, &shutdown_token)
        .spawn()
        .map_err(|err| format!("unable to launch bundled service: {err}"))?;

    drop(rx);
    {
        let state = app.state::<DesktopState>();
        *state
            .sidecar
            .lock()
            .map_err(|_| "desktop state lock poisoned")? = Some(child);
        *state
            .backend_url
            .lock()
            .map_err(|_| "desktop state lock poisoned")? = Some(backend_url.clone());
        *state
            .shutdown_token
            .lock()
            .map_err(|_| "desktop state lock poisoned")? = Some(shutdown_token);
    }

    update_splash_status(&app, "Waiting for local service", "Checking /healthz");
    wait_for_health(&listen_addr, Duration::from_secs(20))?;

    update_splash_status(
        &app,
        "Opening desktop window",
        "Loading the bundled web experience",
    );
    let url = Url::parse(&backend_url).map_err(|err| format!("invalid backend url: {err}"))?;
    WebviewWindowBuilder::new(&app, "main", WebviewUrl::External(url))
        .title("TranslateGemmaUI")
        .inner_size(1440.0, 920.0)
        .min_inner_size(1080.0, 720.0)
        .center()
        .build()
        .map_err(|err| format!("unable to open main window: {err}"))?;

    if let Some(splash) = app.get_webview_window("splash") {
        let _ = splash.close();
    }

    Ok(())
}

fn request_graceful_shutdown(app: &AppHandle) {
    let state = app.state::<DesktopState>();
    let backend_url = state
        .backend_url
        .lock()
        .ok()
        .and_then(|value| value.clone());
    let shutdown_token = state
        .shutdown_token
        .lock()
        .ok()
        .and_then(|value| value.clone());

    let shutdown_requested = match (backend_url.as_deref(), shutdown_token.as_deref()) {
        (Some(url), Some(token)) => post_desktop_shutdown(url, token).is_ok(),
        _ => false,
    };
    if shutdown_requested {
        return;
    }

    if let Some(child) = state.sidecar.lock().ok().and_then(|mut value| value.take()) {
        let _ = child.kill();
    }
}

fn reserve_loopback_listen() -> Result<String, String> {
    let listener = TcpListener::bind("127.0.0.1:0")
        .map_err(|err| format!("unable to reserve loopback port: {err}"))?;
    let addr = listener
        .local_addr()
        .map_err(|err| format!("unable to inspect reserved port: {err}"))?;
    Ok(addr.to_string())
}

fn wait_for_health(listen_addr: &str, timeout: Duration) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    let mut last_err = String::from("service did not become ready");
    while Instant::now() < deadline {
        match probe_health(listen_addr) {
            Ok(()) => return Ok(()),
            Err(err) => {
                last_err = err;
                thread::sleep(Duration::from_millis(250));
            }
        }
    }
    Err(format!("timed out waiting for {listen_addr}: {last_err}"))
}

fn probe_health(listen_addr: &str) -> Result<(), String> {
    let mut stream = TcpStream::connect(listen_addr).map_err(|err| err.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(|err| err.to_string())?;
    stream
        .set_write_timeout(Some(Duration::from_secs(1)))
        .map_err(|err| err.to_string())?;

    let request =
        format!("GET /healthz HTTP/1.1\r\nHost: {listen_addr}\r\nConnection: close\r\n\r\n");
    stream
        .write_all(request.as_bytes())
        .map_err(|err| err.to_string())?;

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(|err| err.to_string())?;
    let first_line = response.lines().next().unwrap_or_default().to_string();
    if first_line.starts_with("HTTP/1.1 200") || first_line.starts_with("HTTP/1.0 200") {
        return Ok(());
    }
    Err(format!("unexpected health response: {first_line}"))
}

fn post_desktop_shutdown(base_url: &str, shutdown_token: &str) -> Result<(), String> {
    let url = Url::parse(base_url).map_err(|err| err.to_string())?;
    let host = url.host_str().ok_or("missing desktop host")?;
    let port = url.port_or_known_default().ok_or("missing desktop port")?;
    let mut stream = TcpStream::connect((host, port)).map_err(|err| err.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(|err| err.to_string())?;
    stream
        .set_write_timeout(Some(Duration::from_secs(1)))
        .map_err(|err| err.to_string())?;

    let request = format!(
		"POST /api/desktop/shutdown HTTP/1.1\r\nHost: {host}:{port}\r\n{SHUTDOWN_TOKEN_HEADER}: {shutdown_token}\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	);
    stream
        .write_all(request.as_bytes())
        .map_err(|err| err.to_string())?;
    Ok(())
}

fn update_splash_status(app: &AppHandle, stage: &str, detail: &str) {
    update_splash(app, "desktopStatus", stage, detail);
}

fn update_splash_error(app: &AppHandle, detail: &str) {
    if let Some(window) = app.get_webview_window("splash") {
        let detail =
            serde_json::to_string(detail).unwrap_or_else(|_| "\"unexpected startup error\"".into());
        let _ = window.eval(&format!("window.desktopError({detail});"));
    }
}

fn update_splash(app: &AppHandle, handler: &str, stage: &str, detail: &str) {
    if let Some(window) = app.get_webview_window("splash") {
        let stage = serde_json::to_string(stage).unwrap_or_else(|_| "\"\"".into());
        let detail = serde_json::to_string(detail).unwrap_or_else(|_| "\"\"".into());
        let _ = window.eval(&format!("window.{handler}({stage}, {detail});"));
    }
}
