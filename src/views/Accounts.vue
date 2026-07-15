<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import {
  api,
  type Account,
  type AccountUsage,
  type AccountTestResult,
  type CloudShare,
  type CloudShareUsage,
  type ImportCommitResult,
  type ImportPreview,
  type Proxy,
	type Settings,
} from "../api/control";
import { useAppStore } from "../store";
import { openUrl } from "../platform";

const { t } = useI18n();
const app = useAppStore();

const accounts = ref<Account[]>([]);
const usage = ref<Record<string, AccountUsage>>({});
const proxies = ref<Proxy[]>([]);
const loading = ref(true);
const accountStrategy = ref<Settings["account_strategy"]>("quota_aware");
const cloudAuthenticated = ref(false);
const cloudShares = ref<CloudShare[]>([]);
const shareOpen = ref(false);
const shareTarget = ref<Account | null>(null);
const shareQuota = ref(0);
const shareExpires = ref("");
const shareConsent = ref(false);
const shareBusy = ref("");
const createdShare = ref<{ share: CloudShare; guest_key: string } | null>(null);
const shareUsage = ref<CloudShareUsage[]>([]);
const shareUsageTarget = ref<CloudShare | null>(null);

// connectivity test flow
const testOpen = ref(false);
const testTarget = ref<Account | null>(null);
const testModel = ref("");
const testRunning = ref(false);
const testResult = ref<AccountTestResult | null>(null);
const testError = ref("");
const modelOptions = ref<string[]>([]);

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
function windowLabel(minutes?: number) {
  if (!minutes) return t("accounts.windowGeneric");
  if (minutes % (60 * 24) === 0) return t("accounts.windowDays", { n: minutes / (60 * 24) });
  if (minutes % 60 === 0) return t("accounts.windowHours", { n: minutes / 60 });
  return t("accounts.windowMinutes", { n: minutes });
}
function usageWindows(u: NonNullable<Account["codex_usage"]>) {
  const wins = [
    {
      minutes: u.primary_window_minutes ?? 0,
      label: windowLabel(u.primary_window_minutes),
      used: u.primary_used_percent,
      reset: u.primary_reset_after_seconds,
    },
    {
      minutes: u.secondary_window_minutes ?? 0,
      label: windowLabel(u.secondary_window_minutes),
      used: u.secondary_used_percent,
      reset: u.secondary_reset_after_seconds,
    },
  ].filter((w) => w.used != null || w.minutes > 0);
  return wins.sort((a, b) => a.minutes - b.minutes);
}
function fmtReset(seconds?: number) {
  if (seconds == null || seconds <= 0) return "";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return t("accounts.durDayHour", { d, h });
  if (h > 0) return t("accounts.durHourMin", { h, m });
  if (m > 0) return t("accounts.durMin", { m });
  return t("accounts.durSec", { s: seconds });
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
const importFileBlob = ref<Blob | null>(null);
const importStage = ref<"input" | "preview" | "result">("input");
const importPreview = ref<ImportPreview | null>(null);
const importResult = ref<ImportCommitResult | null>(null);
const validateAfterImport = ref(true);

// API-key account flow
const apiKeyOpen = ref(false);
const apiKeyAdding = ref(false);
const apiKeyName = ref("");
const apiKeyBaseURL = ref("https://chatgpt.com/backend-api/codex/responses");
const apiKeyValue = ref("");

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
  importFileBlob.value = null;
  importStage.value = "input";
  importPreview.value = null;
  importResult.value = null;
  validateAfterImport.value = true;
  if (importFileInput.value) importFileInput.value.value = "";
  importOpen.value = true;
}

function closeImport() {
  if (importing.value) return;
  importOpen.value = false;
}

function openAPIKey() {
  apiKeyName.value = "";
  apiKeyBaseURL.value = "https://chatgpt.com/backend-api/codex/responses";
  apiKeyValue.value = "";
  apiKeyOpen.value = true;
}

function closeAPIKey() {
  if (apiKeyAdding.value) return;
  apiKeyValue.value = "";
  apiKeyOpen.value = false;
}

