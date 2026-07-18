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
  account_type: "oauth" | "api_key";
  base_url: string;
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
	max_concurrency: number;
	queue_capacity: number;
	in_flight: number;
	waiting: number;
  codex_usage?: CodexUsage | null;
  created_at: string;
  client_uid: string;
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
  cached_tokens: number;
  completion_tokens: number;
  reasoning_tokens: number;
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
  cached_tokens: number;
  completion_tokens: number;
  reasoning_tokens: number;
  total_tokens: number;
  estimated: boolean;
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

export interface AccountRuntimeState {
  id: number;
  status: Account["status"];
  status_reason?: string;
  rate_limited_until?: string | null;
  in_flight: number;
  waiting: number;
}

export interface CloudConflict {
  id: number;
  kind: "account" | "proxy" | "codex_remote" | "settings";
  client_uid: string;
  display_name?: string;
  resolution: "local_won" | "remote_won";
  details?: string;
  created_at: string;
}

export interface CloudStatus {
  configured: boolean;
  authenticated: boolean;
  pending_verification: boolean;
  email?: string;
  role?: "user" | "admin";
  turnstile_site_key?: string;
  last_sync_at?: string;
  last_attempt_at?: string;
  pending_items: number;
  syncing: boolean;
  last_error?: string;
  last_error_code?: string;
  last_error_stage?: "dns" | "connect" | "tls" | "timeout" | "response" | "http" | "network" | "local";
  consecutive_failures: number;
  next_retry_at?: string;
  conflicts: CloudConflict[];
}

export interface CloudAdminUser {
  id: number;
  email: string;
  role: "user" | "admin";
  email_verified: number;
  banned: number;
  created_at: string;
  updated_at: string;
  last_active_at?: string;
  vault_count: number;
}

export interface CloudAdminSetting {
  key: "registration_enabled" | "invite_mode";
  value: string;
  updated_at: string;
}

export interface CloudAdminAudit {
  id: number;
  actor_user_id: number;
  action: string;
  target_type: string;
  target_id: string;
  details: string;
  created_at: string;
}

export interface CloudAdminOverview {
  users: CloudAdminUser[];
  shares: CloudAdminShare[];
  settings: CloudAdminSetting[];
  audit: CloudAdminAudit[];
  stats: { users: number; daily_active_users: number; vault_items: number; active_shares: number; share_requests: number; share_error_rate: number };
}

export interface CloudAdminShare {
  id: number;
  owner_id: number;
  owner_email: string;
  account_uid: string;
  share_code: string;
  quota_requests: number;
  used_requests: number;
  expires_at?: string;
  revoked: number;
  created_at: string;
  updated_at: string;
}

export interface CloudShare {
  id: number;
  account_uid: string;
  share_code: string;
  quota_requests: number;
  used_requests: number;
  expires_at?: string;
  revoked: boolean;
  created_at: string;
  updated_at: string;
  base_url: string;
}

export interface CloudShareUsage {
  id: number;
  ts: string;
  model?: string;
  status: number;
  latency_ms: number;
}

export interface CloudProfile {
  display_name: string;
  friend_code: string;
  encryption_public_key: string;
  encryption_key_version: number;
  created_at: string;
  updated_at: string;
}

export interface CloudFriend {
  public_id: string;
  display_name: string;
  friend_code: string;
  encryption_public_key: string;
  encryption_key_version: number;
  alias?: string;
  created_at: string;
  updated_at: string;
}

export interface CloudFriendRequest {
  public_id: string;
  status: "pending" | "accepted" | "declined" | "cancelled" | "expired";
  direction: "incoming" | "outgoing";
  display_name: string;
  friend_code: string;
  created_at: string;
  responded_at?: string;
  expires_at: string;
}

