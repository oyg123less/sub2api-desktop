use std::{
    collections::VecDeque,
    sync::Mutex,
    time::{Duration, Instant, SystemTime, UNIX_EPOCH},
};

use serde::Serialize;
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_shell::{
    process::{CommandChild, CommandEvent},
    ShellExt,
};

use crate::{resolve_data_dir, AppState};

const RESTART_DELAYS: [u64; 5] = [1, 2, 5, 15, 30];
const RESTART_WINDOW: Duration = Duration::from_secs(10 * 60);
const STABLE_WINDOW: Duration = Duration::from_secs(10 * 60);

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum BackendPhase {
    #[default]
    Stopped,
    Starting,
    Ready,
    Restarting,
    Migrating,
    Failed,
}

#[derive(Clone, Debug, Default, Serialize)]
pub struct BackendState {
    pub phase: BackendPhase,
    pub generation: u64,
    pub control_port: u16,
    pub restart_count: usize,
    pub started_at: Option<String>,
    pub last_ready_at: Option<String>,
    pub last_exit_code: Option<i32>,
    pub last_error: Option<String>,
}

#[derive(Clone, Debug, Default, Serialize)]
pub struct Connection {
    pub control_port: u16,
    pub control_token: String,
    pub generation: u64,
    pub sidecar_version: String,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum StopIntent {
    #[default]
    None,
    Restart,
    Migration,
    Exit,
}

#[derive(Default)]
pub struct Supervisor {
    state: Mutex<BackendState>,
    connection: Mutex<Connection>,
    child: Mutex<Option<CommandChild>>,
    active_run: Mutex<u64>,
    next_run: Mutex<u64>,
    stop_intent: Mutex<StopIntent>,
    restart_history: Mutex<VecDeque<Instant>>,
}

#[derive(Clone, Serialize)]
struct OutputEvent {
    stream: &'static str,
    line: String,
    generation: u64,
}

#[derive(Default)]
struct LineBuffer {
    bytes: Vec<u8>,
}

impl LineBuffer {
    fn push(&mut self, chunk: &[u8]) -> Vec<String> {
        self.bytes.extend_from_slice(chunk);
        if self.bytes.len() > 1024 * 1024 {
            self.bytes.clear();
            return vec!["sidecar output exceeded buffer limit".to_string()];
        }
        let mut lines = Vec::new();
        while let Some(index) = self.bytes.iter().position(|byte| *byte == b'\n') {
            let mut line = self.bytes.drain(..=index).collect::<Vec<_>>();
            while matches!(line.last(), Some(b'\n' | b'\r')) {
                line.pop();
            }
            lines.push(String::from_utf8_lossy(&line).to_string());
        }
        lines
    }
}

#[derive(serde::Deserialize)]
struct Handshake {
    control_port: u64,
    control_token: String,
    #[serde(default)]
    version: String,
}

fn parse_handshake(line: &str) -> Option<Handshake> {
    let payload = line.trim().strip_prefix("SUB2API_READY ")?;
    let handshake = serde_json::from_str::<Handshake>(payload).ok()?;
    if !(1..=u16::MAX as u64).contains(&handshake.control_port)
        || handshake.control_token.len() < 32
    {
        return None;
    }
    Some(handshake)
}

fn timestamp() -> String {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
        .to_string()
}

fn emit_state(app: &AppHandle, state: BackendState) {
    let _ = app.emit("backend-state-changed", state);
}

fn mutate_state(app: &AppHandle, update: impl FnOnce(&mut BackendState)) {
    if let Some(app_state) = app.try_state::<AppState>() {
        let state = {
            let mut state = app_state.supervisor.state.lock().unwrap();
            update(&mut state);
            state.clone()
        };
        emit_state(app, state);
    }
}

fn clear_connection(app: &AppHandle) {
    if let Some(state) = app.try_state::<AppState>() {
        *state.supervisor.connection.lock().unwrap() = Connection::default();
        mutate_state(app, |backend| backend.control_port = 0);
    }
}

pub fn backend_state(app: &AppHandle) -> BackendState {
    app.state::<AppState>()
        .supervisor
        .state
        .lock()
        .unwrap()
        .clone()
}

pub fn connection(app: &AppHandle) -> Connection {
    app.state::<AppState>()
        .supervisor
        .connection
        .lock()
        .unwrap()
        .clone()
}

pub fn is_running(app: &AppHandle) -> bool {
    *app.state::<AppState>()
        .supervisor
        .active_run
        .lock()
        .unwrap()
        != 0
}

pub fn set_migrating(app: &AppHandle) {
    clear_connection(app);
    mutate_state(app, |state| {
        state.phase = BackendPhase::Migrating;
        state.last_error = None;
    });
}

// Ensures a backend stopped for migration is brought back after migration
// aborts. If termination is still pending, handle_terminated performs the
// restart; otherwise a new process is started immediately.
pub fn restart_after_migration_abort(app: &AppHandle) {
    let state = app.state::<AppState>();
    *state.supervisor.stop_intent.lock().unwrap() = StopIntent::Restart;
    if is_running(app) {
        mutate_state(app, |backend| backend.phase = BackendPhase::Restarting);
    } else {
        spawn(app, BackendPhase::Starting);
    }
}

pub fn spawn(app: &AppHandle, phase: BackendPhase) {
    let state = app.state::<AppState>();
    {
        let active = state.supervisor.active_run.lock().unwrap();
        if *active != 0 {
            return;
        }
    }

    let run_id = {
        let mut next = state.supervisor.next_run.lock().unwrap();
        *next += 1;
        *next
    };
    *state.supervisor.stop_intent.lock().unwrap() = StopIntent::None;
    *state.supervisor.active_run.lock().unwrap() = run_id;
    clear_connection(app);
    mutate_state(app, |backend| {
        backend.phase = phase;
        backend.started_at = Some(timestamp());
        backend.last_error = None;
    });

    let data_dir = resolve_data_dir(app);
    let _ = std::fs::create_dir_all(&data_dir);
    let sidecar = match app.shell().sidecar("sub2api-sidecar") {
        Ok(command) => command.args([
            "--data-dir".to_string(),
            data_dir.to_string_lossy().to_string(),
            "--control-port".to_string(),
            "0".to_string(),
        ]),
        Err(error) => {
            *state.supervisor.active_run.lock().unwrap() = 0;
            fail(app, format!("failed to resolve sidecar: {error}"));
            return;
        }
    };

    let (mut receiver, child) = match sidecar.spawn() {
        Ok(value) => value,
        Err(error) => {
            *state.supervisor.active_run.lock().unwrap() = 0;
            fail(app, format!("failed to spawn sidecar: {error}"));
            return;
        }
    };
    *state.supervisor.child.lock().unwrap() = Some(child);

    let handle = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut stdout = LineBuffer::default();
        let mut stderr = LineBuffer::default();
        while let Some(event) = receiver.recv().await {
            match event {
                CommandEvent::Stdout(bytes) => {
                    for line in stdout.push(&bytes) {
                        handle_stdout(&handle, run_id, &line);
                    }
                }
                CommandEvent::Stderr(bytes) => {
                    for line in stderr.push(&bytes) {
                        emit_output(&handle, "stderr", &line);
                    }
                }
                CommandEvent::Terminated(payload) => {
                    handle_terminated(&handle, run_id, payload.code);
                    break;
                }
                CommandEvent::Error(error) => emit_output(&handle, "stderr", &error),
                _ => {}
            }
        }
    });
}

