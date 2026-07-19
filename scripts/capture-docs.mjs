import { chromium } from "@playwright/test";
import path from "node:path";

const baseURL = process.env.AMBER_DOCS_BASE_URL || "http://127.0.0.1:4173/";
const outputDir = path.resolve("src/assets/docs");
const now = "2026-07-18T10:30:00Z";

const settings = {
  listen_port: 8080,
  allow_lan: false,
  local_api_key: "sk-local-demo-amber-documentation",
  inject_instructions: true,
  default_model: "gpt-5.6-sol",
  user_agent: "codex_cli_rs",
  originator: "codex_cli_rs",
  language: "zh-CN",
  auto_start_server: true,
  tls_fingerprint: false,
  codex_model: "gpt-5.6-sol",
  account_strategy: "quota_aware",
  log_retention_days: 30,
  max_log_rows: 100000,
  auto_recovery: true,
  compatibility_profile: "standard",
};

const accounts = [
  {
    id: 1,
    account_type: "oauth",
    base_url: "",
    email: "demo.owner@example.com",
    chatgpt_account_id: "acct_demo_owner",
    plan_type: "plus",
    expires_at: "2026-07-25T08:00:00Z",
    status: "active",
    consecutive_failures: 0,
    max_concurrency: 3,
    queue_capacity: 20,
    in_flight: 1,
    waiting: 0,
    proxy_id: 1,
    last_used_at: now,
    last_success_at: now,
    created_at: "2026-07-10T08:00:00Z",
    client_uid: "demo-oauth-owner",
    codex_usage: {
      primary_used_percent: 36,
      primary_reset_after_seconds: 6300,
      primary_window_minutes: 300,
      secondary_used_percent: 18,
      secondary_reset_after_seconds: 340000,
      secondary_window_minutes: 10080,
      updated_at: now,
    },
  },
  {
    id: 2,
    account_type: "oauth",
    base_url: "",
    email: "backup.account@example.com",
    chatgpt_account_id: "acct_demo_backup",
    plan_type: "team",
    expires_at: "2026-07-24T08:00:00Z",
    status: "active",
    consecutive_failures: 0,
    max_concurrency: 2,
    queue_capacity: 12,
    in_flight: 0,
    waiting: 0,
    proxy_id: null,
    last_used_at: "2026-07-18T09:42:00Z",
    last_success_at: "2026-07-18T09:42:00Z",
    created_at: "2026-07-11T08:00:00Z",
    client_uid: "demo-oauth-backup",
    codex_usage: {
      primary_used_percent: 12,
      primary_reset_after_seconds: 7200,
      primary_window_minutes: 300,
      secondary_used_percent: 8,
      secondary_reset_after_seconds: 410000,
      secondary_window_minutes: 10080,
      updated_at: now,
    },
  },
  {
    id: 3,
    account_type: "api_key",
    base_url: "https://api.example.test/v1",
    email: "演示 API 端点",
    chatgpt_account_id: "",
    plan_type: "api",
    expires_at: "",
    status: "active",
    consecutive_failures: 0,
    max_concurrency: 4,
    queue_capacity: 30,
    in_flight: 0,
    waiting: 0,
    proxy_id: 2,
    last_used_at: "2026-07-18T09:15:00Z",
    last_success_at: "2026-07-18T09:15:00Z",
    created_at: "2026-07-12T08:00:00Z",
    client_uid: "demo-api-endpoint",
  },
];

const proxies = [
  { id: 1, name: "东京出口（演示）", type: "socks5", host: "127.0.0.1", port: 1080, created_at: now },
  { id: 2, name: "本地 HTTP（演示）", type: "http", host: "127.0.0.1", port: 7890, created_at: now },
];

