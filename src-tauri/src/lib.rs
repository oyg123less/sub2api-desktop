use std::sync::Mutex;

use serde::Serialize;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Emitter, Manager, RunEvent, State, WindowEvent,
};
use tauri_plugin_shell::{
    process::{CommandChild, CommandEvent},
    ShellExt,
};

#[derive(Default, Clone, Serialize)]
pub struct Connection {
    pub control_port: u16,
    pub control_token: String,
}

#[derive(Default)]
struct AppState {
    connection: Mutex<Connection>,
    hint_shown: Mutex<bool>,
    sidecar: Mutex<Option<CommandChild>>,
}

fn kill_sidecar(app: &tauri::AppHandle) {
    if let Some(state) = app.try_state::<AppState>() {
        if let Some(child) = state.sidecar.lock().unwrap().take() {
            let _ = child.kill();
        }
    }
}

#[tauri::command]
fn get_connection(state: State<AppState>) -> Connection {
    state.connection.lock().unwrap().clone()
}

/// Returns the directory where the data-dir override pointer file lives. This
/// location never moves (it is always the app config dir) so the app can always
/// find where the user relocated their data.
fn pointer_dir(app: &tauri::AppHandle) -> std::path::PathBuf {
    app.path()
        .app_config_dir()
        .unwrap_or_else(|_| std::env::temp_dir())
}

fn default_data_dir(app: &tauri::AppHandle) -> std::path::PathBuf {
    pointer_dir(app).join("data")
}

/// Resolves the effective data directory: the user override from location.json
/// if set and non-empty, otherwise the default (app_config_dir/data).
fn resolve_data_dir(app: &tauri::AppHandle) -> std::path::PathBuf {
    let pointer = pointer_dir(app).join("location.json");
    if let Ok(bytes) = std::fs::read(&pointer) {
        if let Ok(v) = serde_json::from_slice::<serde_json::Value>(&bytes) {
            if let Some(dir) = v.get("data_dir").and_then(|d| d.as_str()) {
                let dir = dir.trim();
                if !dir.is_empty() {
                    return std::path::PathBuf::from(dir);
                }
            }
        }
    }
    default_data_dir(app)
}

#[derive(Serialize)]
struct DataDirInfo {
    current: String,
    default: String,
    is_custom: bool,
}

#[tauri::command]
fn get_data_dir(app: tauri::AppHandle) -> DataDirInfo {
    let current = resolve_data_dir(&app);
    let default = default_data_dir(&app);
    DataDirInfo {
        is_custom: current != default,
        current: current.to_string_lossy().to_string(),
        default: default.to_string_lossy().to_string(),
    }
}

#[tauri::command]
async fn open_data_dir(app: tauri::AppHandle) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    let dir = resolve_data_dir(&app);
    let _ = std::fs::create_dir_all(&dir);
    app.opener()
        .open_path(dir.to_string_lossy().to_string(), None::<&str>)
        .map_err(|e| e.to_string())
}

/// Moves the data files (db + key + sqlite side files) from one dir to another,
/// retrying briefly to tolerate the sidecar releasing the sqlite file handle.
fn move_data_files(from: &std::path::Path, to: &std::path::Path) -> Result<(), String> {
    std::fs::create_dir_all(to).map_err(|e| e.to_string())?;
    let names = [
        "sub2api.db",
        "sub2api.db-wal",
        "sub2api.db-shm",
        "key",
    ];
    for name in names {
        let src = from.join(name);
        if !src.exists() {
            continue;
        }
        let dst = to.join(name);
        let mut last_err = String::new();
        let mut moved = false;
        for _ in 0..20 {
            match std::fs::rename(&src, &dst) {
                Ok(()) => {
                    moved = true;
                    break;
                }
                Err(_) => {
                    // Cross-device or locked: fall back to copy+remove.
                    match std::fs::copy(&src, &dst) {
                        Ok(_) => {
                            let _ = std::fs::remove_file(&src);
                            moved = true;
                            break;
                        }
                        Err(e) => {
                            last_err = e.to_string();
                            std::thread::sleep(std::time::Duration::from_millis(150));
                        }
                    }
                }
            }
        }
        if !moved {
            return Err(format!("移动 {name} 失败: {last_err}"));
        }
    }
    Ok(())
}

#[tauri::command]
async fn set_data_dir(app: tauri::AppHandle, path: String) -> Result<DataDirInfo, String> {
    let new_dir = std::path::PathBuf::from(path.trim());
    if new_dir.as_os_str().is_empty() {
        return Err("路径不能为空".to_string());
    }
    let old_dir = resolve_data_dir(&app);
    if new_dir == old_dir {
        return Ok(get_data_dir(app));
    }
    std::fs::create_dir_all(&new_dir).map_err(|e| format!("无法创建目录: {e}"))?;

    // Stop the sidecar so it releases the sqlite file, then migrate the data.
    kill_sidecar(&app);
    std::thread::sleep(std::time::Duration::from_millis(400));
    move_data_files(&old_dir, &new_dir)?;

    // Persist the override pointer (empty when reverting to default).
    let pointer = pointer_dir(&app).join("location.json");
    let default = default_data_dir(&app);
    let stored = if new_dir == default { String::new() } else { new_dir.to_string_lossy().to_string() };
    let body = serde_json::json!({ "data_dir": stored }).to_string();
    let _ = std::fs::create_dir_all(pointer_dir(&app));
    std::fs::write(&pointer, body).map_err(|e| format!("保存配置失败: {e}"))?;

    // Respawn the sidecar against the new location.
    spawn_sidecar(&app);
    Ok(get_data_dir(app))
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
    let data_dir = resolve_data_dir(app);
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

    let (mut rx, child) = match sidecar.spawn() {
        Ok(v) => v,
        Err(e) => {
            eprintln!("failed to spawn sidecar: {e}");
            return;
        }
    };

    if let Some(state) = app.try_state::<AppState>() {
        *state.sidecar.lock().unwrap() = Some(child);
    }

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
            "quit" => {
                kill_sidecar(app);
                app.exit(0);
            }
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            // Only react to left-click releases; grabbing focus on a
            // right-click would immediately dismiss the context menu.
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
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
        .plugin(tauri_plugin_dialog::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![
            get_connection,
            open_external,
            get_data_dir,
            set_data_dir,
            open_data_dir
        ])
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
        .build(tauri::generate_context!())
        .expect("error while running tauri application")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                kill_sidecar(app);
            }
        });
}