fn handle_stdout(app: &AppHandle, run_id: u64, line: &str) {
    if let Some(handshake) = parse_handshake(line) {
        let state = app.state::<AppState>();
        if *state.supervisor.active_run.lock().unwrap() != run_id {
            return;
        }
        if !version_compatible(&handshake.version, env!("CARGO_PKG_VERSION")) {
            fail(
                app,
                format!(
                    "sidecar version {} does not match desktop version {}",
                    handshake.version,
                    env!("CARGO_PKG_VERSION")
                ),
            );
            if let Some(child) = state.supervisor.child.lock().unwrap().take() {
                let _ = child.kill();
            }
            return;
        }
        let connection = {
            let mut backend = state.supervisor.state.lock().unwrap();
            backend.generation += 1;
            backend.phase = BackendPhase::Ready;
            backend.control_port = handshake.control_port as u16;
            backend.last_ready_at = Some(timestamp());
            backend.last_error = None;
            let connection = Connection {
                control_port: handshake.control_port as u16,
                control_token: handshake.control_token,
                generation: backend.generation,
                sidecar_version: handshake.version,
            };
            *state.supervisor.connection.lock().unwrap() = connection.clone();
            emit_state(app, backend.clone());
            connection
        };
        let _ = app.emit("sidecar-ready", connection);
        schedule_stable_reset(app.clone(), run_id);
    } else {
        emit_output(app, "stdout", line);
    }
}

