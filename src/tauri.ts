// Thin Tauri bridge. All helpers degrade gracefully in a plain browser (dev).
import { invoke as tauriInvoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

export function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

export async function invoke<T>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  return tauriInvoke<T>(cmd, args);
}

export interface Connection {
  control_port: number;
  control_token: string;
  generation: number;
  sidecar_version: string;
}

export type BackendPhase = "stopped" | "starting" | "ready" | "restarting" | "migrating" | "failed";

export interface BackendState {
  phase: BackendPhase;
  generation: number;
  control_port: number;
  restart_count: number;
  started_at?: string;
  last_ready_at?: string;
  last_exit_code?: number;
  last_error?: string;
}

export interface DataDirInfo {
  current: string;
  default: string;
  is_custom: boolean;
}

export async function getDataDir(): Promise<DataDirInfo | null> {
  if (!isTauri()) return null;
  return invoke<DataDirInfo>("get_data_dir");
}

export async function setDataDir(path: string): Promise<DataDirInfo> {
  return invoke<DataDirInfo>("set_data_dir", { path });
}

export async function openDataDir(): Promise<void> {
  if (!isTauri()) return;
  await invoke("open_data_dir");
}

// Opens a native folder picker, returning the chosen path or null if cancelled.
export async function pickDirectory(defaultPath?: string): Promise<string | null> {
  if (!isTauri()) return null;
  const { open } = await import("@tauri-apps/plugin-dialog");
  const result = await open({ directory: true, multiple: false, defaultPath });
  return typeof result === "string" ? result : null;
}

// Fetches the sidecar connection info from the Rust shell and stashes it on the
// window so the (synchronous) control API client can read it.
let backendState: BackendState | null = null;
let bridgePromise: Promise<void> | null = null;
const backendListeners = new Set<(state: BackendState) => void>();

function publishBackendState(state: BackendState) {
  backendState = state;
  if (state.phase !== "ready") delete window.__SUB2API__;
  for (const listener of backendListeners) listener(state);
}

export interface WorkspaceSummary {
  id: string;
  name: string;
  kind: "local" | "legacy";
  active: boolean;
  data_dir: string;
  created_at: number;
  last_opened_at: number;
}

export async function listWorkspaces(): Promise<WorkspaceSummary[]> {
  if (!isTauri()) return [];
  return invoke<WorkspaceSummary[]>("list_workspaces");
}

export async function createWorkspace(name: string): Promise<WorkspaceSummary> {
  return invoke<WorkspaceSummary>("create_workspace", { name });
}

export async function switchWorkspace(workspaceId: string): Promise<WorkspaceSummary[]> {
  const workspaces = await invoke<WorkspaceSummary[]>("switch_workspace", { workspaceId });
  await refreshConnection();
  return workspaces;
}

function applyConnection(conn: Connection) {
  if (!conn || conn.control_port <= 0 || conn.control_token.length < 32) return;
  const currentGeneration = window.__SUB2API__?.generation ?? 0;
  if (conn.generation < currentGeneration) return;
  window.__SUB2API__ = {
    controlPort: conn.control_port,
    controlToken: conn.control_token,
    generation: conn.generation,
    sidecarVersion: conn.sidecar_version,
  };
}

export function subscribeBackendState(listener: (state: BackendState) => void): () => void {
  backendListeners.add(listener);
  if (backendState) listener(backendState);
  return () => backendListeners.delete(listener);
}

export async function refreshConnection(): Promise<boolean> {
  if (!isTauri()) return false;
  try {
    const conn = await invoke<Connection>("get_connection");
    applyConnection(conn);
    return Boolean(window.__SUB2API__);
  } catch {
    delete window.__SUB2API__;
    return false;
  }
}

export async function bootstrapConnection(): Promise<void> {
  await refreshConnection();
}

export async function initializeBackendBridge(): Promise<void> {
  if (!isTauri()) return;
  if (bridgePromise) return bridgePromise;
  bridgePromise = (async () => {
    await listen<BackendState>("backend-state-changed", ({ payload }) => publishBackendState(payload));
    await listen<Connection>("sidecar-ready", ({ payload }) => {
      applyConnection(payload);
      publishBackendState({
        ...(backendState ?? { restart_count: 0 }),
        phase: "ready",
        generation: payload.generation,
        control_port: payload.control_port,
      });
    });
    publishBackendState(await invoke<BackendState>("get_backend_state"));
    await refreshConnection();
  })().catch((error) => {
    bridgePromise = null;
    throw error;
  });
  return bridgePromise;
}

export async function restartBackend(): Promise<BackendState> {
  const state = await invoke<BackendState>("restart_backend");
  publishBackendState(state);
  return state;
}
