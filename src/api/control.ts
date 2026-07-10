// Control API client. Talks to the Go sidecar's loopback control server.
// Connection info (port + token) is injected by the Tauri shell into
// window.__SUB2API__; in browser dev it falls back to localStorage/defaults.
import { isTauri, refreshConnection } from "../tauri";

function conn(): { port: number; token: string } {
  if (window.__SUB2API__) {
    return {
      port: window.__SUB2API__.controlPort,
      token: window.__SUB2API__.controlToken,
    };
  }
  const port = Number(localStorage.getItem("s2a_control_port") || "0");
  const token = localStorage.getItem("s2a_control_token") || "";
  return { port, token };
}

export function isConnected(): boolean {
  const c = conn();
  return c.port > 0 && c.token.length > 0;
}

async function requestOnce(
  method: string,
  path: string,
  body?: unknown,
  rawBody?: BodyInit,
  extraHeaders?: Record<string, string>,
) {
  const c = conn();
  if (!c.port || !c.token) throw new Error("尚未连接到后台服务");
  const res = await fetch(`http://127.0.0.1:${c.port}${path}`, {
    method,
    headers: {
      "Content-Type": rawBody !== undefined ? "application/octet-stream" : "application/json",
      "X-Control-Token": c.token,
      ...extraHeaders,
    },
    body: rawBody !== undefined ? rawBody : body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data: any = {};
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: { code: "invalid_backend_response", message: text } };
    }
  }
  return { res, data };
}

async function req<T>(
  method: string,
  path: string,
  body?: unknown,
  rawBody?: BodyInit,
  extraHeaders?: Record<string, string>,
): Promise<T> {
  if ((!conn().port || !conn().token) && isTauri()) await refreshConnection();
  let result: Awaited<ReturnType<typeof requestOnce>>;
  try {
    result = await requestOnce(method, path, body, rawBody, extraHeaders);
  } catch (error) {
    if (!isTauri() || !(error instanceof TypeError)) throw error;
    await refreshConnection();
    result = await requestOnce(method, path, body, rawBody, extraHeaders);
  }
  if (result.res.status === 401 && isTauri()) {
    await refreshConnection();
    result = await requestOnce(method, path, body, rawBody, extraHeaders);
  }
  const { res, data } = result;
  if (!res.ok) {
    const payload = data?.error;
    const message = typeof payload === "string" ? payload : payload?.message;
    throw new Error(message || data?.message || `请求失败 (${res.status})`);
  }
  return data as T;
}

// ---- Types ----
export interface Status {
  version: string;
  server_running: boolean;
  port: number;
  host: string;
  endpoint: string;
	lan_addresses: string[];
  local_api_key: string;
  account_count: number;
  schema_version: number;
  migration_backup?: string;
}

export interface CodexUsage {
  primary_used_percent?: number;
  primary_reset_after_seconds?: number;
  primary_window_minutes?: number;
  secondary_used_percent?: number;
  secondary_reset_after_seconds?: number;
  secondary_window_minutes?: number;
  updated_at: string;
}

export interface Account {
  id: number;
  email: string;
  chatgpt_account_id: string;
  plan_type: string;
  expires_at: string;
  status: "active" | "refresh_failed" | "rate_limited" | "disabled" | "pending_validation";
  status_reason?: string;
  rate_limited_until?: string | null;
  proxy_id?: number | null;
  last_used_at?: string | null;
  last_success_at?: string | null;
  consecutive_failures: number;
  next_retry_at?: string | null;
  codex_usage?: CodexUsage | null;
  created_at: string;
}

export interface Proxy {
  id: number;
  name: string;
  type: "http" | "https" | "socks5";
  host: string;
  port: number;
  username?: string;
  password?: string;
  created_at: string;
}

export interface AccountUsage {
  account_id: number;
  requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost_usd: number;
}

export interface AccountTestResult {
  ok: boolean;
  status: number;
  error?: string;
  model: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  latency_ms: number;
  sample?: string;
  account_status: string;
}

export interface RequestLog {
  id: number;
  account_email?: string;
  model: string;
  status_code: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  latency_ms: number;
  stream: boolean;
  error?: string;
	request_id?: string;
	requested_model?: string;
	resolved_model?: string;
	error_kind?: string;
	attempt_count: number;
	terminal_event?: string;
  created_at: string;
}

export interface ModelCatalogResponse {
  models: string[];
  default_model: string;
  default_test_model: string;
  codex_default_model: string;
}

