import { expect, test, type Page } from "@playwright/test";

type FixtureAccount = {
  id: number;
  account_type: "oauth" | "api_key";
  base_url: string;
  email: string;
  chatgpt_account_id: string;
  plan_type: string;
  expires_at: string;
  status: "active" | "disabled";
  status_reason?: string;
  consecutive_failures: number;
  max_concurrency: number;
  queue_capacity: number;
  in_flight: number;
  waiting: number;
  created_at: string;
  client_uid: string;
  source?: "cloud_share";
  cloud_grant_id?: string;
  cloud_owner_name?: string;
  cloud_group_name?: string;
  cloud_local_enabled?: boolean;
};

function account(id: number, email: string): FixtureAccount {
  return {
    id,
    account_type: "api_key",
    base_url: "https://api.openai.com/v1",
    email,
    chatgpt_account_id: "",
    plan_type: "api",
    expires_at: "",
    status: "active",
    consecutive_failures: 0,
    max_concurrency: 3,
    queue_capacity: 20,
    in_flight: id === 1 ? 1 : 0,
    waiting: id === 1 ? 2 : 0,
    created_at: "2026-07-17T00:00:00Z",
    client_uid: `account-${id}`,
  };
}

async function initialize(page: Page, accounts: FixtureAccount[], hooks: {
  importBody?: (body: string) => void;
  importHeaders?: (headers: Record<string, string>) => void;
  limits?: (body: { max_concurrency: number; queue_capacity: number }) => void;
  status?: (body: { status: string }) => void;
  oauthStart?: (body: { proxy_id: number | null }) => void;
  runtime?: () => { accounts: Array<Pick<FixtureAccount, "id" | "status" | "in_flight" | "waiting"> & { status_reason?: string; rate_limited_until?: string | null }> };
  testResult?: { ok: boolean; status: number; error?: string; model: string; prompt_tokens: number; completion_tokens: number; total_tokens: number; latency_ms: number; account_status: string };
  proxies?: Array<{ id: number; name: string; type: "http" | "https" | "socks5"; host: string; port: number; created_at: string }>;
  cloudStatus?: () => { configured: boolean; authenticated: boolean; pending_verification: boolean; pending_items: number; syncing: boolean; conflicts: unknown[] };
  cloudSharesError?: string;
  batchDelete?: (ids: number[]) => void;
  accountTestRun?: (method: string, path: string) => unknown;
  accountTestPath?: (path: string) => void;
} = {}) {
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = {};
    let responseStatus = 200;
    if (path === "/control/status") body = { version: "0.3.3", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: "", account_count: accounts.length, schema_version: 11 };
    if (path === "/control/accounts" && request.method() === "GET") body = {
      accounts,
      usage: { 1: { account_id: 1, requests: 12, prompt_tokens: 800, cached_tokens: 300, completion_tokens: 200, reasoning_tokens: 40, total_tokens: 1040, cost_usd: 0.1234 } },
    };
    if (path === "/control/accounts/runtime") body = hooks.runtime?.() ?? {
      accounts: accounts.map(({ id, status, status_reason, in_flight, waiting }) => ({ id, status, status_reason, in_flight, waiting })),
    };
    if (path === "/control/proxies") body = { proxies: hooks.proxies ?? [] };
    if (path === "/control/settings") body = { account_strategy: "quota_aware", default_model: "gpt-5.6" };
    if (path === "/control/models") body = { models: ["gpt-5.6"], default_test_model: "gpt-5.6" };
    if (path === "/control/cloud/status") body = hooks.cloudStatus?.() ?? { configured: true, authenticated: false, pending_verification: false, pending_items: 0, syncing: false, conflicts: [] };
    if (path === "/control/cloud/shares" && hooks.cloudSharesError) {
      responseStatus = 401;
      body = { error: { code: "invalid_refresh_token", message: hooks.cloudSharesError } };
    }
    if (path === "/control/accounts/batch-delete") {
      const input = request.postDataJSON() as { account_ids: number[] };
      hooks.batchDelete?.(input.account_ids);
      body = { ok: true, requested: input.account_ids.length, deleted: input.account_ids, missing: null, failed: null };
    }
    if (path === "/control/accounts/test-runs/active") body = hooks.accountTestRun?.(request.method(), path) ?? { run: null };
    if (path === "/control/accounts/test-runs" && request.method() === "POST") body = hooks.accountTestRun?.(request.method(), path) ?? {};
    if (/^\/control\/accounts\/test-runs\/account-test-/.test(path)) body = hooks.accountTestRun?.(request.method(), path) ?? {};
    if (path === "/control/accounts/import/preview") {
      hooks.importBody?.(request.postData() || "");
      hooks.importHeaders?.(request.headers());
      const previewProxyID = Number(request.headers()["x-import-proxy-id"] || 0) || undefined;
      const previewProxySpecified = request.headers()["x-import-proxy-mode"] === "override";
      body = {
        content_sha256: "preview-sha",
        summary: { create: 2, update: 0, skip: 0, error: 0, conflict: 0 },
        rows: [
          { index: 1, action: "create", account_type: "oauth", email_masked: "a***@example.test", has_access_token: true, has_refresh_token: false, has_id_token: false, has_api_key: false, warning_codes: [], warnings: [], proxy_id: previewProxyID, proxy_specified: previewProxySpecified },
          { index: 2, action: "create", account_type: "api_key", email_masked: "API endpoint", has_access_token: false, has_refresh_token: false, has_id_token: false, has_api_key: true, warning_codes: [], warnings: [], proxy_id: previewProxyID, proxy_specified: previewProxySpecified },
        ],
      };
    }
    if (path === "/control/accounts/import/commit") body = { imported: 1, updated: 0, skipped: 0, failed: 0, validated: 0, warnings: [], rows: [], summary: { total: 1, create: 1, update: 0, skip: 0, error: 0, conflict: 0 } };
    if (path === "/control/oauth/start") {
      hooks.oauthStart?.(request.postDataJSON() as { proxy_id: number | null });
      body = { auth_url: "https://auth.example.test/authorize", state: "oauth-fixture" };
    }
    if (path === "/control/oauth/poll") body = { done: false };
    if (/^\/control\/accounts\/-?\d+\/test$/.test(path)) {
      hooks.accountTestPath?.(path);
      body = hooks.testResult ?? {
      ok: true, status: 200, model: "gpt-5.6", prompt_tokens: 4, completion_tokens: 2, total_tokens: 6, latency_ms: 80, account_status: "active",
      };
    }
    const limitsMatch = path.match(/^\/control\/accounts\/(\d+)\/limits$/);
    if (limitsMatch && request.method() === "PUT") {
      const input = request.postDataJSON() as { max_concurrency: number; queue_capacity: number };
      hooks.limits?.(input);
      const target = accounts.find((item) => item.id === Number(limitsMatch[1]))!;
      target.max_concurrency = input.max_concurrency;
      target.queue_capacity = input.queue_capacity;
      body = { ok: true, account: target };
    }
    const statusMatch = path.match(/^\/control\/accounts\/(\d+)\/status$/);
    if (statusMatch && request.method() === "POST") {
      const input = request.postDataJSON() as { status: string };
      hooks.status?.(input);
      const target = accounts.find((item) => item.id === Number(statusMatch[1]))!;
      target.status = input.status as "active" | "disabled";
      target.status_reason = target.status === "disabled" ? "manually_disabled" : undefined;
      body = { ok: true, account: target };
    }
    await route.fulfill({ status: responseStatus, contentType: "application/json", body: JSON.stringify(body) });
  });
}

