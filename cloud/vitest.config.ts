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
        durableObjects: {
          OWNER_RELAY: "OwnerRelay",
          SHARE_ACCESS: "ShareAccessCoordinator",
        },
        bindings: {
          ENVIRONMENT: "test",
          MAILER_MODE: "console",
          JWT_SECRET: "test-jwt-secret-that-is-at-least-32-bytes-long",
          TURNSTILE_SECRET: "test-turnstile-secret",
          RESEND_WEBHOOK_SECRET: "whsec_dGVzdC1yZXNlbmQtd2ViaG9vay1zZWNyZXQtMzItYnl0ZXM=",
          ADMIN_API_KEY: "test-admin-second-factor-at-least-32-bytes",
          SHARE_KMS_KEY: "Hx8fHx8fHx8fHx8fHx8fHx8fHx8fHx8fHx8fHx8fHx8",
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
