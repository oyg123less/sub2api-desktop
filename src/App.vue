<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "./components/Icon.vue";
import Toasts from "./components/Toasts.vue";
import UpdateModal from "./components/UpdateModal.vue";
import type { ReleaseInfo } from "./api/control";
import {
  checkForUpdate,
  isUpdateCheckEnabled,
  UPDATE_CHECK_INTERVAL_MS,
  UPDATE_PREFERENCE_EVENT,
} from "./api/update";
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
let updateTimer: number | undefined;
let unsubscribeBackend: (() => void) | undefined;
const availableUpdate = ref<ReleaseInfo | null>(null);
const updateOpen = ref(false);
let updateChecking = false;

async function refreshUpdate(force = false) {
  const version = app.status?.version;
  if (!version || updateChecking || !isUpdateCheckEnabled()) return;
  updateChecking = true;
  try {
    availableUpdate.value = await checkForUpdate(version, force);
    if (!availableUpdate.value) updateOpen.value = false;
  } catch {
    // Update checks are intentionally silent.
  } finally {
    updateChecking = false;
  }
}

function handleUpdatePreference() {
  if (!isUpdateCheckEnabled()) {
    availableUpdate.value = null;
    updateOpen.value = false;
    return;
  }
  void refreshUpdate(true);
}

const backendPhase = computed(() => app.backend?.phase ?? "stopped");
const backendColor = computed(() => {
  if (backendPhase.value === "ready") return "var(--success)";
  if (["starting", "restarting", "migrating"].includes(backendPhase.value)) return "var(--warn)";
  if (backendPhase.value === "failed") return "var(--danger)";
  return "var(--text-faint)";
});
const backendLabel = computed(() => t(`backend.${backendPhase.value}`));
const currentVersion = computed(() => app.status?.version || "0.2.1");

watch(() => app.status?.version, (version) => {
  if (version) void refreshUpdate();
});

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
  updateTimer = window.setInterval(() => void refreshUpdate(true), UPDATE_CHECK_INTERVAL_MS);
  window.addEventListener(UPDATE_PREFERENCE_EVENT, handleUpdatePreference);
});
onUnmounted(() => {
  clearInterval(timer);
  clearInterval(updateTimer);
  window.removeEventListener(UPDATE_PREFERENCE_EVENT, handleUpdatePreference);
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
        <div class="version-row">
          <span>v{{ currentVersion }}</span>
          <button
            v-if="availableUpdate"
            class="update-badge"
            type="button"
            @click="updateOpen = true"
          >
            {{ t("updates.available", { version: availableUpdate.tag_name }) }}
          </button>
        </div>
      </div>
    </aside>

    <main class="main">
      <RouterView />
    </main>

    <Toasts />
    <UpdateModal
      :open="updateOpen"
      :release="availableUpdate"
      :current-version="currentVersion"
      @close="updateOpen = false"
    />
  </div>
</template>

<style scoped>
.version-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 24px;
  margin-top: 6px;
}
.update-badge {
  max-width: 100%;
  padding: 2px 6px;
  border: 1px solid rgba(193, 134, 58, 0.32);
  border-radius: 6px;
  background: var(--warn-soft);
  color: var(--warn);
  font-size: 10.5px;
  line-height: 1.4;
  text-align: left;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.update-badge:hover {
  border-color: var(--warn);
}
</style>
