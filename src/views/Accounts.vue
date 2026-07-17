<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import EmptyState from "../components/EmptyState.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import ShareQRCode from "../components/ShareQRCode.vue";
import AccountProxySelect from "../components/AccountProxySelect.vue";
import {
  api,
  type Account,
  type AccountRuntimeState,
  type AccountUsage,
  type AccountTestResult,
  type CloudShare,
  type CloudShareUsage,
  type ImportCommitResult,
  type ImportPreview,
  type ImportProxyOptions,
  type Proxy,
	type Settings,
} from "../api/control";
import { exactTokens, formatTokens } from "../format";
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
const oauthProxyOpen = ref(false);
const oauthProxyID = ref<number | null>(null);
const oauthStarting = ref(false);
let oauthPollTimer: number | undefined;
let runtimePollTimer: number | undefined;
let runtimePollPending = false;
let pageMounted = false;

// delete flow
const deleteTarget = ref<Account | null>(null);

// import flow
const importChooserOpen = ref(false);
const importOpen = ref(false);
const importText = ref("");
const importing = ref(false);
const importFileNames = ref<string[]>([]);
const importFileSize = ref(0);
const importFileInput = ref<HTMLInputElement | null>(null);
const importStage = ref<"input" | "preview" | "result">("input");
const importPreview = ref<ImportPreview | null>(null);
const importResult = ref<ImportCommitResult | null>(null);
const validateAfterImport = ref(true);
const importProxyMode = ref<ImportProxyOptions["mode"]>("preserve");
const importProxyID = ref<number | null>(null);

// API-key account flow
const apiKeyOpen = ref(false);
const apiKeyAdding = ref(false);
const apiKeyName = ref("");
const apiKeyBaseURL = ref("https://chatgpt.com/backend-api/codex/responses");
const apiKeyValue = ref("");
const apiKeyProxyID = ref<number | null>(null);

// account detail and scheduling controls
const detailTarget = ref<Account | null>(null);
const limitsMaxConcurrency = ref(3);
const limitsQueueCapacity = ref(20);
const limitsSaving = ref(false);
const accountToggling = ref<Record<number, boolean>>({});

const importExample = `[
  {
    "email": "you@example.com",
    "access_token": "",
    "refresh_token": "",
    "id_token": ""
  }
]`;

function openImportChooser() {
	importChooserOpen.value = true;
}

function openImport() {
	importChooserOpen.value = false;
  importText.value = "";
  importFileNames.value = [];
  importFileSize.value = 0;
  importStage.value = "input";
  importPreview.value = null;
  importResult.value = null;
  validateAfterImport.value = true;
  importProxyMode.value = "preserve";
  importProxyID.value = null;
  if (importFileInput.value) importFileInput.value.value = "";
  importOpen.value = true;
}

function closeImport() {
  if (importing.value) return;
  importOpen.value = false;
}

