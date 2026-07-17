<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "./components/Icon.vue";
import RouteErrorBoundary from "./components/RouteErrorBoundary.vue";
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
const mainElement = ref<HTMLElement | null>(null);
const themeMode = ref(localStorage.getItem("s2a_theme") || "system");

const nav = [
  { name: "dashboard", to: "/dashboard", icon: "dashboard" },
  { name: "accounts", to: "/accounts", icon: "accounts" },
  { name: "proxies", to: "/proxies", icon: "proxies" },
  { name: "statistics", to: "/statistics", icon: "statistics" },
  { name: "models", to: "/models", icon: "database" },
  { name: "settings", to: "/settings", icon: "settings" },
  { name: "cloud", to: "/cloud", icon: "cloud" },
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
const currentVersion = computed(() => app.status?.version || "0.3.2");
const darkThemeActive = computed(() => themeMode.value === "dark" || (themeMode.value === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches));

function applyTheme(mode: string) {
  themeMode.value = mode;
  if (mode === "system") document.documentElement.removeAttribute("data-theme");
  else document.documentElement.dataset.theme = mode;
}

function toggleTheme() {
  const next = darkThemeActive.value ? "light" : "dark";
  localStorage.setItem("s2a_theme", next);
  applyTheme(next);
}

async function copyVersion() {
  try {
    await navigator.clipboard.writeText(`v${currentVersion.value}`);
    app.toast(t("common.versionCopied"), "success");
  } catch {
    app.toast(t("common.copyFailed"), "error");
  }
}

watch(() => app.status?.version, (version) => {
  if (version) void refreshUpdate();
});

watch(() => route.fullPath, async () => {
  await nextTick();
  mainElement.value?.scrollTo({ top: 0, left: 0, behavior: "auto" });
});

async function retryBackend() {
  try {
    await restartBackend();
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

onMounted(async () => {
  applyTheme(themeMode.value);
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
          <button class="footer-icon-button" type="button" :title="t(darkThemeActive ? 'common.useLightTheme' : 'common.useDarkTheme')" :aria-label="t(darkThemeActive ? 'common.useLightTheme' : 'common.useDarkTheme')" @click="toggleTheme"><Icon name="theme" :size="13" /></button>
          <button class="version-copy" type="button" :title="t('common.copyVersion')" @click="copyVersion">v{{ currentVersion }}</button>
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

    <main ref="mainElement" class="main">
      <RouterView v-slot="{ Component }">
        <Transition name="page" mode="out-in">
          <RouteErrorBoundary :key="route.fullPath" :component="Component" :reset-key="route.fullPath" />
        </Transition>
      </RouterView>
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
.footer-icon-button, .version-copy { min-height: 24px; padding: 2px 5px; border: 0; border-radius: 5px; background: transparent; color: var(--text-faint); cursor: pointer; }
.footer-icon-button:hover, .version-copy:hover { background: var(--bg-hover); color: var(--text); }
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