const stats = {
  summary: {
    total_requests: 1284,
    eligible_requests: 1271,
    success_requests: 1239,
    failed_requests: 32,
    client_cancelled: 13,
    total_tokens: 18426320,
    prompt_tokens: 11910000,
    cached_tokens: 4210000,
    completion_tokens: 6516320,
    reasoning_tokens: 914000,
    estimated_requests: 4,
    avg_latency_ms: 842,
    cost_usd: 3015.4344,
    pricing_fallback_requests: 0,
  },
  daily: [
    { date: "2026-07-12", requests: 138, total_tokens: 1920000, cost_usd: 312.42 },
    { date: "2026-07-13", requests: 162, total_tokens: 2180000, cost_usd: 365.17 },
    { date: "2026-07-14", requests: 154, total_tokens: 2050000, cost_usd: 341.82 },
    { date: "2026-07-15", requests: 183, total_tokens: 2720000, cost_usd: 429.66 },
    { date: "2026-07-16", requests: 196, total_tokens: 2980000, cost_usd: 481.21 },
    { date: "2026-07-17", requests: 214, total_tokens: 3190000, cost_usd: 542.54 },
    { date: "2026-07-18", requests: 237, total_tokens: 3386320, cost_usd: 542.61 },
  ],
  by_model: [
    { model: "gpt-5.6-sol", requests: 824, total_tokens: 12200000, cost_usd: 2214.12 },
    { model: "gpt-5.6-terra", requests: 338, total_tokens: 4860000, cost_usd: 681.25 },
    { model: "gpt-5.5-pro", requests: 122, total_tokens: 1366320, cost_usd: 120.0644 },
  ],
  pricing: {
    version: "2026-07-16",
    source_url: "https://developers.openai.com/api/docs/pricing",
    tier: "standard",
    currency: "USD",
  },
  failure_breakdown: [
    { kind: "rate_limited", requests: 14 },
    { kind: "upstream_error", requests: 12 },
    { kind: "authentication", requests: 6 },
  ],
  retention: { days: 30, max_rows: 100000, retained_rows: 1284 },
};

const logs = [
  { id: 1, account_email: "demo.owner@example.com", model: "gpt-5.6-sol", status_code: 200, prompt_tokens: 4820, cached_tokens: 1900, completion_tokens: 1130, reasoning_tokens: 120, total_tokens: 5950, estimated: false, latency_ms: 782, stream: true, attempt_count: 1, created_at: now },
  { id: 2, account_email: "backup.account@example.com", model: "gpt-5.6-terra", status_code: 200, prompt_tokens: 2640, cached_tokens: 820, completion_tokens: 740, reasoning_tokens: 80, total_tokens: 3380, estimated: false, latency_ms: 641, stream: true, attempt_count: 1, created_at: "2026-07-18T10:22:00Z" },
  { id: 3, account_email: "demo.owner@example.com", model: "gpt-5.6-sol", status_code: 429, prompt_tokens: 0, cached_tokens: 0, completion_tokens: 0, reasoning_tokens: 0, total_tokens: 0, estimated: false, latency_ms: 214, stream: false, error: "上游暂时限流，Amber 已自动切换其他可用账号。", error_kind: "rate_limited", attempt_count: 2, created_at: "2026-07-18T10:10:00Z" },
];

const workspace = {
  profile: {
    display_name: "Amber 演示用户",
    friend_code: "AMB-DEMO-2026",
    encryption_public_key: "demo-public-key",
    encryption_key_version: 1,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: now,
  },
  friends: {
    friends: [
      { public_id: "friend-1", display_name: "小林", friend_code: "AMB-LIN-2026A", encryption_public_key: "demo-friend-key", encryption_key_version: 1, alias: "开发搭档", created_at: now, updated_at: now },
      { public_id: "friend-2", display_name: "陈同学", friend_code: "AMB-CHEN-26B", encryption_public_key: "demo-friend-key-2", encryption_key_version: 1, created_at: now, updated_at: now },
    ],
  },
  friend_requests: { requests: [] },
  share_groups: {
    groups: [
      { public_id: "group-1", name: "研发共享池", description: "为开发搭档提供稳定的模型调用", status: "active", route_policy: "balanced", default_rpm: 30, default_concurrency: 2, default_quota_requests: 5000, account_count: 2, enabled_account_count: 2, recipient_count: 2, used_requests: 836, base_url: "https://amber-cloud.example.test/v1", created_at: now, updated_at: now },
    ],
  },
  connect_host: {
    configured: true,
    endpoint: { public_id: "conn-demo", connection_code: "572814639", status: "active", group_status: "active", base_url: "https://amber-cloud.example.test/v1", created_at: now, updated_at: now },
    window: { public_id: "win-demo", password_version: 2, max_claims: 3, claimed_count: 1, expires_at: "2026-07-19T12:30:00Z", created_at: now },
    temporary_password: "AB3D5F",
    accounts: [
      { public_id: "sga-demo-1", account_uid: "account-demo-1", account_type: "oauth", relay_mode: "owner_device", enabled: true },
      { public_id: "sga-demo-2", account_uid: "account-demo-2", account_type: "api_key", relay_mode: "worker_direct", enabled: true },
    ],
    recipients: [
      { public_id: "sgr-demo-1", display_name: "小林", friend_code: "AMB-LIN-2026A", status: "active", rpm_limit: 30, concurrency_limit: 2, quota_requests: 2000, used_requests: 318, key_prefix: "sk-amber-demo", created_at: now },
    ],
  },
  received_shares: {
    shares: [
      { public_id: "received-1", status: "active", group: { public_id: "received-group", name: "设计协作", description: "", status: "active", route_policy: "balanced", account_count: 2, owner_device_required: true }, owner: { display_name: "设计团队" }, rpm_limit: 20, concurrency_limit: 2, quota_requests: 2000, used_requests: 318, created_at: now, accepted_at: now, base_url: "https://amber-cloud.example.test/v1", key: { public_id: "key-demo", key_prefix: "sk-amber-demo", key_version: 1, status: "active" }, api_key: "sk-amber-demo-documentation-only", local_enabled: true },
    ],
  },
  devices: {
    relay_enabled: true,
    devices: [
      { public_id: "device-1", name: "Amber Windows 主设备", capabilities: ["relay", "oauth"], is_primary: true, revoked: false, online: true, last_seen_at: now, relay: { active_requests: 1, last_heartbeat_at: now } },
    ],
  },
};

