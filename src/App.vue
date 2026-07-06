<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "./components/Icon.vue";
import Toasts from "./components/Toasts.vue";
import { useAppStore } from "./store";
import logoUrl from "./assets/logo.svg";

const route = useRoute();
const { t } = useI18n();
const app = useAppStore();

const nav = [
  { name: "dashboard", to: "/dashboard", icon: "dashboard" },
  { name: "accounts", to: "/accounts", icon: "accounts" },
  { name: "proxies", to: "/proxies", icon: "proxies" },
  { name: "statistics", to: "/statistics", icon: "statistics" },
  { name: "settings", to: "/settings", icon: "settings" },
  { name: "codex", to: "/codex", icon: "terminal" },
  { name: "shop", to: "/shop", icon: "cart" },
  { name: "docs", to: "/docs", icon: "docs" },
];

let timer: number | undefined;
onMounted(() => {
  app.refreshStatus();
  timer = window.setInterval(() => app.refreshStatus(), 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <img class="brand-logo" :src="logoUrl" alt="Amber" />
        <div>
          <div class="brand-name">Amber</div>
          <div class="brand-sub">琥珀 · 本地网关</div>
        </div>
      </div>

      <nav class="nav">
        <RouterLink
          v-for="item in nav"
          :key="item.name"
          :to="item.to"
          class="nav-item"
          :class="{ active: route.path === item.to }"
        >
          <Icon :name="item.icon" />
          <span>{{ t("nav." + item.name) }}</span>
        </RouterLink>
      </nav>

      <div class="sidebar-footer">
        <div class="flex items-center gap-8">
          <span
            class="badge-dot"
            :style="{
              background: app.serverRunning ? 'var(--success)' : 'var(--text-faint)',
            }"
          ></span>
          <span>{{ app.serverRunning ? t("common.running") : t("common.stopped") }}</span>
        </div>
        <div style="margin-top: 6px">v{{ app.status?.version || "0.1.0" }}</div>
      </div>
    </aside>

    <main class="main">
      <RouterView />
    </main>

    <Toasts />
  </div>
</template>
