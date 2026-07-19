import { expect, test, type Page, type Route } from "@playwright/test";

const controlOrigin = "http://127.0.0.1:45678";
const localStatus = { version: "0.4.3", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: "", account_count: 2, schema_version: 14 };
const cloudStatus = { configured: true, authenticated: true, pending_verification: false, email: "owner@example.test", role: "user", pending_items: 0, syncing: false, consecutive_failures: 0, conflicts: [] };
const profile = { display_name: "Owner", friend_code: "AMB-OWNR-0001", encryption_public_key: "fixture", encryption_key_version: 1, created_at: "", updated_at: "" };
const accounts = [
  { id: 1, account_type: "oauth", base_url: "", email: "oauth@example.test", chatgpt_account_id: "acct-oauth", plan_type: "plus", expires_at: "", status: "active", consecutive_failures: 0, max_concurrency: 3, queue_capacity: 20, in_flight: 0, waiting: 0, created_at: "", client_uid: "018f1f46-7a19-7cc2-88cb-f577e51d3999" },
  { id: 2, account_type: "api_key", base_url: "https://api.example.test/v1", email: "API account", chatgpt_account_id: "", plan_type: "api", expires_at: "", status: "active", consecutive_failures: 0, max_concurrency: 3, queue_capacity: 20, in_flight: 0, waiting: 0, created_at: "", client_uid: "018f1f46-7a19-7cc2-88cb-f577e51d4000" },
] as const;

async function initialize(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
}

async function fulfill(route: Route, body: unknown, status = 200) {
  await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

function workspace(host: Record<string, unknown>, received: unknown[] = []) {
  return {
    profile, friends: { friends: [] }, friend_requests: { requests: [] }, share_groups: { groups: [] },
    received_shares: { shares: received }, devices: { devices: [], relay_enabled: false }, connect_host: host,
  };
}

test("starts quick sharing and connects with only a code and temporary password", async ({ page }) => {
  await initialize(page);
  let host: Record<string, any> = { configured: false, accounts: [], recipients: [] };
  const claimedShare = {
    public_id: "sgr_quick_fixture", status: "active",
    group: { public_id: "grp_quick", name: "Quick share", description: "", status: "active", route_policy: "balanced", account_count: 2, owner_device_required: true },
    owner: { display_name: "Remote owner" }, rpm_limit: 30, concurrency_limit: 2, quota_requests: 0, used_requests: 0,
    created_at: "2026-07-19T00:00:00Z", accepted_at: "2026-07-19T00:00:00Z", base_url: "https://cloud.example.test/v1",
    key: { public_id: "sak_quick", key_prefix: "sk-amber-quick", key_version: 1, status: "active" }, local_enabled: true,
  };
  await page.route(`${controlOrigin}/control/**`, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/control/status") return fulfill(route, localStatus);
    if (path === "/control/cloud/status") return fulfill(route, cloudStatus);
    if (path === "/control/accounts") return fulfill(route, { accounts, usage: {} });
    if (path === "/control/cloud/workspace") return fulfill(route, workspace(host));
    if (path === "/control/cloud/connect/host/accounts") {
      const payload = request.postDataJSON() as { accounts: Array<{ account_id: number }> };
      expect(payload.accounts.map((item) => item.account_id)).toEqual([1, 2]);
      host = {
        configured: true,
        endpoint: { public_id: "conn_quick", connection_code: "572814639", status: "paused", group_status: "paused", base_url: "https://cloud.example.test/v1", created_at: "", updated_at: "" },
        window: null, temporary_password: "", recipients: [],
        accounts: accounts.map((account, index) => ({ public_id: `sga_${index}`, account_uid: account.client_uid, account_type: account.account_type, relay_mode: "owner_device", enabled: true })),
      };
      return fulfill(route, host);
    }
    if (path === "/control/cloud/connect/host/start") {
      expect(request.postDataJSON()).toMatchObject({ max_claims: 1, duration_minutes: 30 });
      host = {
        ...host,
        endpoint: { ...host.endpoint, status: "active", group_status: "active" },
        window: { public_id: "win_quick", password_version: 1, max_claims: 1, claimed_count: 0, expires_at: "2026-07-20T00:00:00Z", created_at: "" },
        temporary_password: "AB3D5F",
      };
      return fulfill(route, { host, temporary_password: "AB3D5F", password_version: 1, expires_at: "2026-07-20T00:00:00Z" });
    }
    if (path === "/control/cloud/connect/claim-and-use") {
      expect(request.postDataJSON()).toMatchObject({ connection_code: "572814639", password: "AB3D5F" });
      return fulfill(route, claimedShare);
    }
    return fulfill(route, {});
  });

  await page.goto("/#/cloud");
  await expect(page.locator('[data-test="quick-share-host"]')).toBeVisible();
  await expect(page.locator('[data-test="quick-share-join"]')).toBeVisible();
  await page.getByRole("button", { name: "Select shared accounts" }).click();
  await page.getByRole("button", { name: "Save" }).click();
  await page.getByRole("button", { name: "Start sharing" }).click();
  await page.getByRole("button", { name: "Create password and start" }).click();
  await expect(page.getByText("572 814 639", { exact: true })).toBeVisible();
  await expect(page.getByText("AB3D5F", { exact: true })).toBeVisible();

  await page.locator('[data-test="connect-code"]').fill("572 814 639");
  await page.locator('[data-test="connect-password"]').fill("AB3D5F");
  await page.locator('[data-test="connect-and-use"]').click();
  await expect(page.getByRole("heading", { name: "Ready to use" })).toBeVisible();
  await expect(page.getByText("Connected to Remote owner's share.", { exact: false })).toBeVisible();

  const dimensions = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
});

test("shows an installer-oriented upgrade screen for rejected old clients", async ({ page }) => {
  await initialize(page);
  await page.route(`${controlOrigin}/control/**`, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/control/status") return fulfill(route, localStatus);
    if (path === "/control/cloud/status") return fulfill(route, cloudStatus);
    if (path === "/control/accounts") return fulfill(route, { accounts: [], usage: {} });
    if (path === "/control/cloud/workspace") return fulfill(route, {
      error: { code: "client_upgrade_required", message: "Update Amber", retryable: false, details: { minimum_version: "0.4.3", latest_version: "0.4.3", update_url: "https://github.com/oyg123less/sub2api-desktop/releases/latest" } },
    }, 426);
    return fulfill(route, {});
  });
  await page.goto("/#/cloud");
  await expect(page.locator('[data-test="cloud-upgrade-required"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "Update Amber to use Cloud account" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download and install latest version" })).toBeVisible();
  await expect(page.getByText("preserves existing accounts and settings", { exact: false })).toBeVisible();
});
