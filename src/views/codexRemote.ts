import { ref } from "vue";
import type { CodexRemoteProbe } from "../api/control";

export interface CodexRemoteFormValue {
  id?: number;
  host: string;
  port: number;
  user: string;
  password: string;
  model: string;
  remotePort: number;
}

export type CodexRemoteFormField = "host" | "port" | "user" | "password" | "model" | "remotePort";

// Module-level state so the Codex view keeps its drafts and connection test
// result when the user navigates away and back. Held in memory only.
export const codexActiveTab = ref<"local" | "remote">("local");
export const remoteForm = ref<CodexRemoteFormValue & { save: boolean }>({
  host: "",
  port: 22,
  user: "",
  password: "",
  model: "gpt-5.6",
  remotePort: 8080,
  save: true,
});
export const remoteModelInitialized = ref(false);
export const remoteProbe = ref<CodexRemoteProbe | null>(null);
export const testedSignature = ref("");
export const hostKeyAccepted = ref(false);
export type CodexRemoteFormErrors = Partial<Record<CodexRemoteFormField, "required" | "invalid">>;

export function isValidCodexModel(model: string): boolean {
  const value = model.trim().toLowerCase();
  return value.startsWith("gpt-5") || value.includes("codex");
}

export function validateCodexRemoteForm(value: CodexRemoteFormValue): CodexRemoteFormErrors {
  const errors: CodexRemoteFormErrors = {};
  if (!value.host.trim()) errors.host = "required";
  if (!value.user.trim()) errors.user = "required";
  if (!value.id && !value.password) errors.password = "required";
  if (!Number.isInteger(value.port) || value.port < 1 || value.port > 65535) errors.port = "invalid";
  if (!Number.isInteger(value.remotePort) || value.remotePort < 1 || value.remotePort > 65535) errors.remotePort = "invalid";
  if (!value.model.trim()) errors.model = "required";
  else if (!isValidCodexModel(value.model)) errors.model = "invalid";
  return errors;
}
