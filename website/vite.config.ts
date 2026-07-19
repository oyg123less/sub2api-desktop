import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { stableRelease } from "./src/config/releases";

export default defineConfig({
  plugins: [
    vue(),
    {
      name: "amber-release-html",
      transformIndexHtml(html) {
        return html
          .replaceAll("__AMBER_STABLE_VERSION__", stableRelease.version)
          .replaceAll("__AMBER_STABLE_DOWNLOAD_URL__", stableRelease.downloadUrl);
      },
    },
  ],
  build: {
    outDir: "dist",
    sourcemap: false,
  },
  test: {
    environment: "jsdom",
    globals: true,
    include: ["tests/**/*.spec.ts"],
    exclude: ["tests/e2e/**"],
    setupFiles: ["./tests/setup.ts"],
  },
});
