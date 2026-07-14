import { expect, test } from "@playwright/test";

test("tests SSH, confirms the host key, and injects a remote target", async ({ page }, testInfo) => {
  let targets: Record<string, unknown>[] = [];
  await page.addInitScript(() => {
    localStorage.setItem("s2a_control_port", "45678");
    localStorage.setItem("s2a_control_token", "fixture-control-token");
    localStorage.setItem("s2a_lang", "en");
  });
  await page.route("http://127.0.0.1:45678/control/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const json = (value: unknown) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(value) });
    if (path === "/control/codex/status") {
      return json({
        config_path: "/home/local/.codex/config.toml",
        auth_path: "/home/local/.codex/auth.json",
        applied: false,
        config_exists: false,
        backup_exists: false,
        base_url: "http://127.0.0.1:8080/v1",
        model: "gpt-5.6",
        models: ["gpt-5.6"],
        config_preview: "model = \"gpt-5.6\"",
        auth_preview: "{\"OPENAI_API_KEY\":\"********\"}",
      });
    }
    if (path === "/control/codex/files") {
      return json({
        config_path: "/home/local/.codex/config.toml",
        auth_path: "/home/local/.codex/auth.json",
        config_content: "",
        auth_content: "",
        config_default: "model = \"gpt-5.6\"",
        auth_default: "{}",
      });
    }
    if (path === "/control/codex/remote/targets") return json({ targets });
    if (path === "/control/codex/remote/test") {
      return json({
        os: "Linux",
        home: "/home/deploy",
        codex_dir: "/home/deploy/.codex",
        host_key_fingerprint: "SHA256:fixture-host-key",
        known: false,
      });
    }
    if (path === "/control/codex/remote/inject") {
      targets = [{
        id: 1,
        name: "deploy@example.test",
        host: "example.test",
        port: 22,
        user: "deploy",
        remote_port: 8080,
        model: "gpt-5.6",
        saved: true,
        injected: true,
        tunnel_enabled: true,
        tunnel_status: "connected",
        config_preview: "model = \"gpt-5.6\"",
        auth_preview: "{\"OPENAI_API_KEY\":\"********\"}",
        updated_at: new Date().toISOString(),
      }];
      return json(targets[0]);
    }
    if (path === "/control/status") return json({ version: "0.2.3", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "", lan_addresses: [], local_api_key: "", account_count: 1, schema_version: 5 });
    return route.fulfill({ status: 404, contentType: "application/json", body: "{}" });
  });

  await page.goto("/#/codex");
  await page.locator('[data-test="tab-remote"]').click();
  await page.locator('[data-test="remote-host"]').fill("example.test");
  await page.locator('[data-test="remote-user"]').fill("deploy");
  await page.locator('[data-test="remote-password"]').fill("fixture-password");
  await page.locator('[data-test="remote-test"]').click();
  await expect(page.getByText("SHA256:fixture-host-key")).toBeVisible();
  await page.getByRole("button", { name: "Trust and continue" }).click();
  await page.locator('[data-test="remote-inject"]').click();
  await expect(page.locator('[data-target-id="1"]')).toContainText("deploy@example.test");
  await expect(page.locator('[data-target-id="1"]')).toContainText("Tunnel connected");
  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  await page.waitForTimeout(3400);
  await page.screenshot({ path: testInfo.outputPath("codex-remote.png"), fullPage: true });
});
