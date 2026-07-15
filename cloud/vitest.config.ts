import { cloudflareTest, readD1Migrations } from "@cloudflare/vitest-pool-workers";
import { defineConfig } from "vitest/config";

const migrations = await readD1Migrations("./migrations");

export default defineConfig({
  plugins: [
    cloudflareTest({
      main: "./src/index.ts",
      miniflare: {
        compatibilityDate: "2026-07-15",
        d1Databases: ["DB"],
        kvNamespaces: ["SESSIONS"],
        bindings: {
          ENVIRONMENT: "test",
          MAILER_MODE: "console",
          JWT_SECRET: "test-jwt-secret-that-is-at-least-32-bytes-long",
          TURNSTILE_SECRET: "test-turnstile-secret",
          ADMIN_API_KEY: "test-admin-second-factor-at-least-32-bytes",
        },
      },
    }),
  ],
  test: {
    include: ["test/**/*.test.ts"],
    setupFiles: ["./test/setup.ts"],
    provide: { migrations },
  },
});
