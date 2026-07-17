import { expect, test } from "@playwright/test";

test("creates a one-time cloud share and manages revocation without exposing the upstream token", async ({ page }, testInfo) => {
  const guestKey = "sk-share-fixture-one-time-guest-key";
  const accountUID = "018f1f46-7a19-7cc2-88cb-f577e51d3999";
  let revoked = false;
  let createCalls = 0;
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  const share = () => ({
    id: 7,
    account_uid: accountUID,
    share_code: "AMBER567",
    quota_requests: 25,
    used_requests: 2,
    revoked,
    created_at: "2026-07-16T01:00:00Z",
    updated_at: "2026-07-16T01:00:00Z",
    base_url: "https://amber-cloud-api.example.workers.dev/v1",
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = {};
    if (path === "/control/status") body = { version: "0.3.1", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: "", account_count: 1, schema_version: 9 };
    if (path === "/control/accounts") body = { accounts: [{ id: 1, account_type: "api_key", base_url: "https://api.openai.com/v1", email: "API key account", chatgpt_account_id: "", plan_type: "api", expires_at: "", status: "active", consecutive_failures: 0, created_at: "2026-07-01T00:00:00Z", client_uid: accountUID }], usage: {} };
    if (path === "/control/proxies") body = { proxies: [] };
    if (path === "/control/settings") body = { account_strategy: "quota_aware", default_model: "gpt-5.6" };
    if (path === "/control/models") body = { models: ["gpt-5.6"], default_test_model: "gpt-5.6" };
    if (path === "/control/cloud/status") body = { configured: true, authenticated: true, pending_verification: false, email: "owner@example.test", role: "user", pending_items: 0, syncing: false, conflicts: [] };
    if (path === "/control/cloud/shares" && request.method() === "GET") body = { shares: createCalls ? [share()] : [] };
    if (path === "/control/cloud/shares" && request.method() === "POST") {
      const input = request.postDataJSON() as { consent: boolean; quota_requests: number };
      expect(input).toMatchObject({ consent: true, quota_requests: 25 });
      createCalls += 1;
      body = { share: share(), guest_key: guestKey };
    }
    if (path === "/control/cloud/shares/7" && request.method() === "PATCH") {
      revoked = Boolean((request.postDataJSON() as { revoked?: boolean }).revoked);
      body = { ...share(), revoked };
    }
    if (path === "/control/cloud/shares/7/usage") body = { usage: [] };
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/accounts");
  await page.locator('[data-test="account-share"]').click();
  await page.getByLabel("Request quota").fill("25");
  await page.getByLabel(/I confirm cloud custody/).check();
  await page.locator('[data-test="share-create"]').click();
  await expect(page.locator('[data-test="share-guest-key"]')).toHaveText(guestKey);
  const qr = page.locator('[data-test="share-qr"]');
  await expect(qr).toBeVisible();
  await expect.poll(async () => qr.evaluate((element) => {
    const canvas = element as HTMLCanvasElement;
    const pixels = canvas.getContext("2d")?.getImageData(0, 0, canvas.width, canvas.height).data;
    if (!pixels) return { width: canvas.width, height: canvas.height, hasDark: false, hasLight: false };
    let hasDark = false;
    let hasLight = false;
    for (let index = 0; index < pixels.length; index += 4) {
      const red = pixels[index];
      const green = pixels[index + 1];
      const blue = pixels[index + 2];
      const alpha = pixels[index + 3];
      if (alpha > 0 && red < 64 && green < 64 && blue < 64) hasDark = true;
      if (alpha > 0 && red > 240 && green > 240 && blue > 240) hasLight = true;
      if (hasDark && hasLight) break;
    }
    return { width: canvas.width, height: canvas.height, hasDark, hasLight };
  })).toEqual({ width: 164, height: 164, hasDark: true, hasLight: true });
  await expect(page.locator("body")).not.toContainText("upstream-owner-token");
  expect(await page.evaluate(() => Object.values(localStorage))).not.toContain(guestKey);
  await page.screenshot({ path: `test-results/amber-share-${testInfo.project.name}.png`, fullPage: true });

  await page.getByRole("button", { name: "Close" }).click();
  await page.locator('[data-test="account-share"]').click();
  await expect(page.locator('[data-test="share-guest-key"]')).toHaveCount(0);
  await page.getByRole("button", { name: "Revoke" }).click();
  expect(revoked).toBe(true);
  await expect(page.getByText("Revoked", { exact: true })).toBeVisible();
});

test("blocks OAuth cloud sharing until owner-device relay is available", async ({ page }) => {
  const accountUID = "018f1f46-7a19-7cc2-88cb-f577e51d3999";
  let createCalls = 0;
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = {};
    if (path === "/control/status") body = { version: "0.3.1", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: "", account_count: 1, schema_version: 9 };
    if (path === "/control/accounts") body = { accounts: [{ id: 1, account_type: "oauth", base_url: "", email: "owner@example.test", chatgpt_account_id: "acct-owner", plan_type: "plus", expires_at: "2026-08-01T00:00:00Z", status: "active", consecutive_failures: 0, created_at: "2026-07-01T00:00:00Z", client_uid: accountUID }], usage: {} };
    if (path === "/control/proxies") body = { proxies: [] };
    if (path === "/control/settings") body = { account_strategy: "quota_aware", default_model: "gpt-5.6" };
    if (path === "/control/models") body = { models: ["gpt-5.6"], default_test_model: "gpt-5.6" };
    if (path === "/control/cloud/status") body = { configured: true, authenticated: true, pending_verification: false, email: "owner@example.test", role: "user", pending_items: 0, syncing: false, conflicts: [] };
    if (path === "/control/cloud/shares" && request.method() === "GET") body = { shares: [] };
    if (path === "/control/cloud/shares" && request.method() === "POST") createCalls += 1;
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/accounts");
  await page.locator('[data-test="account-share"]').click();
  await expect(page.getByText("OAuth accounts require owner-device relay; direct cloud sharing is unavailable in v0.3.1.")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Share account through Amber Cloud" })).toBeVisible();
  await expect(page.locator('[data-test="share-create"]')).toHaveCount(0);
  expect(createCalls).toBe(0);
});
