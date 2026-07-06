use std::sync::Mutex;

use serde::Serialize;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{TrayIconBuilder, TrayIconEvent},
    Emitter, Manager, State, WindowEvent,
};
use tauri_plugin_shell::{process::CommandEvent, ShellExt};

#[derive(Default, Clone, Serialize)]
pub struct Connection {
    pub control_port: u16,
    pub control_token: String,
}

#[derive(Default)]
struct AppState {
    connection: Mutex<Connection>,
    hint_shown: Mutex<bool>,
}

#[tauri::command]
fn get_connection(state: State<AppState>) -> Connection {
    state.connection.lock().unwrap().clone()
}

#[tauri::command]
async fn open_external(app: tauri::AppHandle, url: String) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|e| e.to_string())
}

/// Parses the "SUB2API_READY {json}" handshake line emitted by the sidecar.
fn parse_handshake(line: &str) -> Option<Connection> {
    let trimmed = line.trim();
    let rest = trimmed.strip_prefix("SUB2API_READY ")?;
    let v: serde_json::Value = serde_json::from_str(rest).ok()?;
    Some(Connection {
        control_port: v.get("control_port")?.as_u64()? as u16,
        control_token: v.get("control_token")?.as_str()?.to_string(),
    })
}

fn spawn_sidecar(app: &tauri::AppHandle) {
    let data_dir = app
        .path()
        .app_config_dir()
        .unwrap_or_else(|_| std::env::temp_dir())
        .join("data");
    let _ = std::fs::create_dir_all(&data_dir);

    let sidecar = match app.shell().sidecar("sub2api-sidecar") {
        Ok(cmd) => cmd.args([
            "--data-dir".to_string(),
            data_dir.to_string_lossy().to_string(),
            "--control-port".to_string(),
            "0".to_string(),
        ]),
        Err(e) => {
            eprintln!("failed to resolve sidecar: {e}");
            return;
        }
    };

    let (mut rx, _child) = match sidecar.spawn() {
        Ok(v) => v,
        Err(e) => {
            eprintln!("failed to spawn sidecar: {e}");
            return;
        }
    };

    let app_handle = app.clone();
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(bytes) => {
                    let line = String::from_utf8_lossy(&bytes);
                    for l in line.lines() {
                        if let Some(conn) = parse_handshake(l) {
                            if let Some(state) = app_handle.try_state::<AppState>() {
                                *state.connection.lock().unwrap() = conn.clone();
                            }
                            let _ = app_handle.emit("sidecar-ready", conn);
                        }
                    }
                }
                CommandEvent::Stderr(bytes) => {
                    eprint!("[sidecar] {}", String::from_utf8_lossy(&bytes));
                }
                CommandEvent::Terminated(payload) => {
                    eprintln!("[sidecar] terminated: {:?}", payload.code);
                    break;
                }
                _ => {}
            }
        }
    });
}

fn build_tray(app: &tauri::AppHandle) -> tauri::Result<()> {
    let show = MenuItem::with_id(app, "show", "打开面板", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &quit])?;

    TrayIconBuilder::with_id("main")
        .tooltip("Amber")
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => {
                if let Some(win) = app.get_webview_window("main") {
                    let _ = win.show();
                    let _ = win.set_focus();
                }
            }
            "quit" => app.exit(0),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click { .. } = event {
                let app = tray.app_handle();
                if let Some(win) = app.get_webview_window("main") {
                    let _ = win.show();
                    let _ = win.set_focus();
                }
            }
        })
        .build(app)?;
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Work around a WebView2 compositing bug on Windows where the scrollable
    // content area stops painting (goes blank) shortly after render.
    #[cfg(target_os = "windows")]
    {
        let mut args = std::env::var("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS")
            .unwrap_or_default();
        if !args.contains("--disable-gpu") {
            if !args.is_empty() {
                args.push(' ');
            }
            args.push_str("--disable-gpu");
        }
        std::env::set_var("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", args);
    }

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_shell::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![get_connection, open_external])
        .setup(|app| {
            let handle = app.handle();
            build_tray(handle)?;
            spawn_sidecar(handle);
            Ok(())
        })
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                // Minimize to tray instead of quitting.
                api.prevent_close();
                let _ = window.hide();
                if let Some(state) = window.app_handle().try_state::<AppState>() {
                    let mut shown = state.hint_shown.lock().unwrap();
                    if !*shown {
                        *shown = true;
                        let _ = window.app_handle().emit("tray-hint", ());
                    }
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
