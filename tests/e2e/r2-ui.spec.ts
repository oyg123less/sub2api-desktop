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
  const now = new Date();
  const localToday = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
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
        summary: { total_requests: 4, eligible_requests: 4, success_requests: 4, failed_requests: 0, client_cancelled: 0, total_tokens: 1000, prompt_tokens: 700, cached_tokens: 200, completion_tokens: 300, reasoning_tokens: 0, estimated_requests: 0, avg_latency_ms: 120, cost_usd: 3015.4344, pricing_fallback_requests: 0 },
        daily: [{ date: localToday, requests: 4, total_tokens: 1000, cost_usd: 1842.6159 }],
        by_model: [{ model: "gpt-5.6-sol", requests: 4, total_tokens: 1000, cost_usd: 3015.4344 }], failure_breakdown: [], retention: { days: 30, max_rows: 100000, retained_rows: 4 },
        pricing: { version: "2026-07-16", source_url: "https://developers.openai.com/api/docs/pricing", tier: "standard", currency: "USD" },
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

test("keeps Settings in the sidebar footer and LAN access only on the dashboard", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/dashboard");
  await expect(page.locator('.nav a[href="#/settings"]')).toHaveCount(0);
  await expect(page.locator('.sidebar-footer a[href="#/settings"]')).toBeVisible();
  await expect(page.locator(".lan-control")).toHaveCount(1);
  await page.locator('.sidebar-footer a[href="#/settings"]').click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  await expect(page.getByText("Allow LAN access", { exact: true })).toHaveCount(0);
});

test("shows estimated cost on the dashboard and statistics page", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/dashboard");
  await expect(page.getByText("Estimated cost today")).toBeVisible();
  await expect(page.getByText("$1.8426K")).toBeVisible();
  await expect(page.locator('[data-test="dashboard-estimated-cost"]')).toHaveAttribute("title", /\$1,842\.6159/);
  const dashboardCostFont = await page.locator('[data-test="dashboard-estimated-cost"]').evaluate((element) => getComputedStyle(element).fontFamily);
  const dashboardMetricFont = await page.locator(".dashboard-stats .stat-value").first().evaluate((element) => getComputedStyle(element).fontFamily);
  expect(dashboardCostFont).toBe(dashboardMetricFont);
  await page.locator(".dashboard-stats .stat").first().hover();
  await expect.poll(() => page.locator(".dashboard-stats .stat").first().evaluate((element) => getComputedStyle(element).transform)).not.toBe("none");
  await page.goto("/#/statistics");
  await expect(page.getByText("$3.0154K").first()).toBeVisible();
  await expect(page.locator('[data-test="statistics-estimated-cost"]')).toHaveAttribute("title", /\$3,015\.4344/);
  const statisticsCostFont = await page.locator('[data-test="statistics-estimated-cost"]').evaluate((element) => getComputedStyle(element).fontFamily);
  const statisticsMetricFont = await page.locator(".statistics-metrics .stat-value").first().evaluate((element) => getComputedStyle(element).fontFamily);
  expect(statisticsCostFont).toBe(statisticsMetricFont);
  await page.locator(".statistics-metrics .stat").first().hover();
  await expect.poll(() => page.locator(".statistics-metrics .stat").first().evaluate((element) => getComputedStyle(element).transform)).not.toBe("none");
  await expect(page.getByText("price version 2026-07-16", { exact: false })).toBeVisible();
});

test("renders the model plaza from the pricing control API", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/models");
  await expect(page.getByRole("heading", { name: "Model plaza" })).toBeVisible();
  await expect(page.getByText("gpt-5.6-sol")).toBeVisible();
  await expect(page.getByText("$30.00")).toBeVisible();
  await expect(page.getByText("2026-07-16", { exact: false })).toBeVisible();
});

test("organizes the user guide as expandable illustrated chapters", async ({ page }) => {
  await initialize(page);
  await page.goto("/#/docs");

  const chapters = page.locator("details.doc-section");
  await expect(chapters).toHaveCount(10);
  await expect(chapters.first()).toHaveAttribute("open", "");
  await expect(chapters.nth(1)).not.toHaveAttribute("open", "");

  const bodyFontSize = await chapters.first().locator(".section-copy > p").first().evaluate((element) => parseFloat(getComputedStyle(element).fontSize));
  expect(bodyFontSize).toBeGreaterThanOrEqual(15);
  const imageRatio = await chapters.first().evaluate((chapter) => {
    const content = chapter.querySelector(".section-content")?.getBoundingClientRect();
    const image = chapter.querySelector(".doc-image-button")?.getBoundingClientRect();
    return content && image ? image.width / content.width : 0;
  });
  expect(imageRatio).toBeGreaterThan(0.9);

  await chapters.first().locator(".doc-image-button").click();
  await expect(page.getByRole("dialog", { name: "Dashboard: service state, daily metrics, Base URL, and local API key" })).toBeVisible();
  await expect(page.locator(".doc-lightbox-content > img")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator(".doc-lightbox")).toHaveCount(0);

  await chapters.nth(1).locator("summary").click();
  await expect(chapters.nth(1)).toHaveAttribute("open", "");
  await expect(chapters.nth(1).locator('img[src*="accounts"]')).toBeVisible();
  await expect(chapters.nth(1).locator('img[src*="import"]')).toBeVisible();

  await chapters.nth(7).locator("summary").click();
  await expect(chapters.nth(7).getByText("Click Test connection", { exact: true })).toBeVisible();
  await expect(chapters.nth(7).getByText("Click Trust and continue in local Amber", { exact: true })).toBeVisible();
  await expect(chapters.nth(7).locator(".doc-procedure li")).toHaveCount(6);
  await expect(chapters.nth(7).locator(".doc-image-button")).toHaveCount(3);

  await page.getByRole("button", { name: "Expand all" }).click();
  await expect(page.locator("details.doc-section[open]")).toHaveCount(10);
  await page.getByRole("button", { name: "Collapse all" }).click();
  await expect(page.locator("details.doc-section[open]")).toHaveCount(0);

  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});