async function addAPIKey() {
  const baseURL = apiKeyBaseURL.value.trim();
  const key = apiKeyValue.value.trim();
  if (!baseURL || !key) {
    app.toast(t("accounts.apiKeyRequired"), "error");
    return;
  }
  try {
    const parsed = new URL(baseURL);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") throw new Error();
  } catch {
    app.toast(t("accounts.apiKeyURLInvalid"), "error");
    return;
  }

  apiKeyAdding.value = true;
  try {
    const raw = JSON.stringify([{
      account_type: "api_key",
      base_url: baseURL,
      api_key: key,
      name: apiKeyName.value.trim(),
    }]);
    const preview = await api.previewImport(raw);
    const row = preview.rows[0];
    if (!row || row.action === "error" || row.action === "conflict") {
      throw new Error(row?.error_message || t("accounts.apiKeyAddFailed"));
    }
    await api.commitImport(raw, preview.content_sha256, false);
    apiKeyValue.value = "";
    apiKeyOpen.value = false;
    app.toast(t("accounts.apiKeyAdded"), "success");
    await load();
    await app.refreshStatus();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    apiKeyAdding.value = false;
  }
}

function decodeFileForPreview(bytes: Uint8Array): string {
  if (bytes[0] === 0xff && bytes[1] === 0xfe) return new TextDecoder("utf-16le").decode(bytes.subarray(2));
  if (bytes[0] === 0xfe && bytes[1] === 0xff) return new TextDecoder("utf-16be").decode(bytes.subarray(2));
  return new TextDecoder("utf-8").decode(bytes);
}

