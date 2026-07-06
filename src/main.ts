import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { router } from "./router";
import { i18n } from "./i18n";
import { bootstrapConnection, isTauri } from "./tauri";
import "./styles/main.css";

async function boot() {
  if (isTauri()) {
    // Wait briefly for the sidecar handshake before the first render so the UI
    // connects immediately.
    for (let i = 0; i < 40; i++) {
      await bootstrapConnection();
      if (window.__SUB2API__) break;
      await new Promise((r) => setTimeout(r, 150));
    }
  }

  const app = createApp(App);
  app.config.errorHandler = (err, _instance, info) => {
    console.error(`[vue error] ${info}:`, err);
  };
  app.use(createPinia());
  app.use(router);
  app.use(i18n);
  app.mount("#app");
}

boot();
