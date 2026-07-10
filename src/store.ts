import { defineStore } from "pinia";
import { api, type Status } from "./api/control";
import type { BackendState } from "./tauri";

export interface Toast {
  id: number;
  type: "info" | "success" | "error" | "warn";
  message: string;
}

let toastSeq = 1;

export const useAppStore = defineStore("app", {
  state: () => ({
    status: null as Status | null,
    statusError: "" as string,
    toasts: [] as Toast[],
    ready: false,
    backend: null as BackendState | null,
  }),
  getters: {
    serverRunning: (s) => s.status?.server_running ?? false,
    accountCount: (s) => s.status?.account_count ?? 0,
    backendReady: (s) => s.backend?.phase === "ready",
  },
  actions: {
    async refreshStatus() {
      try {
        this.status = await api.status();
        this.statusError = "";
        this.ready = true;
      } catch (e) {
        this.statusError = (e as Error).message;
      }
    },
    setBackendState(state: BackendState) {
      this.backend = state;
      if (state.phase !== "ready") {
        this.statusError = state.last_error || "";
      }
    },
    toast(message: string, type: Toast["type"] = "info") {
      const id = toastSeq++;
      this.toasts.push({ id, type, message });
      setTimeout(() => this.dismiss(id), 3200);
    },
    dismiss(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id);
    },
  },
});
