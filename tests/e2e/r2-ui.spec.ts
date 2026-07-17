import { expect, test, type Page } from "@playwright/test";

const baseSettings = {
  listen_port: 8080,
  allow_lan: false,
  local_api_key: "sk-local-fixture",
  inject_instructions: true,
  default_model: "gpt-5.6-sol",
  user_agent: "codex_cli_rs",
  originator: "codex_cli_rs",
  language: "en",
  auto_start_server: false,
  tls_fingerprint: false,
  codex_model: "gpt-5.6-sol",
  account_strategy: "quota_aware",
  log_retention_days: 30,
  max_log_rows: 100000,
  auto_recovery: true,
  compatibility_profile: "standard",
};

async function initialize(page: Page) {
  let serverRunning = true;
  const settings = { ...baseSettings };
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown;
    if (path === "/control/settings") {
      if (request.method() === "PUT") Object.assign(settings, request.postDataJSON());
      body = settings;
    } else if (path === "/control/server/stop") {
      serverRunning = false;
      body = { server_running: false };
    } else if (path === "/control/server/start") {
      serverRunning = true;
      body = { server_running: true, port: settings.listen_port };
    } else if (path === "/control/logs") {
      body = { logs: [] };
    } else if (path === "/control/stats") {
      body = {
        summary: { total_requests: 0, eligible_requests: 0, success_requests: 0, failed_requests: 0, client_cancelled: 0, total_tokens: 0, prompt_tokens: 0, cached_tokens: 0, completion_tokens: 0, reasoning_tokens: 0, estimated_requests: 0, avg_latency_ms: 0 },
        daily: [], by_model: [], failure_breakdown: [], retention: { days: 30, max_rows: 100000, retained_rows: 0 },
      };
    } else if (path === "/control/models") {
      body = { models: ["gpt-5.6-sol"], default_model: "gpt-5.6-sol", default_test_model: "gpt-5.6-sol", codex_default_model: "gpt-5.6-sol" };
    } else if (path === "/control/pricing") {
      body = {
        price_version: "2026-07-16",
        source_url: "https://developers.openai.com/api/docs/pricing",
        tier: "standard",
        models: [{ model: "gpt-5.6-sol", input_per_m: 5, cached_per_m: 0.5, output_per_m: 30, long_context_threshold: 272000, long_input_per_m: 10, long_cached_per_m: 1, long_output_per_m: 45 }],
      };
    } else {
      body = { version: "0.3.3", server_running: serverRunning, port: settings.listen_port, host: settings.allow_lan ? "0.0.0.0" : "127.0.0.1", endpoint: `http://${settings.allow_lan ? "0.0.0.0" : "127.0.0.1"}:${settings.listen_port}/v1`, lan_addresses: [], local_api_key: settings.local_api_key, account_count: 1, schema_version: 11 };
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
}

test("requires confirmation before enabling LAN access from the dashboard", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/dashboard");
  await page.locator(".lan-control .switch").click();
  await expect(page.getByText("Enable LAN access?")).toBeVisible();
  await page.getByRole("button", { name: "Confirm" }).click();
  await expect(page.getByText("Listening on 0.0.0.0:8080")).toBeVisible();
  await expect(page.locator(".lan-control input")).toBeChecked();
});

test("embeds diagnostics in the settings page", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/settings");
  await expect(page.locator('a[href="#/diagnostics"]')).toHaveCount(0);
  await page.locator(".settings-diagnostics .collapsible-trigger").click();
  await expect(page.locator("#diagnostics")).toBeVisible();
  await expect(page.getByRole("button", { name: "Run diagnostics" })).toBeVisible();
});

test("renders the model plaza from the pricing control API", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/models");
  await expect(page.getByRole("heading", { name: "Model plaza" })).toBeVisible();
  await expect(page.getByText("gpt-5.6-sol")).toBeVisible();
  await expect(page.getByText("$30.00")).toBeVisible();
  await expect(page.getByText("2026-07-16", { exact: false })).toBeVisible();
});