export interface Settings {
  listen_port: number;
  allow_lan: boolean;
  local_api_key: string;
  inject_instructions: boolean;
  default_model: string;
  user_agent: string;
  originator: string;
  language: string;
  auto_start_server: boolean;
  tls_fingerprint: boolean;
  codex_model: string;
	account_strategy: "failover" | "round_robin" | "quota_aware";
	log_retention_days: number;
	max_log_rows: number;
	auto_recovery: boolean;
	compatibility_profile: "standard" | "codex";
}

export interface ImportResult {
  imported: number;
  updated: number;
  skipped: number;
  errors?: string[];
}

export interface StatsResponse {
  summary: {
    total_requests: number;
    success_requests: number;
    failed_requests: number;
    total_tokens: number;
    prompt_tokens: number;
    completion_tokens: number;
    avg_latency_ms: number;
  };
  daily: { date: string; requests: number; total_tokens: number }[];
  by_model: { model: string; requests: number; total_tokens: number }[];
	retention: {
		days: number;
		max_rows: number;
		retained_rows: number;
		oldest_at?: number;
		newest_at?: number;
	};
}

export type ImportAction = "create" | "update" | "skip" | "error" | "conflict";

export interface ImportPreviewRow {
  index: number;
  action: ImportAction;
  matched_account_id?: number;
  email_masked?: string;
  chatgpt_account_id_masked?: string;
  has_access_token: boolean;
  has_refresh_token: boolean;
  has_id_token: boolean;
  identity_level: "unparsed" | "decoded" | "signed";
  identity_verified: boolean;
  warnings: string[];
  error_code?: string;
  error_message?: string;
}

export interface ImportSummary {
  total: number;
  create: number;
  update: number;
  skip: number;
  error: number;
  conflict: number;
}

export interface ImportPreview {
  content_sha256: string;
  summary: ImportSummary;
  rows: ImportPreviewRow[];
}

export interface ImportCommitResult {
  content_sha256: string;
  imported: number;
  updated: number;
  skipped: number;
  failed: number;
  validated: number;
  warnings?: string[];
  rows: ImportPreviewRow[];
  summary: ImportSummary;
}

export type DiagnosticCheckStatus = "ok" | "warning" | "failed" | "info";

export interface DiagnosticCheck {
  id: string;
  status: DiagnosticCheckStatus;
  title: string;
  duration_ms: number;
  message: string;
  details?: Record<string, unknown>;
}

export interface DiagnosticRun {
  run_id: string;
  status: "running" | "completed";
  progress: number;
  created_at: string;
  completed_at?: string;
  summary: { ok: number; warning: number; failed: number };
  checks: DiagnosticCheck[];
}

async function diagnosticReport(runId: string, format: "json" | "text"): Promise<Blob> {
  if ((!conn().port || !conn().token) && isTauri()) await refreshConnection();
  const fetchReport = () => {
    const c = conn();
    if (!c.port || !c.token) throw new Error("尚未连接到后台服务");
    return fetch(`http://127.0.0.1:${c.port}/control/diagnostics/runs/${encodeURIComponent(runId)}/report?format=${format}`, {
      headers: { "X-Control-Token": c.token },
    });
  };
  let response = await fetchReport();
  if (response.status === 401 && isTauri()) {
    await refreshConnection();
    response = await fetchReport();
  }
  if (!response.ok) throw new Error(`报告导出失败 (${response.status})`);
  return response.blob();
}

async function authenticatedDownload(path: string, failureLabel: string): Promise<Blob> {
	if ((!conn().port || !conn().token) && isTauri()) await refreshConnection();
	const fetchFile = () => {
		const c = conn();
		if (!c.port || !c.token) throw new Error("尚未连接到后台服务");
		return fetch(`http://127.0.0.1:${c.port}${path}`, { headers: { "X-Control-Token": c.token } });
	};
	let response = await fetchFile();
	if (response.status === 401 && isTauri()) {
		await refreshConnection();
		response = await fetchFile();
	}
	if (!response.ok) throw new Error(`${failureLabel} (${response.status})`);
	return response.blob();
}

