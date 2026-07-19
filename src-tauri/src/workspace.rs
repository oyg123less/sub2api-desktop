use std::{
    path::{Component, Path, PathBuf},
    sync::atomic::{AtomicU64, Ordering},
    time::{SystemTime, UNIX_EPOCH},
};

use serde::{Deserialize, Serialize};
use tauri::AppHandle;

use crate::{atomic_replace, resolve_data_root};

const INDEX_VERSION: u32 = 1;
static WORKSPACE_SEQUENCE: AtomicU64 = AtomicU64::new(0);

#[derive(Clone, Debug, Deserialize, Serialize)]
struct WorkspaceEntry {
    id: String,
    name: String,
    relative_path: String,
    kind: String,
    created_at: u64,
    last_opened_at: u64,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
struct WorkspaceIndex {
    version: u32,
    active_workspace_id: String,
    workspaces: Vec<WorkspaceEntry>,
}

#[derive(Clone, Debug, Serialize)]
pub struct WorkspaceSummary {
    pub id: String,
    pub name: String,
    pub kind: String,
    pub active: bool,
    pub data_dir: String,
    pub created_at: u64,
    pub last_opened_at: u64,
}

#[derive(Clone, Debug)]
pub struct ActiveWorkspace {
    pub id: String,
    pub data_dir: PathBuf,
}

fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn new_workspace_id() -> String {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let sequence = WORKSPACE_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    format!("ws_{nanos:x}{sequence:x}")
}

fn index_path(root: &Path) -> PathBuf {
    root.join("workspace-index.json")
}

fn validate_relative_path(value: &str) -> Result<PathBuf, String> {
    if value == "." {
        return Ok(PathBuf::from(value));
    }
    let path = PathBuf::from(value);
    if path.is_absolute()
        || path.components().any(|component| {
            matches!(
                component,
                Component::ParentDir | Component::RootDir | Component::Prefix(_)
            )
        })
    {
        return Err("workspace path escapes the Amber data directory".to_string());
    }
    Ok(path)
}

fn validate_index(index: &WorkspaceIndex) -> Result<(), String> {
    if index.version != INDEX_VERSION || index.workspaces.is_empty() {
        return Err("unsupported or empty workspace index".to_string());
    }
    let mut active = 0;
    let mut ids = std::collections::HashSet::new();
    for entry in &index.workspaces {
        if entry.id.is_empty()
            || !entry
                .id
                .chars()
                .all(|ch| ch.is_ascii_alphanumeric() || ch == '_' || ch == '-')
            || !ids.insert(entry.id.clone())
        {
            return Err("workspace index contains an invalid id".to_string());
        }
        validate_relative_path(&entry.relative_path)?;
        if entry.id == index.active_workspace_id {
            active += 1;
        }
    }
    if active != 1 {
        return Err("workspace index has no unambiguous active workspace".to_string());
    }
    Ok(())
}

fn read_index(root: &Path) -> Result<WorkspaceIndex, String> {
    let bytes =
        std::fs::read(index_path(root)).map_err(|e| format!("read workspace index: {e}"))?;
    let index = serde_json::from_slice::<WorkspaceIndex>(&bytes)
        .map_err(|e| format!("parse workspace index: {e}"))?;
    validate_index(&index)?;
    Ok(index)
}

fn validate_workspace_payload(path: &Path) -> Result<(), String> {
    let database = path.join("sub2api.db");
    let key = path.join("key");
    if !database.exists() && !key.exists() {
        return Ok(());
    }
    if !database.is_file() || !key.is_file() {
        return Err(format!(
            "workspace {} is missing sub2api.db or key",
            path.display()
        ));
    }
    let bytes = std::fs::read(&database)
        .map_err(|error| format!("read copied database {}: {error}", database.display()))?;
    if !bytes.starts_with(b"SQLite format 3\0") {
        return Err(format!(
            "workspace {} has an invalid SQLite header",
            path.display()
        ));
    }
    let key_text = std::fs::read_to_string(&key)
        .map_err(|error| format!("read copied key {}: {error}", key.display()))?;
    if key_text.trim().len() < 40 {
        return Err(format!(
            "workspace {} has an invalid encryption key",
            path.display()
        ));
    }
    Ok(())
}

pub fn validate_data_root(root: &Path) -> Result<(), String> {
    if !index_path(root).is_file() {
        return validate_workspace_payload(root);
    }
    let index = read_index(root)?;
    for entry in index.workspaces {
        let relative = validate_relative_path(&entry.relative_path)?;
        let workspace_dir = if relative == PathBuf::from(".") {
            root.to_path_buf()
        } else {
            root.join(relative)
        };
        if !workspace_dir.is_dir() {
            return Err(format!(
                "workspace directory {} is missing",
                workspace_dir.display()
            ));
        }
        validate_workspace_payload(&workspace_dir)?;
    }
    Ok(())
}

pub fn copy_data_root(source: &Path, destination: &Path) -> Result<(), String> {
    std::fs::create_dir_all(destination)
        .map_err(|error| format!("create migration directory: {error}"))?;
    for entry in std::fs::read_dir(source)
        .map_err(|error| format!("read data directory {}: {error}", source.display()))?
    {
        let entry = entry.map_err(|error| format!("read data directory entry: {error}"))?;
        let file_type = entry
            .file_type()
            .map_err(|error| format!("inspect {}: {error}", entry.path().display()))?;
        let target = destination.join(entry.file_name());
        if file_type.is_symlink() {
            return Err(format!(
                "data directory contains unsupported symlink {}",
                entry.path().display()
            ));
        }
        if file_type.is_dir() {
            copy_data_root(&entry.path(), &target)?;
        } else if file_type.is_file() {
            std::fs::copy(entry.path(), &target)
                .map_err(|error| format!("copy {}: {error}", entry.path().display()))?;
        }
    }
    Ok(())
}

fn write_index(root: &Path, index: &WorkspaceIndex) -> Result<(), String> {
    validate_index(index)?;
    std::fs::create_dir_all(root).map_err(|e| format!("create data directory: {e}"))?;
    let bytes =
        serde_json::to_vec_pretty(index).map_err(|e| format!("encode workspace index: {e}"))?;
    atomic_replace(&index_path(root), &bytes).map_err(|e| format!("write workspace index: {e}"))
}

pub fn ensure(app: &AppHandle) -> Result<(), String> {
    let root = resolve_data_root(app);
    std::fs::create_dir_all(&root).map_err(|e| format!("create data directory: {e}"))?;
    if index_path(&root).is_file() {
        return read_index(&root).map(|_| ());
    }
    let timestamp = now();
    let legacy = root.join("sub2api.db").exists() || root.join("key").exists();
    let entry = if legacy {
        WorkspaceEntry {
            id: "ws_legacy".to_string(),
            name: "本地工作区".to_string(),
            relative_path: ".".to_string(),
            kind: "legacy".to_string(),
            created_at: timestamp,
            last_opened_at: timestamp,
        }
    } else {
        let id = new_workspace_id();
        let relative_path = format!("workspaces/{id}");
        std::fs::create_dir_all(root.join(&relative_path))
            .map_err(|e| format!("create initial workspace: {e}"))?;
        WorkspaceEntry {
            id,
            name: "本地工作区".to_string(),
            relative_path,
            kind: "local".to_string(),
            created_at: timestamp,
            last_opened_at: timestamp,
        }
    };
    write_index(
        &root,
        &WorkspaceIndex {
            version: INDEX_VERSION,
            active_workspace_id: entry.id.clone(),
            workspaces: vec![entry],
        },
    )
}

pub fn active(app: &AppHandle) -> Result<ActiveWorkspace, String> {
    ensure(app)?;
    let root = resolve_data_root(app);
    let index = read_index(&root)?;
    let entry = index
        .workspaces
        .iter()
        .find(|entry| entry.id == index.active_workspace_id)
        .ok_or_else(|| "active workspace is missing".to_string())?;
    let relative = validate_relative_path(&entry.relative_path)?;
    let data_dir = if relative == PathBuf::from(".") {
        root
    } else {
        root.join(relative)
    };
    Ok(ActiveWorkspace {
        id: entry.id.clone(),
        data_dir,
    })
}

pub fn list(app: &AppHandle) -> Result<Vec<WorkspaceSummary>, String> {
    ensure(app)?;
    let root = resolve_data_root(app);
    let index = read_index(&root)?;
    index
        .workspaces
        .into_iter()
        .map(|entry| {
            let relative = validate_relative_path(&entry.relative_path)?;
            let data_dir = if relative == PathBuf::from(".") {
                root.clone()
            } else {
                root.join(relative)
            };
            Ok(WorkspaceSummary {
                active: entry.id == index.active_workspace_id,
                id: entry.id,
                name: entry.name,
                kind: entry.kind,
                data_dir: data_dir.to_string_lossy().to_string(),
                created_at: entry.created_at,
                last_opened_at: entry.last_opened_at,
            })
        })
        .collect()
}

pub fn create(app: &AppHandle, name: &str) -> Result<WorkspaceSummary, String> {
    ensure(app)?;
    let root = resolve_data_root(app);
    let mut index = read_index(&root)?;
    let id = new_workspace_id();
    let relative_path = format!("workspaces/{id}");
    let data_dir = root.join(&relative_path);
    std::fs::create_dir_all(&data_dir).map_err(|e| format!("create workspace: {e}"))?;
    let timestamp = now();
    let entry = WorkspaceEntry {
        id: id.clone(),
        name: name.trim().chars().take(64).collect::<String>(),
        relative_path,
        kind: "local".to_string(),
        created_at: timestamp,
        last_opened_at: 0,
    };
    let entry_name = if entry.name.is_empty() {
        "新工作区".to_string()
    } else {
        entry.name.clone()
    };
    let mut entry = entry;
    entry.name = entry_name;
    index.workspaces.push(entry.clone());
    if let Err(error) = write_index(&root, &index) {
        let _ = std::fs::remove_dir_all(&data_dir);
        return Err(error);
    }
    Ok(WorkspaceSummary {
        id,
        name: entry.name,
        kind: entry.kind,
        active: false,
        data_dir: data_dir.to_string_lossy().to_string(),
        created_at: timestamp,
        last_opened_at: 0,
    })
}

pub fn activate(app: &AppHandle, workspace_id: &str) -> Result<String, String> {
    ensure(app)?;
    let root = resolve_data_root(app);
    let mut index = read_index(&root)?;
    let previous = index.active_workspace_id.clone();
    let entry = index
        .workspaces
        .iter_mut()
        .find(|entry| entry.id == workspace_id)
        .ok_or_else(|| "workspace was not found".to_string())?;
    let relative = validate_relative_path(&entry.relative_path)?;
    let data_dir = if relative == PathBuf::from(".") {
        root.clone()
    } else {
        root.join(relative)
    };
    std::fs::create_dir_all(&data_dir).map_err(|e| format!("open workspace: {e}"))?;
    entry.last_opened_at = now();
    index.active_workspace_id = workspace_id.to_string();
    write_index(&root, &index)?;
    Ok(previous)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn workspace_paths_cannot_escape_root() {
        assert!(validate_relative_path("workspaces/ws_test").is_ok());
        assert!(validate_relative_path(".").is_ok());
        assert!(validate_relative_path("../outside").is_err());
        assert!(validate_relative_path("C:\\outside").is_err());
    }

    #[test]
    fn workspace_index_requires_one_active_entry() {
        let entry = WorkspaceEntry {
            id: "ws_a".to_string(),
            name: "A".to_string(),
            relative_path: "workspaces/ws_a".to_string(),
            kind: "local".to_string(),
            created_at: 1,
            last_opened_at: 1,
        };
        assert!(validate_index(&WorkspaceIndex {
            version: INDEX_VERSION,
            active_workspace_id: "ws_a".to_string(),
            workspaces: vec![entry.clone()],
        })
        .is_ok());
        assert!(validate_index(&WorkspaceIndex {
            version: INDEX_VERSION,
            active_workspace_id: "ws_missing".to_string(),
            workspaces: vec![entry],
        })
        .is_err());
    }

    #[test]
    fn whole_data_root_copy_preserves_every_workspace() {
        let unique = new_workspace_id();
        let root = std::env::temp_dir().join(format!("amber-workspace-source-{unique}"));
        let copied = std::env::temp_dir().join(format!("amber-workspace-copy-{unique}"));
        let second = root.join("workspaces/ws_b");
        std::fs::create_dir_all(&second).unwrap();
        std::fs::write(root.join("sub2api.db"), b"SQLite format 3\0legacy").unwrap();
        std::fs::write(root.join("key"), "a".repeat(64)).unwrap();
        std::fs::write(second.join("sub2api.db"), b"SQLite format 3\0second").unwrap();
        std::fs::write(second.join("key"), "b".repeat(64)).unwrap();
        std::fs::write(second.join("codex_known_hosts"), "host-key").unwrap();
        write_index(
            &root,
            &WorkspaceIndex {
                version: INDEX_VERSION,
                active_workspace_id: "ws_legacy".to_string(),
                workspaces: vec![
                    WorkspaceEntry {
                        id: "ws_legacy".to_string(),
                        name: "Legacy".to_string(),
                        relative_path: ".".to_string(),
                        kind: "legacy".to_string(),
                        created_at: 1,
                        last_opened_at: 1,
                    },
                    WorkspaceEntry {
                        id: "ws_b".to_string(),
                        name: "B".to_string(),
                        relative_path: "workspaces/ws_b".to_string(),
                        kind: "local".to_string(),
                        created_at: 2,
                        last_opened_at: 0,
                    },
                ],
            },
        )
        .unwrap();

        copy_data_root(&root, &copied).unwrap();
        validate_data_root(&copied).unwrap();
        assert_eq!(
            std::fs::read_to_string(copied.join("workspaces/ws_b/codex_known_hosts")).unwrap(),
            "host-key"
        );
        let _ = std::fs::remove_dir_all(root);
        let _ = std::fs::remove_dir_all(copied);
    }
}