test("uses one import entry and previews multiple JSON files as one batch", async ({ page }) => {
  let importBody = "";
  await initialize(page, [account(1, "owner@example.test")], { importBody: (body) => { importBody = body; } });
  await page.goto("/#/accounts");

  await expect(page.locator('[data-test="account-import-open"]')).toHaveCount(1);
  await page.locator('[data-test="account-import-open"]').click();
  await expect(page.locator('[data-test="import-method-api"]')).toBeVisible();
  await expect(page.locator('[data-test="import-method-oauth"]')).toBeVisible();
  await page.locator('[data-test="import-method-json"]').click();

  const input = page.locator('input[type="file"][multiple]');
  await input.setInputFiles([
    { name: "oauth.json", mimeType: "application/json", buffer: Buffer.from('{"email":"a@example.test","access_token":"token-a"}') },
    { name: "api.json", mimeType: "application/json", buffer: Buffer.from('[{"account_type":"api_key","base_url":"https://api.example.test/v1","api_key":"sk-test"}]') },
  ]);
  await expect(page.getByText("2 files selected,", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Preview" }).click();
  await expect(page.getByText("Create 2", { exact: true })).toBeVisible();
  expect(importBody).toContain('"token-a"');
  expect(importBody).toContain('"sk-test"');
});

test("selects all accounts and submits one transactional batch deletion", async ({ page }) => {
  let deletedIDs: number[] = [];
  await initialize(page, [account(1, "owner@example.test"), account(2, "backup@example.test")], {
    batchDelete: (ids) => { deletedIDs = ids; },
  });
  await page.goto("/#/accounts");
  await page.locator(".batch-select-all input").check();
  await expect(page.getByText("2 accounts selected")).toBeVisible();
  await page.getByRole("button", { name: "Delete selected" }).click();
  await expect(page.getByText("Delete 2 selected accounts?")).toBeVisible();
  await page.getByRole("button", { name: "Delete selected" }).last().click();
  await expect.poll(() => deletedIDs).toEqual([1, 2]);
  await expect(page.getByText(/Cannot read properties of null/)).toHaveCount(0);
});

test("paginates accounts at 20 rows and selects only the current page", async ({ page }) => {
  const fixture = Array.from({ length: 21 }, (_, index) => account(index + 1, `account-${index + 1}@example.test`));
  await initialize(page, fixture);
  await page.goto("/#/accounts");

  await expect(page.locator('[data-test="account-row"]')).toHaveCount(20);
  await expect(page.getByText("Showing 1-20 of 21 accounts")).toBeVisible();
  await page.locator(".batch-select-all input").check();
  await expect(page.getByText("20 accounts selected")).toBeVisible();

  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.locator('[data-test="account-row"]')).toHaveCount(1);
  await expect(page.getByText("account-21@example.test", { exact: true })).toBeVisible();
  await expect(page.getByText("Showing 21-21 of 21 accounts")).toBeVisible();
  await expect(page.locator(".batch-select-all input")).not.toBeChecked();
  await expect(page.getByText("20 accounts selected")).toBeVisible();
});

test("runs selected accounts through one resumable batch test", async ({ page }) => {
  let polls = 0;
  const running = {
    run_id: "account-test-fixture", status: "running", model: "gpt-5.6", total: 2, completed: 0,
    succeeded: 0, failed: 0, cancelled: 0, skipped: 0, running: 1, started_at: "2026-07-18T08:00:00Z",
    results: [
      { account_id: 1, account_label: "o***@example.test", status: "running" },
      { account_id: 2, account_label: "b***@example.test", status: "pending" },
    ],
  };
  const completed = {
    ...running, status: "completed", completed: 2, succeeded: 1, failed: 1, running: 0,
    finished_at: "2026-07-18T08:00:02Z",
    results: [
      { account_id: 1, account_label: "o***@example.test", status: "succeeded", http_status: 200, latency_ms: 80 },
      { account_id: 2, account_label: "b***@example.test", status: "failed", http_status: 403, error_kind: "authentication", error: "Authentication failed" },
    ],
  };
  await initialize(page, [account(1, "owner@example.test"), account(2, "backup@example.test")], {
    accountTestRun: (method, path) => {
      if (path.endsWith("/active")) return { run: null };
      if (method === "POST") return running;
      polls += 1;
      return polls > 1 ? completed : running;
    },
  });
  await page.goto("/#/accounts");
  await page.locator(".batch-select-all input").check();
  await page.getByRole("button", { name: "Test selected" }).click();
  await page.locator('[data-test="account-batch-test-start"]').click();
  await expect(page.getByText("Test progress 2/2")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Passed 1")).toBeVisible();
  await expect(page.getByText("Failed 1")).toBeVisible();
  await expect(page.getByText("Authentication failed")).toHaveCount(0);
});

test("shows row accounts, edits limits, and toggles routing", async ({ page }, testInfo) => {
  const fixture = [account(1, "owner@example.test"), account(2, "backup@example.test")];
  let savedLimits: { max_concurrency: number; queue_capacity: number } | undefined;
  let savedStatus = "";
  await initialize(page, fixture, {
    limits: (body) => { savedLimits = body; },
    status: (body) => { savedStatus = body.status; },
  });
  await page.goto("/#/accounts");

  await expect(page.locator('[data-test="account-row"]')).toHaveCount(2);
  const firstAccount = page.locator('[data-test="account-row"]').first();
  for (const row of await page.locator('[data-test="account-row"]').all()) {
    const rowBounds = await row.boundingBox();
    const actionsBounds = await row.locator(".account-row-actions").boundingBox();
    expect(rowBounds).not.toBeNull();
    expect(actionsBounds).not.toBeNull();
    expect(actionsBounds!.x).toBeGreaterThanOrEqual(rowBounds!.x);
    expect(actionsBounds!.x + actionsBounds!.width).toBeLessThanOrEqual(rowBounds!.x + rowBounds!.width);
  }
  await firstAccount.hover();
  await expect.poll(() => firstAccount.evaluate((element) => getComputedStyle(element).transform)).not.toBe("none");
  await expect(page.locator('[data-test="account-detail-modal"]')).toHaveCount(0);
  await page.screenshot({ path: `test-results/accounts-v032-list-${testInfo.project.name}.png`, fullPage: true });
  await page.locator('[data-test="account-details"]').first().click();
  await expect(page.locator('[data-test="account-detail-modal"]')).toBeVisible();
  await expect(page.getByText("Detailed usage", { exact: true })).toBeVisible();
  await expect(page.getByText("Active 1 · waiting 2", { exact: true })).toBeVisible();
  await page.screenshot({ path: `test-results/accounts-v032-detail-${testInfo.project.name}.png`, fullPage: true });
  await page.getByLabel("Maximum concurrency").fill("7");
  await page.getByLabel("Waiting queue limit").fill("42");
  await page.getByRole("button", { name: "Save" }).click();
  expect(savedLimits).toEqual({ max_concurrency: 7, queue_capacity: 42 });
  await expect(page.getByRole("button", { name: "Save" })).toBeEnabled();
  const detailModal = page.locator('[data-test="account-detail-modal"]');
  const detailScroll = page.locator('[data-test="account-detail-scroll"]');
  const detailClose = page.locator('[data-test="account-detail-close"]');
  const closeTop = await detailClose.evaluate((element) => element.getBoundingClientRect().top);
  await detailScroll.evaluate((element) => { element.scrollTop = element.scrollHeight; });
  await expect.poll(() => detailClose.evaluate((element) => element.getBoundingClientRect().top)).toBeCloseTo(closeTop, 0);
  await expect(detailClose).toBeVisible();
  await expect(detailModal).toHaveCSS("overflow", "hidden");
  await expect(detailScroll).toHaveCSS("overflow-y", "auto");
  await detailClose.click();
  await expect(detailModal).toHaveCount(0);

  await page.locator(".account-switch").first().click();
  expect(savedStatus).toBe("disabled");
  await expect(page.getByText("Routing was disabled manually", { exact: true })).toBeVisible();

  const dimensions = await page.locator("body").evaluate((element) => ({ clientWidth: element.clientWidth, scrollWidth: element.scrollWidth }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});

test("polls live account load without overlap and pauses while hidden", async ({ page }) => {
  const fixture = [account(1, "owner@example.test")];
  fixture[0].in_flight = 0;
  fixture[0].waiting = 0;
  let runtimeCalls = 0;
  await initialize(page, fixture, {
    runtime: () => {
      runtimeCalls += 1;
      const inFlight = runtimeCalls === 2 || runtimeCalls === 3 ? 1 : 0;
      return { accounts: [{ id: 1, status: "active", in_flight: inFlight, waiting: 0 }] };
    },
  });
  await page.goto("/#/accounts");

  await expect(page.getByText("Active 0/3", { exact: true })).toBeVisible();
  await expect(page.getByText("Active 1/3", { exact: true })).toBeVisible({ timeout: 3_000 });
  await expect(page.getByText("Active 0/3", { exact: true })).toBeVisible({ timeout: 5_000 });

  await page.evaluate(() => {
    Object.defineProperty(document, "hidden", { configurable: true, value: true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.waitForTimeout(150);
  const callsWhileVisible = runtimeCalls;
  await page.waitForTimeout(1_250);
  expect(runtimeCalls).toBe(callsWhileVisible);

  await page.evaluate(() => {
    Object.defineProperty(document, "hidden", { configurable: true, value: false });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect.poll(() => runtimeCalls).toBeGreaterThan(callsWhileVisible);
});

test("keeps local accounts usable when an expired cloud session is cleared", async ({ page }) => {
  let statusCalls = 0;
  await initialize(page, [account(1, "owner@example.test")], {
    cloudStatus: () => ({
      configured: true,
      authenticated: statusCalls++ === 0,
      pending_verification: false,
      pending_items: 0,
      syncing: false,
      conflicts: [],
    }),
    cloudSharesError: "The session has expired.",
  });
  await page.goto("/#/accounts");

  await expect(page.locator('[data-test="account-row"]')).toHaveCount(1);
  await expect(page.getByText("The session has expired.", { exact: true })).toHaveCount(0);
  await expect(page.locator('[data-test="account-share"]')).toHaveCount(0);
});

test("tests from the account row and contains long upstream errors", async ({ page }) => {
  const longError = JSON.stringify({ error: { message: "X".repeat(2_000) } });
  await initialize(page, [account(1, "owner@example.test")], {
    testResult: { ok: false, status: 429, error: longError, model: "gpt-5.6", prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, latency_ms: 91, account_status: "rate_limited" },
  });
  await page.goto("/#/accounts");

  await expect(page.locator('[data-test="account-test"]')).toBeVisible();
  await page.locator('[data-test="account-test"]').click();
  await expect(page.locator('[data-test="account-test-modal"]')).toBeVisible();
  await page.getByRole("button", { name: "Run test" }).click();
  const modal = page.locator('[data-test="account-test-modal"]');
  const error = page.locator('[data-test="account-test-error"]');
  await expect(error).toContainText("XXXX");
  const bounds = await Promise.all([
    modal.evaluate((element) => element.getBoundingClientRect().toJSON()),
    error.evaluate((element) => element.getBoundingClientRect().toJSON()),
  ]);
  expect(bounds[1].left).toBeGreaterThanOrEqual(bounds[0].left);
  expect(bounds[1].right).toBeLessThanOrEqual(bounds[0].right);
  expect(await error.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole("button", { name: "Close" }).click();

  await page.locator('[data-test="account-details"]').click();
  await expect(page.locator('[data-test="account-detail-modal"]')).toBeVisible();
  await expect(page.locator('[data-test="account-detail-modal"] [data-test="account-test"]')).toHaveCount(0);
});

test("manages received cloud shares as canonical accounts without stale-key actions", async ({ page }) => {
  const managed = account(-7, "Shared workspace");
  managed.source = "cloud_share";
  managed.cloud_grant_id = "sgr_current";
  managed.cloud_owner_name = "Share owner";
  managed.cloud_group_name = "Shared workspace";
  managed.cloud_local_enabled = true;
  let testedPath = "";
  await initialize(page, [managed], { accountTestPath: (path) => { testedPath = path; } });
  await page.goto("/#/accounts");

  const row = page.locator('[data-test="account-row"]');
  await expect(row.getByText("Cloud share", { exact: true })).toBeVisible();
  await expect(row.getByText("Shared by Share owner", { exact: true })).toBeVisible();
  await expect(row.locator('[data-test="account-delete"]')).toHaveCount(0);
  await expect(row.locator('[data-test="cloud-share-manage"]')).toBeVisible();

  await row.locator('[data-test="account-test"]').click();
  await page.getByRole("button", { name: "Run test" }).click();
  await expect.poll(() => testedPath).toBe("/control/accounts/-7/test");
  await page.getByRole("button", { name: "Close" }).click();

  await page.locator(".batch-select-all input").check();
  await expect(page.getByRole("button", { name: "Delete selected" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Test selected" })).toBeEnabled();

  await row.locator('[data-test="account-details"]').click();
  await expect(page.getByText("Concurrency, quota, and expiry are managed by the owner.", { exact: false })).toBeVisible();
  await expect(page.getByLabel("Maximum concurrency")).toHaveCount(0);
});

test("explains revoked shared access without asking for ChatGPT re-login", async ({ page }) => {
  const stale = account(4, "Old shared key");
  stale.status = "disabled";
  stale.status_reason = "share_access_revoked";
  await initialize(page, [stale]);
  await page.goto("/#/accounts");

  await expect(page.getByText("This shared access has expired or been revoked; reconnect it from the Cloud account page", { exact: true })).toBeVisible();
  await expect(page.getByText("Re-login needed", { exact: true })).toHaveCount(0);
});

test("submits selected proxies for API, OAuth, and JSON imports", async ({ page }) => {
  let apiImportBody = "";
  let oauthProxyID: number | null | undefined;
  let importHeaders: Record<string, string> = {};
  const proxy = { id: 7, name: "Tokyo relay", type: "socks5" as const, host: "127.0.0.1", port: 1080, created_at: "2026-07-17T00:00:00Z" };
  await initialize(page, [account(1, "owner@example.test")], {
    proxies: [proxy],
    importBody: (body) => { apiImportBody = body; },
    importHeaders: (headers) => { importHeaders = headers; },
    oauthStart: (body) => { oauthProxyID = body.proxy_id; },
  });
  await page.goto("/#/accounts");

  await page.locator('[data-test="account-import-open"]').click();
  await page.locator('[data-test="import-method-api"]').click();
  await page.locator('[data-test="import-proxy-trigger"]').click();
  await page.getByText("Tokyo relay", { exact: true }).click();
  await page.locator('.modal input[type="password"]').fill("sk-fixture");
  await page.getByRole("button", { name: "Add" }).click();
  await expect.poll(() => JSON.parse(apiImportBody).at(0).proxy_id).toBe(7);

  await page.locator('[data-test="account-import-open"]').click();
  await page.locator('[data-test="import-method-oauth"]').click();
  const oauthModal = page.locator('[data-test="oauth-proxy-modal"]');
  await oauthModal.locator('[data-test="import-proxy-trigger"]').click();
  await oauthModal.getByText("Tokyo relay", { exact: true }).click();
  await page.locator('[data-test="oauth-proxy-continue"]').click();
  await expect.poll(() => oauthProxyID).toBe(7);
  await page.getByRole("button", { name: "Cancel" }).click();

  await page.locator('[data-test="account-import-open"]').click();
  await page.locator('[data-test="import-method-json"]').click();
  await page.locator('[data-test="import-proxy-mode"]').selectOption("override");
  await page.locator('.import-modal [data-test="import-proxy-trigger"]').click();
  await page.locator('.import-modal').getByText("Tokyo relay", { exact: true }).click();
  await page.locator(".import-textarea").fill('[{"account_type":"api_key","base_url":"https://api.example.test/v1","api_key":"sk-json"}]');
  await page.getByRole("button", { name: "Preview" }).click();
  expect(importHeaders["x-import-proxy-mode"]).toBe("override");
  expect(importHeaders["x-import-proxy-id"]).toBe("7");
  const previewProxies = page.locator(".import-table .import-proxy-value");
  await expect(previewProxies).toHaveCount(2);
  await expect(previewProxies).toHaveText(["Tokyo relay", "Tokyo relay"]);
});