// ---- Endpoints ----
export const api = {
  status: () => req<Status>("GET", "/control/status"),
  startServer: () => req<{ server_running: boolean; port: number }>("POST", "/control/server/start"),
  stopServer: () => req<{ server_running: boolean }>("POST", "/control/server/stop"),

  getSettings: () => req<Settings>("GET", "/control/settings"),
  saveSettings: (s: Partial<Settings>) => req<Settings>("PUT", "/control/settings", s),
  regenerateKey: () => req<{ local_api_key: string }>("POST", "/control/settings/regenerate-key"),

  listAccounts: () => req<{ accounts: Account[]; usage: Record<string, AccountUsage> }>("GET", "/control/accounts"),
  importAccounts: (rawText: string) =>
    req<ImportResult>("POST", "/control/accounts/import", undefined, rawText),
  previewImport: (raw: BodyInit) =>
    req<ImportPreview>("POST", "/control/accounts/import/preview", undefined, raw),
  commitImport: (raw: BodyInit, sha256: string, validate: boolean) =>
    req<ImportCommitResult>("POST", "/control/accounts/import/commit", undefined, raw, {
      "X-Import-Preview-SHA256": sha256,
      "X-Validate-After-Import": String(validate),
    }),
  deleteAccount: (id: number) => req<{ ok: boolean }>("DELETE", `/control/accounts/${id}`),
  refreshAccount: (id: number) => req<{ ok: boolean }>("POST", `/control/accounts/${id}/refresh`),
  bindProxy: (id: number, proxyId: number | null) =>
    req<{ ok: boolean }>("POST", `/control/accounts/${id}/proxy`, { proxy_id: proxyId }),
  testAccount: (id: number, model?: string, prompt?: string) =>
    req<AccountTestResult>("POST", `/control/accounts/${id}/test`, { model: model ?? "", prompt: prompt ?? "" }),
  setAccountStatus: (id: number, status: string) =>
    req<{ ok: boolean; account: Account }>("POST", `/control/accounts/${id}/status`, { status }),

  oauthStart: (proxyId?: number | null) =>
    req<{ auth_url: string; state: string }>("POST", "/control/oauth/start", { proxy_id: proxyId ?? null }),
  oauthPoll: (state: string) =>
    req<{ done: boolean; error: string; account?: Account }>("GET", `/control/oauth/poll?state=${encodeURIComponent(state)}`),

  listProxies: () => req<{ proxies: Proxy[] }>("GET", "/control/proxies"),
  createProxy: (p: Partial<Proxy> & { password?: string }) => req<Proxy>("POST", "/control/proxies", p),
  updateProxy: (id: number, p: Partial<Proxy> & { password?: string; clear_password?: boolean }) =>
    req<Proxy>("PUT", `/control/proxies/${id}`, p),
  deleteProxy: (id: number) => req<{ ok: boolean }>("DELETE", `/control/proxies/${id}`),
  testProxy: (id: number) =>
		req<{ ok: boolean; latency_ms?: number; error_kind?: string; error?: string; stages: { id: string; status: "ok" | "failed" | "skipped" | "not_run" }[] }>("POST", `/control/proxies/${id}/test`),

  listModels: () => req<ModelCatalogResponse>("GET", "/control/models"),

  logs: (limit = 50) => req<{ logs: RequestLog[] }>("GET", `/control/logs?limit=${limit}`),
  stats: (days = 7) => req<StatsResponse>("GET", `/control/stats?days=${days}`),
	exportLogs: (format: "json" | "csv", days = 0) =>
		authenticatedDownload(`/control/logs/export?format=${format}&days=${days}`, "日志导出失败"),
	clearLogs: () => req<{ deleted: number }>("DELETE", "/control/logs", undefined, undefined, {
		"X-Confirm-Clear": "clear-request-logs",
	}),
  startDiagnostics: () => req<DiagnosticRun>("POST", "/control/diagnostics/runs"),
  getDiagnostics: (runId: string) =>
    req<DiagnosticRun>("GET", `/control/diagnostics/runs/${encodeURIComponent(runId)}`),
  diagnosticReport,

  codexStatus: () => req<CodexStatus>("GET", "/control/codex/status"),
  codexApply: (model?: string) => req<CodexStatus>("POST", "/control/codex/apply", { model: model ?? "" }),
  codexRestore: () => req<CodexStatus>("POST", "/control/codex/restore"),
  codexFiles: () => req<CodexFiles>("GET", "/control/codex/files"),
  saveCodexFiles: (config: string, auth: string) =>
    req<CodexFiles>("PUT", "/control/codex/files", { config, auth }),
};

export interface CodexStatus {
  config_path: string;
  auth_path: string;
  applied: boolean;
  config_exists: boolean;
  backup_exists: boolean;
	backup_at?: string;
	backup_source?: string;
  base_url: string;
  model: string;
  models: string[];
  config_preview: string;
  auth_preview: string;
}

export interface CodexFiles {
  config_path: string;
  auth_path: string;
  config_content: string;
  auth_content: string;
  config_default: string;
  auth_default: string;
}
