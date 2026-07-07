<script setup lang="ts">
import { onMounted, onUnmounted, ref, computed } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import CopyField from "../components/CopyField.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type RequestLog, type StatsResponse } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();
const router = useRouter();

const logs = ref<RequestLog[]>([]);
const stats = ref<StatsResponse | null>(null);
const busy = ref(false);
const regenOpen = ref(false);
const editingUrl = ref(false);
const urlDraft = ref("");
const savingUrl = ref(false);

const hasAccount = computed(() => app.accountCount > 0);
const endpoint = computed(() => app.status?.endpoint || "http://127.0.0.1:8080/v1");
const apiKey = computed(() => app.status?.local_api_key || "");

const todayRequests = computed(() => {
  const today = new Date().toISOString().slice(0, 10);
  const d = stats.value?.daily?.find((x) => x.date === today);
  return d?.requests ?? 0;
});
const todayTokens = computed(() => {
  const today = new Date().toISOString().slice(0, 10);
  const d = stats.value?.daily?.find((x) => x.date === today);
  return d?.total_tokens ?? 0;
});

async function toggleServer() {
  busy.value = true;
  try {
    if (app.serverRunning) {
      await api.stopServer();
    } else {
      await api.startServer();
    }
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

function startEditUrl() {
  urlDraft.value = endpoint.value;
  editingUrl.value = true;
}

function parsePort(url: string): number | null {
  const m = /^http:\/\/127\.0\.0\.1:(\d{1,5})\/v1\/?$/.exec(url.trim());
  if (!m) return null;
  const port = Number(m[1]);
  if (port < 1 || port > 65535) return null;
  return port;
}

async function saveUrl() {
  const port = parsePort(urlDraft.value);
  if (port === null) {
    app.toast(t("dashboard.invalidBaseUrl"), "error");
    return;
  }
  savingUrl.value = true;
  try {
    const s = await api.getSettings();
    if (s.listen_port !== port) {
      await api.saveSettings({ ...s, listen_port: port });
      if (app.serverRunning) {
        await api.stopServer();
        await api.startServer();
      }
    }
    await app.refreshStatus();
    editingUrl.value = false;
    app.toast(t("dashboard.baseUrlSaved", { port }), "success");
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    savingUrl.value = false;
  }
}

async function confirmRegen() {
  try {
    await api.regenerateKey();
    await app.refreshStatus();
    regenOpen.value = false;
    app.toast(t("settings.saved"), "success");
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function load() {
  try {
    [logs.value, stats.value] = [
      (await api.logs(8)).logs || [],
      await api.stats(7),
    ];
  } catch {
    /* ignore transient */
  }
}

function fmtTime(s: string) {
  return new Date(s).toLocaleTimeString();
}
function statusClass(code: number) {
  return code >= 200 && code < 300 ? "badge-success" : "badge-danger";
}
function fmtNum(n: number) {
  return n.toLocaleString();
}

let timer: number | undefined;
onMounted(() => {
  load();
  timer = window.setInterval(load, 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t("dashboard.title") }}</h1>
      <p class="page-desc">{{ t("dashboard.desc") }}</p>
    </div>

    <!-- No account warning -->
    <div
      v-if="!hasAccount"
      class="card"
      style="border-color: var(--warn); background: var(--warn-soft); margin-bottom: 16px"
    >
      <div class="row-between">
        <div class="flex items-center gap-12">
          <Icon name="warn" :size="20" style="color: var(--warn)" />
          <span>{{ t("dashboard.noAccountWarn") }}</span>
        </div>
        <button class="btn btn-sm" @click="router.push('/accounts')">
          {{ t("dashboard.goAddAccount") }}
        </button>
      </div>
    </div>

    <!-- Service control -->
    <div class="card">
      <div class="row-between" style="margin-bottom: 18px">
        <div class="flex items-center gap-12">
          <span
            class="badge"
            :class="app.serverRunning ? 'badge-success' : 'badge-neutral'"
          >
            <span class="badge-dot"></span>
            {{ app.serverRunning ? t("common.running") : t("common.stopped") }}
          </span>
          <span class="faint text-sm" v-if="app.status">
            {{ app.status.host }}:{{ app.status.port }}
          </span>
        </div>
        <button
          class="btn btn-lg"
          :class="app.serverRunning ? 'btn-danger' : 'btn-primary'"
          :disabled="busy || (!app.serverRunning && !hasAccount)"
          @click="toggleServer"
        >
          <Icon :name="app.serverRunning ? 'stop' : 'play'" :size="16" />
          {{ app.serverRunning ? t("dashboard.stop") : t("dashboard.start") }}
        </button>
      </div>
    </div>

    <!-- Stat tiles -->
    <div class="grid grid-3" style="margin-top: 16px">
      <div class="stat">
        <div class="stat-label">{{ t("dashboard.accounts") }}</div>
        <div class="stat-value">{{ app.accountCount }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("dashboard.todayRequests") }}</div>
        <div class="stat-value">{{ fmtNum(todayRequests) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("dashboard.todayTokens") }}</div>
        <div class="stat-value">{{ fmtNum(todayTokens) }}</div>
      </div>
    </div>

    <!-- Quick config -->
    <div class="card" style="margin-top: 16px">
      <h3 class="card-title"><Icon name="link" :size="16" /> {{ t("dashboard.quickConfig") }}</h3>
      <p class="faint text-sm" style="margin-top: -6px; margin-bottom: 14px">
        {{ t("dashboard.quickConfigDesc") }}
      </p>
      <div class="grid grid-2">
        <div>
          <div class="row-between" style="margin-bottom: 6px">
            <label class="field-label" style="margin: 0">{{ t("dashboard.baseUrl") }}</label>
            <button v-if="!editingUrl" class="btn btn-ghost btn-sm" @click="startEditUrl">
              <Icon name="edit" :size="13" /> {{ t("dashboard.edit") }}
            </button>
            <div v-else class="flex items-center gap-12" style="gap: 6px">
              <button class="btn btn-primary btn-sm" :disabled="savingUrl" @click="saveUrl">
                {{ t("dashboard.save") }}
              </button>
              <button class="btn btn-ghost btn-sm" :disabled="savingUrl" @click="editingUrl = false">
                {{ t("dashboard.cancel") }}
              </button>
            </div>
          </div>
          <CopyField v-if="!editingUrl" :value="endpoint" />
          <input
            v-else
            v-model="urlDraft"
            class="input mono"
            placeholder="http://127.0.0.1:8080/v1"
            @keyup.enter="saveUrl"
            @keyup.esc="editingUrl = false"
          />
        </div>
        <div>
          <div class="row-between" style="margin-bottom: 6px">
            <label class="field-label" style="margin: 0">API Key</label>
            <button class="btn btn-ghost btn-sm" @click="regenOpen = true">
              <Icon name="refresh" :size="13" /> {{ t("settings.regenerate") }}
            </button>
          </div>
          <CopyField :value="apiKey" mask />
        </div>
      </div>
      <div class="faint text-sm mt-16">
        {{ t("dashboard.clientHint") }}
      </div>
    </div>

    <ConfirmModal
      :open="regenOpen"
      :title="t('settings.regenKeyConfirm')"
      :desc="t('settings.regenKeyDesc')"
      danger
      :confirm-text="t('settings.regenerate')"
      @confirm="confirmRegen"
      @cancel="regenOpen = false"
    />

    <!-- Recent requests -->
    <div class="card" style="margin-top: 16px">
      <div class="row-between" style="margin-bottom: 4px">
        <h3 class="card-title" style="margin: 0"><Icon name="clock" :size="16" /> {{ t("dashboard.recent") }}</h3>
        <button class="btn btn-sm btn-ghost" @click="router.push('/statistics')">
          {{ t("dashboard.viewAll") }}
        </button>
      </div>
      <div v-if="logs.length === 0" class="empty">
        <div class="empty-icon">◔</div>
        <div>{{ t("dashboard.noRequests") }}</div>
      </div>
      <div v-else class="list">
        <div v-for="l in logs" :key="l.id" class="list-row">
          <span class="badge" :class="statusClass(l.status_code)" style="min-width: 52px; justify-content: center">
            {{ l.status_code }}
          </span>
          <span class="mono" style="flex: 1">{{ l.model }}</span>
          <span class="faint text-sm">{{ fmtNum(l.total_tokens) }} tok</span>
          <span class="faint text-sm" style="width: 62px; text-align: right">{{ l.latency_ms }}ms</span>
          <span class="faint text-sm" style="width: 84px; text-align: right">{{ fmtTime(l.created_at) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
