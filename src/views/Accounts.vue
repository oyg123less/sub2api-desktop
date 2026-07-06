<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type Account, type AccountUsage, type AccountTestResult, type Proxy } from "../api/control";
import { useAppStore } from "../store";
import { openUrl } from "../platform";

const { t } = useI18n();
const app = useAppStore();

const accounts = ref<Account[]>([]);
const usage = ref<Record<string, AccountUsage>>({});
const proxies = ref<Proxy[]>([]);
const loading = ref(true);

// connectivity test flow
const testOpen = ref(false);
const testTarget = ref<Account | null>(null);
const testModel = ref("gpt-5.4-high");
const testRunning = ref(false);
const testResult = ref<AccountTestResult | null>(null);
const testError = ref("");
const modelOptions = ref<string[]>([
  "gpt-5.5",
  "gpt-5.4-low",
  "gpt-5.4-medium",
  "gpt-5.4-high",
  "gpt-5.4-xhigh",
  "gpt-5.3-codex-high",
  "gpt-5",
  "gpt-5-codex",
]);

function pct(v?: number) {
  if (v == null) return 0;
  return Math.max(0, Math.min(100, v));
}
function pctLabel(v?: number) {
  if (v == null) return "—";
  return v.toFixed(1) + "%";
}
function usageBarClass(v?: number) {
  if (v == null) return "";
  if (v >= 90) return "usage-fill-danger";
  if (v >= 70) return "usage-fill-warn";
  return "";
}
function fmtWindow(minutes?: number) {
  if (!minutes) return "";
  if (minutes % (60 * 24) === 0) return minutes / (60 * 24) + "d";
  if (minutes % 60 === 0) return minutes / 60 + "h";
  return minutes + "m";
}
function fmtReset(seconds?: number) {
  if (seconds == null || seconds <= 0) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h${m}m`;
  if (m > 0) return `${m}m`;
  return `${seconds}s`;
}

// force-reset flow
const resetting = ref<Record<number, boolean>>({});

// login flow
const loginOpen = ref(false);
const loginUrl = ref("");
const loginState = ref("");
const loginError = ref("");
let pollTimer: number | undefined;

// delete flow
const deleteTarget = ref<Account | null>(null);

// import flow
const importOpen = ref(false);
const importText = ref("");
const importing = ref(false);
const importFileName = ref("");
const importFileInput = ref<HTMLInputElement | null>(null);

const importExample = `[
  {
    "email": "you@example.com",
    "access_token": "",
    "refresh_token": "",
    "id_token": ""
  }
]`;

function openImport() {
  importText.value = "";
  importFileName.value = "";
  if (importFileInput.value) importFileInput.value.value = "";
  importOpen.value = true;
}

async function onImportFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  try {
    importText.value = await file.text();
    importFileName.value = file.name;
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function submitImport() {
  const raw = importText.value.trim();
  if (!raw) {
    app.toast(t("accounts.importEmpty"), "error");
    return;
  }
  importing.value = true;
  try {
    const r = await api.importAccounts(raw);
    const msg = t("accounts.importResult", {
      imported: r.imported,
      updated: r.updated,
      skipped: r.skipped,
    });
    app.toast(msg, r.skipped > 0 ? "warn" : "success");
    if (r.errors && r.errors.length) {
      app.toast(r.errors.join("; "), "error");
    }
    if (r.imported > 0 || r.updated > 0) {
      importOpen.value = false;
      await load();
      await app.refreshStatus();
    }
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    importing.value = false;
  }
}

async function load() {
  try {
    const [acc, prox] = [await api.listAccounts(), await api.listProxies()];
    accounts.value = acc.accounts || [];
    usage.value = acc.usage || {};
    proxies.value = prox.proxies || [];
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

function openTest(a: Account) {
  testTarget.value = a;
  testResult.value = null;
  testError.value = "";
  testRunning.value = false;
  testOpen.value = true;
}

async function runTest() {
  if (!testTarget.value) return;
  testRunning.value = true;
  testResult.value = null;
  testError.value = "";
  try {
    const r = await api.testAccount(testTarget.value.id, testModel.value);
    testResult.value = r;
    await load();
  } catch (e) {
    testError.value = (e as Error).message;
  } finally {
    testRunning.value = false;
  }
}

async function forceReset(a: Account) {
  resetting.value[a.id] = true;
  try {
    await api.setAccountStatus(a.id, "active");
    app.toast(t("accounts.resetOk"), "success");
    await load();
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    resetting.value[a.id] = false;
  }
}

function fmtCost(n?: number) {
  if (!n) return "$0.0000";
  return "$" + n.toFixed(4);
}
function fmtNum(n?: number) {
  return (n ?? 0).toLocaleString();
}

async function startLogin() {
  loginError.value = "";
  try {
    const r = await api.oauthStart(null);
    loginUrl.value = r.auth_url;
    loginState.value = r.state;
    loginOpen.value = true;
    openUrl(r.auth_url);
    beginPoll();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

function beginPoll() {
  clearInterval(pollTimer);
  pollTimer = window.setInterval(async () => {
    try {
      const r = await api.oauthPoll(loginState.value);
      if (r.done) {
        clearInterval(pollTimer);
        if (r.error) {
          loginError.value = r.error;
        } else {
          loginOpen.value = false;
          app.toast(t("accounts.loginSuccess"), "success");
          await load();
          await app.refreshStatus();
        }
      }
    } catch {
      /* keep polling */
    }
  }, 1500);
}

function cancelLogin() {
  clearInterval(pollTimer);
  loginOpen.value = false;
}

async function confirmDelete() {
  if (!deleteTarget.value) return;
  try {
    await api.deleteAccount(deleteTarget.value.id);
    app.toast(t("common.delete") + " ✓", "success");
    deleteTarget.value = null;
    await load();
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function reLogin() {
  await startLogin();
}

async function refreshToken(a: Account) {
  try {
    await api.refreshAccount(a.id);
    app.toast(t("accounts.refreshOk"), "success");
    await load();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function bindProxy(a: Account, proxyId: number | null) {
  try {
    await api.bindProxy(a.id, proxyId);
    await load();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

function statusBadge(s: Account["status"]) {
  switch (s) {
    case "active":
      return "badge-success";
    case "rate_limited":
      return "badge-warn";
    default:
      return "badge-danger";
  }
}
function fmtDate(s?: string | null) {
  if (!s) return "—";
  return new Date(s).toLocaleString();
}

onMounted(() => {
  load();
  api
    .listModels()
    .then((r) => {
      if (r.models?.length) {
        modelOptions.value = r.models;
        if (!r.models.includes(testModel.value)) testModel.value = r.models[0];
      }
    })
    .catch(() => {});
});
onUnmounted(() => clearInterval(pollTimer));
</script>

<template>
  <div>
    <div class="page-header row-between">
      <div>
        <h1 class="page-title">{{ t("accounts.title") }}</h1>
        <p class="page-desc">{{ t("accounts.desc") }}</p>
      </div>
      <div class="flex gap-8">
        <button class="btn btn-ghost" @click="openImport">
          <Icon name="upload" :size="16" /> {{ t("accounts.import") }}
        </button>
        <button class="btn btn-primary" @click="startLogin">
          <Icon name="plus" :size="16" /> {{ t("accounts.login") }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="empty">{{ t("common.loading") }}</div>

    <div v-else-if="accounts.length === 0" class="card empty">
      <div class="empty-icon">👤</div>
      <div class="empty-title">{{ t("accounts.empty") }}</div>
      <div class="faint">{{ t("accounts.emptyDesc") }}</div>
      <button class="btn btn-primary mt-16" @click="startLogin">
        <Icon name="plus" :size="16" /> {{ t("accounts.login") }}
      </button>
    </div>

    <div v-else class="grid grid-2">
      <div v-for="a in accounts" :key="a.id" class="card">
        <div class="row-between" style="margin-bottom: 12px">
          <div class="flex items-center gap-12">
            <div class="brand-logo" style="background: linear-gradient(135deg, #d97757, #b8532f)">
              {{ (a.email || "?").charAt(0).toUpperCase() }}
            </div>
            <div>
              <div style="font-weight: 600">{{ a.email || t("common.unknown") }}</div>
              <div class="faint text-sm">{{ a.plan_type || "ChatGPT" }}</div>
            </div>
          </div>
          <span class="badge" :class="statusBadge(a.status)">
            <span class="badge-dot"></span>
            {{ t("accounts.status." + a.status) }}
          </span>
        </div>

        <div
          v-if="a.status === 'refresh_failed'"
          class="text-sm"
          style="color: var(--danger); margin-bottom: 10px"
        >
          {{ a.status_reason || "" }}
        </div>

        <div class="list" style="margin-bottom: 12px">
          <div class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.expiresAt") }}</span>
            <span class="text-sm">{{ fmtDate(a.expires_at) }}</span>
          </div>
          <div class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.lastUsed") }}</span>
            <span class="text-sm">{{ fmtDate(a.last_used_at) }}</span>
          </div>
          <div class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.tokensUsed") }}</span>
            <span class="text-sm mono">{{ fmtNum(usage[a.id]?.total_tokens) }}</span>
          </div>
          <div class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.estCost") }}</span>
            <span class="text-sm mono">{{ fmtCost(usage[a.id]?.cost_usd) }}</span>
          </div>
        </div>

        <div v-if="a.codex_usage" class="usage-windows">
          <div class="usage-window">
            <div class="usage-window-head">
              <span class="faint text-sm">
                {{ t("accounts.window5h") }}
                <span v-if="fmtWindow(a.codex_usage.secondary_window_minutes)" class="faint">· {{ fmtWindow(a.codex_usage.secondary_window_minutes) }}</span>
              </span>
              <span class="text-sm mono">{{ pctLabel(a.codex_usage.secondary_used_percent) }}</span>
            </div>
            <div class="usage-bar">
              <div class="usage-fill" :class="usageBarClass(a.codex_usage.secondary_used_percent)" :style="{ width: pct(a.codex_usage.secondary_used_percent) + '%' }"></div>
            </div>
            <div v-if="fmtReset(a.codex_usage.secondary_reset_after_seconds)" class="faint text-xs" style="margin-top: 3px">
              {{ t("accounts.resetIn") }} {{ fmtReset(a.codex_usage.secondary_reset_after_seconds) }}
            </div>
          </div>
          <div class="usage-window">
            <div class="usage-window-head">
              <span class="faint text-sm">
                {{ t("accounts.window7d") }}
                <span v-if="fmtWindow(a.codex_usage.primary_window_minutes)" class="faint">· {{ fmtWindow(a.codex_usage.primary_window_minutes) }}</span>
              </span>
              <span class="text-sm mono">{{ pctLabel(a.codex_usage.primary_used_percent) }}</span>
            </div>
            <div class="usage-bar">
              <div class="usage-fill" :class="usageBarClass(a.codex_usage.primary_used_percent)" :style="{ width: pct(a.codex_usage.primary_used_percent) + '%' }"></div>
            </div>
            <div v-if="fmtReset(a.codex_usage.primary_reset_after_seconds)" class="faint text-xs" style="margin-top: 3px">
              {{ t("accounts.resetIn") }} {{ fmtReset(a.codex_usage.primary_reset_after_seconds) }}
            </div>
          </div>
        </div>

        <div class="field" style="margin-bottom: 12px">
          <label class="field-label">{{ t("accounts.bindProxy") }}</label>
          <select
            class="select"
            :value="a.proxy_id ?? ''"
            @change="bindProxy(a, ($event.target as HTMLSelectElement).value ? Number(($event.target as HTMLSelectElement).value) : null)"
          >
            <option value="">{{ t("accounts.noProxy") }}</option>
            <option v-for="p in proxies" :key="p.id" :value="p.id">
              {{ p.name }} ({{ p.type }})
            </option>
          </select>
        </div>

        <div class="flex gap-8" style="margin-bottom: 8px">
          <button class="btn btn-primary btn-sm" style="flex: 1" @click="openTest(a)">
            <Icon name="bolt" :size="14" /> {{ t("accounts.test") }}
          </button>
          <button
            v-if="a.status !== 'active'"
            class="btn btn-ghost btn-sm"
            style="flex: 1"
            :disabled="resetting[a.id]"
            @click="forceReset(a)"
          >
            <Icon name="check" :size="14" /> {{ t("accounts.forceReset") }}
          </button>
        </div>

        <div class="flex gap-8">
          <button
            v-if="a.status === 'refresh_failed'"
            class="btn btn-primary btn-sm"
            style="flex: 1"
            @click="reLogin"
          >
            <Icon name="refresh" :size="14" /> {{ t("accounts.relogin") }}
          </button>
          <button v-else class="btn btn-ghost btn-sm" style="flex: 1" @click="refreshToken(a)">
            <Icon name="refresh" :size="14" /> {{ t("common.refresh") }}
          </button>
          <button class="btn btn-danger btn-sm" @click="deleteTarget = a">
            <Icon name="trash" :size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- Login modal -->
    <Teleport to="body">
      <div v-if="loginOpen" class="modal-backdrop" @click.self="cancelLogin">
        <div class="modal">
          <h3 class="modal-title">{{ t("accounts.login") }}</h3>
          <p class="modal-desc">{{ t("accounts.loginHint") }}</p>
          <div v-if="!loginError" class="flex items-center gap-12" style="margin: 12px 0">
            <Icon name="refresh" class="spin" :size="18" style="color: var(--primary)" />
            <span class="muted">{{ t("accounts.loggingIn") }}</span>
          </div>
          <div v-else class="text-sm" style="color: var(--danger); margin: 10px 0">
            {{ t("accounts.loginFailed") }}: {{ loginError }}
          </div>
          <div class="code-box" style="margin-top: 6px">
            <span>{{ loginUrl }}</span>
            <button class="copy-btn" @click="openUrl(loginUrl)"><Icon name="external" :size="15" /></button>
          </div>
          <div class="modal-actions">
            <button class="btn btn-ghost" @click="cancelLogin">{{ t("common.cancel") }}</button>
            <button v-if="loginError" class="btn btn-primary" @click="startLogin">
              {{ t("common.retry") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Connectivity test modal -->
    <Teleport to="body">
      <div v-if="testOpen" class="modal-backdrop" @click.self="testOpen = false">
        <div class="modal" style="max-width: 520px">
          <h3 class="modal-title">{{ t("accounts.testTitle") }}</h3>
          <p class="modal-desc">{{ testTarget?.email }}</p>
          <div class="field">
            <label class="field-label">{{ t("accounts.testModel") }}</label>
            <select v-model="testModel" class="select" :disabled="testRunning">
              <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
            </select>
          </div>

          <div v-if="testRunning" class="flex items-center gap-12" style="margin: 14px 0">
            <Icon name="refresh" class="spin" :size="18" style="color: var(--primary)" />
            <span class="muted">{{ t("accounts.testing") }}</span>
          </div>

          <div v-else-if="testError" class="card" style="margin: 12px 0; border-color: var(--danger)">
            <div class="text-sm" style="color: var(--danger)">{{ t("accounts.testFailed") }}: {{ testError }}</div>
          </div>

          <div v-else-if="testResult" class="card" style="margin: 12px 0">
            <div class="row-between" style="margin-bottom: 8px">
              <span class="badge" :class="testResult.ok ? 'badge-success' : 'badge-danger'">
                <span class="badge-dot"></span>
                {{ testResult.ok ? t("accounts.testPass") : t("accounts.testFailed") }}
              </span>
              <span class="faint text-sm">HTTP {{ testResult.status }} · {{ testResult.latency_ms }}ms</span>
            </div>
            <div v-if="!testResult.ok && testResult.error" class="text-sm" style="color: var(--danger); margin-bottom: 8px">
              {{ testResult.error }}
            </div>
            <div v-if="testResult.sample" class="code-box" style="margin-bottom: 8px">
              <span>{{ testResult.sample }}</span>
            </div>
            <div class="list-row" style="padding: 5px 0">
              <span class="faint text-sm" style="flex: 1">{{ t("accounts.tokensUsed") }}</span>
              <span class="text-sm mono">{{ fmtNum(testResult.total_tokens) }} ({{ fmtNum(testResult.prompt_tokens) }}+{{ fmtNum(testResult.completion_tokens) }})</span>
            </div>
            <div class="list-row" style="padding: 5px 0">
              <span class="faint text-sm" style="flex: 1">{{ t("accounts.statusAfter") }}</span>
              <span class="text-sm">{{ t("accounts.status." + testResult.account_status) }}</span>
            </div>
          </div>

          <div class="modal-actions">
            <button class="btn btn-ghost" @click="testOpen = false">{{ t("common.close") }}</button>
            <button class="btn btn-primary" :disabled="testRunning" @click="runTest">
              <Icon v-if="testRunning" name="refresh" class="spin" :size="14" />
              {{ t("accounts.runTest") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Import modal -->
    <Teleport to="body">
      <div v-if="importOpen" class="modal-backdrop" @click.self="importOpen = false">
        <div class="modal" style="max-width: 560px">
          <h3 class="modal-title">{{ t("accounts.import") }}</h3>
          <p class="modal-desc">{{ t("accounts.importHint") }}</p>
          <div class="flex items-center gap-8" style="margin-bottom: 10px">
            <button class="btn btn-ghost btn-sm" @click="importFileInput?.click()">
              <Icon name="upload" :size="14" /> {{ t("accounts.importChooseFile") }}
            </button>
            <span v-if="importFileName" class="faint text-sm">{{ importFileName }}</span>
            <input
              ref="importFileInput"
              type="file"
              accept="application/json,.json"
              style="display: none"
              @change="onImportFile"
            />
          </div>
          <textarea
            v-model="importText"
            class="input"
            rows="10"
            spellcheck="false"
            :placeholder="importExample"
            style="width: 100%; font-family: var(--font-mono, monospace); font-size: 12px; resize: vertical"
          ></textarea>
          <div class="modal-actions">
            <button class="btn btn-ghost" @click="importOpen = false">{{ t("common.cancel") }}</button>
            <button class="btn btn-primary" :disabled="importing" @click="submitImport">
              <Icon v-if="importing" name="refresh" class="spin" :size="14" />
              {{ t("accounts.import") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <ConfirmModal
      :open="!!deleteTarget"
      :title="t('accounts.deleteConfirm')"
      :desc="t('accounts.deleteDesc')"
      danger
      :confirm-text="t('common.delete')"
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>