async function onImportFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  try {
    const bytes = new Uint8Array(await file.arrayBuffer());
    importText.value = decodeFileForPreview(bytes);
    importFileBlob.value = file.slice();
    importFileName.value = file.name;
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

function onImportTextInput() {
  importFileBlob.value = null;
  importFileName.value = "";
}

function importPayload(): BodyInit {
  return importFileBlob.value ?? importText.value;
}

async function previewImport() {
  if (!importText.value.trim()) {
    app.toast(t("accounts.importEmpty"), "error");
    return;
  }
  importing.value = true;
  try {
    importPreview.value = await api.previewImport(importPayload());
    importStage.value = "preview";
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    importing.value = false;
  }
}

async function commitImport() {
  if (!importPreview.value || importPreview.value.summary.conflict > 0) return;
  importing.value = true;
  try {
    importResult.value = await api.commitImport(
      importPayload(),
      importPreview.value.content_sha256,
      validateAfterImport.value,
    );
    importStage.value = "result";
    await load();
    await app.refreshStatus();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    importing.value = false;
  }
}

async function load() {
  try {
		const [acc, prox, settings] = await Promise.all([api.listAccounts(), api.listProxies(), api.getSettings()]);
    accounts.value = acc.accounts || [];
    usage.value = acc.usage || {};
    proxies.value = prox.proxies || [];
		accountStrategy.value = settings.account_strategy;
    if (!testModel.value) testModel.value = settings.default_model;
    if (testModel.value && !modelOptions.value.includes(testModel.value)) {
      modelOptions.value.unshift(testModel.value);
    }
    const cloudStatus = await api.cloudStatus().catch(() => null);
    cloudAuthenticated.value = Boolean(cloudStatus?.authenticated);
    if (cloudAuthenticated.value) {
      cloudShares.value = (await api.cloudShares()).shares || [];
    } else {
      cloudShares.value = [];
    }
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

function sharesForAccount(account: Account): CloudShare[] {
  return cloudShares.value.filter((share) => share.account_uid === account.client_uid);
}

function openShare(account: Account) {
  shareTarget.value = account;
  shareQuota.value = 0;
  shareExpires.value = "";
  shareConsent.value = false;
  createdShare.value = null;
  shareUsage.value = [];
  shareUsageTarget.value = null;
  shareOpen.value = true;
}

function closeShare() {
  if (shareBusy.value) return;
  shareOpen.value = false;
  shareTarget.value = null;
  createdShare.value = null;
  shareUsage.value = [];
  shareUsageTarget.value = null;
}

async function createCloudShare() {
  if (!shareTarget.value || !shareConsent.value || shareQuota.value < 0 || shareQuota.value > 1_000_000) {
    app.toast(t("accounts.shareIncomplete"), "error");
    return;
  }
  let expiresAt = "";
  if (shareExpires.value) {
    const expiry = new Date(`${shareExpires.value}T23:59:59`);
    if (Number.isNaN(expiry.getTime()) || expiry.getTime() <= Date.now()) {
      app.toast(t("accounts.shareExpiryInvalid"), "error");
      return;
    }
    expiresAt = expiry.toISOString();
  }
  shareBusy.value = "create";
  try {
    createdShare.value = await api.cloudCreateShare({
      account_id: shareTarget.value.id,
      quota_requests: shareQuota.value,
      expires_at: expiresAt,
      consent: true,
    });
    cloudShares.value = (await api.cloudShares()).shares || [];
    shareConsent.value = false;
    app.toast(t("accounts.shareCreated"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    shareBusy.value = "";
  }
}

async function toggleCloudShare(share: CloudShare) {
  shareBusy.value = `toggle-${share.id}`;
  try {
    await api.cloudUpdateShare(share.id, { revoked: !share.revoked });
    cloudShares.value = (await api.cloudShares()).shares || [];
    app.toast(t(share.revoked ? "accounts.shareRestored" : "accounts.shareRevoked"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    shareBusy.value = "";
  }
}

async function loadShareUsage(share: CloudShare) {
  shareBusy.value = `usage-${share.id}`;
  try {
    shareUsageTarget.value = share;
    shareUsage.value = (await api.cloudShareUsage(share.id)).usage || [];
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    shareBusy.value = "";
  }
}

async function copyShareValue(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    app.toast(t("common.copied"), "success");
  } catch {
    app.toast(t("accounts.copyFailed"), "error");
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
        if (!r.models.includes(testModel.value)) testModel.value = r.default_test_model || r.models[0];
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
				<p class="faint text-sm">{{ t(`accounts.strategy.${accountStrategy}`) }}</p>
      </div>
      <div class="flex gap-8" style="flex-wrap: wrap; justify-content: flex-end">
        <button class="btn btn-ghost" @click="openAPIKey">
          <Icon name="key" :size="16" /> {{ t("accounts.addAPIKey") }}
        </button>
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
      <button class="btn btn-ghost mt-8" @click="openAPIKey">
        <Icon name="key" :size="16" /> {{ t("accounts.addAPIKey") }}
      </button>
    </div>

    <div v-else class="grid grid-2">
      <div v-for="a in accounts" :key="a.id" class="card">
        <div class="row-between" style="margin-bottom: 12px">
          <div class="flex items-center gap-12" style="min-width: 0; flex: 1">
            <div class="brand-logo" style="flex-shrink: 0; background: linear-gradient(135deg, #d97757, #b8532f)">
              {{ (a.email || "?").charAt(0).toUpperCase() }}
            </div>
            <div style="min-width: 0">
              <div style="font-weight: 600">{{ a.email || t("common.unknown") }}</div>
              <div class="flex items-center gap-8" style="margin-top: 3px">
                <span class="badge badge-neutral">{{ t(`accounts.accountType.${a.account_type}`) }}</span>
                <span class="faint text-sm" style="min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ a.account_type === "api_key" ? a.base_url : (a.plan_type || "ChatGPT") }}
                </span>
              </div>
            </div>
          </div>
          <span class="badge" style="flex-shrink: 0" :class="statusBadge(a.status)">
            <span class="badge-dot"></span>
            {{ t("accounts.status." + a.status) }}
          </span>
        </div>

        <div
          v-if="a.status === 'refresh_failed'"
          class="text-sm"
          style="color: var(--danger); margin-bottom: 10px; overflow-wrap: anywhere"
        >
          {{ a.status_reason || "" }}
        </div>

        <div class="list" style="margin-bottom: 12px">
          <div v-if="a.account_type === 'oauth'" class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.expiresAt") }}</span>
            <span class="text-sm">{{ fmtDate(a.expires_at) }}</span>
          </div>
          <div v-if="a.account_type === 'oauth'" class="list-row" style="padding: 7px 0">
            <span class="faint text-sm" style="flex: 1">{{ t("accounts.lastUsed") }}</span>
            <span class="text-sm">{{ fmtDate(a.last_used_at) }}</span>
          </div>
					<div class="list-row" style="padding: 7px 0">
						<span class="faint text-sm" style="flex: 1">{{ t("accounts.lastSuccess") }}</span>
						<span class="text-sm">{{ fmtDate(a.last_success_at) }}</span>
					</div>
					<div v-if="a.next_retry_at" class="list-row" style="padding: 7px 0">
						<span class="faint text-sm" style="flex: 1">{{ t("accounts.nextRetry") }}</span>
						<span class="text-sm">{{ fmtDate(a.next_retry_at) }}</span>
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

        <div v-if="a.account_type === 'oauth' && a.codex_usage" class="usage-windows">
          <div v-for="(w, wi) in usageWindows(a.codex_usage)" :key="wi" class="usage-window">
            <div class="usage-window-head">
              <span class="faint text-sm">{{ w.label }}</span>
              <span class="text-sm mono">{{ pctLabel(w.used) }}</span>
            </div>
            <div class="usage-bar">
              <div class="usage-fill" :class="usageBarClass(w.used)" :style="{ width: pct(w.used) + '%' }"></div>
            </div>
            <div v-if="fmtReset(w.reset)" class="faint text-xs" style="margin-top: 3px">
              {{ t("accounts.resetIn", { time: fmtReset(w.reset) }) }}
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
          <button v-if="cloudAuthenticated" class="btn btn-ghost btn-sm" style="flex: 1" data-test="account-share" @click="openShare(a)">
            <Icon name="link" :size="14" /> {{ t("accounts.share") }}<span v-if="sharesForAccount(a).length">· {{ sharesForAccount(a).length }}</span>
          </button>
          <button
            v-if="a.account_type === 'oauth' && a.status === 'refresh_failed'"
            class="btn btn-primary btn-sm"
            style="flex: 1"
            @click="reLogin"
          >
            <Icon name="refresh" :size="14" /> {{ t("accounts.relogin") }}
          </button>
          <button v-else-if="a.account_type === 'oauth'" class="btn btn-ghost btn-sm" style="flex: 1" @click="refreshToken(a)">
            <Icon name="refresh" :size="14" /> {{ t("common.refresh") }}
          </button>
          <button class="btn btn-danger btn-sm" @click="deleteTarget = a">
            <Icon name="trash" :size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- Cloud share modal -->
    <Teleport to="body">
      <div v-if="shareOpen && shareTarget" class="modal-backdrop" @click.self="closeShare">
        <div class="modal share-modal" role="dialog" aria-modal="true" tabindex="-1" @keydown.esc="closeShare">
          <h3 class="modal-title">{{ t("accounts.shareTitle") }}</h3>
          <p class="modal-desc">{{ shareTarget.email }}</p>

          <div class="share-custody-warning" role="note">
            <Icon name="warn" :size="18" />
            <div><strong>{{ t("accounts.shareCustodyTitle") }}</strong><p>{{ t("accounts.shareCustodyDesc") }}</p></div>
          </div>

          <div v-if="createdShare" class="share-created" data-test="share-created">
            <div><strong>{{ t("accounts.shareKeyOnce") }}</strong><span>{{ t("accounts.shareKeyOnceDesc") }}</span></div>
            <label class="field"><span class="field-label">Base URL</span><div class="code-box"><span>{{ createdShare.share.base_url }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.share.base_url)"><Icon name="copy" :size="14" /></button></div></label>
            <label class="field"><span class="field-label">API Key</span><div class="code-box"><span data-test="share-guest-key">{{ createdShare.guest_key }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.guest_key)"><Icon name="copy" :size="14" /></button></div></label>
            <label class="field"><span class="field-label">{{ t("accounts.shareCode") }}</span><div class="code-box"><span>{{ createdShare.share.share_code }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.share.share_code)"><Icon name="copy" :size="14" /></button></div></label>
          </div>

          <div class="share-create-form">
            <label class="field"><span class="field-label">{{ t("accounts.shareQuota") }}</span><input v-model.number="shareQuota" class="input" type="number" min="0" max="1000000" /><small>{{ t("accounts.shareQuotaHint") }}</small></label>
            <label class="field"><span class="field-label">{{ t("accounts.shareExpires") }}</span><input v-model="shareExpires" class="input" type="date" /></label>
            <label class="share-consent"><input v-model="shareConsent" type="checkbox" /><span>{{ t("accounts.shareConsent") }}</span></label>
            <button class="btn btn-primary" data-test="share-create" type="button" :disabled="shareBusy !== '' || !shareConsent" @click="createCloudShare"><Icon name="link" :size="14" />{{ shareBusy === "create" ? t("accounts.shareCreating") : t("accounts.shareCreate") }}</button>
          </div>

          <div v-if="sharesForAccount(shareTarget).length" class="share-existing">
            <h4>{{ t("accounts.shareExisting") }}</h4>
            <article v-for="share in sharesForAccount(shareTarget)" :key="share.id">
              <div class="share-row-main"><span class="badge" :class="share.revoked ? 'badge-danger' : 'badge-success'">{{ t(share.revoked ? "accounts.shareStatusRevoked" : "accounts.shareStatusActive") }}</span><strong class="mono">{{ share.share_code }}</strong><span>{{ share.used_requests }} / {{ share.quota_requests || "∞" }}</span><small>{{ fmtDate(share.expires_at) }}</small></div>
              <div class="share-row-actions"><button class="btn btn-ghost btn-sm" type="button" :disabled="shareBusy !== ''" @click="loadShareUsage(share)"><Icon name="statistics" :size="13" />{{ t("accounts.shareUsage") }}</button><button class="btn btn-sm" :class="share.revoked ? 'btn-ghost' : 'btn-danger'" type="button" :disabled="shareBusy !== ''" @click="toggleCloudShare(share)">{{ t(share.revoked ? "accounts.shareRestore" : "accounts.shareRevoke") }}</button></div>
            </article>
          </div>

          <div v-if="shareUsageTarget" class="share-usage">
            <h4>{{ t("accounts.shareUsageTitle", { code: shareUsageTarget.share_code }) }}</h4>
            <div v-if="shareUsage.length === 0" class="faint text-sm">{{ t("accounts.shareNoUsage") }}</div>
            <div v-for="entry in shareUsage" :key="entry.id" class="share-usage-row"><span>{{ fmtDate(entry.ts) }}</span><strong>{{ entry.model || "-" }}</strong><span :class="entry.status >= 400 ? 'text-danger' : 'text-success'">HTTP {{ entry.status }}</span><span>{{ entry.latency_ms }}ms</span></div>
          </div>

          <div class="modal-actions"><button class="btn btn-ghost" :disabled="shareBusy !== ''" @click="closeShare">{{ t("common.close") }}</button></div>
        </div>
      </div>
    </Teleport>

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

    <!-- API-key account modal -->
    <Teleport to="body">
      <div v-if="apiKeyOpen" class="modal-backdrop" @click.self="closeAPIKey">
        <div class="modal" role="dialog" aria-modal="true" tabindex="-1" @keydown.esc="closeAPIKey">
          <h3 class="modal-title">{{ t("accounts.addAPIKey") }}</h3>
          <p class="modal-desc">{{ t("accounts.apiKeyHint") }}</p>
          <div class="field">
            <label class="field-label">{{ t("accounts.apiKeyBaseURL") }}</label>
            <input v-model="apiKeyBaseURL" class="input mono" type="url" spellcheck="false" />
          </div>
          <div class="field">
            <label class="field-label">{{ t("accounts.apiKey") }}</label>
            <input v-model="apiKeyValue" class="input mono" type="password" autocomplete="new-password" spellcheck="false" />
          </div>
          <div class="field">
            <label class="field-label">{{ t("accounts.apiKeyName") }}</label>
            <input v-model="apiKeyName" class="input" type="text" :placeholder="t('accounts.apiKeyNamePlaceholder')" />
          </div>
          <div class="modal-actions">
            <button class="btn btn-ghost" :disabled="apiKeyAdding" @click="closeAPIKey">{{ t("common.cancel") }}</button>
            <button class="btn btn-primary" :disabled="apiKeyAdding" @click="addAPIKey">
              <Icon v-if="apiKeyAdding" name="refresh" class="spin" :size="14" />
              <Icon v-else name="key" :size="14" />
              {{ t("common.add") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Import modal -->
    <Teleport to="body">
      <div v-if="importOpen" class="modal-backdrop" @click.self="closeImport">
        <div class="modal import-modal" role="dialog" aria-modal="true" tabindex="-1" @keydown.esc="closeImport">
          <div class="row-between">
            <h3 class="modal-title">{{ t("accounts.import") }}</h3>
            <span class="badge badge-neutral">{{ t(`accounts.importStage.${importStage}`) }}</span>
          </div>

          <template v-if="importStage === 'input'">
            <p class="modal-desc">{{ t("accounts.importHint") }}</p>
            <div class="flex items-center gap-8" style="margin-bottom: 10px">
              <button class="btn btn-ghost btn-sm" @click="importFileInput?.click()">
                <Icon name="upload" :size="14" /> {{ t("accounts.importChooseFile") }}
              </button>
              <span v-if="importFileName" class="faint text-sm import-file-name">{{ importFileName }}</span>
              <input
                ref="importFileInput"
                type="file"
                accept="application/json,.json,.jsonl,text/plain"
                style="display: none"
                @change="onImportFile"
              />
            </div>
            <textarea
              v-model="importText"
              class="input import-textarea"
              rows="12"
              spellcheck="false"
              :placeholder="importExample"
              @input="onImportTextInput"
            ></textarea>
            <div class="modal-actions">
              <button class="btn btn-ghost" @click="closeImport">{{ t("common.cancel") }}</button>
              <button class="btn btn-primary" :disabled="importing" @click="previewImport">
                <Icon v-if="importing" name="refresh" class="spin" :size="14" />
                {{ t("accounts.importPreview") }}
              </button>
            </div>
          </template>

          <template v-else-if="importStage === 'preview' && importPreview">
            <div class="import-summary">
              <span class="badge badge-success">{{ t("accounts.importAction.create") }} {{ importPreview.summary.create }}</span>
              <span class="badge badge-neutral">{{ t("accounts.importAction.update") }} {{ importPreview.summary.update }}</span>
              <span class="badge badge-warn">{{ t("accounts.importAction.skip") }} {{ importPreview.summary.skip }}</span>
              <span v-if="importPreview.summary.error" class="badge badge-danger">{{ t("accounts.importAction.error") }} {{ importPreview.summary.error }}</span>
              <span v-if="importPreview.summary.conflict" class="badge badge-danger">{{ t("accounts.importAction.conflict") }} {{ importPreview.summary.conflict }}</span>
            </div>
            <div class="import-table-wrap">
              <table class="table import-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>{{ t("accounts.importActionLabel") }}</th>
                    <th>{{ t("accounts.importAccount") }}</th>
                    <th>{{ t("accounts.importCredentials") }}</th>
                    <th>{{ t("accounts.importMessage") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in importPreview.rows" :key="row.index">
                    <td>{{ row.index }}</td>
                    <td><span class="badge" :class="`import-action-${row.action}`">{{ t(`accounts.importAction.${row.action}`) }}</span></td>
                    <td>
                      <div>{{ row.email_masked || row.chatgpt_account_id_masked || "-" }}</div>
                      <small v-if="row.account_type === 'api_key'" class="text-success">{{ t("accounts.accountType.api_key") }}</small>
                      <small v-else-if="row.identity_verified" class="text-success">{{ t("accounts.importVerified") }}</small>
                      <small v-else class="faint">{{ t("accounts.importPending") }}</small>
                    </td>
                    <td class="import-credentials">
                      <span :class="{ active: row.has_access_token }">A</span>
                      <span :class="{ active: row.has_refresh_token }">R</span>
                      <span :class="{ active: row.has_id_token }">I</span>
                      <span :class="{ active: row.has_api_key }">K</span>
                    </td>
                    <td class="import-message">
                      <div v-if="row.error_message" class="text-danger">{{ row.error_message }}</div>
                      <div v-for="code in row.warning_codes" :key="code">{{ t(`accounts.importWarning.${code}`) }}</div>
                      <div v-for="warning in row.warnings" :key="warning">{{ warning }}</div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <label class="import-validate">
              <input v-model="validateAfterImport" type="checkbox" />
              <span>{{ t("accounts.importValidate") }}</span>
            </label>
            <div class="modal-actions">
              <button class="btn btn-ghost" :disabled="importing" @click="importStage = 'input'">{{ t("common.edit") }}</button>
              <button class="btn btn-primary" :disabled="importing || importPreview.summary.conflict > 0" @click="commitImport">
                <Icon v-if="importing" name="refresh" class="spin" :size="14" />
                {{ t("accounts.importConfirm", { count: importPreview.summary.create + importPreview.summary.update }) }}
              </button>
            </div>
          </template>

          <template v-else-if="importStage === 'result' && importResult">
            <div class="import-result">
              <Icon name="check" :size="28" />
              <h4>{{ t("accounts.importComplete") }}</h4>
              <p>{{ t("accounts.importResult", { imported: importResult.imported, updated: importResult.updated, skipped: importResult.skipped + importResult.failed }) }}</p>
              <p v-if="importResult.validated" class="faint">{{ t("accounts.importValidated", { count: importResult.validated }) }}</p>
            </div>
            <div v-if="importResult.warnings?.length" class="import-result-warnings">
              <div v-for="warning in importResult.warnings" :key="warning">{{ warning }}</div>
            </div>
            <div class="modal-actions">
              <button class="btn btn-primary" @click="closeImport">{{ t("common.close") }}</button>
            </div>
          </template>
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

<style scoped>
.share-modal { max-width: 720px; max-height: min(88vh, 820px); overflow-y: auto; }
.share-custody-warning { display: flex; gap: 10px; margin: 14px 0; padding: 12px 14px; border: 1px solid rgba(193, 134, 58, .3); border-radius: 8px; background: var(--warn-soft); color: var(--warn); }
.share-custody-warning p { margin: 3px 0 0; color: var(--text-dim); line-height: 1.5; }
.share-created { display: grid; gap: 10px; padding: 14px; border: 1px solid rgba(63, 143, 95, .28); border-radius: 8px; background: var(--success-soft); }
.share-created > div:first-child { display: grid; gap: 3px; color: var(--success); }
.share-created .field { margin: 0; }
.share-create-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; align-items: end; margin-top: 16px; }
.share-create-form .field { margin: 0; }
.share-create-form small { color: var(--text-faint); }
.share-consent { grid-column: 1 / -1; display: flex; align-items: flex-start; gap: 9px; color: var(--text-dim); cursor: pointer; }
.share-create-form > .btn { justify-self: start; }
.share-existing, .share-usage { margin-top: 18px; border-top: 1px solid var(--border-soft); }
.share-existing h4, .share-usage h4 { margin: 14px 0 8px; font-size: 13px; }
.share-existing article { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border-soft); }
.share-row-main { min-width: 0; display: flex; align-items: center; gap: 9px; flex-wrap: wrap; }
.share-row-main small { color: var(--text-faint); }
.share-row-actions { display: flex; gap: 6px; flex-shrink: 0; }
.share-usage-row { display: grid; grid-template-columns: minmax(150px, 1.3fr) minmax(100px, 1fr) auto auto; gap: 10px; padding: 8px 0; border-bottom: 1px solid var(--border-soft); font-size: 12px; }
@media (max-width: 720px) {
  .share-create-form { grid-template-columns: minmax(0, 1fr); }
  .share-existing article { align-items: stretch; flex-direction: column; }
  .share-usage-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
