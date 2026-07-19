<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, ControlAPIError, type CloudAdminOverview, type CloudAdminUser, type CloudNetworkProbe, type CloudNetworkSettings, type CloudStatus, type Proxy } from "../api/control";
import ConfirmModal from "../components/ConfirmModal.vue";
import CloudWorkspace from "../components/cloud/CloudWorkspace.vue";
import Icon from "../components/Icon.vue";
import TurnstileWidget from "../components/TurnstileWidget.vue";
import { useAppStore } from "../store";
import { createWorkspace, listWorkspaces, switchWorkspace, type WorkspaceSummary } from "../tauri";

const { t } = useI18n();
const app = useAppStore();
const status = ref<CloudStatus | null>(null);
const loading = ref(true);
const loadFailed = ref(false);
const mode = ref<"login" | "register">("login");
const busy = ref("");

const email = ref("");
const password = ref("");
const passwordConfirm = ref("");
const recoveryAcknowledged = ref(false);
const turnstileToken = ref("");
const turnstileFailed = ref(false);
const verificationCode = ref("");
const resendUntil = ref(0);
const clockNow = ref(Date.now());
const currentPassword = ref("");
const newPassword = ref("");
const newPasswordConfirm = ref("");
const passwordOpen = ref(false);
const turnstile = ref<InstanceType<typeof TurnstileWidget> | null>(null);
const adminOpen = ref(false);
const adminKey = ref("");
const adminOverview = ref<CloudAdminOverview | null>(null);
const adminTab = ref<"users" | "shares" | "settings" | "stats" | "audit">("users");
const adminBusy = ref("");
const adminSearch = ref("");
const registrationEnabled = ref(true);
const inviteMode = ref(false);
const deleteTarget = ref<CloudAdminUser | null>(null);
const deleteFirstOpen = ref(false);
const deleteFinalOpen = ref(false);
const deleteConfirmText = ref("");
const networkOpen = ref(false);
const networkBusy = ref("");
const networkSettings = ref<CloudNetworkSettings | null>(null);
const networkMode = ref<CloudNetworkSettings["mode"]>("system");
const networkProxyID = ref<number | null>(null);
const networkProxies = ref<Proxy[]>([]);
const networkProbe = ref<CloudNetworkProbe | null>(null);
const workspaceOpen = ref(false);
const workspaceBusy = ref("");
const workspaces = ref<WorkspaceSummary[]>([]);
const newWorkspaceName = ref("");
const workspaceSwitchTarget = ref<WorkspaceSummary | null>(null);
const bindingOpen = ref(false);
const bindingAction = ref<"login" | "verify" | "">("");

const pendingVerification = computed(() => Boolean(status.value?.pending_verification));
const authenticated = computed(() => Boolean(status.value?.authenticated));
const resendSeconds = computed(() => Math.max(0, Math.ceil((resendUntil.value - clockNow.value) / 1000)));
const retrySeconds = computed(() => {
  if (!status.value?.next_retry_at) return 0;
  return Math.max(0, Math.ceil((new Date(status.value.next_retry_at).getTime() - clockNow.value) / 1000));
});
const syncErrorMessage = computed(() => {
  const stage = status.value?.last_error_stage;
  return stage ? t(`cloud.syncErrorStage.${stage}`) : status.value?.last_error || "";
});
const filteredAdminUsers = computed(() => {
  const users = adminOverview.value?.users ?? [];
  const query = adminSearch.value.trim().toLowerCase();
  return query ? users.filter((user) => user.email.toLowerCase().includes(query)) : users;
});

function normalizeStatus(value: CloudStatus): CloudStatus {
  return {
    ...value,
    pending_items: Number.isFinite(value.pending_items) ? value.pending_items : 0,
    consecutive_failures: Number.isFinite(value.consecutive_failures) ? value.consecutive_failures : 0,
    conflicts: Array.isArray(value.conflicts) ? value.conflicts : [],
    workspace: value.workspace || {
      workspace_id: "",
      state: "local",
      account_count: 0,
      proxy_count: 0,
      pending_outbox: 0,
      quarantined_items: 0,
    },
  };
}

function workspaceNeedsBindingConfirmation() {
  const workspace = status.value?.workspace;
  if (!workspace) return false;
  if (workspace.state === "recovery") return workspace.recovery_reason === "legacy_owner_confirmation_required";
  return workspace.state === "local" && (workspace.account_count > 0 || workspace.proxy_count > 0 || workspace.pending_outbox > 0);
}

function handleWorkspaceAuthError(error: unknown, action: "login" | "verify") {
  if (!(error instanceof ControlAPIError)) return false;
  if (error.code === "workspace_binding_required") {
    bindingAction.value = action;
    bindingOpen.value = true;
    return true;
  }
  if (error.code === "workspace_account_mismatch" || error.code === "legacy_workspace_ambiguous" || error.code === "cloud_workspace_owner_mismatch") {
    app.toast(t(`cloud.workspace.errors.${error.code}`), "error");
    void openWorkspaceManager();
    return true;
  }
  return false;
}

let statusRefreshing = false;
async function refreshStatus(silent = true) {
  if (statusRefreshing) return;
  statusRefreshing = true;
  try {
    status.value = normalizeStatus(await api.cloudStatus());
  } catch (error) {
    if (!silent) app.toast((error as Error).message, "error");
  } finally {
    statusRefreshing = false;
  }
}

