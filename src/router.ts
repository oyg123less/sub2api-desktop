import { createRouter, createWebHashHistory } from "vue-router";
import Dashboard from "./views/Dashboard.vue";
import Accounts from "./views/Accounts.vue";
import Proxies from "./views/Proxies.vue";
import Statistics from "./views/Statistics.vue";
import Settings from "./views/Settings.vue";
import Codex from "./views/Codex.vue";
import Shop from "./views/Shop.vue";
import Docs from "./views/Docs.vue";

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: "/", redirect: "/dashboard" },
    { path: "/dashboard", component: Dashboard, name: "dashboard" },
    { path: "/accounts", component: Accounts, name: "accounts" },
    { path: "/proxies", component: Proxies, name: "proxies" },
    { path: "/statistics", component: Statistics, name: "statistics" },
    { path: "/settings", component: Settings, name: "settings" },
    { path: "/codex", component: Codex, name: "codex" },
    { path: "/shop", component: Shop, name: "shop" },
    { path: "/docs", component: Docs, name: "docs" },
  ],
});
