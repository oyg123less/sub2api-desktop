import { ref } from "vue";
import type { CodexRemoteProbe, CodexRemoteTarget } from "../api/control";

export type CodexRemoteMode = "tunnel" | "direct";

export interface CodexRemoteFormValue {
  id?: number;
  host: string;
  port: number;
  user: string;
  password: string;
  model: string;
  remotePort: number;
  mode: CodexRemoteMode;
  baseUrl: string;
  apiKey: string;
}

export type CodexRemoteFormField =
  | "host"
  | "port"
  | "user"
  | "password"
  | "model"
  | "remotePort"
  | "baseUrl"
  | "apiKey";

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
  mode: "tunnel",
  baseUrl: "",
  apiKey: "",
  save: true,
});
export const remoteModelInitialized = ref(false);
export const remoteProbe = ref<CodexRemoteProbe | null>(null);
export const testedSignature = ref("");
export const hostKeyAccepted = ref(false);
export type CodexRemoteFormErrors = Partial<Record<CodexRemoteFormField, "required" | "invalid">>;

const targetStatuses = new Set<CodexRemoteTarget["tunnel_status"]>([
  "connected",
  "down",
  "disabled",
  "not_injected",
  "injected_direct",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function asPort(value: unknown, fallback: number): number {
  const port = Number(value);
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : fallback;
}

export function normalizeCodexRemoteTarget(value: unknown): CodexRemoteTarget | null {
  if (!isRecord(value)) return null;
  const id = Number(value.id);
  if (!Number.isInteger(id) || id === 0) return null;

  const mode: CodexRemoteTarget["mode"] = value.mode === "direct" ? "direct" : "tunnel";
  const rawStatus = asString(value.tunnel_status) as CodexRemoteTarget["tunnel_status"];
  const tunnelStatus = targetStatuses.has(rawStatus) ? rawStatus : "not_injected";
  const host = asString(value.host);
  const user = asString(value.user);

  return {
    id,
    name: asString(value.name, user && host ? `${user}@${host}` : host),
    host,
    port: asPort(value.port, 22),
    user,
    remote_port: asPort(value.remote_port, 8080),
    model: asString(value.model, "gpt-5.6"),
    mode,
    base_url: asString(value.base_url),
    saved: value.saved === true,
    injected: value.injected === true,
    tunnel_enabled: mode === "tunnel" && value.tunnel_enabled === true,
    tunnel_status: tunnelStatus,
    last_error: asString(value.last_error) || undefined,
    config_preview: asString(value.config_preview),
    auth_preview: asString(value.auth_preview),
    updated_at: asString(value.updated_at),
  };
}

export function normalizeCodexRemoteTargets(response: unknown): CodexRemoteTarget[] {
  const values = Array.isArray(response)
    ? response
    : isRecord(response) && Array.isArray(response.targets)
      ? response.targets
      : [];
  return values
    .map(normalizeCodexRemoteTarget)
    .filter((target): target is CodexRemoteTarget => target !== null);
}

export function isValidCodexModel(model: string): boolean {
  const value = model.trim().toLowerCase();
  return value.startsWith("gpt-5") || value.includes("codex");
}

export function hasEmbeddedSSHUser(host: string): boolean {
  const [user, hostname] = host.trim().split("@", 2);
  return Boolean(user?.trim() && hostname?.trim());
}

export function sshUserForRequest(host: string, user: string): string {
  return hasEmbeddedSSHUser(host) ? "" : user.trim();
}

export function isValidDirectBaseURL(value: string): boolean {
  try {
    const parsed = new URL(value.trim());
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      Boolean(parsed.host) &&
      !parsed.username &&
      !parsed.password
    );
  } catch {
    return false;
  }
}

export function validateCodexRemoteForm(value: CodexRemoteFormValue): CodexRemoteFormErrors {
  const errors: CodexRemoteFormErrors = {};
  if (!value.host.trim()) errors.host = "required";
  if (!value.user.trim() && !hasEmbeddedSSHUser(value.host)) errors.user = "required";
  if (!value.id && !value.password) errors.password = "required";
  if (!Number.isInteger(value.port) || value.port < 1 || value.port > 65535) errors.port = "invalid";
  if (value.mode === "direct") {
    if (!value.baseUrl.trim()) errors.baseUrl = "required";
    else if (!isValidDirectBaseURL(value.baseUrl)) errors.baseUrl = "invalid";
    if (!value.apiKey.trim() && (!value.id || value.id <= 0)) errors.apiKey = "required";
  } else if (!Number.isInteger(value.remotePort) || value.remotePort < 1 || value.remotePort > 65535) {
    errors.remotePort = "invalid";
  }
  if (!value.model.trim()) errors.model = "required";
  else if (!isValidCodexModel(value.model)) errors.model = "invalid";
  return errors;
}