fn version_compatible(sidecar: &str, desktop: &str) -> bool {
    let sidecar = sidecar.trim();
    sidecar == desktop || sidecar.ends_with("-dev")
}

fn schedule_stable_reset(app: AppHandle, run_id: u64) {
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(STABLE_WINDOW).await;
        let state = app.state::<AppState>();
        if *state.supervisor.active_run.lock().unwrap() == run_id
            && state.supervisor.state.lock().unwrap().phase == BackendPhase::Ready
        {
            state.supervisor.restart_history.lock().unwrap().clear();
            mutate_state(&app, |backend| backend.restart_count = 0);
        }
    });
}

fn handle_terminated(app: &AppHandle, run_id: u64, exit_code: Option<i32>) {
    let state = app.state::<AppState>();
    {
        let mut active = state.supervisor.active_run.lock().unwrap();
        if *active != run_id {
            return;
        }
        *active = 0;
    }
    state.supervisor.child.lock().unwrap().take();
    clear_connection(app);
    let intent = std::mem::take(&mut *state.supervisor.stop_intent.lock().unwrap());
    mutate_state(app, |backend| backend.last_exit_code = exit_code);

    match intent {
        StopIntent::Restart => spawn(app, BackendPhase::Starting),
        StopIntent::Migration => {
            mutate_state(app, |backend| backend.phase = BackendPhase::Migrating)
        }
        StopIntent::Exit => mutate_state(app, |backend| backend.phase = BackendPhase::Stopped),
        StopIntent::None => schedule_restart(app),
    }
}

fn schedule_restart(app: &AppHandle) {
    let state = app.state::<AppState>();
    let (count, delay) = {
        let now = Instant::now();
        let mut history = state.supervisor.restart_history.lock().unwrap();
        while history
            .front()
            .is_some_and(|instant| now.duration_since(*instant) > RESTART_WINDOW)
        {
            history.pop_front();
        }
        if history.len() >= RESTART_DELAYS.len() {
            fail(app, "automatic restart limit reached".to_string());
            return;
        }
        history.push_back(now);
        let count = history.len();
        (count, RESTART_DELAYS[count - 1])
    };
    mutate_state(app, |backend| {
        backend.phase = BackendPhase::Restarting;
        backend.restart_count = count;
        backend.last_error = Some(format!("sidecar exited unexpectedly; retrying in {delay}s"));
    });
    let handle = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(Duration::from_secs(delay)).await;
        if backend_state(&handle).phase == BackendPhase::Restarting && !is_running(&handle) {
            spawn(&handle, BackendPhase::Starting);
        }
    });
}

fn fail(app: &AppHandle, message: String) {
    clear_connection(app);
    mutate_state(app, |backend| {
        backend.phase = BackendPhase::Failed;
        backend.last_error = Some(message);
    });
}

