import { expect, test, type Page, type Route } from "@playwright/test";

const controlOrigin = "http://127.0.0.1:45678";
const accountUIDs = [
  "018f1f46-7a19-7cc2-88cb-f577e51d3999",
  "018f1f46-7a19-7cc2-88cb-f577e51d4000",
];
const friendshipIDs = ["frn_018f1f46-7a19-7cc2-88cb-f577e51d4101", "frn_018f1f46-7a19-7cc2-88cb-f577e51d4102"];

const localStatus = {
  version: "0.4.0",
  server_running: true,
  port: 8080,
  host: "127.0.0.1",
  endpoint: "http://127.0.0.1:8080/v1",
  lan_addresses: [],
  local_api_key: "",
  account_count: 2,
  schema_version: 12,
};

const cloudStatus = {
  configured: true,
  authenticated: true,
  pending_verification: false,
  email: "owner@example.test",
  role: "user",
  pending_items: 0,
  syncing: false,
  conflicts: [],
};

const profile = {
  display_name: "Owner",
  friend_code: "AMB-OWNR-0001",
  encryption_public_key: "fixture-public-key",
  encryption_key_version: 1,
  created_at: "2026-07-18T00:00:00Z",
  updated_at: "2026-07-18T00:00:00Z",
};

const friends = friendshipIDs.map((public_id, index) => ({
  public_id,
  display_name: index ? "Alex" : "Lin",
  friend_code: index ? "AMB-ALEX-0002" : "AMB-LIN0-0001",
  encryption_public_key: `fixture-friend-key-${index}`,
  encryption_key_version: 1,
  created_at: "2026-07-18T00:00:00Z",
  updated_at: "2026-07-18T00:00:00Z",
}));

const accounts = [
  {
    id: 1, account_type: "oauth", base_url: "", email: "oauth@example.test", chatgpt_account_id: "acct-oauth",
    plan_type: "plus", expires_at: "2026-08-01T00:00:00Z", status: "active", consecutive_failures: 0,
    max_concurrency: 3, queue_capacity: 20, in_flight: 0, waiting: 0, created_at: "2026-07-01T00:00:00Z",
    client_uid: accountUIDs[0],
  },
  {
    id: 2, account_type: "api_key", base_url: "https://api.openai.com/v1", email: "API account", chatgpt_account_id: "",
    plan_type: "api", expires_at: "", status: "active", consecutive_failures: 0, max_concurrency: 3,
    queue_capacity: 20, in_flight: 0, waiting: 0, created_at: "2026-07-01T00:00:00Z", client_uid: accountUIDs[1],
  },
];

function groupDetails() {
  const group = {
    public_id: "shg_018f1f46-7a19-7cc2-88cb-f577e51d4200",
    name: "Team Pool",
    description: "Two-account fixture",
    status: "active",
    route_policy: "balanced",
    default_rpm: 30,
    default_concurrency: 2,
    default_quota_requests: 1000,
    account_count: 2,
    enabled_account_count: 2,
    recipient_count: 2,
    used_requests: 0,
    base_url: "https://amber-cloud-api.example.test/v1",
    created_at: "2026-07-18T00:00:00Z",
    updated_at: "2026-07-18T00:00:00Z",
  } as const;
  return {
    group,
    accounts: accounts.map((account, index) => ({
      public_id: `sga_018f1f46-7a19-7cc2-88cb-f577e51d42${index + 1}0`,
      account_uid: account.client_uid,
      account_type: account.account_type,
      relay_mode: index ? "worker_direct" : "owner_device",
      priority: 100,
      weight: 100,
      enabled: true,
      created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z",
    })),
    recipients: friends.map((friend, index) => ({
      public_id: `sgr_018f1f46-7a19-7cc2-88cb-f577e51d43${index + 1}0`,
      display_name: friend.display_name,
      friendship_id: friend.public_id,
      status: "pending",
      rpm_limit: 30,
      concurrency_limit: 2,
      quota_requests: 1000,
      used_requests: 0,
      reserved_requests: 0,
      key_prefix: `sk-amber-friend-${index}`,
      created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z",
    })),
  };
}

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

