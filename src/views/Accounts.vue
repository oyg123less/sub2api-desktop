<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import EmptyState from "../components/EmptyState.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import AccountProxySelect from "../components/AccountProxySelect.vue";
import {
  api,
  type Account,
  type AccountRuntimeState,
  type AccountUsage,
  type AccountTestResult,
  type AccountTestRun,
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
const router = useRouter();

const accounts = ref<Account[]>([]);
const usage = ref<Record<string, AccountUsage>>({});
const proxies = ref<Proxy[]>([]);
const loading = ref(true);
const accountStrategy = ref<Settings["account_strategy"]>("quota_aware");

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
const selectedAccountIDs = ref<Set<number>>(new Set());
const batchDeleteOpen = ref(false);
const batchDeleting = ref(false);

// batch connectivity test flow
const batchTestOpen = ref(false);
const batchTestStarting = ref(false);
const batchTestRun = ref<AccountTestRun | null>(null);
const batchTestFilter = ref<"all" | "succeeded" | "failed">("all");
let batchTestPollTimer: number | undefined;

const accountsPerPage = 20;
const currentPage = ref(1);
const totalPages = computed(() => Math.max(1, Math.ceil(accounts.value.length / accountsPerPage)));
const pagedAccounts = computed(() => {
  const start = (currentPage.value - 1) * accountsPerPage;
  return accounts.value.slice(start, start + accountsPerPage);
});
const pageNumbers = computed(() => {
  const count = Math.min(5, totalPages.value);
  const start = Math.max(1, Math.min(currentPage.value - 2, totalPages.value - count + 1));
  return Array.from({ length: count }, (_, index) => start + index);
});
const pageStart = computed(() => accounts.value.length ? (currentPage.value - 1) * accountsPerPage + 1 : 0);
const pageEnd = computed(() => Math.min(currentPage.value * accountsPerPage, accounts.value.length));
const allAccountsSelected = computed(() => pagedAccounts.value.length > 0 && pagedAccounts.value.every((account) => selectedAccountIDs.value.has(account.id)));
const someAccountsSelected = computed(() => pagedAccounts.value.some((account) => selectedAccountIDs.value.has(account.id)) && !allAccountsSelected.value);
const batchTestTargetCount = computed(() => selectedAccountIDs.value.size || accounts.value.length);
const selectedHasCloudShare = computed(() => accounts.value.some((account) => selectedAccountIDs.value.has(account.id) && account.source === "cloud_share"));
const filteredBatchResults = computed(() => {
  const rows = batchTestRun.value?.results || [];
  if (batchTestFilter.value === "succeeded") return rows.filter((row) => row.status === "succeeded");
  if (batchTestFilter.value === "failed") return rows.filter((row) => row.status === "failed" || row.status === "skipped");
  return rows;
});

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
    currentPage.value = Math.min(currentPage.value, totalPages.value);
    const existingIDs = new Set(accounts.value.map((account) => account.id));
    selectedAccountIDs.value = new Set([...selectedAccountIDs.value].filter((id) => existingIDs.has(id)));
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

function manageCloudShare() {
	detailTarget.value = null;
	void router.push("/cloud");
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

function accountRoutingEnabled(account: Account) {
	return account.source === "cloud_share" ? Boolean(account.cloud_local_enabled) : account.status !== "disabled";
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
  if (!deleteTarget.value || deleteTarget.value.source === "cloud_share") return;
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

function toggleAccountSelection(id: number, selected: boolean) {
  const next = new Set(selectedAccountIDs.value);
  if (selected) next.add(id);
  else next.delete(id);
  selectedAccountIDs.value = next;
}

function toggleAllAccounts(selected: boolean) {
  const next = new Set(selectedAccountIDs.value);
  for (const account of pagedAccounts.value) {
    if (selected) next.add(account.id);
    else next.delete(account.id);
  }
  selectedAccountIDs.value = next;
}

function goToPage(page: number) {
  currentPage.value = Math.max(1, Math.min(page, totalPages.value));
}

async function confirmBatchDelete() {
  const ids = [...selectedAccountIDs.value];
  if (!ids.length) return;
  batchDeleting.value = true;
  try {
    const result = await api.deleteAccounts(ids);
    selectedAccountIDs.value = new Set();
    batchDeleteOpen.value = false;
    app.toast(t("accounts.batchDeleteComplete", { deleted: result.deleted?.length ?? 0, missing: result.missing?.length ?? 0 }), "success");
    await load();
    await app.refreshStatus();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    batchDeleting.value = false;
  }
}

function scheduleBatchTestPoll(delay = 900) {
  window.clearTimeout(batchTestPollTimer);
  if (!pageMounted || !batchTestRun.value || batchTestRun.value.status !== "running") return;
  batchTestPollTimer = window.setTimeout(refreshBatchTestRun, delay);
}

async function refreshBatchTestRun() {
  if (!batchTestRun.value?.run_id) return;
  try {
    batchTestRun.value = await api.accountTestRun(batchTestRun.value.run_id);
    if (batchTestRun.value.status !== "running") await load();
  } catch {
    // Keep the last visible snapshot; the next explicit open can retry.
  } finally {
    scheduleBatchTestPoll();
  }
}

async function restoreBatchTestRun() {
  try {
    const response = await api.activeAccountTestRun();
    if (response.run) {
      batchTestRun.value = response.run;
      scheduleBatchTestPoll();
    }
  } catch {
    // Older sidecars do not expose batch runs.
  }
}

function openBatchTest() {
  batchTestFilter.value = "all";
  batchTestOpen.value = true;
  if (batchTestRun.value?.status === "running") scheduleBatchTestPoll(0);
}

async function startBatchTest() {
  batchTestStarting.value = true;
  try {
    const ids = [...selectedAccountIDs.value];
    batchTestRun.value = await api.startAccountTestRun({
      scope: ids.length ? "selected" : "all",
      account_ids: ids.length ? ids : undefined,
      model: testModel.value,
    });
    batchTestFilter.value = "all";
    scheduleBatchTestPoll(0);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    batchTestStarting.value = false;
  }
}

async function cancelBatchTest() {
  if (!batchTestRun.value || batchTestRun.value.status !== "running") return;
  try {
    batchTestRun.value = await api.cancelAccountTestRun(batchTestRun.value.run_id);
    scheduleBatchTestPoll(250);
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

async function copyBatchTestError(value?: string) {
  if (!value) return;
  await copyTestDetails(value);
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
	if (["manually_disabled", "auto_disabled_auth_failures", "auto_disabled_account_inactive", "transient_rate_limit", "quota_exhausted", "share_access_revoked", "cloud_share_disabled", "cloud_share_paused"].includes(account.status_reason)) {
		return t(`accounts.statusReason.${account.status_reason}`);
	}
	return account.status_reason;
}

onMounted(() => {
  pageMounted = true;
  document.addEventListener("visibilitychange", onVisibilityChange);
  void load().then(() => pollRuntime());
  void restoreBatchTestRun();
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
  clearTimeout(batchTestPollTimer);
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
      <div class="row-gap account-page-actions">
        <button v-if="accounts.length" class="btn btn-ghost" data-test="account-batch-test-open" @click="openBatchTest">
          <Icon name="activity" :size="16" /> {{ batchTestRun?.status === "running" ? t("accounts.batchTestProgress", { done: batchTestRun.completed, total: batchTestRun.total }) : t("accounts.testAll") }}
        </button>
        <button class="btn btn-primary" data-test="account-import-open" @click="openImportChooser">
          <Icon name="upload" :size="16" /> {{ t("accounts.importAccountTitle") }}
        </button>
      </div>
    </div>

    <SkeletonBlock v-if="loading" :cards="2" :rows="4" />

    <div v-else-if="accounts.length === 0" class="card">
      <EmptyState icon="accounts" :title="t('accounts.empty')" :description="t('accounts.emptyDesc')">
        <button class="btn btn-primary" @click="openImportChooser"><Icon name="upload" :size="16" /> {{ t("accounts.importAccountTitle") }}</button>
      </EmptyState>
    </div>

    <div v-else class="account-list-shell">
      <div class="account-batch-bar" :class="{ active: selectedAccountIDs.size > 0 }" data-test="account-batch-bar">
        <label class="batch-select-all">
          <input type="checkbox" :checked="allAccountsSelected" :indeterminate="someAccountsSelected" :aria-label="t('accounts.selectAll')" @change="toggleAllAccounts(($event.target as HTMLInputElement).checked)" />
          <span>{{ selectedAccountIDs.size ? t("accounts.selectedCount", { count: selectedAccountIDs.size }) : t("accounts.selectAll") }}</span>
        </label>
        <div v-if="selectedAccountIDs.size" class="batch-actions">
          <button class="btn btn-ghost btn-sm" :disabled="batchTestRun?.status === 'running'" @click="openBatchTest"><Icon name="activity" :size="14" />{{ t("accounts.testSelected") }}</button>
          <button class="btn btn-danger btn-sm" :disabled="batchDeleting || selectedHasCloudShare" :title="selectedHasCloudShare ? t('accounts.cloudManagedDelete') : ''" @click="batchDeleteOpen = true"><Icon name="trash" :size="14" />{{ t("accounts.deleteSelected") }}</button>
        </div>
      </div>
      <div class="account-list" data-test="account-list">
      <article v-for="a in pagedAccounts" :key="a.id" class="account-row" :class="{ 'is-disabled': a.status === 'disabled', 'is-selected': selectedAccountIDs.has(a.id) }" data-test="account-row">
        <label class="account-select" :title="t('accounts.selectAccount')">
          <input type="checkbox" :checked="selectedAccountIDs.has(a.id)" :aria-label="t('accounts.selectAccount')" @change="toggleAccountSelection(a.id, ($event.target as HTMLInputElement).checked)" />
        </label>
        <div class="account-identity">
          <div class="brand-logo account-avatar">{{ (a.email || "?").charAt(0).toUpperCase() }}</div>
          <div class="account-name-wrap">
            <strong :title="a.email || t('common.unknown')">{{ a.email || t("common.unknown") }}</strong>
            <div class="account-subline">
              <span class="badge" :class="a.source === 'cloud_share' ? 'badge-success' : 'badge-neutral'">{{ a.source === "cloud_share" ? t("accounts.cloudShare") : t(`accounts.accountType.${a.account_type}`) }}</span>
              <span class="account-subtitle" :title="a.source === 'cloud_share' ? t('accounts.cloudSharedBy', { name: a.cloud_owner_name || a.email }) : a.account_type === 'api_key' ? a.base_url : (a.plan_type || 'ChatGPT')">
                {{ a.source === "cloud_share" ? t("accounts.cloudSharedBy", { name: a.cloud_owner_name || a.email }) : a.account_type === "api_key" ? a.base_url : (a.plan_type || "ChatGPT") }}
              </span>
            </div>
            <div class="account-load-inline">
              <span>{{ t("accounts.concurrentLoad", { current: a.in_flight ?? 0, max: a.max_concurrency ?? 3 }) }}</span>
              <i></i>
              <span>{{ t("accounts.queueLoad", { current: a.waiting ?? 0, max: a.queue_capacity ?? 20 }) }}</span>
            </div>
          </div>
        </div>
        <div class="account-health">
          <span class="badge" :class="statusBadge(a.status)"><span class="badge-dot"></span>{{ t("accounts.status." + a.status) }}</span>
          <small v-if="accountStatusReason(a)" :title="accountStatusReason(a)">{{ accountStatusReason(a) }}</small>
          <small v-else>{{ t("accounts.lastSuccess") }} · {{ fmtDate(a.last_success_at) }}</small>
        </div>
        <div class="account-quota">
          <template v-if="a.account_type === 'oauth' && a.codex_usage && usageWindows(a.codex_usage).length">
            <div v-for="(window, index) in usageWindows(a.codex_usage).slice(0, 2)" :key="index" class="account-quota-window">
              <div><span>{{ window.label }}</span><strong class="mono">{{ pctLabel(window.used) }}</strong></div>
              <div class="usage-bar"><div class="usage-fill" :class="usageBarClass(window.used)" :style="{ width: pct(window.used) + '%' }"></div></div>
              <small v-if="fmtReset(window.reset)">{{ t("accounts.resetIn", { time: fmtReset(window.reset) }) }}</small>
            </div>
          </template>
          <span v-else class="quota-unavailable">{{ t("accounts.quotaUnavailable") }}</span>
        </div>
        <div class="account-usage-summary">
          <div><span>{{ t("accounts.tokensUsed") }}</span><strong class="mono" :title="`${exactTokens(usage[a.id]?.total_tokens)} tokens`">{{ formatTokens(usage[a.id]?.total_tokens) }}</strong></div>
          <div><span>{{ t("accounts.estCost") }}</span><strong class="mono">{{ fmtCost(usage[a.id]?.cost_usd) }}</strong></div>
        </div>
        <div class="account-row-actions">
          <label class="switch account-switch" :title="t(accountRoutingEnabled(a) ? 'accounts.disableAccount' : 'accounts.enableAccount')">
            <input
              type="checkbox"
              :aria-label="t(accountRoutingEnabled(a) ? 'accounts.disableAccount' : 'accounts.enableAccount')"
              :checked="accountRoutingEnabled(a)"
              :disabled="accountToggling[a.id]"
              @change="toggleAccount(a, ($event.target as HTMLInputElement).checked)"
            />
            <span class="slider"></span>
          </label>
          <button class="btn btn-ghost btn-sm" data-test="account-test" :title="t('accounts.test')" :aria-label="t('accounts.test')" @click="openTest(a)">
            <Icon name="bolt" :size="14" /><span class="action-label">{{ t("accounts.test") }}</span>
          </button>
          <button class="btn btn-ghost btn-sm" data-test="account-details" :title="t('accounts.viewDetails')" :aria-label="t('accounts.viewDetails')" @click="openDetails(a)">
            <Icon name="info" :size="14" /><span class="action-label">{{ t("accounts.viewDetails") }}</span>
          </button>
          <button v-if="a.source !== 'cloud_share'" class="btn btn-danger btn-sm account-delete" data-test="account-delete" :title="t('common.delete')" :aria-label="t('common.delete')" @click="deleteTarget = a"><Icon name="trash" :size="14" /></button>
          <button v-else class="btn btn-ghost btn-sm" data-test="cloud-share-manage" :title="t('accounts.manageCloudShare')" :aria-label="t('accounts.manageCloudShare')" @click="manageCloudShare"><Icon name="cloud" :size="14" /></button>
        </div>
      </article>
      </div>
      <nav v-if="totalPages > 1" class="account-pagination" data-test="account-pagination" :aria-label="t('accounts.pagination')">
        <span>{{ t("accounts.pageSummary", { start: pageStart, end: pageEnd, total: accounts.length }) }}</span>
        <div class="account-page-controls">
          <button class="icon-button" type="button" :disabled="currentPage === 1" :title="t('accounts.previousPage')" :aria-label="t('accounts.previousPage')" @click="goToPage(currentPage - 1)">
            <Icon name="chevron-left" :size="16" />
          </button>
          <button v-for="page in pageNumbers" :key="page" class="account-page-button" type="button" :class="{ active: page === currentPage }" :aria-current="page === currentPage ? 'page' : undefined" :aria-label="t('accounts.pageNumber', { page })" @click="goToPage(page)">{{ page }}</button>
          <button class="icon-button" type="button" :disabled="currentPage === totalPages" :title="t('accounts.nextPage')" :aria-label="t('accounts.nextPage')" @click="goToPage(currentPage + 1)">
            <Icon name="chevron-right" :size="16" />
          </button>
        </div>
      </nav>
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
              <div class="account-name-wrap"><h3 class="modal-title">{{ detailTarget.email || t("common.unknown") }}</h3><div class="account-subline"><span class="badge badge-neutral">{{ detailTarget.source === "cloud_share" ? t("accounts.cloudShare") : t(`accounts.accountType.${detailTarget.account_type}`) }}</span><span class="badge" :class="statusBadge(detailTarget.status)">{{ t(`accounts.status.${detailTarget.status}`) }}</span></div></div>
            </div>
            <button class="btn btn-ghost btn-sm" data-test="account-detail-close" @click="closeDetails">{{ t("common.close") }}</button>
          </div>
          <div class="account-detail-scroll" data-test="account-detail-scroll">
            <p v-if="accountStatusReason(detailTarget)" class="detail-status-reason">{{ accountStatusReason(detailTarget) }}</p>

            <section class="detail-section">
              <h4>{{ t("accounts.detailIdentity") }}</h4>
              <div class="detail-grid">
                <div><span>{{ t("accounts.plan") }}</span><strong>{{ detailTarget.source === "cloud_share" ? t("accounts.cloudShare") : (detailTarget.plan_type || "—") }}</strong></div>
                <div v-if="detailTarget.source === 'cloud_share'"><span>{{ t("accounts.cloudOwner") }}</span><strong>{{ detailTarget.cloud_owner_name || "—" }}</strong></div>
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
              <div v-if="detailTarget.source !== 'cloud_share'" class="limit-grid">
                <label class="field"><span class="field-label">{{ t("accounts.maxConcurrency") }}</span><input v-model.number="limitsMaxConcurrency" class="input" type="number" min="1" max="100" /></label>
                <label class="field"><span class="field-label">{{ t("accounts.queueCapacity") }}</span><input v-model.number="limitsQueueCapacity" class="input" type="number" min="0" max="1000" /></label>
                <button class="btn btn-ghost" :disabled="limitsSaving" @click="saveAccountLimits"><Icon name="check" :size="14" />{{ t("common.save") }}</button>
              </div>
              <p v-else class="inline-note"><Icon name="info" :size="15" />{{ t("accounts.cloudLimitsManaged") }}</p>
              <label class="field detail-proxy"><span class="field-label">{{ t("accounts.bindProxy") }}</span><select class="select" :value="detailTarget.proxy_id ?? ''" @change="bindProxy(detailTarget, ($event.target as HTMLSelectElement).value ? Number(($event.target as HTMLSelectElement).value) : null)"><option value="">{{ t("accounts.noProxy") }}</option><option v-for="p in proxies" :key="p.id" :value="p.id">{{ p.name }} ({{ p.type }})</option></select></label>
            </section>

            <div class="detail-actions">
              <button v-if="detailTarget.source === 'cloud_share'" class="btn btn-ghost btn-sm" @click="manageCloudShare"><Icon name="cloud" :size="14" />{{ t("accounts.manageCloudShare") }}</button>
              <button v-else-if="detailTarget.status !== 'active'" class="btn btn-ghost btn-sm" :disabled="resetting[detailTarget.id]" @click="forceReset(detailTarget)"><Icon name="check" :size="14" />{{ t("accounts.forceReset") }}</button>
              <button v-if="detailTarget.account_type === 'oauth' && detailTarget.status === 'refresh_failed'" class="btn btn-primary btn-sm" @click="reLogin(detailTarget)"><Icon name="refresh" :size="14" />{{ t("accounts.relogin") }}</button>
              <button v-else-if="detailTarget.account_type === 'oauth'" class="btn btn-ghost btn-sm" @click="refreshToken(detailTarget)"><Icon name="refresh" :size="14" />{{ t("common.refresh") }}</button>
            </div>
          </div>
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
    <ConfirmModal
      :open="batchDeleteOpen"
      :title="t('accounts.batchDeleteTitle', { count: selectedAccountIDs.size })"
      :desc="t('accounts.batchDeleteDesc')"
      danger
      :confirm-text="t('accounts.deleteSelected')"
      @confirm="confirmBatchDelete"
      @cancel="batchDeleteOpen = false"
    />

    <Teleport to="body">
      <div v-if="batchTestOpen" class="modal-backdrop" @click.self="batchTestOpen = false">
        <div class="modal account-batch-test-modal" data-test="account-batch-test-modal" role="dialog" aria-modal="true" @keydown.esc="batchTestOpen = false">
          <div class="batch-test-head">
            <div><h3 class="modal-title">{{ t("accounts.batchTestTitle") }}</h3><p class="modal-desc">{{ t("accounts.batchTestDesc") }}</p></div>
            <button class="btn btn-ghost btn-sm" @click="batchTestOpen = false">{{ t("common.close") }}</button>
          </div>
          <template v-if="!batchTestRun || batchTestRun.status !== 'running' && batchTestRun.completed === batchTestRun.total">
            <div class="batch-test-config">
              <div class="batch-test-notice"><Icon name="info" :size="18" /><span>{{ t("accounts.batchTestNotice", { count: batchTestTargetCount }) }}</span></div>
              <label class="field"><span class="field-label">{{ t("accounts.testModel") }}</span><select v-model="testModel" class="select"><option v-for="model in modelOptions" :key="model" :value="model">{{ model }}</option></select></label>
              <div v-if="batchTestRun" class="batch-test-previous"><strong>{{ t("accounts.batchTestPrevious") }}</strong><span>{{ t("accounts.batchTestSummary", { success: batchTestRun.succeeded, failed: batchTestRun.failed, total: batchTestRun.total }) }}</span></div>
              <div class="modal-actions"><button class="btn btn-primary" data-test="account-batch-test-start" :disabled="batchTestStarting" @click="startBatchTest"><Icon name="activity" :size="14" />{{ batchTestStarting ? t("accounts.batchTestStarting") : t("accounts.batchTestStart") }}</button></div>
            </div>
          </template>
          <div v-if="batchTestRun" class="batch-test-run">
            <div class="batch-test-progress-head"><strong>{{ t("accounts.batchTestProgress", { done: batchTestRun.completed, total: batchTestRun.total }) }}</strong><span>{{ Math.round((batchTestRun.completed / Math.max(1, batchTestRun.total)) * 100) }}%</span></div>
            <div class="batch-progress"><span :style="{ width: `${(batchTestRun.completed / Math.max(1, batchTestRun.total)) * 100}%` }"></span></div>
            <div class="batch-test-metrics">
              <span class="text-success">{{ t("accounts.batchTestSucceeded", { count: batchTestRun.succeeded }) }}</span>
              <span class="text-danger">{{ t("accounts.batchTestFailed", { count: batchTestRun.failed }) }}</span>
              <span>{{ t("accounts.batchTestRunning", { count: batchTestRun.running }) }}</span>
              <span>{{ t("accounts.batchTestCancelled", { count: batchTestRun.cancelled + batchTestRun.skipped }) }}</span>
            </div>
            <div class="batch-test-toolbar">
              <div class="batch-test-filters">
                <button v-for="filter in (['all', 'succeeded', 'failed'] as const)" :key="filter" type="button" :class="{ active: batchTestFilter === filter }" @click="batchTestFilter = filter">{{ t(`accounts.batchTestFilter.${filter}`) }}</button>
              </div>
              <button v-if="batchTestRun.status === 'running'" class="btn btn-danger btn-sm" data-test="account-batch-test-cancel" @click="cancelBatchTest"><Icon name="power" :size="13" />{{ t("common.cancel") }}</button>
            </div>
            <div class="batch-test-results">
              <div v-for="row in filteredBatchResults" :key="row.account_id" class="batch-test-row" :class="`result-${row.status}`">
                <Icon :name="row.status === 'succeeded' ? 'check' : row.status === 'running' ? 'refresh' : row.status === 'pending' ? 'info' : 'warn'" :size="14" :class="{ spin: row.status === 'running' }" />
                <strong>{{ row.account_label }}</strong>
                <span>{{ t(`accounts.batchTestStatus.${row.status}`) }}</span>
                <small>{{ row.http_status || '' }}<template v-if="row.latency_ms"> · {{ row.latency_ms }} ms</template></small>
                <button v-if="row.error" class="copy-btn" type="button" :title="row.error" @click="copyBatchTestError(row.error)"><Icon name="copy" :size="12" /></button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.accounts-page { min-width: 0; container: accounts-page / inline-size; }
.account-page-actions { flex-wrap: wrap; justify-content: flex-end; }
.account-pagination { min-height: 50px; display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 4px 0; color: var(--text-faint); font-size: 11.5px; }
.account-page-controls { display: flex; align-items: center; gap: 5px; }
.account-page-controls .icon-button, .account-page-button { width: 32px; height: 32px; display: inline-grid; place-items: center; flex: 0 0 auto; border: 1px solid var(--border-soft); border-radius: 6px; background: var(--bg-card); color: var(--text-dim); cursor: pointer; }
.account-page-controls .icon-button:disabled { opacity: .42; cursor: default; }
.account-page-controls .icon-button:not(:disabled):hover, .account-page-button:hover { border-color: var(--border); background: var(--bg-hover); color: var(--text); }
.account-page-button.active { border-color: color-mix(in srgb, var(--primary) 45%, var(--border)); background: var(--primary-soft); color: var(--primary); font-weight: 650; }
.account-list-shell { min-width: 0; }
.account-batch-bar { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 10px; padding: 8px 12px; border: 1px solid var(--border-soft); border-radius: 7px; background: var(--bg-card); transition: border-color var(--motion-fast) var(--motion-ease), background var(--motion-fast) var(--motion-ease); }
.account-batch-bar.active { border-color: color-mix(in srgb, var(--accent) 40%, var(--border)); background: var(--accent-soft); }
.batch-select-all { display: flex; align-items: center; gap: 9px; color: var(--text-dim); cursor: pointer; }
.batch-select-all input, .account-select input { width: 16px; height: 16px; accent-color: var(--accent); cursor: pointer; }
.batch-actions { display: flex; align-items: center; gap: 8px; }
.account-list { display: grid; gap: 10px; }
.account-row { min-width: 0; min-height: 108px; display: grid; grid-template-columns: auto minmax(250px, 1.75fr) minmax(145px, 1fr) minmax(205px, 1.25fr) minmax(122px, .7fr) auto; align-items: center; gap: 14px; padding: 14px 16px; border: 1px solid var(--border-soft); border-radius: 8px; background: var(--bg-card); box-shadow: var(--shadow-xs); transform-origin: center; transition: transform var(--motion-normal) var(--motion-ease), box-shadow var(--motion-normal) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease), opacity var(--motion-fast) var(--motion-ease); }
.account-row:hover, .account-row:focus-within { transform: translateY(-1px) scale(1.002); box-shadow: var(--shadow-hover); border-color: var(--border); }
.account-row.is-selected { border-color: color-mix(in srgb, var(--accent) 50%, var(--border)); }
.account-row.is-disabled { opacity: .72; }
.account-select { align-self: stretch; display: grid; place-items: center; padding-right: 2px; }
.account-identity { min-width: 0; display: flex; align-items: center; gap: 11px; }
.account-avatar { flex: 0 0 auto; background: linear-gradient(135deg, #d97757, #b8532f); }
.account-name-wrap { min-width: 0; }
.account-name-wrap > strong, .account-name-wrap .modal-title { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-subline { min-width: 0; display: flex; align-items: center; gap: 7px; margin-top: 4px; }
.account-subtitle { min-width: 0; overflow: hidden; color: var(--text-faint); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.account-load-inline { display: flex; align-items: center; gap: 7px; margin-top: 7px; color: var(--text-faint); font-size: 11px; }
.account-load-inline i { width: 3px; height: 3px; border-radius: 50%; background: var(--text-faint); }
.account-health { min-width: 0; display: grid; justify-items: start; gap: 6px; }
.account-health small { max-width: 100%; overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }
.account-quota { min-width: 0; display: grid; gap: 7px; }
.account-quota-window { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr); gap: 3px; }
.account-quota-window > div:first-child { display: flex; align-items: center; justify-content: space-between; gap: 8px; color: var(--text-faint); font-size: 10.5px; }
.account-quota-window > div:first-child strong { color: var(--text-dim); font-size: 10.5px; }
.account-quota-window .usage-bar { height: 5px; }
.account-quota-window small { color: var(--text-faint); font-size: 9.5px; white-space: nowrap; }
.quota-unavailable { color: var(--text-faint); font-size: 11px; }
.account-usage-summary { display: grid; gap: 8px; }
.account-usage-summary > div { display: grid; gap: 3px; }
.account-usage-summary span { color: var(--text-faint); font-size: 10.5px; }
.account-usage-summary strong { font-size: 12.5px; }
.account-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; }
.account-delete { width: 34px; justify-content: center; padding-inline: 0; }
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
.account-batch-test-modal { width: min(760px, calc(100vw - 32px)); max-width: 760px; max-height: min(88vh, 820px); display: flex; flex-direction: column; overflow: hidden; padding: 0; }
.batch-test-head { flex: 0 0 auto; display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 20px 22px 14px; border-bottom: 1px solid var(--border-soft); }
.batch-test-config { padding: 16px 22px; }
.batch-test-notice { display: flex; align-items: flex-start; gap: 10px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg-elev); color: var(--text-dim); font-size: 12px; line-height: 1.5; }
.batch-test-config .field { margin-top: 14px; }
.batch-test-previous { display: flex; justify-content: space-between; gap: 12px; margin-top: 12px; color: var(--text-faint); font-size: 12px; }
.batch-test-run { min-height: 0; display: flex; flex-direction: column; padding: 16px 22px 20px; overflow: hidden; }
.batch-test-progress-head { display: flex; justify-content: space-between; gap: 12px; }
.batch-progress { height: 7px; margin-top: 8px; overflow: hidden; border-radius: 4px; background: var(--bg-hover); }
.batch-progress > span { display: block; height: 100%; border-radius: inherit; background: var(--accent); transition: width var(--motion-normal) var(--motion-ease); }
.batch-test-metrics { display: flex; flex-wrap: wrap; gap: 8px 18px; margin-top: 10px; color: var(--text-faint); font-size: 11px; }
.batch-test-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; }
.batch-test-filters { display: flex; gap: 4px; padding: 3px; border-radius: 6px; background: var(--bg-elev); }
.batch-test-filters button { min-height: 28px; padding: 3px 10px; border: 0; border-radius: 4px; background: transparent; color: var(--text-faint); cursor: pointer; }
.batch-test-filters button.active { background: var(--bg-card); color: var(--text); box-shadow: var(--shadow-xs); }
.batch-test-results { min-height: 120px; margin-top: 10px; overflow-y: auto; border-top: 1px solid var(--border-soft); }
.batch-test-row { min-width: 0; display: grid; grid-template-columns: auto minmax(130px, 1fr) 90px 100px auto; align-items: center; gap: 9px; min-height: 40px; border-bottom: 1px solid var(--border-soft); font-size: 11px; }
.batch-test-row strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.batch-test-row > span, .batch-test-row > small { color: var(--text-faint); }
.batch-test-row.result-succeeded > .icon { color: var(--success); }.batch-test-row.result-failed > .icon { color: var(--danger); }
.batch-test-row .copy-btn { position: static; }
@container accounts-page (max-width: 1180px) {
	.account-row { grid-template-columns: auto minmax(210px, 1.5fr) minmax(125px, .9fr) minmax(170px, 1.1fr) minmax(105px, .7fr) minmax(150px, auto); gap: 10px; }
	.account-row-actions { gap: 6px; }
	.account-row-actions .btn { width: 32px; justify-content: center; padding-inline: 0; }
	.account-row-actions .action-label { display: none; }
	.account-quota-window small { display: none; }
}
@container accounts-page (max-width: 900px) {
	.account-row { grid-template-columns: auto minmax(0, 1fr) minmax(145px, auto) minmax(150px, auto); grid-template-rows: auto auto; gap: 10px 14px; }
	.account-select { grid-column: 1; grid-row: 1 / span 2; }
	.account-identity { grid-column: 2; grid-row: 1; }
	.account-health { grid-column: 3; grid-row: 1; }
	.account-quota { grid-column: 2 / span 2; grid-row: 2; grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.account-usage-summary { display: none; }
	.account-row-actions { grid-column: 4; grid-row: 1 / span 2; }
}
@container accounts-page (max-width: 720px) {
	.account-pagination { align-items: flex-start; flex-direction: column; }
	.account-row { grid-template-columns: auto minmax(0, 1fr); gap: 10px 12px; }
	.account-select { grid-column: 1; grid-row: 1 / span 3; }
	.account-identity, .account-health, .account-quota { grid-column: 2; grid-row: auto; }
	.account-quota { grid-template-columns: minmax(0, 1fr); }
	.account-row-actions { grid-column: 2; grid-row: auto; flex-direction: row; flex-wrap: wrap; justify-content: flex-start; }
	.account-batch-bar { align-items: stretch; flex-direction: column; }
	.batch-actions { width: 100%; }
}
@media (max-width: 1100px) {
	.import-methods { grid-template-columns: minmax(0, 1fr); }
	.import-methods > button { min-height: 92px; grid-template-columns: auto minmax(0, 1fr); }
}
@media (max-width: 720px) {
	.detail-grid, .limit-grid, .import-proxy-config { grid-template-columns: minmax(0, 1fr); }
	.usage-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.detail-wide { grid-column: auto; }
	.batch-test-row { grid-template-columns: auto minmax(0, 1fr) auto; }
	.batch-test-row > span { grid-column: 2; }.batch-test-row > small { grid-column: 3; grid-row: 1; }.batch-test-row .copy-btn { grid-column: 3; }
}
</style>