let cloudAuthenticated = false;

function json(route, body, status = 200) {
  return route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

async function handleControlRoute(route) {
  const request = route.request();
  const pathname = new URL(request.url()).pathname;

  if (pathname === "/control/status") {
    return json(route, { version: "0.4.2", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: settings.local_api_key, account_count: accounts.length, schema_version: 14 });
  }
  if (pathname === "/control/settings") return json(route, settings);
  if (pathname === "/control/accounts" && request.method() === "GET") {
    return json(route, {
      accounts,
      usage: {
        1: { account_id: 1, requests: 624, prompt_tokens: 5100000, cached_tokens: 1900000, completion_tokens: 2380000, reasoning_tokens: 420000, total_tokens: 7480000, cost_usd: 1240.4832 },
        2: { account_id: 2, requests: 438, prompt_tokens: 3980000, cached_tokens: 1420000, completion_tokens: 1900000, reasoning_tokens: 280000, total_tokens: 5880000, cost_usd: 984.125 },
        3: { account_id: 3, requests: 222, prompt_tokens: 2830000, cached_tokens: 890000, completion_tokens: 1216320, reasoning_tokens: 214000, total_tokens: 4046320, cost_usd: 790.8262 },
      },
    });
  }
  if (pathname === "/control/accounts/runtime") {
    return json(route, { accounts: accounts.map(({ id, status, status_reason, rate_limited_until, in_flight, waiting }) => ({ id, status, status_reason, rate_limited_until, in_flight, waiting })) });
  }
  if (pathname === "/control/accounts/test-runs/active") return json(route, { run: null });
  if (pathname === "/control/accounts/proxy-summary") {
    return json(route, { total: 3, bound: 2, unbound: 1, mixed: true, bindings: [{ proxy_id: 1, count: 1 }, { proxy_id: 2, count: 1 }] });
  }
  if (pathname === "/control/proxies") return json(route, { proxies });
  if (pathname === "/control/logs") return json(route, { logs });
  if (pathname === "/control/stats") return json(route, stats);
  if (pathname === "/control/models") return json(route, { models: ["gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.5-pro"], default_model: "gpt-5.6-sol", default_test_model: "gpt-5.6-sol", codex_default_model: "gpt-5.6-sol" });
  if (pathname === "/control/pricing") {
    return json(route, {
      price_version: "2026-07-16",
      source_url: "https://developers.openai.com/api/docs/pricing",
      tier: "standard",
      models: [
        { model: "gpt-5.6-sol", input_per_m: 5, cached_per_m: 0.5, output_per_m: 30, long_context_threshold: 272000, long_input_per_m: 10, long_cached_per_m: 1, long_output_per_m: 45 },
        { model: "gpt-5.6-terra", input_per_m: 2.5, cached_per_m: 0.25, output_per_m: 15 },
        { model: "gpt-5.5-pro", input_per_m: 15, output_per_m: 120 },
        { model: "gpt-5.5", input_per_m: 3.5, cached_per_m: 0.35, output_per_m: 24 },
        { model: "gpt-5.4", input_per_m: 2, cached_per_m: 0.2, output_per_m: 12 },
        { model: "gpt-5.3-codex", input_per_m: 1.5, cached_per_m: 0.15, output_per_m: 10 },
      ],
    });
  }
  if (pathname === "/control/codex/status") {
    return json(route, { config_path: "C:\\Users\\Demo\\.codex\\config.toml", auth_path: "C:\\Users\\Demo\\.codex\\auth.json", applied: true, config_exists: true, backup_exists: true, stale: false, base_url: "http://127.0.0.1:8080/v1", model: "gpt-5.6-sol", models: ["gpt-5.6-sol", "gpt-5.6-terra"], config_preview: 'model = "gpt-5.6-sol"', auth_preview: '{ "OPENAI_API_KEY": "sk-local-demo" }' });
  }
  if (pathname === "/control/codex/files") {
    return json(route, { config_path: "C:\\Users\\Demo\\.codex\\config.toml", auth_path: "C:\\Users\\Demo\\.codex\\auth.json", config_content: 'model = "gpt-5.6-sol"', auth_content: '{ "OPENAI_API_KEY": "sk-local-demo" }', config_default: 'model = "gpt-5.6-sol"', auth_default: '{ "OPENAI_API_KEY": "sk-local-demo" }' });
  }
  if (pathname === "/control/codex/remote/targets") {
    return json(route, { targets: [{ id: 1, name: "开发服务器（演示）", host: "dev.example.test", port: 22, user: "deploy", remote_port: 8080, model: "gpt-5.6-sol", mode: "tunnel", saved: true, injected: true, tunnel_enabled: true, tunnel_status: "connected", config_preview: "", auth_preview: "", updated_at: now }] });
  }
  if (pathname === "/control/codex/remote/test" && request.method() === "POST") {
    return json(route, {
      os: "Linux",
      home: "/home/demo",
      codex_dir: "/home/demo/.codex",
      host_key_fingerprint: "SHA256:n2B5GfGx7kVtQ4c8Yw1Rz6Pj9H3Lm0SaEdUiOqWbCFA",
      known: false,
    });
  }
  if (pathname === "/control/cloud/status") {
    return json(route, cloudAuthenticated
      ? { configured: true, authenticated: true, pending_verification: false, email: "demo.cloud@example.com", role: "user", last_sync_at: now, pending_items: 0, syncing: false, consecutive_failures: 0, conflicts: [] }
      : { configured: true, authenticated: false, pending_verification: false, turnstile_site_key: "1x00000000000000000000AA", pending_items: 0, syncing: false, consecutive_failures: 0, conflicts: [] });
  }
  if (pathname === "/control/cloud/workspace") return json(route, workspace);
  if (pathname === "/control/cloud/network") return json(route, { mode: "system", effective_source: "direct" });

  return json(route, {});
}

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 1,
  locale: "zh-CN",
  colorScheme: "light",
  reducedMotion: "reduce",
});
await context.addInitScript(() => {
  localStorage.setItem("s2a_control_port", "45678");
  localStorage.setItem("s2a_control_token", "documentation-fixture-token");
  localStorage.setItem("s2a_lang", "zh-CN");
  localStorage.setItem("s2a_theme", "light");
});
await context.route("http://127.0.0.1:45678/control/**", handleControlRoute);

