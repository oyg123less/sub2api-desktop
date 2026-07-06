// Control API client. Talks to the Go sidecar's loopback control server.
// Connection info (port + token) is injected by the Tauri shell into
// window.__SUB2API__; in browser dev it falls back to localStorage/defaults.

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

async function req<T>(method: string, path: string, body?: unknown, rawBody?: string): Promise<T> {
  const c = conn();
  if (!c.port || !c.token) {
    throw new Error("尚未连接到后台服务");
  }
  const res = await fetch(`http://127.0.0.1:${c.port}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-Control-Token": c.token,
    },
    body: rawBody !== undefined ? rawBody : body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data?.error || `请求失败 (${res.status})`);
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
  local_api_key: string;
  account_count: number;
}

export interface Account {
  id: number;
  email: string;
  chatgpt_account_id: string;
  plan_type: string;
  expires_at: string;
  status: "active" | "refresh_failed" | "rate_limited" | "disabled";
  status_reason?: string;
  rate_limited_until?: string | null;
  proxy_id?: number | null;
  last_used_at?: string | null;
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
  created_at: string;
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
  updateProxy: (id: number, p: Partial<Proxy> & { password?: string }) => req<Proxy>("PUT", `/control/proxies/${id}`, p),
  deleteProxy: (id: number) => req<{ ok: boolean }>("DELETE", `/control/proxies/${id}`),
  testProxy: (id: number) =>
    req<{ ok: boolean; latency_ms?: number; error?: string }>("POST", `/control/proxies/${id}/test`),

  logs: (limit = 50) => req<{ logs: RequestLog[] }>("GET", `/control/logs?limit=${limit}`),
  stats: (days = 7) => req<StatsResponse>("GET", `/control/stats?days=${days}`),

  codexStatus: () => req<CodexStatus>("GET", "/control/codex/status"),
  codexApply: () => req<CodexStatus>("POST", "/control/codex/apply"),
  codexRestore: () => req<CodexStatus>("POST", "/control/codex/restore"),
};

export interface CodexStatus {
  config_path: string;
  auth_path: string;
  applied: boolean;
  config_exists: boolean;
  backup_exists: boolean;
  base_url: string;
  model: string;
  config_preview: string;
  auth_preview: string;
}
