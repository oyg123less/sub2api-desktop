// Platform helpers that work both inside Tauri and in a plain browser (dev).
import { isTauri, invoke } from "./tauri";

export function openUrl(url: string) {
  if (isTauri()) {
    invoke("open_external", { url }).catch(() => window.open(url, "_blank"));
    return;
  }
  window.open(url, "_blank");
}
