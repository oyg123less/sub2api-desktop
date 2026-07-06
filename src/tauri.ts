// Thin Tauri bridge. All helpers degrade gracefully in a plain browser (dev).
import { invoke as tauriInvoke } from "@tauri-apps/api/core";

export function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

export async function invoke<T>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  return tauriInvoke<T>(cmd, args);
}

export interface Connection {
  control_port: number;
  control_token: string;
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
export async function bootstrapConnection(): Promise<void> {
  if (!isTauri()) return;
  try {
    const conn = await invoke<Connection>("get_connection");
    if (conn && conn.control_port > 0) {
      window.__SUB2API__ = {
        controlPort: conn.control_port,
        controlToken: conn.control_token,
      };
    }
  } catch {
    /* sidecar may still be starting; App will retry status polling */
  }
}
