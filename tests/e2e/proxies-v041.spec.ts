import { expect, test } from "@playwright/test";

test("applies and clears one proxy for every current account", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  const proxies = [
    { id: 7, name: "Tokyo relay", type: "socks5", host: "127.0.0.1", port: 1080, created_at: "2026-07-18T00:00:00Z" },
    { id: 8, name: "Local HTTP", type: "http", host: "127.0.0.1", port: 7890, created_at: "2026-07-18T00:00:00Z" },
  ];
  let summary: Record<string, unknown> = { total: 4, bound: 2, unbound: 2, mixed: true, bindings: [{ proxy_id: 8, count: 2 }] };
  const applied: Array<number | null> = [];
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown = { version: "0.4.1", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "http://127.0.0.1:8080/v1", lan_addresses: [], local_api_key: "", account_count: 4, schema_version: 13 };
    if (path === "/control/proxies") body = { proxies };
    if (path === "/control/accounts/proxy-summary") body = summary;
    if (path === "/control/accounts/batch-proxy") {
      const input = request.postDataJSON() as { scope: string; proxy_id: number | null };
      applied.push(input.proxy_id);
      summary = input.proxy_id == null
        ? { total: 4, bound: 0, unbound: 4, mixed: false, bindings: null }
        : { total: 4, bound: 4, unbound: 0, mixed: false, uniform_proxy_id: input.proxy_id, bindings: [{ proxy_id: input.proxy_id, count: 4 }] };
      body = { ok: true, matched: 4, updated: 4, unchanged: 0, proxy_id: input.proxy_id };
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });

  await page.goto("/#/proxies");
  await expect(page.getByText("Mixed configuration")).toBeVisible();
  const panelStyle = await page.locator('[data-test="proxy-account-panel"]').evaluate((element) => {
    const style = getComputedStyle(element);
    return { height: element.getBoundingClientRect().height, boxShadow: style.boxShadow };
  });
  expect(panelStyle.height).toBeLessThan(90);
  expect(panelStyle.boxShadow).toBe("none");
  await page.locator('[data-test="proxy-global-select"]').selectOption("7");
  await page.locator('[data-test="proxy-global-apply"]').click();
  await expect(page.getByText("Update every account proxy?")).toBeVisible();
  await page.getByRole("button", { name: "Apply to all accounts" }).last().click();
  await expect.poll(() => applied).toEqual([7]);
  await expect(page.getByText("Tokyo relay", { exact: true }).first()).toBeVisible();

  await page.getByRole("button", { name: "Clear all account proxies" }).click();
  await page.getByRole("button", { name: "Apply to all accounts" }).last().click();
  await expect.poll(() => applied).toEqual([7, null]);
  await expect(page.getByText("All direct").first()).toBeVisible();
  await expect(page.locator('[data-test="route-error"]')).toHaveCount(0);
});
