import { defineConfig, devices } from "@playwright/test";

const e2ePort = Number(process.env.AMBER_E2E_PORT || "4173");
const e2eBaseURL = `http://127.0.0.1:${e2ePort}`;
const nodeRuntime = process.execPath.replaceAll('"', '\\"');

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  retries: process.env.CI ? 2 : 0,
  use: {
    baseURL: e2eBaseURL,
    trace: "on-first-retry",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 800 } } },
    { name: "compact", use: { ...devices["Desktop Chrome"], viewport: { width: 900, height: 650 } } },
  ],
  webServer: {
    command: `"${nodeRuntime}" node_modules/vite/bin/vite.js preview --host 127.0.0.1 --port ${e2ePort}`,
    url: e2eBaseURL,
    reuseExistingServer: !process.env.CI,
  },
});
