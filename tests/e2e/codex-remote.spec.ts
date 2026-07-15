import { expect, test } from "@playwright/test";

test("injects tunnel and direct remote targets without exposing credentials", async ({ page }, testInfo) => {
  let targets: Record<string, unknown>[] = [];
  let directInject: Record<string, unknown> | null = null;
  let directReinject: Record<string, unknown> | null = null;
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
      const body = request.postDataJSON() as Record<string, unknown>;
      if (body.mode === "direct") {
        if (body.api_key) directInject = body;
        else directReinject = body;
        const target = {
          id: 2,
          name: "deploy@direct.example.test",
          host: "direct.example.test",
          port: 22,
          user: "deploy",
          remote_port: 8080,
          model: "gpt-5.6",
          mode: "direct",
          base_url: body.base_url,
          saved: true,
          injected: true,
          tunnel_enabled: false,
          tunnel_status: "injected_direct",
          config_preview: `model = \"gpt-5.6\"\nbase_url = \"${body.base_url}\"`,
          auth_preview: "{\"OPENAI_API_KEY\":\"********\"}",
          updated_at: new Date().toISOString(),
        };
        const index = targets.findIndex((item) => item.id === target.id);
        if (index >= 0) targets[index] = target;
        else targets.push(target);
        return json(target);
      }
      const target = {
        id: 1,
        name: "deploy@example.test",
        host: "example.test",
        port: 22,
        user: "deploy",
        remote_port: 8080,
        model: "gpt-5.6",
        mode: "tunnel",
        saved: true,
        injected: true,
        tunnel_enabled: true,
        tunnel_status: "connected",
        config_preview: "model = \"gpt-5.6\"",
        auth_preview: "{\"OPENAI_API_KEY\":\"********\"}",
        updated_at: new Date().toISOString(),
      };
      targets = [target];
      return json(target);
    }
    if (path === "/control/status") return json({ version: "0.2.3", server_running: true, port: 8080, host: "127.0.0.1", endpoint: "", lan_addresses: [], local_api_key: "", account_count: 1, schema_version: 6 });
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

  await page.getByRole("button", { name: "New target" }).click();
  await page.locator('[data-test="remote-mode-direct"]').click();
  await expect(page.locator('[data-test="remote-forward-port"]')).toHaveCount(0);
  await page.locator('[data-test="remote-host"]').fill("direct.example.test");
  await page.locator('[data-test="remote-user"]').fill("deploy");
  await page.locator('[data-test="remote-password"]').fill("fixture-password");
  await page.locator('[data-test="remote-base-url"]').fill("https://api.example.test/v1");
  await page.locator('[data-test="remote-api-key"]').fill("fixture-direct-key");
  await page.locator('[data-test="remote-test"]').click();
  await page.getByRole("button", { name: "Trust and continue" }).click();
  await page.locator('[data-test="remote-inject"]').click();

  const directCard = page.locator('[data-target-id="2"]');
  await expect(directCard).toContainText("Direct");
  await expect(directCard).toContainText("Injected (direct)");
  await expect(directCard).toContainText("https://api.example.test/v1");
  await expect(directCard.locator(".switch")).toHaveCount(0);
  await expect(page.locator('[data-test="remote-api-key"]')).toHaveValue("");
  expect(directInject).toMatchObject({
    mode: "direct",
    base_url: "https://api.example.test/v1",
    api_key: "fixture-direct-key",
  });
  await expect(page.locator("body")).not.toContainText("fixture-direct-key");

  const directReinjectButton = directCard.getByRole("button", { name: "Reinject" });
  await expect(directReinjectButton).toHaveCount(1);
  await directReinjectButton.click();
  await expect(page.locator('[data-test="remote-api-key"]')).toHaveAttribute("placeholder", "Leave blank to use the saved API Key");
  await page.locator('[data-test="remote-inject"]').click();
  expect(directReinject).toMatchObject({
    id: 2,
    mode: "direct",
    base_url: "https://api.example.test/v1",
    api_key: "",
  });
  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  await page.waitForTimeout(3400);
  await page.screenshot({ path: testInfo.outputPath("codex-remote.png"), fullPage: true });
});
