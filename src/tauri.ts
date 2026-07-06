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