const page = await context.newPage();

async function open(route) {
  await page.goto(`${baseURL}#/${route}`, { waitUntil: "domcontentloaded" });
  await page.locator(".page-title, .workspace-shell, .cloud-auth-shell").first().waitFor({ state: "visible" });
  await page.waitForTimeout(900);
}

async function shot(name) {
  await page.screenshot({ path: path.join(outputDir, name), fullPage: false });
  console.log(`captured ${name}`);
}

await open("dashboard");
await shot("dashboard.png");

await open("accounts");
await shot("accounts.png");
await page.locator('[data-test="account-import-open"]').click();
await page.locator('[data-test="import-method-json"]').waitFor({ state: "visible" });
await shot("import.png");
await page.locator(".import-chooser .modal-actions button").click();
await page.locator('[data-test="account-details"]').first().click();
await page.locator('[data-test="account-detail-modal"]').waitFor({ state: "visible" });
await shot("account-details.png");
await page.keyboard.press("Escape");

await open("proxies");
await shot("proxies.png");

await open("statistics");
await shot("statistics.png");

await open("models");
await shot("models.png");

await open("codex");
await shot("codex-local.png");
await page.locator('[data-test="tab-remote"]').click();
await page.locator('[data-test="remote-targets"]').waitFor({ state: "visible" });
await page.waitForTimeout(300);
await shot("codex.png");
await page.locator('[data-test="remote-host"]').fill("server.example.test");
await page.locator('[data-test="remote-user"]').fill("demo");
await page.locator('[data-test="remote-password"]').fill("documentation-only-password");
await page.locator('[data-test="remote-test"]').click();
await page.getByRole("dialog", { name: "确认服务器主机密钥" }).waitFor({ state: "visible" });
await shot("codex-host-key.png");
await page.getByRole("dialog", { name: "确认服务器主机密钥" }).getByRole("button", { name: "取消" }).click();

cloudAuthenticated = false;
await open("cloud");
await page.locator(".cloud-auth-tabs button").nth(1).click();
await page.waitForTimeout(300);
await shot("cloud-register.png");

cloudAuthenticated = true;
await page.reload({ waitUntil: "domcontentloaded" });
await page.locator(".metric-strip").waitFor({ state: "visible" });
await page.waitForTimeout(500);
await shot("cloud-workspace.png");

await open("settings");
await shot("settings.png");

await browser.close();
