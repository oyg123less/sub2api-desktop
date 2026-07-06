<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type Account, type Proxy } from "../api/control";
import { useAppStore } from "../store";
import { openUrl } from "../platform";

const { t } = useI18n();
const app = useAppStore();

const accounts = ref<Account[]>([]);
const proxies = ref<Proxy[]>([]);
const loading = ref(true);

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
    [accounts.value, proxies.value] = [
      (await api.listAccounts()).accounts || [],
      (await api.listProxies()).proxies || [],
    ];
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
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

onMounted(load);
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