function openAPIKey() {
	importChooserOpen.value = false;
  apiKeyName.value = "";
  apiKeyBaseURL.value = "https://chatgpt.com/backend-api/codex/responses";
  apiKeyValue.value = "";
  apiKeyProxyID.value = null;
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
      proxy_id: apiKeyProxyID.value,
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
  const files = Array.from(input.files ?? []);
  if (!files.length) return;
  try {
		const contents: string[] = [];
		let totalSize = 0;
		for (const file of files) {
			const bytes = new Uint8Array(await file.arrayBuffer());
			contents.push(decodeFileForPreview(bytes));
			totalSize += file.size;
		}
		importText.value = contents.join("\n");
		importFileNames.value = files.map((file) => file.name);
		importFileSize.value = totalSize;
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

function onImportTextInput() {
	importFileNames.value = [];
	importFileSize.value = 0;
}

function importPayload(): BodyInit {
	return importText.value;
}

function currentImportProxyOptions(): ImportProxyOptions {
  return {
    mode: importProxyMode.value,
    proxyId: importProxyMode.value === "override" ? importProxyID.value : null,
  };
}

function previewProxyLabel(proxyID?: number, specified = false) {
  if (!specified) return t("accounts.importProxyPreserveShort");
  if (!proxyID) return t("accounts.noProxy");
  return proxies.value.find((proxy) => proxy.id === proxyID)?.name ?? t("accounts.importProxyMissing", { id: proxyID });
}

async function previewImport() {
  if (!importText.value.trim()) {
    app.toast(t("accounts.importEmpty"), "error");
    return;
  }
  if (importProxyMode.value === "override" && !importProxyID.value) {
    app.toast(t("accounts.importProxyRequired"), "error");
    return;
  }
  importing.value = true;
  try {
    importPreview.value = await api.previewImport(importPayload(), currentImportProxyOptions());
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
      currentImportProxyOptions(),
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
		if (detailTarget.value) {
			const refreshed = accounts.value.find((account) => account.id === detailTarget.value?.id) ?? null;
			detailTarget.value = refreshed;
			if (refreshed) {
				limitsMaxConcurrency.value = refreshed.max_concurrency ?? 3;
				limitsQueueCapacity.value = refreshed.queue_capacity ?? 20;
			}
		}
		accountStrategy.value = settings.account_strategy;
    if (!testModel.value) testModel.value = settings.default_model;
    if (testModel.value && !modelOptions.value.includes(testModel.value)) {
      modelOptions.value.unshift(testModel.value);
    }
    const cloudStatus = await api.cloudStatus().catch(() => null);
    cloudAuthenticated.value = Boolean(cloudStatus?.authenticated);
    if (cloudAuthenticated.value) {
      try {
        cloudShares.value = (await api.cloudShares()).shares || [];
      } catch {
        cloudShares.value = [];
        const refreshedStatus = await api.cloudStatus().catch(() => null);
        cloudAuthenticated.value = Boolean(refreshedStatus?.authenticated);
      }
    } else {
      cloudShares.value = [];
    }
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

function mergeRuntimeStates(states: AccountRuntimeState[]) {
  const byID = new Map(states.map((state) => [state.id, state]));
  accounts.value = accounts.value.map((account) => {
    const state = byID.get(account.id);
    if (!state) return account;
    return {
      ...account,
      status: state.status,
      status_reason: state.status_reason || undefined,
      rate_limited_until: state.rate_limited_until ?? null,
      in_flight: state.in_flight,
      waiting: state.waiting,
    };
  });
  if (detailTarget.value) {
    detailTarget.value = accounts.value.find((account) => account.id === detailTarget.value?.id) ?? null;
  }
}

function scheduleRuntimePoll(delay = 1000) {
  window.clearTimeout(runtimePollTimer);
  if (!pageMounted || document.hidden) return;
  runtimePollTimer = window.setTimeout(pollRuntime, delay);
}

async function pollRuntime() {
  if (!pageMounted || document.hidden || runtimePollPending) return;
  runtimePollPending = true;
  try {
    const response = await api.accountRuntime();
    mergeRuntimeStates(response.accounts || []);
  } catch {
    // The full account load remains available if this lightweight refresh is temporarily unavailable.
  } finally {
    runtimePollPending = false;
    scheduleRuntimePoll();
  }
}

function onVisibilityChange() {
  if (document.hidden) {
    window.clearTimeout(runtimePollTimer);
    return;
  }
  void pollRuntime();
}

function sharesForAccount(account: Account): CloudShare[] {
  return cloudShares.value.filter((share) => share.account_uid === account.client_uid);
}

function openShare(account: Account) {
	detailTarget.value = null;
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
  if (shareTarget.value?.account_type === "oauth") {
    app.toast(t("accounts.oauthShareRequiresDevice"), "warn");
    return;
  }
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

async function copyTestDetails(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    app.toast(t("accounts.testErrorCopied"), "success");
  } catch {
    app.toast(t("accounts.copyFailed"), "error");
  }
}

function openTest(a: Account) {
	detailTarget.value = null;
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

function openDetails(account: Account) {
	detailTarget.value = account;
	limitsMaxConcurrency.value = account.max_concurrency ?? 3;
	limitsQueueCapacity.value = account.queue_capacity ?? 20;
}

function closeDetails() {
	if (limitsSaving.value) return;
	detailTarget.value = null;
}

async function toggleAccount(account: Account, enabled: boolean) {
	accountToggling.value[account.id] = true;
	try {
		await api.setAccountStatus(account.id, enabled ? "active" : "disabled");
		app.toast(t(enabled ? "accounts.accountEnabled" : "accounts.accountDisabled"), "success");
		await load();
		await app.refreshStatus();
	} catch (error) {
		app.toast((error as Error).message, "error");
	} finally {
		accountToggling.value[account.id] = false;
	}
}

async function saveAccountLimits() {
	if (!detailTarget.value) return;
	if (limitsMaxConcurrency.value < 1 || limitsMaxConcurrency.value > 100 || limitsQueueCapacity.value < 0 || limitsQueueCapacity.value > 1000) {
		app.toast(t("accounts.limitsInvalid"), "error");
		return;
	}
	limitsSaving.value = true;
	try {
		const result = await api.setAccountLimits(detailTarget.value.id, {
			max_concurrency: limitsMaxConcurrency.value,
			queue_capacity: limitsQueueCapacity.value,
		});
		detailTarget.value = result.account;
		app.toast(t("accounts.limitsSaved"), "success");
		await load();
	} catch (error) {
		app.toast((error as Error).message, "error");
	} finally {
		limitsSaving.value = false;
	}
}

function fmtCost(n?: number) {
  if (!n) return "$0.0000";
  return "$" + n.toFixed(4);
}

function openOAuthProxy() {
	importChooserOpen.value = false;
  oauthProxyID.value = null;
  oauthProxyOpen.value = true;
}

async function startLogin(proxyID: number | null = oauthProxyID.value) {
	importChooserOpen.value = false;
  loginError.value = "";
  oauthStarting.value = true;
  try {
    const r = await api.oauthStart(proxyID);
    loginUrl.value = r.auth_url;
    loginState.value = r.state;
    oauthProxyOpen.value = false;
    loginOpen.value = true;
    openUrl(r.auth_url);
    beginPoll();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    oauthStarting.value = false;
  }
}

function beginPoll() {
  clearInterval(oauthPollTimer);
  oauthPollTimer = window.setInterval(async () => {
    try {
      const r = await api.oauthPoll(loginState.value);
      if (r.done) {
        clearInterval(oauthPollTimer);
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
  clearInterval(oauthPollTimer);
  loginOpen.value = false;
}

async function confirmDelete() {
  if (!deleteTarget.value) return;
  try {
    await api.deleteAccount(deleteTarget.value.id);
		if (detailTarget.value?.id === deleteTarget.value.id) detailTarget.value = null;
    app.toast(t("common.delete") + " ✓", "success");
    deleteTarget.value = null;
    await load();
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function reLogin(account: Account) {
  await startLogin(account.proxy_id ?? null);
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

function accountStatusReason(account: Account) {
	if (!account.status_reason && account.status === "rate_limited" && account.rate_limited_until) {
		const seconds = Math.max(0, Math.ceil((new Date(account.rate_limited_until).getTime() - Date.now()) / 1000));
		return seconds > 0 ? t("accounts.statusReason.rate_limited_until", { time: fmtReset(seconds) }) : "";
	}
	if (!account.status_reason) return "";
	if (account.status_reason === "transient_rate_limit") {
		const seconds = account.rate_limited_until
			? Math.max(0, Math.ceil((new Date(account.rate_limited_until).getTime() - Date.now()) / 1000))
			: 0;
		return seconds > 0
			? t("accounts.statusReason.transient_rate_limit_until", { time: fmtReset(seconds) })
			: t("accounts.statusReason.transient_rate_limit");
	}
	if (["manually_disabled", "auto_disabled_auth_failures", "auto_disabled_account_inactive", "transient_rate_limit", "quota_exhausted"].includes(account.status_reason)) {
		return t(`accounts.statusReason.${account.status_reason}`);
	}
	return account.status_reason;
}

onMounted(() => {
  pageMounted = true;
  document.addEventListener("visibilitychange", onVisibilityChange);
  void load().then(() => pollRuntime());
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
onUnmounted(() => {
  pageMounted = false;
  clearInterval(oauthPollTimer);
  clearTimeout(runtimePollTimer);
  document.removeEventListener("visibilitychange", onVisibilityChange);
});
</script>

<template>
  <div class="accounts-page">
    <div class="page-header row-between">
      <div>
        <h1 class="page-title">{{ t("accounts.title") }}</h1>
        <p class="page-desc">{{ t("accounts.desc") }}</p>
				<p class="faint text-sm">{{ t(`accounts.strategy.${accountStrategy}`) }}</p>
      </div>
      <button class="btn btn-primary" data-test="account-import-open" @click="openImportChooser">
        <Icon name="upload" :size="16" /> {{ t("accounts.importAccountTitle") }}
      </button>
    </div>

    <SkeletonBlock v-if="loading" :cards="2" :rows="4" />

    <div v-else-if="accounts.length === 0" class="card">
      <EmptyState icon="accounts" :title="t('accounts.empty')" :description="t('accounts.emptyDesc')">
        <button class="btn btn-primary" @click="openImportChooser"><Icon name="upload" :size="16" /> {{ t("accounts.importAccountTitle") }}</button>
      </EmptyState>
    </div>

    <div v-else class="account-list" data-test="account-list">
      <article v-for="a in accounts" :key="a.id" class="account-row" :class="{ 'is-disabled': a.status === 'disabled' }" data-test="account-row">
        <div class="account-identity">
          <div class="brand-logo account-avatar">{{ (a.email || "?").charAt(0).toUpperCase() }}</div>
          <div class="account-name-wrap">
            <strong :title="a.email || t('common.unknown')">{{ a.email || t("common.unknown") }}</strong>
            <div class="account-subline">
              <span class="badge badge-neutral">{{ t(`accounts.accountType.${a.account_type}`) }}</span>
              <span class="account-subtitle" :title="a.account_type === 'api_key' ? a.base_url : (a.plan_type || 'ChatGPT')">
                {{ a.account_type === "api_key" ? a.base_url : (a.plan_type || "ChatGPT") }}
              </span>
            </div>
          </div>
        </div>
        <div class="account-health">
          <span class="badge" :class="statusBadge(a.status)"><span class="badge-dot"></span>{{ t("accounts.status." + a.status) }}</span>
          <small v-if="accountStatusReason(a)" :title="accountStatusReason(a)">{{ accountStatusReason(a) }}</small>
          <small v-else>{{ t("accounts.lastSuccess") }} · {{ fmtDate(a.last_success_at) }}</small>
        </div>
        <div class="account-metric">
          <span>{{ t("accounts.tokensUsed") }}</span>
          <strong class="mono" :title="`${exactTokens(usage[a.id]?.total_tokens)} tokens`">{{ formatTokens(usage[a.id]?.total_tokens) }}</strong>
        </div>
        <div class="account-metric">
          <span>{{ t("accounts.estCost") }}</span>
          <strong class="mono">{{ fmtCost(usage[a.id]?.cost_usd) }}</strong>
        </div>
        <div class="account-load">
          <span>{{ t("accounts.concurrentLoad", { current: a.in_flight ?? 0, max: a.max_concurrency ?? 3 }) }}</span>
          <span>{{ t("accounts.queueLoad", { current: a.waiting ?? 0, max: a.queue_capacity ?? 20 }) }}</span>
        </div>
        <div class="account-row-actions">
          <label class="switch account-switch" :title="t(a.status === 'disabled' ? 'accounts.enableAccount' : 'accounts.disableAccount')">
            <input
              type="checkbox"
              :aria-label="t(a.status === 'disabled' ? 'accounts.enableAccount' : 'accounts.disableAccount')"
              :checked="a.status !== 'disabled'"
              :disabled="accountToggling[a.id]"
              @change="toggleAccount(a, ($event.target as HTMLInputElement).checked)"
            />
            <span class="slider"></span>
          </label>
          <button class="btn btn-ghost btn-sm" data-test="account-test" @click="openTest(a)">
            <Icon name="bolt" :size="14" /> {{ t("accounts.test") }}
          </button>
          <button class="btn btn-ghost btn-sm" data-test="account-details" @click="openDetails(a)">
            <Icon name="info" :size="14" /> {{ t("accounts.viewDetails") }}
          </button>
        </div>
      </article>
    </div>

    <!-- Import method chooser -->
    <Teleport to="body">
      <div v-if="importChooserOpen" class="modal-backdrop" @click.self="importChooserOpen = false">
        <div class="modal import-chooser" role="dialog" aria-modal="true" @keydown.esc="importChooserOpen = false">
          <h3 class="modal-title">{{ t("accounts.importAccountTitle") }}</h3>
          <p class="modal-desc">{{ t("accounts.importMethodHint") }}</p>
          <div class="import-methods">
            <button type="button" data-test="import-method-api" @click="openAPIKey">
              <Icon name="key" :size="20" /><span><strong>{{ t("accounts.importMethodAPI") }}</strong><small>{{ t("accounts.importMethodAPIDesc") }}</small></span>
            </button>
            <button type="button" data-test="import-method-oauth" @click="openOAuthProxy">
              <Icon name="external" :size="20" /><span><strong>{{ t("accounts.importMethodOAuth") }}</strong><small>{{ t("accounts.importMethodOAuthDesc") }}</small></span>
            </button>
            <button type="button" data-test="import-method-json" @click="openImport">
              <Icon name="upload" :size="20" /><span><strong>{{ t("accounts.importMethodJSON") }}</strong><small>{{ t("accounts.importMethodJSONDesc") }}</small></span>
            </button>
          </div>
          <div class="modal-actions"><button class="btn btn-ghost" @click="importChooserOpen = false">{{ t("common.cancel") }}</button></div>
        </div>
      </div>
    </Teleport>

    <!-- Account details -->
    <Teleport to="body">
      <div v-if="detailTarget" class="modal-backdrop" @click.self="closeDetails">
        <div class="modal account-detail-modal" data-test="account-detail-modal" role="dialog" aria-modal="true" @keydown.esc="closeDetails">
          <div class="account-detail-head">
            <div class="account-identity">
              <div class="brand-logo account-avatar">{{ (detailTarget.email || "?").charAt(0).toUpperCase() }}</div>
              <div class="account-name-wrap"><h3 class="modal-title">{{ detailTarget.email || t("common.unknown") }}</h3><div class="account-subline"><span class="badge badge-neutral">{{ t(`accounts.accountType.${detailTarget.account_type}`) }}</span><span class="badge" :class="statusBadge(detailTarget.status)">{{ t(`accounts.status.${detailTarget.status}`) }}</span></div></div>
            </div>
            <button class="btn btn-ghost btn-sm" data-test="account-detail-close" @click="closeDetails">{{ t("common.close") }}</button>
          </div>
          <div class="account-detail-scroll" data-test="account-detail-scroll">
            <p v-if="accountStatusReason(detailTarget)" class="detail-status-reason">{{ accountStatusReason(detailTarget) }}</p>

            <section class="detail-section">
              <h4>{{ t("accounts.detailIdentity") }}</h4>
              <div class="detail-grid">
                <div><span>{{ t("accounts.plan") }}</span><strong>{{ detailTarget.plan_type || "—" }}</strong></div>
                <div><span>{{ t("accounts.accountId") }}</span><strong class="mono detail-value">{{ detailTarget.chatgpt_account_id || "—" }}</strong></div>
                <div><span>{{ t("accounts.expiresAt") }}</span><strong>{{ fmtDate(detailTarget.expires_at) }}</strong></div>
                <div><span>{{ t("accounts.createdAt") }}</span><strong>{{ fmtDate(detailTarget.created_at) }}</strong></div>
                <div v-if="detailTarget.account_type === 'api_key'" class="detail-wide"><span>Base URL</span><strong class="mono detail-value">{{ detailTarget.base_url }}</strong></div>
              </div>
            </section>

            <section class="detail-section">
              <h4>{{ t("accounts.detailHealth") }}</h4>
              <div class="detail-grid">
                <div><span>{{ t("accounts.lastUsed") }}</span><strong>{{ fmtDate(detailTarget.last_used_at) }}</strong></div>
                <div><span>{{ t("accounts.lastSuccess") }}</span><strong>{{ fmtDate(detailTarget.last_success_at) }}</strong></div>
                <div><span>{{ t("accounts.nextRetry") }}</span><strong>{{ fmtDate(detailTarget.next_retry_at) }}</strong></div>
                <div><span>{{ t("accounts.failureCount") }}</span><strong>{{ detailTarget.consecutive_failures ?? 0 }}</strong></div>
              </div>
            </section>

            <section class="detail-section">
              <h4>{{ t("accounts.detailUsage") }}</h4>
              <div class="usage-stat-grid">
                <div><span>{{ t("statistics.requests") }}</span><strong>{{ usage[detailTarget.id]?.requests ?? 0 }}</strong></div>
                <div><span>{{ t("statistics.totalTokens") }}</span><strong class="mono">{{ formatTokens(usage[detailTarget.id]?.total_tokens) }}</strong></div>
                <div><span>{{ t("accounts.cachedTokens") }}</span><strong class="mono">{{ formatTokens(usage[detailTarget.id]?.cached_tokens) }}</strong></div>
                <div><span>{{ t("accounts.estCost") }}</span><strong class="mono">{{ fmtCost(usage[detailTarget.id]?.cost_usd) }}</strong></div>
              </div>
              <div v-if="detailTarget.account_type === 'oauth' && detailTarget.codex_usage" class="usage-windows detail-usage-windows">
                <div v-for="(w, wi) in usageWindows(detailTarget.codex_usage)" :key="wi" class="usage-window">
                  <div class="usage-window-head"><span class="faint text-sm">{{ w.label }}</span><span class="text-sm mono">{{ pctLabel(w.used) }}</span></div>
                  <div class="usage-bar"><div class="usage-fill" :class="usageBarClass(w.used)" :style="{ width: pct(w.used) + '%' }"></div></div>
                  <div v-if="fmtReset(w.reset)" class="faint text-xs">{{ t("accounts.resetIn", { time: fmtReset(w.reset) }) }}</div>
                </div>
              </div>
            </section>

            <section class="detail-section">
              <div class="detail-section-head"><h4>{{ t("accounts.detailScheduling") }}</h4><span>{{ t("accounts.runtimeLoad", { active: detailTarget.in_flight ?? 0, waiting: detailTarget.waiting ?? 0 }) }}</span></div>
              <div class="limit-grid">
                <label class="field"><span class="field-label">{{ t("accounts.maxConcurrency") }}</span><input v-model.number="limitsMaxConcurrency" class="input" type="number" min="1" max="100" /></label>
                <label class="field"><span class="field-label">{{ t("accounts.queueCapacity") }}</span><input v-model.number="limitsQueueCapacity" class="input" type="number" min="0" max="1000" /></label>
                <button class="btn btn-ghost" :disabled="limitsSaving" @click="saveAccountLimits"><Icon name="check" :size="14" />{{ t("common.save") }}</button>
              </div>
              <label class="field detail-proxy"><span class="field-label">{{ t("accounts.bindProxy") }}</span><select class="select" :value="detailTarget.proxy_id ?? ''" @change="bindProxy(detailTarget, ($event.target as HTMLSelectElement).value ? Number(($event.target as HTMLSelectElement).value) : null)"><option value="">{{ t("accounts.noProxy") }}</option><option v-for="p in proxies" :key="p.id" :value="p.id">{{ p.name }} ({{ p.type }})</option></select></label>
            </section>

            <div class="detail-actions">
              <button v-if="detailTarget.status !== 'active'" class="btn btn-ghost btn-sm" :disabled="resetting[detailTarget.id]" @click="forceReset(detailTarget)"><Icon name="check" :size="14" />{{ t("accounts.forceReset") }}</button>
              <button v-if="cloudAuthenticated" class="btn btn-ghost btn-sm" data-test="account-share" @click="openShare(detailTarget)"><Icon name="link" :size="14" />{{ t("accounts.share") }}</button>
              <button v-if="detailTarget.account_type === 'oauth' && detailTarget.status === 'refresh_failed'" class="btn btn-primary btn-sm" @click="reLogin(detailTarget)"><Icon name="refresh" :size="14" />{{ t("accounts.relogin") }}</button>
              <button v-else-if="detailTarget.account_type === 'oauth'" class="btn btn-ghost btn-sm" @click="refreshToken(detailTarget)"><Icon name="refresh" :size="14" />{{ t("common.refresh") }}</button>
              <button class="btn btn-danger btn-sm detail-delete" :title="t('common.delete')" @click="deleteTarget = detailTarget"><Icon name="trash" :size="14" /></button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Cloud share modal -->
    <Teleport to="body">
      <div v-if="shareOpen && shareTarget" class="modal-backdrop" @click.self="closeShare">
        <div class="modal share-modal" role="dialog" aria-modal="true" tabindex="-1" @keydown.esc="closeShare">
          <h3 class="modal-title">{{ t("accounts.shareTitle") }}</h3>
          <p class="modal-desc">{{ shareTarget.email }}</p>

          <div v-if="shareTarget.account_type === 'oauth'" class="share-custody-warning" role="note">
            <Icon name="warn" :size="18" />
            <div><strong>{{ t("accounts.oauthShareUnavailableTitle") }}</strong><p>{{ t("accounts.oauthShareRequiresDevice") }}</p></div>
          </div>
          <div v-else class="share-custody-warning" role="note">
            <Icon name="warn" :size="18" />
            <div><strong>{{ t("accounts.shareCustodyTitle") }}</strong><p>{{ t("accounts.shareCustodyDesc") }}</p></div>
          </div>

          <div v-if="createdShare" class="share-created" data-test="share-created">
            <div><strong>{{ t("accounts.shareKeyOnce") }}</strong><span>{{ t("accounts.shareKeyOnceDesc") }}</span></div>
            <div class="share-created-content">
              <div class="share-qr-wrap"><ShareQRCode :base-url="createdShare.share.base_url" :guest-key="createdShare.guest_key" :share-code="createdShare.share.share_code" /><small>{{ t("accounts.shareQR") }}</small></div>
              <div class="share-created-fields">
                <label class="field"><span class="field-label">Base URL</span><div class="code-box"><span>{{ createdShare.share.base_url }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.share.base_url)"><Icon name="copy" :size="14" /></button></div></label>
                <label class="field"><span class="field-label">API Key</span><div class="code-box"><span data-test="share-guest-key">{{ createdShare.guest_key }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.guest_key)"><Icon name="copy" :size="14" /></button></div></label>
                <label class="field"><span class="field-label">{{ t("accounts.shareCode") }}</span><div class="code-box"><span>{{ createdShare.share.share_code }}</span><button class="copy-btn" type="button" :title="t('common.copy')" @click="copyShareValue(createdShare.share.share_code)"><Icon name="copy" :size="14" /></button></div></label>
              </div>
            </div>
            <p class="share-access-note">{{ t("accounts.shareAccessHint") }}</p>
          </div>

          <div v-if="shareTarget.account_type !== 'oauth'" class="share-create-form">
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

    <!-- OAuth proxy selection -->
    <Teleport to="body">
      <div v-if="oauthProxyOpen" class="modal-backdrop" @click.self="oauthProxyOpen = false">
        <div class="modal oauth-proxy-modal" data-test="oauth-proxy-modal" role="dialog" aria-modal="true" @keydown.esc="oauthProxyOpen = false">
          <h3 class="modal-title">{{ t("accounts.oauthProxyTitle") }}</h3>
          <p class="modal-desc">{{ t("accounts.oauthProxyHint") }}</p>
          <div class="field">
            <span class="field-label">{{ t("accounts.importProxy") }}</span>
            <AccountProxySelect v-model="oauthProxyID" :proxies="proxies" :disabled="oauthStarting" />
          </div>
          <div class="modal-actions">
            <button class="btn btn-ghost" :disabled="oauthStarting" @click="oauthProxyOpen = false">{{ t("common.cancel") }}</button>
            <button class="btn btn-primary" data-test="oauth-proxy-continue" :disabled="oauthStarting" @click="startLogin()">
              <Icon v-if="oauthStarting" name="refresh" class="spin" :size="14" />
              <Icon v-else name="external" :size="14" />
              {{ t("accounts.oauthContinue") }}
            </button>
          </div>
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
            <button v-if="loginError" class="btn btn-primary" @click="startLogin()">
              {{ t("common.retry") }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Connectivity test modal -->
    <Teleport to="body">
      <div v-if="testOpen" class="modal-backdrop" @click.self="testOpen = false">
        <div class="modal account-test-modal" data-test="account-test-modal" role="dialog" aria-modal="true" @keydown.esc="testOpen = false">
          <div class="account-test-head">
            <h3 class="modal-title">{{ t("accounts.testTitle") }}</h3>
            <p class="modal-desc">{{ testTarget?.email }}</p>
          </div>
          <div class="account-test-scroll">
            <div class="field">
              <label class="field-label">{{ t("accounts.testModel") }}</label>
              <select v-model="testModel" class="select" :disabled="testRunning">
                <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
              </select>
            </div>

            <div v-if="testRunning" class="test-running">
              <Icon name="refresh" class="spin" :size="18" />
              <span class="muted">{{ t("accounts.testing") }}</span>
            </div>

            <div v-else-if="testError" class="test-result-panel is-failed">
              <div class="test-result-head">
                <strong>{{ t("accounts.testFailed") }}</strong>
                <button class="copy-btn" type="button" :title="t('accounts.copyTestError')" @click="copyTestDetails(testError)"><Icon name="copy" :size="14" /></button>
              </div>
              <pre class="test-error-detail" data-test="account-test-error">{{ testError }}</pre>
            </div>

            <div v-else-if="testResult" class="test-result-panel" :class="{ 'is-failed': !testResult.ok }">
              <div class="test-result-head">
                <span class="badge" :class="testResult.ok ? 'badge-success' : 'badge-danger'">
                  <span class="badge-dot"></span>
                  {{ testResult.ok ? t("accounts.testPass") : t("accounts.testFailed") }}
                </span>
                <span class="faint text-sm">HTTP {{ testResult.status }} · {{ testResult.latency_ms }}ms</span>
              </div>
              <div v-if="!testResult.ok && testResult.error" class="test-error-wrap">
                <button class="copy-btn" type="button" :title="t('accounts.copyTestError')" @click="copyTestDetails(testResult.error)"><Icon name="copy" :size="14" /></button>
                <pre class="test-error-detail" data-test="account-test-error">{{ testResult.error }}</pre>
              </div>
              <pre v-if="testResult.sample" class="test-sample">{{ testResult.sample }}</pre>
              <div class="list-row test-stat-row">
                <span class="faint text-sm">{{ t("accounts.tokensUsed") }}</span>
                <span class="text-sm mono" :title="`${exactTokens(testResult.total_tokens)} tokens`">{{ formatTokens(testResult.total_tokens) }} ({{ formatTokens(testResult.prompt_tokens) }}+{{ formatTokens(testResult.completion_tokens) }})</span>
              </div>
              <div class="list-row test-stat-row">
                <span class="faint text-sm">{{ t("accounts.statusAfter") }}</span>
                <span class="text-sm">{{ t("accounts.status." + testResult.account_status) }}</span>
              </div>
            </div>
          </div>

          <div class="modal-actions account-test-actions">
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
          <div class="field">
            <span class="field-label">{{ t("accounts.importProxy") }}</span>
            <AccountProxySelect v-model="apiKeyProxyID" :proxies="proxies" :disabled="apiKeyAdding" />
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
            <div class="import-proxy-config">
              <label class="field">
                <span class="field-label">{{ t("accounts.importProxyMode") }}</span>
                <select v-model="importProxyMode" class="select" data-test="import-proxy-mode" :disabled="importing">
                  <option value="preserve">{{ t("accounts.importProxyPreserve") }}</option>
                  <option value="direct">{{ t("accounts.importProxyDirect") }}</option>
                  <option value="override">{{ t("accounts.importProxyOverride") }}</option>
                </select>
              </label>
              <div v-if="importProxyMode === 'override'" class="field">
                <span class="field-label">{{ t("accounts.importProxy") }}</span>
                <AccountProxySelect v-model="importProxyID" :proxies="proxies" :disabled="importing" />
              </div>
            </div>
            <div class="flex items-center gap-8" style="margin-bottom: 10px">
              <button class="btn btn-ghost btn-sm" @click="importFileInput?.click()">
                <Icon name="upload" :size="14" /> {{ t("accounts.importChooseFile") }}
              </button>
              <span v-if="importFileNames.length" class="faint text-sm import-file-name">
                {{ t("accounts.importFilesSelected", { count: importFileNames.length, size: (importFileSize / 1024).toFixed(1) }) }}
              </span>
              <input
                ref="importFileInput"
                type="file"
                accept="application/json,.json,.jsonl,text/plain"
                multiple
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
              <button class="btn btn-primary" :disabled="importing || (importProxyMode === 'override' && !importProxyID)" @click="previewImport">
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
                    <th>{{ t("accounts.importProxy") }}</th>
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
                    <td class="import-proxy-value">{{ previewProxyLabel(row.proxy_id, row.proxy_specified) }}</td>
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
.accounts-page { min-width: 0; container: accounts-page / inline-size; }
.account-list { display: grid; gap: 10px; }
.account-row { min-width: 0; min-height: 92px; display: grid; grid-template-columns: minmax(220px, 2fr) minmax(150px, 1.25fr) minmax(92px, .65fr) minmax(92px, .65fr) minmax(132px, .85fr) auto; align-items: center; gap: 14px; padding: 14px 16px; border: 1px solid var(--border-soft); border-radius: 8px; background: var(--bg-card); box-shadow: var(--shadow-xs); transform-origin: center; transition: transform var(--motion-normal) var(--motion-ease), box-shadow var(--motion-normal) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease), opacity var(--motion-fast) var(--motion-ease); }
.account-row:hover, .account-row:focus-within { transform: translateY(-1px) scale(1.002); box-shadow: var(--shadow-hover); border-color: var(--border); }
.account-row.is-disabled { opacity: .72; }
.account-identity { min-width: 0; display: flex; align-items: center; gap: 11px; }
.account-avatar { flex: 0 0 auto; background: linear-gradient(135deg, #d97757, #b8532f); }
.account-name-wrap { min-width: 0; }
.account-name-wrap > strong, .account-name-wrap .modal-title { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-subline { min-width: 0; display: flex; align-items: center; gap: 7px; margin-top: 4px; }
.account-subtitle { min-width: 0; overflow: hidden; color: var(--text-faint); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.account-health { min-width: 0; display: grid; justify-items: start; gap: 6px; }
.account-health small { max-width: 100%; overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }
.account-metric { display: grid; gap: 4px; }
.account-metric span, .account-load span { color: var(--text-faint); font-size: 11px; }
.account-metric strong { font-size: 13px; }
.account-load { display: grid; gap: 5px; }
.account-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; }
.account-switch { flex: 0 0 auto; }
.import-chooser { max-width: 680px; }
.import-methods { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin-top: 18px; }
.import-methods > button { min-width: 0; min-height: 128px; display: grid; align-content: start; justify-items: start; gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); color: var(--text); text-align: left; cursor: pointer; }
.import-methods > button:hover, .import-methods > button:focus-visible { border-color: var(--accent); background: var(--accent-soft); outline: none; }
.import-methods span { min-width: 0; display: grid; gap: 6px; }
.import-methods strong { font-size: 13px; }
.import-methods small { color: var(--text-dim); line-height: 1.5; }
.oauth-proxy-modal { max-width: 520px; overflow: visible; }
.import-proxy-config { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; align-items: end; margin-bottom: 10px; }
.import-proxy-config .field { margin: 0; }
.import-proxy-value { max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-test-modal { width: min(560px, calc(100vw - 32px)); max-width: 560px; max-height: min(86vh, 720px); display: flex; flex-direction: column; padding: 0; overflow: hidden; }
.account-test-head { flex: 0 0 auto; padding: 20px 22px 14px; border-bottom: 1px solid var(--border-soft); background: var(--bg-card); }
.account-test-head .modal-desc { margin-bottom: 0; }
.account-test-scroll { min-width: 0; min-height: 0; padding: 16px 22px; overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
.account-test-scroll .field { margin-top: 0; }
.account-test-actions { flex: 0 0 auto; margin: 0; padding: 14px 22px 18px; border-top: 1px solid var(--border-soft); background: var(--bg-card); }
.test-running { display: flex; align-items: center; gap: 12px; margin: 14px 0; }
.test-running .icon { color: var(--accent); }
.test-result-panel { min-width: 0; max-width: 100%; margin-top: 12px; padding: 13px 14px; border: 1px solid var(--border); border-radius: 7px; background: var(--bg-elev); overflow: hidden; }
.test-result-panel.is-failed { border-color: color-mix(in srgb, var(--danger) 45%, var(--border)); background: var(--danger-soft); }
.test-result-head { min-width: 0; display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.test-result-head > strong { color: var(--danger); font-size: 13px; }
.test-error-wrap { position: relative; min-width: 0; max-width: 100%; margin-top: 10px; }
.test-error-wrap .copy-btn { position: absolute; z-index: 1; top: 6px; right: 6px; }
.test-error-detail, .test-sample { box-sizing: border-box; min-width: 0; max-width: 100%; max-height: 180px; margin: 10px 0 0; padding: 10px 12px; overflow: auto; border: 1px solid var(--border-soft); border-radius: 6px; background: var(--bg-card); color: var(--danger); font-family: var(--font-mono); font-size: 11px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; }
.test-error-wrap .test-error-detail { margin-top: 0; padding-right: 42px; }
.test-sample { color: var(--text-dim); }
.test-stat-row { min-width: 0; gap: 12px; padding: 8px 0 0; }
.test-stat-row > :first-child { flex: 1; }
.test-stat-row > :last-child { min-width: 0; overflow-wrap: anywhere; text-align: right; }
.account-detail-modal { width: min(860px, calc(100vw - 32px)); max-width: 860px; max-height: min(90vh, 900px); display: flex; flex-direction: column; padding: 0; overflow: hidden; }
.account-detail-head { position: relative; z-index: 1; flex: 0 0 auto; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px 22px 16px; border-bottom: 1px solid var(--border-soft); background: var(--bg-card); }
.account-detail-scroll { min-height: 0; padding: 0 22px 22px; overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
.detail-status-reason { margin: 12px 0 0; padding: 10px 12px; border-left: 3px solid var(--danger); background: var(--danger-soft); color: var(--danger); overflow-wrap: anywhere; }
.detail-section { padding: 16px 0; border-bottom: 1px solid var(--border-soft); }
.detail-section h4 { margin: 0 0 11px; font-size: 13px; }
.detail-section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.detail-section-head > span { color: var(--text-faint); font-size: 11px; }
.detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 20px; }
.detail-grid > div { min-width: 0; display: grid; gap: 4px; }
.detail-grid span, .usage-stat-grid span { color: var(--text-faint); font-size: 11px; }
.detail-grid strong, .usage-stat-grid strong { font-size: 13px; font-weight: 600; }
.detail-wide { grid-column: 1 / -1; }
.detail-value { overflow-wrap: anywhere; }
.usage-stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.usage-stat-grid > div { display: grid; gap: 5px; padding: 10px 12px; background: var(--bg-elev); border-radius: 6px; }
.detail-usage-windows { margin-top: 14px; }
.limit-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)) auto; gap: 10px; align-items: end; }
.limit-grid .field, .detail-proxy { margin: 0; }
.detail-proxy { margin-top: 12px; }
.detail-actions { display: flex; flex-wrap: wrap; gap: 8px; padding-top: 16px; }
.detail-delete { margin-left: auto; }
.share-modal { max-width: 720px; max-height: min(88vh, 820px); overflow-y: auto; }
.share-custody-warning { display: flex; gap: 10px; margin: 14px 0; padding: 12px 14px; border: 1px solid rgba(193, 134, 58, .3); border-radius: 8px; background: var(--warn-soft); color: var(--warn); }
.share-custody-warning p { margin: 3px 0 0; color: var(--text-dim); line-height: 1.5; }
.share-created { display: grid; gap: 10px; padding: 14px; border: 1px solid rgba(63, 143, 95, .28); border-radius: 8px; background: var(--success-soft); }
.share-created > div:first-child { display: grid; gap: 3px; color: var(--success); }
.share-created .field { margin: 0; }
.share-created-content { display: grid; grid-template-columns: 176px minmax(0, 1fr); gap: 14px; align-items: start; }
.share-created-fields { display: grid; gap: 10px; min-width: 0; }
.share-qr-wrap { display: grid; gap: 5px; justify-items: center; color: var(--text-dim); text-align: center; }
.share-access-note { margin: 0; color: var(--text-dim); font-size: 12px; }
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
@container accounts-page (max-width: 1180px) {
	.account-row { grid-template-columns: minmax(180px, 2fr) minmax(125px, 1.25fr) minmax(76px, .65fr) minmax(82px, .65fr) minmax(105px, .85fr) minmax(160px, auto); gap: 10px; }
	.account-row-actions { flex-direction: column; align-items: stretch; }
	.account-row-actions .btn { justify-content: center; }
}
@container accounts-page (max-width: 900px) {
	.account-row { grid-template-columns: minmax(0, 1fr) minmax(150px, auto) minmax(160px, auto); grid-template-rows: auto auto; gap: 8px 14px; }
	.account-identity { grid-column: 1; grid-row: 1 / span 2; }
	.account-health { grid-column: 2; grid-row: 1; }
	.account-load { grid-column: 2; grid-row: 2; }
	.account-metric { display: none; }
	.account-row-actions { grid-column: 3; grid-row: 1 / span 2; }
}
@container accounts-page (max-width: 720px) {
	.account-row { grid-template-columns: minmax(0, 1fr) auto; gap: 10px 12px; }
	.account-identity, .account-health, .account-load { grid-column: 1; grid-row: auto; }
	.account-row-actions { grid-column: 2; grid-row: 1 / span 3; align-items: flex-end; }
}
@media (max-width: 1100px) {
	.import-methods { grid-template-columns: minmax(0, 1fr); }
	.import-methods > button { min-height: 92px; grid-template-columns: auto minmax(0, 1fr); }
}
@media (max-width: 720px) {
	.detail-grid, .limit-grid, .import-proxy-config { grid-template-columns: minmax(0, 1fr); }
	.usage-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.detail-wide { grid-column: auto; }
  .share-create-form { grid-template-columns: minmax(0, 1fr); }
  .share-created-content { grid-template-columns: minmax(0, 1fr); }
  .share-existing article { align-items: stretch; flex-direction: column; }
  .share-usage-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
