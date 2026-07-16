import { expect, test } from "@playwright/test";

const localStatus = {
  version: "0.3.1",
  server_running: true,
  port: 8080,
  host: "127.0.0.1",
  endpoint: "http://127.0.0.1:8080/v1",
  lan_addresses: [],
  local_api_key: "",
  account_count: 0,
  schema_version: 7,
};

async function initialize(page: import("@playwright/test").Page) {
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
}

test("shows offline-safe state when Amber Cloud is not configured", async ({ page }) => {
  await initialize(page);
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const body = path === "/control/cloud/status"
      ? { configured: false, authenticated: false, pending_verification: false, pending_items: 0, syncing: false, conflicts: [] }
      : localStatus;
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto("/#/cloud");
  await expect(page.getByRole("heading", { name: "Cloud account" })).toBeVisible();
  await expect(page.getByText("Amber Cloud is not configured in this build")).toBeVisible();
  await expect(page.getByText("Local accounts, proxies, and Codex setup remain fully available offline.")).toBeVisible();
});

test("resends verification with a countdown and returns to registration", async ({ page }) => {
  await initialize(page);
  let pending = true;
  let resendCalls = 0;
  const cloudStatus = () => ({
    configured: true,
    authenticated: false,
    pending_verification: pending,
    email: pending ? "pending@example.test" : undefined,
    turnstile_site_key: "",
    pending_items: 0,
    syncing: false,
    conflicts: [],
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = localStatus;
    if (path === "/control/cloud/status") body = cloudStatus();
    if (path === "/control/cloud/resend-verification") {
      expect(request.postDataJSON()).toEqual({ email: "pending@example.test" });
      resendCalls += 1;
      body = cloudStatus();
    }
    if (path === "/control/cloud/cancel-registration") {
      pending = false;
      body = cloudStatus();
    }
    await route.fulfill({ status: path === "/control/cloud/resend-verification" ? 202 : 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/cloud");
  await expect(page.getByText("Enter the six-digit code sent to pending@example.test.")).toBeVisible();
  const resend = page.locator('[data-test="cloud-resend-verification"]');
  await resend.click();
  expect(resendCalls).toBe(1);
  await expect(resend).toBeDisabled();
  await expect(resend).toContainText(/Resend in (59|60)s/);
  await page.locator('[data-test="cloud-cancel-registration"]').click();
  await expect(page.getByRole("heading", { name: "Create a cloud account" })).toBeVisible();
});

test("logs in, synchronizes, and logs out without exposing session credentials", async ({ page }) => {
  await initialize(page);
  let authenticated = false;
  let syncCalls = 0;
  const cloudStatus = () => ({
    configured: true,
    authenticated,
    pending_verification: false,
    email: authenticated ? "owner@example.test" : undefined,
    role: authenticated ? "user" : undefined,
    turnstile_site_key: "",
    last_sync_at: authenticated && syncCalls ? "2026-07-16T01:00:00Z" : undefined,
    pending_items: authenticated ? 2 : 0,
    syncing: false,
    conflicts: [],
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = localStatus;
    if (path === "/control/cloud/status") body = cloudStatus();
    if (path === "/control/cloud/login") {
      authenticated = true;
      body = cloudStatus();
    }
    if (path === "/control/cloud/sync") {
      syncCalls += 1;
      body = { ...cloudStatus(), pending_items: 0 };
    }
    if (path === "/control/cloud/logout") {
      authenticated = false;
      body = cloudStatus();
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/cloud");
  await page.locator('[data-test="cloud-email"]').fill("owner@example.test");
  await page.locator('[data-test="cloud-password"]').fill("fixture-master-password");
  await page.locator('[data-test="cloud-login"]').click();
  await expect(page.getByText("owner@example.test")).toBeVisible();
  await expect(page.locator('[data-test="cloud-sync"]')).toBeVisible();
  await expect(page.locator('[data-test="cloud-admin-open"]')).toHaveCount(0);
  expect(syncCalls).toBe(1);
  await expect(page.locator("body")).not.toContainText("fixture-master-password");

  await page.locator('[data-test="cloud-logout"]').click();
  await expect(page.locator('[data-test="cloud-login"]')).toBeVisible();
  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});

test("keeps administrator governance behind a transient second factor", async ({ page }) => {
  await initialize(page);
  const adminKey = "fixture-transient-admin-key";
  let registrationEnabled = true;
  let capturedAdminKey = "";
  const cloudStatus = {
    configured: true,
    authenticated: true,
    pending_verification: false,
    email: "admin@example.test",
    role: "admin",
    turnstile_site_key: "",
    pending_items: 0,
    syncing: false,
    conflicts: [],
  };
  const overview = () => ({
    users: [
      { id: 1, email: "admin@example.test", role: "admin", email_verified: 1, banned: 0, created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T00:00:00Z", last_active_at: "2026-07-16T01:00:00Z", vault_count: 1 },
      { id: 2, email: "member@example.test", role: "user", email_verified: 1, banned: 0, created_at: "2026-07-02T00:00:00Z", updated_at: "2026-07-02T00:00:00Z", last_active_at: "", vault_count: 3 },
    ],
    settings: [
      { key: "registration_enabled", value: String(registrationEnabled), updated_at: "2026-07-16T00:00:00Z" },
      { key: "invite_mode", value: "false", updated_at: "2026-07-16T00:00:00Z" },
    ],
    stats: { users: 2, daily_active_users: 1, vault_items: 4 },
    audit: [{ id: 1, actor_user_id: 1, action: "user.logout_all", target_type: "user", target_id: "2", details: "{}", created_at: "2026-07-16T01:00:00Z" }],
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = localStatus;
    if (path === "/control/cloud/status") body = cloudStatus;
    if (path.startsWith("/control/cloud/admin/")) {
      const payload = request.postDataJSON() as { admin_key?: string; registration_enabled?: boolean };
      capturedAdminKey = payload.admin_key || "";
      if (path === "/control/cloud/admin/settings" && typeof payload.registration_enabled === "boolean") {
        registrationEnabled = payload.registration_enabled;
        body = { ok: true };
      } else if (path === "/control/cloud/admin/overview") {
        body = overview();
      } else {
        body = { ok: true };
      }
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/cloud");
  await page.locator('[data-test="cloud-admin-open"]').click();
  await page.locator('[data-test="cloud-admin-key"]').fill(adminKey);
  await page.locator('[data-test="cloud-admin-unlock"]').click();
  await expect(page.getByText("End-to-end encryption boundary:")).toBeVisible();
  await expect(page.getByText("member@example.test")).toBeVisible();
  expect(capturedAdminKey).toBe(adminKey);
  await expect(page.locator("body")).not.toContainText(adminKey);
  expect(await page.evaluate(() => Object.values(localStorage))).not.toContain(adminKey);

  await page.getByRole("tab", { name: "Platform settings" }).click();
  await page.getByLabel("Open registration").uncheck();
  await page.getByRole("button", { name: "Save" }).click();
  expect(registrationEnabled).toBe(false);

  await page.getByRole("button", { name: "Lock admin area" }).click();
  await expect(page.locator('[data-test="cloud-admin-panel"]')).toHaveCount(0);
});