export interface CloudShareGroup {
  public_id: string;
  name: string;
  description: string;
  status: "active" | "paused" | "deleted";
  route_policy: "balanced" | "failover";
  default_rpm: number;
  default_concurrency: number;
  default_quota_requests: number;
  default_expires_at?: string;
  account_count: number;
  enabled_account_count?: number;
  recipient_count: number;
  used_requests?: number;
  base_url: string;
  created_at: string;
  updated_at: string;
}

export interface CloudShareGroupAccount {
  public_id: string;
  account_uid: string;
  account_type: "oauth" | "api_key";
  relay_mode: "owner_device" | "worker_direct";
  priority: number;
  weight: number;
  enabled: boolean | number;
  created_at: string;
  updated_at: string;
}

export interface CloudShareGroupRecipient {
  public_id: string;
  display_name: string;
  friendship_id?: string;
  status: "pending" | "active" | "paused" | "declined" | "expired" | "revoked" | "left";
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  used_requests: number;
  reserved_requests: number;
  expires_at?: string;
  key_id?: string;
  key_prefix?: string;
  key_version?: number;
  key_status?: string;
  created_at: string;
  updated_at: string;
}

export interface CloudShareGroupDetails {
  group: CloudShareGroup;
  accounts: CloudShareGroupAccount[];
  recipients: CloudShareGroupRecipient[];
}

export interface CloudShareGroupUsage {
  request_id: string;
  route_mode: "owner_device" | "worker_direct";
  model: string;
  status: number;
  error_code?: string;
  input_tokens?: number;
  output_tokens?: number;
  latency_ms: number;
  created_at: string;
  recipient_id: string;
  display_name: string;
  account_id?: string;
}

export interface CloudShareAuditEntry {
  action: string;
  target_type: string;
  target_public_id?: string;
  details: string;
  created_at: string;
}

export interface CloudReceivedShare {
  public_id: string;
  status: "pending" | "active" | "paused" | "declined" | "expired" | "revoked" | "left";
  group: {
    public_id: string;
    name: string;
    description: string;
    status: string;
    route_policy: string;
    account_count: number;
    owner_device_required: boolean;
  };
  owner: { display_name: string };
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  used_requests: number;
  expires_at?: string;
  created_at: string;
  accepted_at?: string;
  base_url: string;
  key?: { public_id: string; key_prefix: string; key_version: number; status: string };
  api_key?: string;
}

export interface CloudDevice {
  public_id: string;
  name: string;
  capabilities: string[];
  is_primary: boolean;
  revoked: boolean;
  online: boolean;
  last_seen_at?: string;
  relay?: { active_requests?: number; last_heartbeat_at?: string };
}

export interface CloudShareConnectionTest {
  ok: boolean;
  status: number;
  code?: string;
  message: string;
}

export interface CloudWorkspaceResponse {
  profile: CloudProfile;
  friends: { friends: CloudFriend[] };
  friend_requests: { requests: CloudFriendRequest[] };
  share_groups: { groups: CloudShareGroup[] };
  received_shares: { shares: CloudReceivedShare[] };
  devices: { devices: CloudDevice[]; relay_enabled: boolean };
}

export interface CreateShareGroupInput {
  idempotency_key: string;
  name: string;
  description: string;
  route_policy: "balanced" | "failover";
  default_rpm: number;
  default_concurrency: number;
  default_quota_requests: number;
  default_expires_at: string;
  accounts: Array<{ account_id: number; relay_mode: "owner_device" | "worker_direct"; priority?: number; weight?: number }>;
  recipients: Array<{ friendship_id: string; rpm_limit?: number; concurrency_limit?: number; quota_requests?: number; expires_at?: string }>;
}

export interface ReleaseInfo {
  tag_name: string;
  name: string;
  body: string;
  html_url: string;
  published_at: string;
  checked_at: string;
}

export interface ModelCatalogResponse {
  models: string[];
  default_model: string;
  default_test_model: string;
  codex_default_model: string;
}

