<script setup lang="ts">
import { computed, onMounted, onUnmounted } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "./components/Icon.vue";
import Toasts from "./components/Toasts.vue";
import { useAppStore } from "./store";
import logoUrl from "./assets/logo.svg";
import { initializeBackendBridge, restartBackend, subscribeBackendState } from "./tauri";

const route = useRoute();
const { t } = useI18n();
const app = useAppStore();

const nav = [
  { name: "dashboard", to: "/dashboard", icon: "dashboard" },
  { name: "accounts", to: "/accounts", icon: "accounts" },
  { name: "proxies", to: "/proxies", icon: "proxies" },
  { name: "statistics", to: "/statistics", icon: "statistics" },
  { name: "diagnostics", to: "/diagnostics", icon: "bolt" },
  { name: "settings", to: "/settings", icon: "settings" },
  { name: "codex", to: "/codex", icon: "terminal" },
  { name: "shop", to: "/shop", icon: "cart" },
  { name: "docs", to: "/docs", icon: "docs" },
];

let timer: number | undefined;
let unsubscribeBackend: (() => void) | undefined;

const backendPhase = computed(() => app.backend?.phase ?? "stopped");
const backendColor = computed(() => {
  if (backendPhase.value === "ready") return "var(--success)";
  if (["starting", "restarting", "migrating"].includes(backendPhase.value)) return "var(--warn)";
  if (backendPhase.value === "failed") return "var(--danger)";
  return "var(--text-faint)";
});
const backendLabel = computed(() => t(`backend.${backendPhase.value}`));

async function retryBackend() {
  try {
    await restartBackend();
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

onMounted(async () => {
  unsubscribeBackend = subscribeBackendState((state) => {
    app.setBackendState(state);
    if (state.phase === "ready") app.refreshStatus();
  });
  try {
    await initializeBackendBridge();
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
  if (app.backendReady) await app.refreshStatus();
  timer = window.setInterval(() => app.refreshStatus(), 5000);
});
onUnmounted(() => {
  clearInterval(timer);
  unsubscribeBackend?.();
});
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
        <div class="flex items-center gap-8 backend-indicator">
          <span
            class="badge-dot"
            :style="{ background: backendColor }"
          ></span>
          <span>{{ backendLabel }}</span>
          <button
            v-if="backendPhase === 'failed'"
            class="backend-retry"
            type="button"
            :title="t('backend.retry')"
            :aria-label="t('backend.retry')"
            @click="retryBackend"
          >
            <Icon name="refresh" :size="13" />
          </button>
        </div>
        <div style="margin-top: 6px">v{{ app.status?.version || "0.2.0" }}</div>
      </div>
    </aside>

    <main class="main">
      <RouterView />
    </main>

    <Toasts />
  </div>
</template>