pub fn stop(app: &AppHandle, intent: StopIntent) {
    let state = app.state::<AppState>();
    *state.supervisor.stop_intent.lock().unwrap() = intent;
    clear_connection(app);
    match intent {
        StopIntent::Migration => set_migrating(app),
        StopIntent::Restart => {
            mutate_state(app, |backend| backend.phase = BackendPhase::Restarting)
        }
        StopIntent::Exit => mutate_state(app, |backend| backend.phase = BackendPhase::Stopped),
        StopIntent::None => {}
    }
    let child = { state.supervisor.child.lock().unwrap().take() };
    if let Some(child) = child {
        if let Err(error) = child.kill() {
            fail(app, format!("failed to stop sidecar: {error}"));
        }
    } else {
        *state.supervisor.active_run.lock().unwrap() = 0;
    }
}

pub fn manual_restart(app: &AppHandle) {
    let state = backend_state(app);
    if is_running(app) {
        stop(app, StopIntent::Restart);
    } else {
        app.state::<AppState>()
            .supervisor
            .restart_history
            .lock()
            .unwrap()
            .clear();
        mutate_state(app, |backend| backend.restart_count = 0);
        spawn(app, BackendPhase::Starting);
    }
    if state.phase == BackendPhase::Failed {
        mutate_state(app, |backend| backend.last_error = None);
    }
}

pub async fn wait_stopped(app: &AppHandle, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if !is_running(app) {
            return true;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
    false
}

pub async fn wait_ready(app: &AppHandle, generation: u64, timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        let state = backend_state(app);
        if state.phase == BackendPhase::Ready && state.generation > generation {
            return true;
        }
        if state.phase == BackendPhase::Failed {
            return false;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    false
}

fn emit_output(app: &AppHandle, stream: &'static str, line: &str) {
    let line = redact_output(line);
    let _ = app.emit(
        "sidecar-output",
        OutputEvent {
            stream,
            line,
            generation: backend_state(app).generation,
        },
    );
}

fn redact_output(line: &str) -> String {
    line.split_whitespace()
        .map(|word| {
            let trimmed = word.trim_matches(|ch: char| {
                !ch.is_ascii_alphanumeric() && ch != '-' && ch != '_' && ch != '.'
            });
            let candidate = trimmed.rsplit(['=', ':']).next().unwrap_or(trimmed);
            if candidate.starts_with("sk-")
                || (candidate.len() > 40 && candidate.matches('.').count() == 2)
            {
                word.replace(candidate, "<redacted-token>")
            } else {
                word.to_string()
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn handshake_survives_fragmented_stdout() {
        let mut buffer = LineBuffer::default();
        assert!(buffer.push(b"SUB2API_RE").is_empty());
        let lines = buffer.push(b"ADY {\"control_port\":4321,\"control_token\":\"12345678901234567890123456789012\",\"version\":\"0.2.0\"}\n");
        assert_eq!(lines.len(), 1);
        let handshake = parse_handshake(&lines[0]).expect("valid handshake");
        assert_eq!(handshake.control_port, 4321);
        assert_eq!(handshake.version, "0.2.0");
    }

    #[test]
    fn handshake_rejects_invalid_port_and_short_token() {
        assert!(parse_handshake("SUB2API_READY {\"control_port\":0,\"control_token\":\"12345678901234567890123456789012\"}").is_none());
        assert!(
            parse_handshake("SUB2API_READY {\"control_port\":42,\"control_token\":\"short\"}")
                .is_none()
        );
    }

    #[test]
    fn output_redacts_tokens() {
        let jwt = format!("{}.{}.{}", "a".repeat(20), "b".repeat(20), "c".repeat(20));
        let redacted = redact_output(&format!("token={jwt} key=sk-local-secret-value"));
        assert!(!redacted.contains(&jwt));
        assert!(!redacted.contains("sk-local-secret-value"));
    }

    #[test]
    fn release_versions_must_match() {
        assert!(version_compatible("0.2.0", "0.2.0"));
        assert!(version_compatible("0.2.0-dev", "0.2.0"));
        assert!(!version_compatible("0.1.1", "0.2.0"));
    }
}