export interface ModelPrice {
  model: string;
  input_per_m: number;
  cached_per_m?: number;
  output_per_m: number;
  long_context_threshold?: number;
  long_input_per_m?: number;
  long_cached_per_m?: number;
  long_output_per_m?: number;
}

export interface PricingResponse {
  price_version: string;
  source_url: string;
  tier: "standard";
  models: ModelPrice[];
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
    eligible_requests: number;
    success_requests: number;
    failed_requests: number;
    client_cancelled: number;
    total_tokens: number;
    prompt_tokens: number;
    cached_tokens: number;
    completion_tokens: number;
    reasoning_tokens: number;
    estimated_requests: number;
    avg_latency_ms: number;
  };
  daily: { date: string; requests: number; total_tokens: number }[];
  by_model: { model: string; requests: number; total_tokens: number }[];
	failure_breakdown: { kind: "upstream_error" | "rate_limited" | "authentication" | "stream_interrupted"; requests: number }[];
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
  account_type: "oauth" | "api_key";
  matched_account_id?: number;
  email_masked?: string;
  chatgpt_account_id_masked?: string;
  has_access_token: boolean;
  has_refresh_token: boolean;
  has_id_token: boolean;
  has_api_key: boolean;
  identity_level: "unparsed" | "decoded" | "signed";
  identity_verified: boolean;
  warnings: string[];
  warning_codes: ("jwks_unreachable" | "signature_invalid")[];
  error_code?: string;
  error_message?: string;
  proxy_id?: number;
  proxy_specified: boolean;
}