test("shows the v0.4.4 direct sharing flow without legacy friend or QR controls", async ({ page }) => {
  await initialize(page);
  await page.route(`${controlOrigin}/control/**`, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/control/status") return fulfill(route, localStatus);
    if (path === "/control/cloud/status") return fulfill(route, cloudStatus);
    if (path === "/control/cloud/profile") return fulfill(route, profile);
    if (path === "/control/cloud/workspace") return fulfill(route, {
      profile,
      friends: { friends },
      friend_requests: { requests: [] },
      share_groups: { groups: [groupDetails().group] },
      received_shares: { shares: [] },
      devices: { devices: [], relay_enabled: false },
      connect_host: { configured: false, accounts: [], recipients: [] },
    });
    if (path === "/control/cloud/received-shares") return fulfill(route, { shares: [] });
    if (path === "/control/cloud/connect/host") return fulfill(route, { configured: false, accounts: [], recipients: [] });
    if (path === "/control/cloud/connect/events") return fulfill(route, { cursor: 0, events: [], has_more: false });
    if (path === "/control/cloud/devices") return fulfill(route, { devices: [], relay_enabled: false });
    if (path === "/control/accounts") return fulfill(route, { accounts, usage: {} });
    return fulfill(route, {});
  });

  await page.goto("/#/cloud");
  await expect(page.locator('[data-test="quick-share-host"]')).toBeVisible();
  await expect(page.locator('[data-test="quick-share-join"]')).toBeVisible();
  await expect(page.locator('[data-test="connect-code"]')).toBeVisible();
  await expect(page.locator('[data-test="connect-password"]')).toBeVisible();
  await expect(page.locator('[data-test="cloud-tab-shares"]')).toHaveCount(0);
  await expect(page.locator('[data-test="cloud-tab-received"]')).toHaveCount(0);
  await expect(page.locator('[data-test="cloud-tab-friends"]')).toHaveCount(0);
  await expect(page.locator('[data-test="share-qr"]')).toHaveCount(0);
  await expect(page.locator('[data-test="add-friend-open"]')).toHaveCount(0);
});

test("keeps legacy received shares out of the primary v0.4.4 navigation", async ({ page }) => {
  await initialize(page);
  const receivedShare = {
    public_id: "sgr_018f1f46-7a19-7cc2-88cb-f577e51d4400",
    status: "pending",
    group: {
      public_id: "shg_018f1f46-7a19-7cc2-88cb-f577e51d4401",
      name: "Friend Pool",
      description: "Shared with you",
      status: "active",
      route_policy: "balanced",
      account_count: 2,
      owner_device_required: true,
    },
    owner: { display_name: "Lin" },
    rpm_limit: 30,
    concurrency_limit: 2,
    quota_requests: 100,
    used_requests: 1,
    created_at: "2026-07-18T00:00:00Z",
    base_url: "https://amber-cloud-api.example.test/v1",
  };
  await page.route(`${controlOrigin}/control/**`, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/control/status") return fulfill(route, localStatus);
    if (path === "/control/cloud/status") return fulfill(route, cloudStatus);
    if (path === "/control/cloud/profile") return fulfill(route, profile);
    if (path === "/control/cloud/workspace") return fulfill(route, {
      profile,
      friends: { friends: [] },
      friend_requests: { requests: [] },
      share_groups: { groups: [] },
      received_shares: { shares: [receivedShare] },
      devices: { devices: [], relay_enabled: false },
      connect_host: { configured: false, accounts: [], recipients: [] },
    });
    if (path === "/control/cloud/received-shares") return fulfill(route, { shares: [receivedShare] });
    if (path === "/control/cloud/connect/host") return fulfill(route, { configured: false, accounts: [], recipients: [] });
    if (path === "/control/cloud/connect/events") return fulfill(route, { cursor: 0, events: [], has_more: false });
    if (path === "/control/cloud/devices") return fulfill(route, { devices: [], relay_enabled: false });
    if (path === "/control/accounts") return fulfill(route, { accounts: [], usage: {} });
    return fulfill(route, {});
  });

  await page.goto("/#/cloud");
  await expect(page.locator('[data-test="quick-share-host"]')).toBeVisible();
  await expect(page.locator('[data-test="quick-share-join"]')).toBeVisible();
  await expect(page.locator('[data-test="cloud-tab-received"]')).toHaveCount(0);
  await expect(page.locator('[data-test="accept-received-share"]')).toHaveCount(0);
  await expect(page.locator('[data-test="received-share-row"]')).toHaveCount(0);
});

test("account details no longer expose legacy one-account sharing or QR controls", async ({ page }) => {
  await initialize(page);
  await page.route(`${controlOrigin}/control/**`, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/control/status") return fulfill(route, localStatus);
    if (path === "/control/accounts") return fulfill(route, { accounts: [accounts[0]], usage: {} });
    if (path === "/control/proxies") return fulfill(route, { proxies: [] });
    if (path === "/control/settings") return fulfill(route, { account_strategy: "quota_aware", default_model: "gpt-5.6" });
    if (path === "/control/models") return fulfill(route, { models: ["gpt-5.6"], default_test_model: "gpt-5.6" });
    return fulfill(route, {});
  });
  await page.goto("/#/accounts");
  await page.locator('[data-test="account-details"]').click();
  await expect(page.locator('[data-test="account-share"]')).toHaveCount(0);
  await expect(page.locator('[data-test="share-qr"]')).toHaveCount(0);
});