async function load() {
  loading.value = true;
  loadFailed.value = false;
  try {
    status.value = normalizeStatus(await api.cloudStatus());
    if (status.value.email && !email.value) email.value = status.value.email;
  } catch (error) {
    loadFailed.value = true;
    app.toast((error as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

function resetTurnstile() {
  turnstileToken.value = "";
  turnstile.value?.reset();
}

async function register() {
  if (!email.value.trim() || password.value.length < 12 || password.value !== passwordConfirm.value || !recoveryAcknowledged.value || !turnstileToken.value) {
    app.toast(t("cloud.registrationIncomplete"), "error");
    return;
  }
  busy.value = "register";
  try {
    await api.cloudRegister({
      email: email.value.trim(),
      password: password.value,
      turnstile_token: turnstileToken.value,
      recovery_acknowledged: recoveryAcknowledged.value,
    });
    password.value = "";
    passwordConfirm.value = "";
    status.value = normalizeStatus(await api.cloudStatus());
    resendUntil.value = Date.now() + 60_000;
    app.toast(t("cloud.verificationSent"), "success");
  } catch (error) {
    resetTurnstile();
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function verifyEmail(confirmWorkspace = false) {
  if (!/^\d{6}$/.test(verificationCode.value.trim())) {
    app.toast(t("cloud.codeInvalid"), "error");
    return;
  }
  if (!confirmWorkspace && workspaceNeedsBindingConfirmation()) {
    bindingAction.value = "verify";
    bindingOpen.value = true;
    return;
  }
  busy.value = "verify";
  try {
    status.value = normalizeStatus(await api.cloudVerifyEmail(email.value.trim(), verificationCode.value.trim(), confirmWorkspace));
    verificationCode.value = "";
    app.toast(t("cloud.verified"), "success");
    await syncNow(true);
  } catch (error) {
    if (!handleWorkspaceAuthError(error, "verify")) app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function resendVerification() {
  if (resendSeconds.value > 0 || !email.value.trim()) return;
  busy.value = "resend";
  try {
    status.value = normalizeStatus(await api.cloudResendVerification(email.value.trim()));
    resendUntil.value = Date.now() + 60_000;
    app.toast(t("cloud.verificationResent"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function cancelRegistration() {
  busy.value = "cancel-registration";
  try {
    status.value = normalizeStatus(await api.cloudCancelRegistration());
    verificationCode.value = "";
    resendUntil.value = 0;
    mode.value = "register";
    app.toast(t("cloud.registrationCancelled"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function login(confirmWorkspace = false) {
  if (!email.value.trim() || !password.value) {
    app.toast(t("cloud.loginIncomplete"), "error");
    return;
  }
  if (!confirmWorkspace && workspaceNeedsBindingConfirmation()) {
    bindingAction.value = "login";
    bindingOpen.value = true;
    return;
  }
  busy.value = "login";
  try {
    status.value = normalizeStatus(await api.cloudLogin(email.value.trim(), password.value, confirmWorkspace));
    password.value = "";
    app.toast(t("cloud.loggedIn"), "success");
    await syncNow(true);
  } catch (error) {
    if (!handleWorkspaceAuthError(error, "login")) app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function confirmWorkspaceBinding() {
  const action = bindingAction.value;
  bindingOpen.value = false;
  if (action === "login") await login(true);
  if (action === "verify") await verifyEmail(true);
}

async function openWorkspaceManager() {
  workspaceOpen.value = true;
  workspaceBusy.value = "load";
  try {
    workspaces.value = await listWorkspaces();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    workspaceBusy.value = "";
  }
}

function requestWorkspaceSwitch(workspace: WorkspaceSummary) {
  if (workspace.active) return;
  workspaceSwitchTarget.value = workspace;
}

async function confirmWorkspaceSwitch() {
  const target = workspaceSwitchTarget.value;
  if (!target) return;
  workspaceSwitchTarget.value = null;
  workspaceBusy.value = `switch-${target.id}`;
  try {
    workspaces.value = await switchWorkspace(target.id);
    window.location.reload();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    workspaceBusy.value = "";
  }
}

async function createNewWorkspace() {
  workspaceBusy.value = "create";
  try {
    const workspace = await createWorkspace(newWorkspaceName.value.trim());
    workspaces.value = await listWorkspaces();
    newWorkspaceName.value = "";
    requestWorkspaceSwitch(workspace);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    workspaceBusy.value = "";
  }
}

async function syncNow(silent = false) {
  busy.value = "sync";
  try {
    status.value = normalizeStatus(await api.cloudSync());
    if (!silent) app.toast(t("cloud.syncComplete"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
    await refreshStatus();
  } finally {
    busy.value = "";
  }
}

async function openNetworkSettings() {
  networkOpen.value = true;
  networkBusy.value = "load";
  networkProbe.value = null;
  try {
    const [settings, proxyResult] = await Promise.all([api.cloudNetwork(), api.listProxies()]);
    networkSettings.value = settings;
    networkMode.value = settings.mode;
    networkProxyID.value = settings.proxy_id ?? null;
    networkProxies.value = proxyResult.proxies || [];
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    networkBusy.value = "";
  }
}

async function runNetworkProbe() {
  networkBusy.value = "probe";
  try {
    networkProbe.value = await api.probeCloudNetwork();
    if (networkProbe.value.ok) app.toast(t("cloud.networkProbePassed"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    networkBusy.value = "";
  }
}

async function applyNetworkAndRetry() {
  if (networkMode.value === "proxy" && !networkProxyID.value) {
    app.toast(t("cloud.networkProxyRequired"), "error");
    return;
  }
  networkBusy.value = "apply";
  try {
    networkSettings.value = await api.saveCloudNetwork({
      mode: networkMode.value,
      proxy_id: networkMode.value === "proxy" ? networkProxyID.value : null,
    });
    networkProbe.value = await api.probeCloudNetwork();
    if (!networkProbe.value.ok) {
      app.toast(networkProbe.value.error || t("cloud.networkProbeFailed"), "error");
      return;
    }
    status.value = normalizeStatus(await api.cloudSync());
    networkOpen.value = false;
    app.toast(t("cloud.networkRecovered"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
    await refreshStatus();
  } finally {
    networkBusy.value = "";
  }
}

async function logout() {
  busy.value = "logout";
  try {
    status.value = normalizeStatus(await api.cloudLogout());
    password.value = "";
    currentPassword.value = "";
    newPassword.value = "";
    newPasswordConfirm.value = "";
    closeAdmin();
    app.toast(t("cloud.loggedOut"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

function applyAdminOverview(value: CloudAdminOverview) {
  adminOverview.value = {
    ...value,
    users: Array.isArray(value.users) ? value.users : [],
    shares: Array.isArray(value.shares) ? value.shares : [],
    connect_endpoints: Array.isArray(value.connect_endpoints) ? value.connect_endpoints : [],
    settings: Array.isArray(value.settings) ? value.settings : [],
    audit: Array.isArray(value.audit) ? value.audit : [],
  };
  registrationEnabled.value = value.settings.find((setting) => setting.key === "registration_enabled")?.value !== "false";
  inviteMode.value = value.settings.find((setting) => setting.key === "invite_mode")?.value === "true";
}

async function loadAdmin() {
  if (!adminKey.value.trim()) {
    app.toast(t("cloud.adminKeyRequired"), "error");
    return;
  }
  adminBusy.value = "load";
  try {
    applyAdminOverview(await api.cloudAdminOverview(adminKey.value));
    app.toast(t("cloud.adminUnlocked"), "success");
  } catch (error) {
    adminOverview.value = null;
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

function closeAdmin() {
  adminOpen.value = false;
  adminKey.value = "";
  adminOverview.value = null;
  adminBusy.value = "";
  adminSearch.value = "";
  deleteTarget.value = null;
  deleteFirstOpen.value = false;
  deleteFinalOpen.value = false;
  deleteConfirmText.value = "";
}

async function refreshAdmin() {
  if (!adminOverview.value) return;
  applyAdminOverview(await api.cloudAdminOverview(adminKey.value));
}

async function setAdminUserBanned(user: CloudAdminUser) {
  adminBusy.value = `ban-${user.id}`;
  try {
    await api.cloudAdminSetUserBanned(adminKey.value, user.id, !Boolean(user.banned));
    await refreshAdmin();
    app.toast(t(user.banned ? "cloud.adminUserUnbanned" : "cloud.adminUserBanned"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

async function logoutAdminUser(user: CloudAdminUser) {
  adminBusy.value = `logout-${user.id}`;
  try {
    await api.cloudAdminLogoutUser(adminKey.value, user.id);
    await refreshAdmin();
    app.toast(t("cloud.adminUserLoggedOut"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

function requestAdminDelete(user: CloudAdminUser) {
  deleteTarget.value = user;
  deleteFirstOpen.value = true;
}

function continueAdminDelete() {
  deleteFirstOpen.value = false;
  deleteConfirmText.value = "";
  deleteFinalOpen.value = true;
}

function cancelAdminDelete() {
  deleteFirstOpen.value = false;
  deleteFinalOpen.value = false;
  deleteConfirmText.value = "";
  deleteTarget.value = null;
}

async function confirmAdminDelete() {
  if (!deleteTarget.value || deleteConfirmText.value !== "DELETE") return;
  const user = deleteTarget.value;
  adminBusy.value = `delete-${user.id}`;
  try {
    await api.cloudAdminDeleteUser(adminKey.value, user.id);
    cancelAdminDelete();
    await refreshAdmin();
    app.toast(t("cloud.adminUserDeleted"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

async function saveAdminSettings() {
  adminBusy.value = "settings";
  try {
    await api.cloudAdminUpdateSettings(adminKey.value, {
      registration_enabled: registrationEnabled.value,
      invite_mode: inviteMode.value,
    });
    await refreshAdmin();
    app.toast(t("cloud.adminSettingsSaved"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

async function setAdminShareRevoked(shareId: number, revoked: boolean) {
  adminBusy.value = `share-${shareId}`;
  try {
    await api.cloudAdminSetShareRevoked(adminKey.value, shareId, revoked);
    await refreshAdmin();
    app.toast(t(revoked ? "cloud.adminShareRevoked" : "cloud.adminShareRestored"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    adminBusy.value = "";
  }
}

async function changePassword() {
  if (!currentPassword.value || newPassword.value.length < 12 || newPassword.value !== newPasswordConfirm.value) {
    app.toast(t("cloud.passwordIncomplete"), "error");
    return;
  }
  busy.value = "password";
  try {
    status.value = normalizeStatus(await api.cloudChangePassword(currentPassword.value, newPassword.value));
    currentPassword.value = "";
    newPassword.value = "";
    newPasswordConfirm.value = "";
    passwordOpen.value = false;
    app.toast(t("cloud.passwordChanged"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

function formatTime(value?: string): string {
  if (!value) return t("cloud.never");
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? t("common.unknown") : date.toLocaleString();
}

let clockTimer = 0;
let statusTimer = 0;
onMounted(() => {
  void load();
  clockTimer = window.setInterval(() => { clockNow.value = Date.now(); }, 1000);
  statusTimer = window.setInterval(() => {
    if (authenticated.value && !loading.value) void refreshStatus();
  }, 5000);
});
onUnmounted(() => {
  window.clearInterval(clockTimer);
  window.clearInterval(statusTimer);
});
</script>

<template>
  <div class="cloud-page">
    <div v-if="!authenticated" class="page-header cloud-page-heading">
      <div><h1 class="page-title">{{ t("cloud.title") }}</h1><p class="page-desc">{{ t("cloud.desc") }}</p></div>
      <button class="btn btn-ghost" type="button" @click="openWorkspaceManager"><Icon name="database" :size="14" />{{ t("cloud.workspace.manage") }}</button>
    </div>

    <div v-if="loading" class="cloud-skeleton" aria-hidden="true">
      <span></span><span></span><span></span>
    </div>

    <section v-else-if="loadFailed" class="cloud-panel cloud-notice danger" role="alert">
      <Icon name="warn" :size="22" />
      <div><strong>{{ t("cloud.loadFailed") }}</strong><p>{{ t("cloud.loadFailedDesc") }}</p></div>
      <button class="btn btn-primary" type="button" @click="load"><Icon name="refresh" :size="14" />{{ t("common.retry") }}</button>
    </section>

    <section v-else-if="status && !status.configured" class="cloud-panel cloud-notice" role="status">
      <Icon name="cloud" :size="24" />
      <div><strong>{{ t("cloud.notConfigured") }}</strong><p>{{ t("cloud.notConfiguredDesc") }}</p></div>
    </section>

    <template v-else-if="status && !authenticated">
      <div class="cloud-auth-shell">
      <div v-if="!pendingVerification" class="cloud-auth-tabs" role="tablist" :aria-label="t('cloud.title')">
        <button type="button" role="tab" :aria-selected="mode === 'login'" :class="{ active: mode === 'login' }" @click="mode = 'login'">
          <Icon name="key" :size="15" />{{ t("cloud.loginTab") }}
        </button>
        <button type="button" role="tab" :aria-selected="mode === 'register'" :class="{ active: mode === 'register' }" @click="mode = 'register'">
          <Icon name="plus" :size="15" />{{ t("cloud.registerTab") }}
        </button>
      </div>

      <section v-if="pendingVerification" class="cloud-panel auth-panel">
        <div class="section-heading"><Icon name="check" :size="20" /><div><h2>{{ t("cloud.verifyTitle") }}</h2><p>{{ t("cloud.verifyDesc", { email }) }}</p></div></div>
        <div class="cloud-form compact-form">
          <label class="field"><span class="field-label">{{ t("cloud.verificationCode") }}</span><input v-model="verificationCode" class="input mono code-input" inputmode="numeric" maxlength="6" autocomplete="one-time-code" /></label>
          <button class="btn btn-primary" type="button" :disabled="busy !== ''" @click="verifyEmail()"><Icon name="check" :size="15" />{{ busy === "verify" ? t("cloud.verifying") : t("cloud.verify") }}</button>
          <div class="verification-actions">
            <button class="btn btn-ghost btn-sm" data-test="cloud-resend-verification" type="button" :disabled="busy !== '' || resendSeconds > 0" @click="resendVerification"><Icon name="refresh" :size="14" />{{ busy === "resend" ? t("cloud.resendingVerification") : resendSeconds > 0 ? t("cloud.resendCountdown", { seconds: resendSeconds }) : t("cloud.resendVerification") }}</button>
            <button class="btn btn-ghost btn-sm" data-test="cloud-cancel-registration" type="button" :disabled="busy !== ''" @click="cancelRegistration"><Icon name="plus" :size="14" />{{ t("cloud.backToRegistration") }}</button>
          </div>
        </div>
      </section>

      <section v-else-if="mode === 'login'" class="cloud-panel auth-panel">
        <div class="section-heading"><Icon name="key" :size="20" /><div><h2>{{ t("cloud.loginTitle") }}</h2><p>{{ t("cloud.loginDesc") }}</p></div></div>
        <div class="cloud-form">
          <label class="field"><span class="field-label">{{ t("cloud.email") }}</span><input v-model="email" data-test="cloud-email" class="input" type="email" autocomplete="username" :placeholder="t('cloud.emailPlaceholder')" /></label>
          <label class="field"><span class="field-label">{{ t("cloud.masterPassword") }}</span><input v-model="password" data-test="cloud-password" class="input" type="password" autocomplete="current-password" /></label>
          <button class="btn btn-primary form-submit" data-test="cloud-login" type="button" :disabled="busy !== ''" @click="login()"><Icon name="key" :size="15" />{{ busy === "login" ? t("cloud.loggingIn") : t("cloud.login") }}</button>
        </div>
      </section>

      <section v-else class="cloud-panel auth-panel">
        <div class="section-heading"><Icon name="cloud" :size="20" /><div><h2>{{ t("cloud.registerTitle") }}</h2><p>{{ t("cloud.registerDesc") }}</p></div></div>
        <div class="recovery-warning" role="note"><Icon name="warn" :size="18" /><div><strong>{{ t("cloud.recoveryTitle") }}</strong><span>{{ t("cloud.recoveryDesc") }}</span></div></div>
        <div class="cloud-form">
          <label class="field"><span class="field-label">{{ t("cloud.email") }}</span><input v-model="email" class="input" type="email" autocomplete="username" :placeholder="t('cloud.emailPlaceholder')" /></label>
          <label class="field"><span class="field-label">{{ t("cloud.masterPassword") }}</span><input v-model="password" class="input" type="password" autocomplete="new-password" /><small>{{ t("cloud.passwordHint") }}</small></label>
          <label class="field"><span class="field-label">{{ t("cloud.confirmPassword") }}</span><input v-model="passwordConfirm" class="input" type="password" autocomplete="new-password" /></label>
          <label class="recovery-check"><input v-model="recoveryAcknowledged" type="checkbox" /><span>{{ t("cloud.recoveryConfirm") }}</span></label>
          <TurnstileWidget v-if="status.turnstile_site_key" ref="turnstile" :site-key="status.turnstile_site_key" @token="turnstileToken = $event; turnstileFailed = false" @expired="turnstileToken = ''" @error="turnstileFailed = true" />
          <p v-if="turnstileFailed" class="field-error">{{ t("cloud.turnstileFailed") }}</p>
          <p v-if="!status.turnstile_site_key" class="field-error">{{ t("cloud.turnstileUnavailable") }}</p>
          <button class="btn btn-primary form-submit" type="button" :disabled="busy !== '' || !status.turnstile_site_key" @click="register"><Icon name="plus" :size="15" />{{ busy === "register" ? t("cloud.registering") : t("cloud.register") }}</button>
        </div>
      </section>
      </div>
    </template>

    <template v-else-if="status && authenticated">
      <CloudWorkspace
        :status="status"
        :busy="busy"
        :admin-open="adminOpen"
        @sync="syncNow(false)"
        @network="openNetworkSettings"
        @logout="logout"
        @admin="adminOpen ? closeAdmin() : adminOpen = true"
        @password="passwordOpen = true"
		@workspace="openWorkspaceManager"
      />

      <div v-if="networkOpen" class="modal-backdrop" @click.self="networkBusy ? undefined : networkOpen = false">
        <div class="modal cloud-network-modal" data-test="cloud-network-modal" role="dialog" aria-modal="true" @keydown.esc="networkBusy ? undefined : networkOpen = false">
          <div class="network-heading">
            <div><h3 class="modal-title">{{ t("cloud.networkTitle") }}</h3><p class="modal-desc">{{ t("cloud.networkDesc") }}</p></div>
            <button class="btn btn-ghost btn-sm" type="button" :disabled="networkBusy !== ''" @click="networkOpen = false">{{ t("common.close") }}</button>
          </div>
          <div class="network-modes" role="group" :aria-label="t('cloud.networkMode')">
            <button v-for="modeOption in (['system', 'proxy', 'direct'] as const)" :key="modeOption" type="button" :class="{ active: networkMode === modeOption }" :disabled="networkBusy !== ''" @click="networkMode = modeOption">{{ t(`cloud.networkModeOption.${modeOption}`) }}</button>
          </div>
          <label v-if="networkMode === 'proxy'" class="field">
            <span class="field-label">{{ t("cloud.networkProxy") }}</span>
            <select v-model.number="networkProxyID" data-test="cloud-network-proxy" class="select" :disabled="networkBusy !== ''">
              <option :value="null">{{ t("cloud.networkProxyPlaceholder") }}</option>
              <option v-for="proxy in networkProxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }} ({{ proxy.type.toUpperCase() }})</option>
            </select>
          </label>
          <div v-if="networkSettings" class="network-current">
            <span>{{ t("cloud.networkCurrent") }}</span>
            <div>
              <strong>{{ networkSettings.proxy_name || t(`cloud.networkSource.${networkSettings.effective_source}`) }}</strong>
              <small v-if="networkSettings.endpoint" class="mono">{{ networkSettings.endpoint }} · {{ t(networkSettings.fallback ? "cloud.networkEndpointFallback" : "cloud.networkEndpointPrimary") }}</small>
            </div>
          </div>
          <div v-if="networkProbe" class="network-probe" :class="{ failed: !networkProbe.ok }" data-test="cloud-network-probe">
            <div class="network-probe-head"><strong>{{ networkProbe.ok ? t("cloud.networkProbePassed") : t("cloud.networkProbeFailed") }}</strong><span class="mono">{{ networkProbe.endpoint || networkProbe.target }}</span></div>
            <div class="network-stages">
              <div v-for="stage in networkProbe.stages" :key="stage.id" :class="`stage-${stage.status}`">
                <Icon :name="stage.status === 'ok' ? 'check' : stage.status === 'failed' ? 'warn' : 'info'" :size="14" />
                <span>{{ t(`cloud.networkStage.${stage.id}`) }}</span>
                <small>{{ stage.http_status || (stage.latency_ms != null ? `${stage.latency_ms} ms` : t(`cloud.networkStageStatus.${stage.status}`)) }}</small>
              </div>
            </div>
            <p v-if="networkProbe.error">{{ networkProbe.error }}</p>
          </div>
          <div class="modal-actions">
            <button class="btn btn-ghost" type="button" :disabled="networkBusy !== ''" @click="runNetworkProbe"><Icon name="activity" :size="14" />{{ networkBusy === "probe" ? t("cloud.networkProbing") : t("cloud.networkProbe") }}</button>
            <button class="btn btn-primary" data-test="cloud-network-apply" type="button" :disabled="networkBusy !== '' || (networkMode === 'proxy' && !networkProxyID)" @click="applyNetworkAndRetry"><Icon name="refresh" :size="14" :class="{ spin: networkBusy === 'apply' }" />{{ networkBusy === "apply" ? t("cloud.networkApplying") : t("cloud.networkApplyRetry") }}</button>
          </div>
        </div>
      </div>

      <section v-if="status.role === 'admin' && adminOpen" class="cloud-admin-shell" data-test="cloud-admin-panel">
        <div class="section-toolbar admin-heading">
          <div><h2>{{ t("cloud.adminTitle") }}</h2><p>{{ t("cloud.adminDesc") }}</p></div>
          <button class="btn btn-ghost btn-sm" type="button" @click="closeAdmin"><Icon name="power" :size="14" />{{ t("cloud.adminLock") }}</button>
        </div>

        <div v-if="!adminOverview" class="cloud-panel admin-unlock">
          <div class="section-heading"><Icon name="key" :size="20" /><div><h2>{{ t("cloud.adminUnlockTitle") }}</h2><p>{{ t("cloud.adminUnlockDesc") }}</p></div></div>
          <div class="cloud-form compact-form">
            <label class="field"><span class="field-label">{{ t("cloud.adminKey") }}</span><input v-model="adminKey" data-test="cloud-admin-key" class="input mono" type="password" autocomplete="off" @keyup.enter="loadAdmin" /></label>
            <button class="btn btn-primary" data-test="cloud-admin-unlock" type="button" :disabled="adminBusy !== ''" @click="loadAdmin"><Icon name="key" :size="14" />{{ adminBusy === "load" ? t("cloud.adminUnlocking") : t("cloud.adminUnlock") }}</button>
          </div>
        </div>

        <template v-else>
          <div class="admin-boundary" role="note"><Icon name="warn" :size="18" /><p><strong>{{ t("cloud.adminBoundaryTitle") }}</strong> {{ t("cloud.adminBoundaryDesc") }}</p></div>
          <div class="admin-tabs" role="tablist" :aria-label="t('cloud.adminTitle')">
            <button v-for="tab in (['users', 'shares', 'settings', 'stats', 'audit'] as const)" :key="tab" type="button" role="tab" :aria-selected="adminTab === tab" :class="{ active: adminTab === tab }" @click="adminTab = tab">{{ t(`cloud.adminTab.${tab}`) }}</button>
          </div>

          <div v-if="adminTab === 'shares'" class="admin-view">
            <div><h3>{{ t("cloud.adminSharesTitle") }}</h3><p>{{ t("cloud.adminSharesDesc") }}</p></div>
            <div class="admin-table-wrap">
              <table class="admin-table admin-share-table">
                <thead><tr><th>{{ t("cloud.adminShareOwner") }}</th><th>{{ t("cloud.adminShareCode") }}</th><th>{{ t("cloud.adminStatus") }}</th><th>{{ t("cloud.adminShareUsage") }}</th><th>{{ t("cloud.adminShareExpiry") }}</th><th class="admin-actions-heading">{{ t("cloud.adminActions") }}</th></tr></thead>
                <tbody>
                  <tr v-for="share in adminOverview.shares" :key="share.id">
                    <td><strong>{{ share.owner_email }}</strong><small>#{{ share.owner_id }}</small></td>
                    <td class="mono">{{ share.share_code }}</td>
                    <td><span class="badge" :class="share.revoked ? 'badge-danger' : 'badge-success'">{{ t(share.revoked ? "cloud.adminShareRevokedStatus" : "cloud.adminShareActive") }}</span></td>
                    <td>{{ share.used_requests }} / {{ share.quota_requests || "∞" }}</td>
                    <td>{{ formatTime(share.expires_at) }}</td>
                    <td class="admin-row-actions"><button class="btn btn-sm" :class="share.revoked ? 'btn-ghost' : 'btn-danger'" type="button" :disabled="adminBusy !== ''" @click="setAdminShareRevoked(share.id, !Boolean(share.revoked))">{{ t(share.revoked ? "cloud.adminShareRestore" : "cloud.adminShareRevoke") }}</button></td>
                  </tr>
                  <tr v-if="adminOverview.shares.length === 0"><td colspan="6" class="admin-empty">{{ t("cloud.adminNoShares") }}</td></tr>
                </tbody>
              </table>
            </div>
            <div><h3>{{ t("cloud.adminConnectTitle") }}</h3><p>{{ t("cloud.adminConnectDesc") }}</p></div>
            <div class="admin-table-wrap">
              <table class="admin-table admin-connect-table">
                <thead><tr><th>{{ t("cloud.adminShareOwner") }}</th><th>{{ t("cloud.adminStatus") }}</th><th>{{ t("cloud.adminConnectResources") }}</th><th>{{ t("cloud.adminConnectClaims") }}</th><th>{{ t("cloud.adminShareExpiry") }}</th></tr></thead>
                <tbody>
                  <tr v-for="endpoint in adminOverview.connect_endpoints" :key="endpoint.public_id">
                    <td><strong>{{ endpoint.owner_email }}</strong><small>#{{ endpoint.owner_id }} · {{ endpoint.public_id }}</small></td>
                    <td><span class="badge" :class="endpoint.status === 'active' ? 'badge-success' : 'badge-neutral'">{{ t(endpoint.status === "active" ? "cloud.adminShareActive" : "cloud.v4.pause") }}</span></td>
                    <td>{{ t("cloud.adminConnectResourceCounts", { accounts: endpoint.account_count, recipients: endpoint.recipient_count }) }}</td>
                    <td>{{ endpoint.claimed_count || 0 }} / {{ endpoint.max_claims || 0 }}</td>
                    <td>{{ formatTime(endpoint.expires_at) }}</td>
                  </tr>
                  <tr v-if="adminOverview.connect_endpoints.length === 0"><td colspan="5" class="admin-empty">{{ t("cloud.adminNoConnect") }}</td></tr>
                </tbody>
              </table>
            </div>
          </div>

          <div v-else-if="adminTab === 'users'" class="admin-view">
            <div class="admin-view-toolbar"><div><h3>{{ t("cloud.adminUsersTitle") }}</h3><p>{{ t("cloud.adminUsersDesc") }}</p></div><input v-model="adminSearch" data-test="cloud-admin-search" class="input admin-search" type="search" :placeholder="t('cloud.adminSearch')" /></div>
            <div class="admin-table-wrap">
              <table class="admin-table">
                <thead><tr><th>{{ t("cloud.email") }}</th><th>{{ t("cloud.adminStatus") }}</th><th>{{ t("cloud.adminVaultItems") }}</th><th>{{ t("cloud.adminLastActive") }}</th><th class="admin-actions-heading">{{ t("cloud.adminActions") }}</th></tr></thead>
                <tbody>
                  <tr v-for="user in filteredAdminUsers" :key="user.id">
                    <td><strong>{{ user.email }}</strong><small>#{{ user.id }} · {{ t(user.role === 'admin' ? 'cloud.roleAdmin' : 'cloud.roleUser') }}</small></td>
                    <td><span class="badge" :class="user.banned ? 'badge-danger' : 'badge-success'">{{ t(user.banned ? "cloud.adminBanned" : "cloud.adminActive") }}</span></td>
                    <td>{{ user.vault_count }}</td>
                    <td>{{ formatTime(user.last_active_at) }}</td>
                    <td class="admin-row-actions">
                      <button class="btn btn-ghost btn-sm" type="button" :disabled="adminBusy !== '' || user.email === status.email" @click="setAdminUserBanned(user)">{{ t(user.banned ? "cloud.adminUnban" : "cloud.adminBan") }}</button>
                      <button class="btn btn-ghost btn-sm" type="button" :disabled="adminBusy !== ''" @click="logoutAdminUser(user)"><Icon name="power" :size="13" />{{ t("cloud.adminForceLogout") }}</button>
                      <button class="btn btn-danger btn-sm" type="button" :disabled="adminBusy !== '' || user.email === status.email" @click="requestAdminDelete(user)"><Icon name="trash" :size="13" />{{ t("common.delete") }}</button>
                    </td>
                  </tr>
                  <tr v-if="filteredAdminUsers.length === 0"><td colspan="5" class="admin-empty">{{ t("cloud.adminNoUsers") }}</td></tr>
                </tbody>
              </table>
            </div>
          </div>

          <div v-else-if="adminTab === 'settings'" class="admin-view admin-settings">
            <div><h3>{{ t("cloud.adminSettingsTitle") }}</h3><p>{{ t("cloud.adminSettingsDesc") }}</p></div>
            <label class="admin-setting-row"><div><strong>{{ t("cloud.adminRegistration") }}</strong><span>{{ t("cloud.adminRegistrationDesc") }}</span></div><input v-model="registrationEnabled" type="checkbox" /></label>
            <label class="admin-setting-row"><div><strong>{{ t("cloud.adminInviteMode") }}</strong><span>{{ t("cloud.adminInviteModeDesc") }}</span></div><input v-model="inviteMode" type="checkbox" /></label>
            <button class="btn btn-primary" type="button" :disabled="adminBusy !== ''" @click="saveAdminSettings"><Icon name="check" :size="14" />{{ adminBusy === "settings" ? t("cloud.adminSaving") : t("common.save") }}</button>
          </div>

          <div v-else-if="adminTab === 'stats'" class="admin-view">
            <div><h3>{{ t("cloud.adminStatsTitle") }}</h3><p>{{ t("cloud.adminStatsDesc") }}</p></div>
            <div class="admin-stats">
              <div><span>{{ t("cloud.adminTotalUsers") }}</span><strong>{{ adminOverview.stats.users }}</strong></div>
              <div><span>{{ t("cloud.adminDailyActive") }}</span><strong>{{ adminOverview.stats.daily_active_users }}</strong></div>
              <div><span>{{ t("cloud.adminVaultItems") }}</span><strong>{{ adminOverview.stats.vault_items }}</strong></div>
              <div><span>{{ t("cloud.adminActiveShares") }}</span><strong>{{ adminOverview.stats.active_shares }}</strong></div>
              <div><span>{{ t("cloud.adminShareRequests") }}</span><strong>{{ adminOverview.stats.share_requests }}</strong></div>
              <div><span>{{ t("cloud.adminShareErrorRate") }}</span><strong>{{ ((adminOverview.stats.share_error_rate || 0) * 100).toFixed(1) }}%</strong></div>
            </div>
          </div>

          <div v-else class="admin-view">
            <div><h3>{{ t("cloud.adminAuditTitle") }}</h3><p>{{ t("cloud.adminAuditDesc") }}</p></div>
            <div class="admin-audit-list">
              <article v-for="entry in adminOverview.audit" :key="entry.id"><span class="badge badge-neutral">{{ entry.action }}</span><div><strong>{{ entry.target_type }} #{{ entry.target_id }}</strong><small>{{ t("cloud.adminActor", { id: entry.actor_user_id }) }} · {{ formatTime(entry.created_at) }}</small></div></article>
              <div v-if="adminOverview.audit.length === 0" class="admin-empty">{{ t("cloud.adminNoAudit") }}</div>
            </div>
          </div>
        </template>
      </section>

      <div v-if="passwordOpen" class="modal-backdrop" @click.self="passwordOpen = false">
        <div class="modal password-modal" role="dialog" aria-modal="true" @keydown.esc="passwordOpen = false">
          <h3 class="modal-title">{{ t("cloud.changePassword") }}</h3>
          <p class="modal-desc">{{ t("cloud.securityDesc") }}</p>
          <div class="password-modal-form">
            <label class="field"><span class="field-label">{{ t("cloud.currentPassword") }}</span><input v-model="currentPassword" class="input" type="password" autocomplete="current-password" /></label>
            <label class="field"><span class="field-label">{{ t("cloud.newPassword") }}</span><input v-model="newPassword" class="input" type="password" autocomplete="new-password" /></label>
            <label class="field"><span class="field-label">{{ t("cloud.confirmPassword") }}</span><input v-model="newPasswordConfirm" class="input" type="password" autocomplete="new-password" /></label>
          </div>
          <div class="modal-actions"><button class="btn btn-ghost" @click="passwordOpen = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="busy !== ''" @click="changePassword"><Icon name="check" :size="14" />{{ busy === "password" ? t("cloud.changingPassword") : t("common.save") }}</button></div>
        </div>
      </div>
    </template>

    <div v-if="workspaceOpen" class="modal-backdrop" @click.self="workspaceBusy ? undefined : workspaceOpen = false">
      <div class="modal workspace-modal" role="dialog" aria-modal="true" :aria-label="t('cloud.workspace.title')" @keydown.esc="workspaceBusy ? undefined : workspaceOpen = false">
        <div class="workspace-modal-heading"><div><h3 class="modal-title">{{ t("cloud.workspace.title") }}</h3><p class="modal-desc">{{ t("cloud.workspace.desc") }}</p></div><button class="btn btn-ghost btn-sm" type="button" :disabled="workspaceBusy !== ''" @click="workspaceOpen = false">{{ t("common.close") }}</button></div>
        <div class="workspace-list">
          <article v-for="workspace in workspaces" :key="workspace.id" :class="{ active: workspace.active }">
            <span class="workspace-list-icon"><Icon name="database" :size="18" /></span>
            <div><strong>{{ workspace.name }}</strong><small>{{ t(workspace.kind === 'legacy' ? 'cloud.workspace.legacy' : 'cloud.workspace.local') }}</small></div>
            <span v-if="workspace.active" class="badge badge-success">{{ t("cloud.workspace.current") }}</span>
            <button v-else class="btn btn-ghost btn-sm" type="button" :disabled="workspaceBusy !== ''" @click="requestWorkspaceSwitch(workspace)">{{ t("cloud.workspace.switch") }}</button>
          </article>
          <div v-if="!workspaces.length && workspaceBusy !== 'load'" class="workspace-empty">{{ t("cloud.workspace.none") }}</div>
        </div>
        <div class="workspace-create"><label class="field"><span class="field-label">{{ t("cloud.workspace.newName") }}</span><input v-model="newWorkspaceName" class="input" maxlength="64" :placeholder="t('cloud.workspace.newPlaceholder')" /></label><button class="btn btn-primary" type="button" :disabled="workspaceBusy !== ''" @click="createNewWorkspace"><Icon name="plus" :size="14" />{{ workspaceBusy === "create" ? t("cloud.workspace.creating") : t("cloud.workspace.create") }}</button></div>
        <div class="workspace-safety"><Icon name="info" :size="16" /><span>{{ t("cloud.workspace.safety") }}</span></div>
      </div>
    </div>

    <ConfirmModal
      :open="bindingOpen"
      :title="t('cloud.workspace.bindTitle')"
      :desc="t('cloud.workspace.bindDesc', { accounts: status?.workspace.account_count || 0, proxies: status?.workspace.proxy_count || 0 })"
      :confirm-text="t('cloud.workspace.bindConfirm')"
      @confirm="confirmWorkspaceBinding"
      @cancel="bindingOpen = false; bindingAction = ''"
    />
    <ConfirmModal
      :open="Boolean(workspaceSwitchTarget)"
      :title="t('cloud.workspace.switchTitle')"
      :desc="t('cloud.workspace.switchDesc', { name: workspaceSwitchTarget?.name || '' })"
      :confirm-text="t('cloud.workspace.switchConfirm')"
      @confirm="confirmWorkspaceSwitch"
      @cancel="workspaceSwitchTarget = null"
    />

    <ConfirmModal
      :open="deleteFirstOpen"
      :title="t('cloud.adminDeleteTitle')"
      :desc="t('cloud.adminDeleteFirstDesc', { email: deleteTarget?.email || '' })"
      :confirm-text="t('cloud.adminContinueDelete')"
      danger
      @confirm="continueAdminDelete"
      @cancel="cancelAdminDelete"
    />
    <div v-if="deleteFinalOpen" class="modal-backdrop" @click.self="cancelAdminDelete">
      <div class="modal admin-delete-modal" role="dialog" aria-modal="true" :aria-label="t('cloud.adminDeleteTitle')" tabindex="-1" @keydown.esc="cancelAdminDelete">
        <h3 class="modal-title">{{ t("cloud.adminDeleteTitle") }}</h3>
        <p class="modal-desc">{{ t("cloud.adminDeleteFinalDesc", { email: deleteTarget?.email || '' }) }}</p>
        <label class="field"><span class="field-label">{{ t("cloud.adminDeleteInput") }}</span><input v-model="deleteConfirmText" data-test="cloud-admin-delete-confirm" class="input mono" autocomplete="off" /></label>
        <div class="modal-actions"><button class="btn btn-ghost" type="button" @click="cancelAdminDelete">{{ t("common.cancel") }}</button><button class="btn btn-danger" data-test="cloud-admin-delete-final" type="button" :disabled="deleteConfirmText !== 'DELETE' || adminBusy !== ''" @click="confirmAdminDelete"><Icon name="trash" :size="14" />{{ t("common.delete") }}</button></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cloud-page { width: 100%; max-width: 1240px; margin: 0 auto; }
.cloud-page-heading, .workspace-modal-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.cloud-page-heading .page-desc { margin-bottom: 0; }
.workspace-modal { width: min(620px, calc(100vw - 32px)); max-width: 620px; }
.workspace-list { display: grid; gap: 8px; margin-top: 18px; }
.workspace-list article { min-height: 60px; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 11px; padding: 10px 12px; border: 1px solid var(--border-soft); border-radius: 7px; background: var(--bg-card); }
.workspace-list article.active { border-color: color-mix(in srgb, var(--success) 35%, var(--border)); background: var(--success-soft); }
.workspace-list-icon { width: 34px; height: 34px; display: grid; place-items: center; border-radius: 6px; background: var(--bg-elev); color: var(--text-dim); }
.workspace-list article > div { min-width: 0; display: grid; gap: 3px; }
.workspace-list article small { color: var(--text-faint); }
.workspace-empty { padding: 22px; color: var(--text-faint); text-align: center; }
.workspace-create { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: end; gap: 10px; margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--border-soft); }
.workspace-create .field { margin: 0; }
.workspace-safety { display: flex; align-items: flex-start; gap: 8px; margin-top: 14px; color: var(--text-faint); font-size: 12px; line-height: 1.5; }
.cloud-network-modal { width: min(680px, calc(100vw - 32px)); max-width: 680px; }
.network-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.network-modes { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4px; margin: 18px 0; padding: 4px; border: 1px solid var(--border); border-radius: 7px; background: var(--bg-elev); }
.network-modes button { min-height: 36px; border: 0; border-radius: 5px; background: transparent; color: var(--text-dim); cursor: pointer; }
.network-modes button.active { background: var(--bg-card); color: var(--text); box-shadow: var(--shadow-xs); }
.network-current { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; padding: 10px 12px; border: 1px solid var(--border-soft); border-radius: 6px; color: var(--text-faint); }
.network-current > div { min-width: 0; display: grid; justify-items: end; gap: 3px; }
.network-current strong, .network-current small { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.network-current strong { color: var(--text); }
.network-current small { font-size: 10px; color: var(--text-faint); }
.network-current strong { color: var(--text); }
.network-probe { margin-top: 14px; padding: 14px; border: 1px solid color-mix(in srgb, var(--success) 35%, var(--border)); border-radius: 7px; background: var(--success-soft); }
.network-probe.failed { border-color: color-mix(in srgb, var(--danger) 40%, var(--border)); background: var(--danger-soft); }
.network-probe-head { display: flex; justify-content: space-between; gap: 12px; }
.network-probe-head span { min-width: 0; overflow: hidden; color: var(--text-faint); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.network-stages { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin-top: 12px; }
.network-stages > div { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 3px 6px; padding: 8px; border: 1px solid var(--border-soft); border-radius: 6px; background: var(--bg-card); }
.network-stages small { grid-column: 2; color: var(--text-faint); }
.network-stages .stage-ok { color: var(--success); }.network-stages .stage-failed { color: var(--danger); }.network-stages .stage-skipped, .network-stages .stage-not_run { color: var(--text-faint); }
.network-probe > p { margin: 10px 0 0; color: var(--danger); overflow-wrap: anywhere; }
@media (max-width: 620px) { .network-stages { grid-template-columns: repeat(2, minmax(0, 1fr)); }.network-modes { grid-template-columns: minmax(0, 1fr); } }
.cloud-auth-shell { width: min(460px, 100%); margin: 28px auto 0; }
.cloud-panel { min-width: 0; padding: 20px; border: 1px solid var(--border-soft); border-radius: 8px; background: var(--bg-card); }
.cloud-skeleton { display: grid; gap: 12px; }
.cloud-skeleton span { display: block; height: 86px; border-radius: 8px; background: linear-gradient(90deg, var(--bg-elev), var(--bg-card), var(--bg-elev)); background-size: 200% 100%; animation: cloud-shimmer 1.4s ease-in-out infinite; }
.cloud-notice { min-height: 160px; display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 16px; }
.cloud-notice.danger { color: var(--danger); }
.cloud-notice p, .section-heading p, .section-toolbar p { margin: 4px 0 0; color: var(--text-dim); }
.sync-error-content { min-width: 0; }
.sync-error-meta { display: flex; flex-wrap: wrap; gap: 6px 14px; margin-top: 8px; color: var(--text-faint); font-size: 11px; }
.sync-retry { margin-top: 12px; color: var(--danger); }
.cloud-auth-tabs { width: 100%; display: grid; grid-template-columns: repeat(2, minmax(132px, 1fr)); gap: 3px; padding: 3px; margin-bottom: 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-elev); }
.cloud-auth-tabs button { min-height: 38px; display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 7px 16px; border: 0; border-radius: 6px; background: transparent; color: var(--text-dim); font-weight: 600; cursor: pointer; }
.cloud-auth-tabs button.active { background: var(--bg-card); color: var(--text); box-shadow: 0 1px 4px rgba(50, 43, 34, .1); }
.auth-panel { width: 100%; }
.section-heading, .account-identity { display: flex; align-items: center; gap: 12px; }
.section-heading h2, .account-identity h2, .section-toolbar h2 { margin: 0; font-size: 15px; }
.cloud-form { display: grid; grid-template-columns: minmax(0, 1fr); gap: 14px; margin-top: 20px; }
.cloud-form .field { margin: 0; }
.cloud-form small { color: var(--text-faint); }
.form-submit { align-self: end; justify-self: start; min-width: 130px; }
.compact-form { grid-template-columns: minmax(0, 1fr); align-items: end; }
.verification-actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.code-input { letter-spacing: 4px; font-size: 17px; }
.recovery-warning { display: flex; gap: 10px; margin-top: 18px; padding: 13px 14px; border: 1px solid rgba(193, 134, 58, .28); border-radius: 8px; background: var(--warn-soft); color: var(--warn); }
.recovery-warning div { display: grid; gap: 3px; }
.recovery-warning span { color: var(--text-dim); }
.recovery-check { grid-column: 1 / -1; display: flex; align-items: flex-start; gap: 9px; color: var(--text-dim); cursor: pointer; }
.account-summary { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.account-identity p { margin: 3px 0 0; color: var(--text-dim); }
.cloud-avatar { width: 44px; height: 44px; display: grid; place-items: center; border-radius: 8px; background: var(--primary-soft); color: var(--primary); }
.cloud-actions { display: flex; gap: 8px; }
.sync-strip { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin: 16px 0 24px; border-block: 1px solid var(--border-soft); }
.sync-strip div { display: grid; gap: 3px; padding: 14px 18px; border-right: 1px solid var(--border-soft); }
.sync-strip div:last-child { border-right: 0; }
.sync-strip span { color: var(--text-faint); font-size: 12px; }
.sync-strip strong { font-size: 15px; }
.cloud-section { margin-top: 24px; }
.password-panel { margin-top: 12px; }
.password-form { grid-template-columns: repeat(3, minmax(0, 1fr)); align-items: end; margin: 0; }
.cloud-empty { min-height: 120px; display: flex; align-items: center; justify-content: center; gap: 10px; color: var(--success); border-block: 1px solid var(--border-soft); }
.conflict-list { border-top: 1px solid var(--border-soft); }
.conflict-row { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 12px; padding: 13px 0; border-bottom: 1px solid var(--border-soft); }
.conflict-row div { display: grid; gap: 2px; }
.conflict-row div span { color: var(--text-dim); font-size: 12px; }
.conflict-row small { color: var(--text-faint); }
.cloud-admin-shell { margin-top: 18px; padding-block: 18px; border-block: 1px solid var(--border); }
.admin-heading { margin-bottom: 14px; }
.admin-unlock { max-width: 760px; }
.admin-boundary { display: flex; align-items: flex-start; gap: 10px; padding: 12px 14px; border: 1px solid rgba(193, 134, 58, .28); border-radius: 8px; background: var(--warn-soft); color: var(--warn); }
.admin-boundary p { margin: 0; color: var(--text-dim); line-height: 1.55; }
.admin-boundary strong { color: var(--warn); }
.admin-tabs { display: flex; gap: 2px; margin: 14px 0; padding-bottom: 8px; overflow-x: auto; border-bottom: 1px solid var(--border-soft); }
.admin-tabs button { min-width: max-content; padding: 8px 13px; border: 0; border-radius: 6px; background: transparent; color: var(--text-dim); font-weight: 600; cursor: pointer; }
.admin-tabs button.active { background: var(--primary-soft); color: var(--primary); }
.admin-view { min-width: 0; }
.admin-view h3 { margin: 0; font-size: 15px; }
.admin-view p { margin: 4px 0 0; color: var(--text-dim); }
.admin-view-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 16px; margin-bottom: 12px; }
.admin-search { width: min(280px, 100%); }
.admin-table-wrap { overflow-x: auto; border-block: 1px solid var(--border-soft); }
.admin-table { width: 100%; min-width: 850px; border-collapse: collapse; font-size: 13px; }
.admin-table th { padding: 10px 8px; color: var(--text-faint); font-size: 11px; text-align: left; text-transform: uppercase; }
.admin-table td { padding: 11px 8px; border-top: 1px solid var(--border-soft); vertical-align: middle; }
.admin-table td:first-child { display: grid; gap: 3px; }
.admin-table td small { color: var(--text-faint); }
.admin-actions-heading { text-align: right !important; }
.admin-row-actions { display: flex; justify-content: flex-end; gap: 5px; }
.admin-empty { padding: 24px !important; color: var(--text-faint); text-align: center; }
.admin-settings { display: grid; gap: 12px; max-width: 720px; }
.admin-setting-row { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 14px 0; border-bottom: 1px solid var(--border-soft); cursor: pointer; }
.admin-setting-row div { display: grid; gap: 3px; }
.admin-setting-row span { color: var(--text-dim); }
.admin-setting-row input { width: 18px; height: 18px; accent-color: var(--primary); }
.admin-settings > .btn { justify-self: start; margin-top: 4px; }
.admin-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: 14px; border-block: 1px solid var(--border-soft); }
.admin-stats div { display: grid; gap: 4px; padding: 18px; border-right: 1px solid var(--border-soft); }
.admin-stats div:last-child { border-right: 0; }
.admin-stats span { color: var(--text-faint); font-size: 12px; }
.admin-stats strong { font-size: 22px; }
.admin-audit-list { margin-top: 14px; border-top: 1px solid var(--border-soft); }
.admin-audit-list article { display: grid; grid-template-columns: minmax(160px, auto) minmax(0, 1fr); gap: 12px; align-items: center; padding: 12px 0; border-bottom: 1px solid var(--border-soft); }
.admin-audit-list article div { display: grid; gap: 3px; }
.admin-audit-list small { color: var(--text-faint); }
.admin-delete-modal { max-width: 500px; }
.password-modal { max-width: 500px; }
.password-modal-form { display: grid; gap: 13px; }
@keyframes cloud-shimmer { to { background-position: -200% 0; } }
@media (prefers-reduced-motion: reduce) { .cloud-skeleton span { animation: none; } }
@media (max-width: 720px) {
  .cloud-page-heading, .workspace-modal-heading { flex-direction: column; }
  .workspace-create { grid-template-columns: minmax(0, 1fr); }
  .workspace-create .btn { justify-self: start; }
  .cloud-notice, .cloud-form, .compact-form, .password-form, .sync-strip { grid-template-columns: minmax(0, 1fr); }
  .cloud-auth-tabs { width: 100%; }
  .account-summary { align-items: stretch; flex-direction: column; }
  .cloud-actions { flex-wrap: wrap; }
  .sync-strip div { border-right: 0; border-bottom: 1px solid var(--border-soft); }
  .sync-strip div:last-child { border-bottom: 0; }
  .admin-view-toolbar { align-items: stretch; flex-direction: column; }
  .admin-search { width: 100%; }
  .admin-stats { grid-template-columns: minmax(0, 1fr); }
  .admin-stats div { border-right: 0; border-bottom: 1px solid var(--border-soft); }
  .admin-stats div:last-child { border-bottom: 0; }
  .admin-audit-list article { grid-template-columns: minmax(0, 1fr); }
}
</style>
