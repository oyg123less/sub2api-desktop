mod supervisor;

use std::{io::Write, sync::Mutex, time::Duration};

use serde::Serialize;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    Emitter, Manager, RunEvent, WindowEvent,
};

#[derive(Default)]
pub(crate) struct AppState {
    supervisor: supervisor::Supervisor,
    hint_shown: Mutex<bool>,
}

fn kill_sidecar(app: &tauri::AppHandle) {
    supervisor::stop(app, supervisor::StopIntent::Exit);
}

#[tauri::command]
fn get_connection(app: tauri::AppHandle) -> supervisor::Connection {
    supervisor::connection(&app)
}

#[tauri::command]
fn get_backend_state(app: tauri::AppHandle) -> supervisor::BackendState {
    supervisor::backend_state(&app)
}

#[tauri::command]
fn restart_backend(app: tauri::AppHandle) -> supervisor::BackendState {
    supervisor::manual_restart(&app);
    supervisor::backend_state(&app)
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

const DATA_FILES: [&str; 4] = ["sub2api.db", "sub2api.db-wal", "sub2api.db-shm", "key"];

fn copy_data_files(from: &std::path::Path, to: &std::path::Path) -> Result<(), String> {
    std::fs::create_dir_all(to).map_err(|e| e.to_string())?;
    for name in DATA_FILES {
        let source = from.join(name);
        if source.exists() {
            std::fs::copy(&source, to.join(name)).map_err(|e| format!("copy {name}: {e}"))?;
        }
    }
    Ok(())
}

fn validate_migration_copy(dir: &std::path::Path) -> Result<(), String> {
    let database = dir.join("sub2api.db");
    let key = dir.join("key");
    if !database.is_file() || !key.is_file() {
        return Err("migration copy is missing sub2api.db or key".to_string());
    }
    let bytes = std::fs::read(&database).map_err(|e| format!("read copied database: {e}"))?;
    if !bytes.starts_with(b"SQLite format 3\0") {
        return Err("copied database has an invalid SQLite header".to_string());
    }
    let key_text = std::fs::read_to_string(&key).map_err(|e| format!("read copied key: {e}"))?;
    if key_text.trim().len() < 40 {
        return Err("copied encryption key is invalid".to_string());
    }
    Ok(())
}

fn atomic_replace(path: &std::path::Path, bytes: &[u8]) -> Result<(), String> {
    let temporary = path.with_extension("json.tmp");
    let mut file =
        std::fs::File::create(&temporary).map_err(|e| format!("create temporary pointer: {e}"))?;
    file.write_all(bytes)
        .map_err(|e| format!("write temporary pointer: {e}"))?;
    file.sync_all()
        .map_err(|e| format!("sync temporary pointer: {e}"))?;
    drop(file);
    replace_file(&temporary, path).map_err(|e| format!("replace location pointer: {e}"))
}

#[cfg(not(windows))]
fn replace_file(source: &std::path::Path, target: &std::path::Path) -> std::io::Result<()> {
    std::fs::rename(source, target)
}

#[cfg(windows)]
fn replace_file(source: &std::path::Path, target: &std::path::Path) -> std::io::Result<()> {
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::Storage::FileSystem::{
        MoveFileExW, MOVEFILE_REPLACE_EXISTING, MOVEFILE_WRITE_THROUGH,
    };
    let source: Vec<u16> = source
        .as_os_str()
        .encode_wide()
        .chain(std::iter::once(0))
        .collect();
    let target: Vec<u16> = target
        .as_os_str()
        .encode_wide()
        .chain(std::iter::once(0))
        .collect();
    let result = unsafe {
        MoveFileExW(
            source.as_ptr(),
            target.as_ptr(),
            MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH,
        )
    };
    if result == 0 {
        Err(std::io::Error::last_os_error())
    } else {
        Ok(())
    }
}

fn write_location_pointer(app: &tauri::AppHandle, data_dir: &str) -> Result<(), String> {
    let dir = pointer_dir(app);
    std::fs::create_dir_all(&dir).map_err(|e| format!("create pointer directory: {e}"))?;
    let pointer = dir.join("location.json");
    let body = serde_json::json!({ "data_dir": data_dir }).to_string();
    atomic_replace(&pointer, body.as_bytes())
}

fn restore_location_pointer(app: &tauri::AppHandle, previous: Option<&[u8]>) -> Result<(), String> {
    let pointer = pointer_dir(app).join("location.json");
    match previous {
        Some(bytes) => atomic_replace(&pointer, bytes),
        None => match std::fs::remove_file(pointer) {
            Ok(()) => Ok(()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(error) => Err(error.to_string()),
        },
    }
}

fn cleanup_migrated_files(dir: &std::path::Path) {
    for name in DATA_FILES {
        let _ = std::fs::remove_file(dir.join(name));
    }
}

fn canonicalize_allow_missing(path: &std::path::Path) -> Result<std::path::PathBuf, String> {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir()
            .map_err(|e| format!("resolve current directory: {e}"))?
            .join(path)
    };
    let mut cursor = absolute.as_path();
    let mut missing = Vec::new();
    loop {
        match std::fs::canonicalize(cursor) {
            Ok(mut canonical) => {
                for component in missing.iter().rev() {
                    canonical.push(component);
                }
                return Ok(canonical);
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                let name = cursor
                    .file_name()
                    .ok_or_else(|| format!("unable to resolve path {}", path.display()))?;
                missing.push(name.to_os_string());
                cursor = cursor
                    .parent()
                    .ok_or_else(|| format!("unable to resolve path {}", path.display()))?;
            }
            Err(error) => return Err(format!("canonicalize {}: {error}", path.display())),
        }
    }
}

fn canonical_data_dirs(
    old_dir: &std::path::Path,
    new_dir: &std::path::Path,
) -> Result<(std::path::PathBuf, std::path::PathBuf), String> {
    let old = std::fs::canonicalize(old_dir)
        .map_err(|e| format!("canonicalize current data directory: {e}"))?;
    let new = canonicalize_allow_missing(new_dir)?;
    if new != old && (new.starts_with(&old) || old.starts_with(&new)) {
        return Err(
            "the new data directory cannot contain or be contained by the current directory"
                .to_string(),
        );
    }
    Ok((old, new))
}

fn commit_migration_files(
    temporary: &std::path::Path,
    destination: &std::path::Path,
) -> Result<(), String> {
    for name in DATA_FILES {
        let source = temporary.join(name);
        if source.exists() {
            if let Err(error) = std::fs::rename(&source, destination.join(name)) {
                cleanup_migrated_files(destination);
                return Err(format!("commit migrated {name}: {error}"));
            }
        }
    }
    Ok(())
}

#[tauri::command]
async fn set_data_dir(app: tauri::AppHandle, path: String) -> Result<DataDirInfo, String> {
    let requested_dir = std::path::PathBuf::from(path.trim());
    if requested_dir.as_os_str().is_empty() {
        return Err("路径不能为空".to_string());
    }
    let (old_dir, mut new_dir) = canonical_data_dirs(&resolve_data_dir(&app), &requested_dir)?;
    if new_dir == old_dir {
        return Ok(get_data_dir(app));
    }
    if DATA_FILES.iter().any(|name| new_dir.join(name).exists()) {
        return Err("the target already contains Amber data".to_string());
    }
    std::fs::create_dir_all(&new_dir).map_err(|e| format!("unable to create directory: {e}"))?;
    new_dir = std::fs::canonicalize(&new_dir)
        .map_err(|e| format!("canonicalize target data directory: {e}"))?;
    if new_dir.starts_with(&old_dir) || old_dir.starts_with(&new_dir) {
        return Err(
            "the new data directory cannot contain or be contained by the current directory"
                .to_string(),
        );
    }
    let probe = new_dir.join(".amber-write-test");
    std::fs::write(&probe, b"ok").map_err(|e| format!("target directory is not writable: {e}"))?;
    std::fs::remove_file(&probe).map_err(|e| format!("target directory cleanup failed: {e}"))?;

    let previous_pointer = std::fs::read(pointer_dir(&app).join("location.json")).ok();
    let previous_generation = supervisor::backend_state(&app).generation;
    supervisor::set_migrating(&app);
    supervisor::stop(&app, supervisor::StopIntent::Migration);
    if !supervisor::wait_stopped(&app, Duration::from_secs(5)).await {
        supervisor::restart_after_migration_abort(&app);
        return Err("timed out while stopping the backend".to_string());
    }

    let migration_id = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis();
    let temporary = new_dir.join(format!(".amber-migration-{migration_id}"));
    if let Err(error) =
        copy_data_files(&old_dir, &temporary).and_then(|_| validate_migration_copy(&temporary))
    {
        let _ = std::fs::remove_dir_all(&temporary);
        supervisor::spawn(&app, supervisor::BackendPhase::Starting);
        return Err(error);
    }
    if let Err(error) = commit_migration_files(&temporary, &new_dir) {
        let _ = std::fs::remove_dir_all(&temporary);
        supervisor::spawn(&app, supervisor::BackendPhase::Starting);
        return Err(error);
    }
    let _ = std::fs::remove_dir_all(&temporary);

    let default = default_data_dir(&app);
    let stored = if new_dir == default {
        String::new()
    } else {
        new_dir.to_string_lossy().to_string()
    };
    if let Err(error) = write_location_pointer(&app, &stored) {
        cleanup_migrated_files(&new_dir);
        supervisor::spawn(&app, supervisor::BackendPhase::Starting);
        return Err(error);
    }

    supervisor::spawn(&app, supervisor::BackendPhase::Starting);
    if !supervisor::wait_ready(&app, previous_generation, Duration::from_secs(15)).await {
        supervisor::stop(&app, supervisor::StopIntent::Migration);
        let stopped = supervisor::wait_stopped(&app, Duration::from_secs(5)).await;
        let restore_error = restore_location_pointer(&app, previous_pointer.as_deref()).err();
        if stopped {
            cleanup_migrated_files(&new_dir);
            supervisor::spawn(&app, supervisor::BackendPhase::Starting);
        } else {
            supervisor::restart_after_migration_abort(&app);
        }
        return Err(match restore_error {
            Some(error) => {
                format!("new backend did not become ready and pointer rollback failed: {error}")
            }
            None if stopped => {
                "new backend did not become ready; the previous data directory was restored"
                    .to_string()
            }
            None => "new backend did not become ready or stop in time; the previous data directory was restored and migrated files were retained for safety".to_string(),
        });
    }

    let backup_name = format!(
        "{}.pre-v0.2.0-{}.bak",
        old_dir
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("amber-data"),
        migration_id
    );
    if let Some(parent) = old_dir.parent() {
        let _ = std::fs::rename(&old_dir, parent.join(backup_name));
    }
    Ok(get_data_dir(app))
}

#[tauri::command]
async fn open_external(app: tauri::AppHandle, url: String) -> Result<(), String> {
    use tauri_plugin_opener::OpenerExt;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|e| e.to_string())
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
        let mut args = std::env::var("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS").unwrap_or_default();
        if !args.contains("--disable-gpu") {
            if !args.is_empty() {
                args.push(' ');
            }
            args.push_str("--disable-gpu");
        }
        std::env::set_var("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", args);
    }

    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // A second launch focuses the existing window instead of
            // starting another instance (and another sidecar).
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.unminimize();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![
            get_connection,
            get_backend_state,
            restart_backend,
            open_external,
            get_data_dir,
            set_data_dir,
            open_data_dir
        ])
        .setup(|app| {
            let handle = app.handle();
            build_tray(handle)?;
            supervisor::spawn(handle, supervisor::BackendPhase::Starting);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn migration_copy_requires_sqlite_and_key() {
        let root =
            std::env::temp_dir().join(format!("amber-migration-test-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        std::fs::write(root.join("sub2api.db"), b"SQLite format 3\0payload").unwrap();
        assert!(validate_migration_copy(&root).is_err());
        std::fs::write(root.join("key"), "A".repeat(44)).unwrap();
        assert!(validate_migration_copy(&root).is_ok());
        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn atomic_pointer_write_replaces_existing_file() {
        let root = std::env::temp_dir().join(format!("amber-pointer-test-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        let pointer = root.join("location.json");
        std::fs::write(&pointer, b"old").unwrap();
        atomic_replace(&pointer, b"new").unwrap();
        assert_eq!(std::fs::read(&pointer).unwrap(), b"new");
        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn canonical_paths_reject_nested_and_parent_directories() {
        let root = std::env::temp_dir().join(format!(
            "amber-canonical-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let current = root.join("current").join("data");
        std::fs::create_dir_all(&current).unwrap();
        let nested = current.join("child").join("..").join("target");
        assert!(canonical_data_dirs(&current, &nested).is_err());
        assert!(canonical_data_dirs(&current, &root).is_err());
        let sibling = root.join("sibling");
        let (_, canonical_sibling) = canonical_data_dirs(&current, &sibling).unwrap();
        assert!(canonical_sibling.is_absolute());
        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn migration_commit_failure_removes_partially_moved_files() {
        let root = std::env::temp_dir().join(format!(
            "amber-commit-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let temporary = root.join("temporary");
        let destination = root.join("destination");
        std::fs::create_dir_all(&temporary).unwrap();
        std::fs::create_dir_all(destination.join("key")).unwrap();
        std::fs::write(temporary.join("sub2api.db"), b"database").unwrap();
        std::fs::write(temporary.join("key"), b"key").unwrap();
        assert!(commit_migration_files(&temporary, &destination).is_err());
        assert!(!destination.join("sub2api.db").exists());
        let _ = std::fs::remove_dir_all(root);
    }

    #[cfg(windows)]
    #[test]
    fn canonical_paths_ignore_windows_path_casing() {
        let root = std::env::temp_dir().join(format!(
            "amber-case-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let current = root.join("MixedCaseData");
        std::fs::create_dir_all(&current).unwrap();
        let alias = std::path::PathBuf::from(current.to_string_lossy().to_uppercase());
        let (canonical_current, canonical_alias) = canonical_data_dirs(&current, &alias).unwrap();
        assert_eq!(canonical_current, canonical_alias);
        let _ = std::fs::remove_dir_all(root);
    }

    #[cfg(unix)]
    #[test]
    fn canonical_paths_resolve_symlink_aliases() {
        use std::os::unix::fs::symlink;
        let root = std::env::temp_dir().join(format!(
            "amber-symlink-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let current = root.join("current");
        let alias = root.join("alias");
        std::fs::create_dir_all(&current).unwrap();
        symlink(&current, &alias).unwrap();
        let (canonical_current, canonical_alias) = canonical_data_dirs(&current, &alias).unwrap();
        assert_eq!(canonical_current, canonical_alias);
        let _ = std::fs::remove_dir_all(root);
    }
}
