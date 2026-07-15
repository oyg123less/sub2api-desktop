import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { router } from "./router";
import { i18n } from "./i18n";
import "./styles/tokens.css";
import "./styles/main.css";

function boot() {
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