export interface ImportProxyOptions {
  mode: "preserve" | "direct" | "override";
  proxyId?: number | null;
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

function importProxyHeaders(options?: ImportProxyOptions): Record<string, string> {
  if (!options) return {};
  const headers: Record<string, string> = { "X-Import-Proxy-Mode": options.mode };
  if (options.mode === "override" && options.proxyId) {
    headers["X-Import-Proxy-ID"] = String(options.proxyId);
  }
  return headers;
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
  latestRelease: () => req<ReleaseInfo>("GET", "/control/update"),
  startServer: () => req<{ server_running: boolean; port: number }>("POST", "/control/server/start"),
  stopServer: () => req<{ server_running: boolean }>("POST", "/control/server/stop"),

  getSettings: () => req<Settings>("GET", "/control/settings"),
  saveSettings: (s: Partial<Settings>) => req<Settings>("PUT", "/control/settings", s),
  regenerateKey: () => req<{ local_api_key: string }>("POST", "/control/settings/regenerate-key"),

  cloudStatus: () => req<CloudStatus>("GET", "/control/cloud/status"),
  cloudRegister: (input: { email: string; password: string; turnstile_token: string; recovery_acknowledged: boolean }) =>
    req<{ ok: boolean; verification_required: boolean }>("POST", "/control/cloud/register", input),
  cloudVerifyEmail: (email: string, code: string) =>
    req<CloudStatus>("POST", "/control/cloud/verify-email", { email, code }),
  cloudResendVerification: (email: string) =>
    req<CloudStatus>("POST", "/control/cloud/resend-verification", { email }),
  cloudCancelRegistration: () =>
    req<CloudStatus>("POST", "/control/cloud/cancel-registration"),
  cloudLogin: (email: string, password: string) =>
    req<CloudStatus>("POST", "/control/cloud/login", { email, password }),
  cloudLogout: () => req<CloudStatus>("POST", "/control/cloud/logout"),
  cloudSync: () => req<CloudStatus>("POST", "/control/cloud/sync"),
  cloudChangePassword: (currentPassword: string, newPassword: string) =>
    req<CloudStatus>("PUT", "/control/cloud/master-password", { current_password: currentPassword, new_password: newPassword }),
  cloudAdminOverview: (adminKey: string) =>
    req<CloudAdminOverview>("POST", "/control/cloud/admin/overview", { admin_key: adminKey }),
  cloudAdminSetUserBanned: (adminKey: string, userId: number, banned: boolean) =>
    req<{ ok: boolean }>("PATCH", `/control/cloud/admin/users/${userId}`, { admin_key: adminKey, banned }),
  cloudAdminLogoutUser: (adminKey: string, userId: number) =>
    req<{ ok: boolean }>("POST", `/control/cloud/admin/users/${userId}/logout-all`, { admin_key: adminKey }),
  cloudAdminDeleteUser: (adminKey: string, userId: number) =>
    req<{ ok: boolean }>("DELETE", `/control/cloud/admin/users/${userId}`, { admin_key: adminKey, confirm: "DELETE" }),
  cloudAdminUpdateSettings: (adminKey: string, settings: { registration_enabled?: boolean; invite_mode?: boolean }) =>
    req<{ ok: boolean }>("PATCH", "/control/cloud/admin/settings", { admin_key: adminKey, ...settings }),
  cloudAdminSetShareRevoked: (adminKey: string, shareId: number, revoked: boolean) =>
    req<{ ok: boolean }>("PATCH", `/control/cloud/admin/shares/${shareId}`, { admin_key: adminKey, revoked }),
  cloudShares: () => req<{ shares: CloudShare[] }>("GET", "/control/cloud/shares"),
  cloudCreateShare: (input: { account_id: number; quota_requests: number; expires_at: string; consent: boolean }) =>
    req<{ share: CloudShare; guest_key: string }>("POST", "/control/cloud/shares", input),
  cloudUpdateShare: (shareId: number, updates: { revoked?: boolean; quota_requests?: number; expires_at?: string }) =>
    req<CloudShare>("PATCH", `/control/cloud/shares/${shareId}`, updates),
  cloudShareUsage: (shareId: number) =>
    req<{ usage: CloudShareUsage[] }>("GET", `/control/cloud/shares/${shareId}/usage`),
  cloudProfile: () => req<CloudProfile>("GET", "/control/cloud/profile"),
  cloudWorkspace: () => req<CloudWorkspaceResponse>("GET", "/control/cloud/workspace"),
  cloudUpdateProfile: (displayName: string) => req<CloudProfile>("PUT", "/control/cloud/profile", { display_name: displayName }),
  cloudFriends: () => req<{ friends: CloudFriend[] }>("GET", "/control/cloud/friends"),
  cloudFriendRequests: () => req<{ requests: CloudFriendRequest[] }>("GET", "/control/cloud/friend-requests"),
  cloudAddFriend: (friendCode: string) => req<{ request: CloudFriendRequest }>("POST", "/control/cloud/friend-requests", { friend_code: friendCode }),
  cloudFriendRequestAction: (requestId: string, action: "accept" | "decline" | "cancel") =>
    req<Record<string, unknown>>("POST", `/control/cloud/friend-requests/${requestId}/${action}`),
  cloudUpdateFriend: (friendId: string, alias: string) => req<{ ok: boolean }>("PATCH", `/control/cloud/friends/${friendId}`, { alias }),
  cloudDeleteFriend: (friendId: string, revoke = false) => req<{ ok: boolean }>("DELETE", `/control/cloud/friends/${friendId}?mode=${revoke ? "revoke" : "pause"}`),
  cloudBlockFriend: (friendId: string) => req<{ ok: boolean }>("POST", `/control/cloud/friends/${friendId}/block`),
  cloudShareGroups: () => req<{ groups: CloudShareGroup[] }>("GET", "/control/cloud/share-groups"),
  cloudCreateShareGroup: (input: CreateShareGroupInput) => req<CloudShareGroupDetails>("POST", "/control/cloud/share-groups", input),
  cloudShareGroup: (groupId: string) => req<CloudShareGroupDetails>("GET", `/control/cloud/share-groups/${groupId}`),
  cloudUpdateShareGroup: (groupId: string, updates: Record<string, unknown>) => req<CloudShareGroupDetails>("PATCH", `/control/cloud/share-groups/${groupId}`, updates),
  cloudDeleteShareGroup: (groupId: string) => req<{ ok: boolean }>("DELETE", `/control/cloud/share-groups/${groupId}`),
  cloudAddShareGroupAccount: (groupId: string, input: { account_id: number; relay_mode: "owner_device" | "worker_direct"; priority?: number; weight?: number }) =>
    req<{ account: CloudShareGroupAccount }>("POST", `/control/cloud/share-groups/${groupId}/accounts`, input),
  cloudUpdateShareGroupAccount: (groupId: string, accountId: string, updates: Record<string, unknown>) =>
    req<{ ok: boolean }>("PATCH", `/control/cloud/share-groups/${groupId}/accounts/${accountId}`, updates),
  cloudDeleteShareGroupAccount: (groupId: string, accountId: string) =>
    req<{ ok: boolean }>("DELETE", `/control/cloud/share-groups/${groupId}/accounts/${accountId}`),
  cloudInviteShareGroupRecipients: (groupId: string, input: { idempotency_key: string; recipients: CreateShareGroupInput["recipients"] }) =>
    req<{ recipients: CloudShareGroupRecipient[] }>("POST", `/control/cloud/share-groups/${groupId}/recipients`, input),
  cloudUpdateShareGroupRecipient: (groupId: string, recipientId: string, updates: Record<string, unknown>) =>
    req<{ recipient: CloudShareGroupRecipient }>("PATCH", `/control/cloud/share-groups/${groupId}/recipients/${recipientId}`, updates),
  cloudDeleteShareGroupRecipient: (groupId: string, recipientId: string) =>
    req<{ ok: boolean }>("DELETE", `/control/cloud/share-groups/${groupId}/recipients/${recipientId}`),
  cloudRotateShareGroupKey: (groupId: string, recipientId: string, friendshipId: string, idempotencyKey: string) =>
    req<Record<string, unknown>>("POST", `/control/cloud/share-groups/${groupId}/recipients/${recipientId}/keys/rotate`, { friendship_id: friendshipId, idempotency_key: idempotencyKey }),
  cloudShareGroupUsage: (groupId: string) => req<{ usage: CloudShareGroupUsage[] }>("GET", `/control/cloud/share-groups/${groupId}/usage`),
  cloudShareGroupAudit: (groupId: string) => req<{ audit: CloudShareAuditEntry[] }>("GET", `/control/cloud/share-groups/${groupId}/audit`),
  cloudReceivedShares: () => req<{ shares: CloudReceivedShare[] }>("GET", "/control/cloud/received-shares"),
  cloudReceivedShareAction: (shareId: string, action: "accept" | "decline" | "leave") =>
    req<CloudReceivedShare | { ok: boolean }>("POST", `/control/cloud/received-shares/${shareId}/${action}`),
  cloudTestReceivedShare: (shareId: string) =>
    req<CloudShareConnectionTest>("POST", `/control/cloud/received-shares/${shareId}/test`),
  cloudDevices: () => req<{ devices: CloudDevice[]; relay_enabled: boolean }>("GET", "/control/cloud/devices"),
  cloudEnsureDevice: () => req<CloudDevice>("POST", "/control/cloud/devices/ensure"),
  cloudDeleteDevice: (deviceId: string) => req<{ ok: boolean }>("DELETE", `/control/cloud/devices/${deviceId}`),
  cloudSetRelay: (enabled: boolean) => req<{ enabled: boolean }>("PUT", "/control/cloud/relay", { enabled }),

  listAccounts: () => req<{ accounts: Account[]; usage: Record<string, AccountUsage> }>("GET", "/control/accounts"),
  accountRuntime: () => req<{ accounts: AccountRuntimeState[] }>("GET", "/control/accounts/runtime"),
  importAccounts: (rawText: string) =>
    req<ImportResult>("POST", "/control/accounts/import", undefined, rawText),
  previewImport: (raw: BodyInit, proxyOptions?: ImportProxyOptions) =>
    req<ImportPreview>("POST", "/control/accounts/import/preview", undefined, raw, importProxyHeaders(proxyOptions)),
  commitImport: (raw: BodyInit, sha256: string, validate: boolean, proxyOptions?: ImportProxyOptions) =>
    req<ImportCommitResult>("POST", "/control/accounts/import/commit", undefined, raw, {
      "X-Import-Preview-SHA256": sha256,
      "X-Validate-After-Import": String(validate),
      ...importProxyHeaders(proxyOptions),
    }),
  deleteAccount: (id: number) => req<{ ok: boolean }>("DELETE", `/control/accounts/${id}`),
  refreshAccount: (id: number) => req<{ ok: boolean }>("POST", `/control/accounts/${id}/refresh`),
  bindProxy: (id: number, proxyId: number | null) =>
    req<{ ok: boolean }>("POST", `/control/accounts/${id}/proxy`, { proxy_id: proxyId }),
  testAccount: (id: number, model?: string, prompt?: string) =>
    req<AccountTestResult>("POST", `/control/accounts/${id}/test`, { model: model ?? "", prompt: prompt ?? "" }),
  setAccountStatus: (id: number, status: string) =>
    req<{ ok: boolean; account: Account }>("POST", `/control/accounts/${id}/status`, { status }),
	setAccountLimits: (id: number, limits: { max_concurrency: number; queue_capacity: number }) =>
		req<{ ok: boolean; account: Account }>("PUT", `/control/accounts/${id}/limits`, limits),

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
  pricing: () => req<PricingResponse>("GET", "/control/pricing"),

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
  codexRemoteTest: (target: CodexRemoteConnectionInput) =>
    req<CodexRemoteProbe>("POST", "/control/codex/remote/test", target),
  codexRemoteInject: (target: CodexRemoteInjectInput) =>
    req<CodexRemoteTarget>("POST", "/control/codex/remote/inject", target),
  codexRemoteTargets: () =>
    req<{ targets: CodexRemoteTarget[] }>("GET", "/control/codex/remote/targets"),
  codexRemoteSetTunnel: (id: number, enabled: boolean) =>
    req<CodexRemoteTarget>("POST", `/control/codex/remote/${id}/tunnel`, { enabled }),
  codexRemoteRestore: (id: number) =>
    req<CodexRemoteTarget>("POST", `/control/codex/remote/${id}/restore`),
  codexRemoteDelete: (id: number) =>
    req<{ ok: boolean }>("DELETE", `/control/codex/remote/${id}`),
};

export interface CodexStatus {
  config_path: string;
  auth_path: string;
  applied: boolean;
  config_exists: boolean;
  backup_exists: boolean;
	backup_at?: string;
  backup_source?: string;
  stale: boolean;
  stale_reason?: string;
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

export interface CodexRemoteProbe {
  os: string;
  home: string;
  codex_dir: string;
  host_key_fingerprint: string;
  known: boolean;
}

export interface CodexRemoteTarget {
  id: number;
  name: string;
  host: string;
  port: number;
  user: string;
  remote_port: number;
  model: string;
  mode: "tunnel" | "direct";
  base_url?: string;
  saved: boolean;
  injected: boolean;
  tunnel_enabled: boolean;
  tunnel_status: "connected" | "down" | "disabled" | "not_injected" | "injected_direct";
  last_error?: string;
  config_preview: string;
  auth_preview: string;
  updated_at: string;
}

export interface CodexRemoteConnectionInput {
  id?: number;
  name?: string;
  host: string;
  port: number;
  user: string;
  password: string;
}

export interface CodexRemoteInjectInput extends CodexRemoteConnectionInput {
  model: string;
  remote_port: number;
  mode: "tunnel" | "direct";
  base_url: string;
  api_key: string;
  save: boolean;
  accept_host_key: boolean;
}
