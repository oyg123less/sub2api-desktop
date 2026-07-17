<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, type CloudAdminOverview, type CloudAdminUser, type CloudStatus } from "../api/control";
import ConfirmModal from "../components/ConfirmModal.vue";
import Icon from "../components/Icon.vue";
import TurnstileWidget from "../components/TurnstileWidget.vue";
import { useAppStore } from "../store";

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
  };
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

async function verifyEmail() {
  if (!/^\d{6}$/.test(verificationCode.value.trim())) {
    app.toast(t("cloud.codeInvalid"), "error");
    return;
  }
  busy.value = "verify";
  try {
    status.value = normalizeStatus(await api.cloudVerifyEmail(email.value.trim(), verificationCode.value.trim()));
    verificationCode.value = "";
    app.toast(t("cloud.verified"), "success");
    await syncNow(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
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

async function login() {
  if (!email.value.trim() || !password.value) {
    app.toast(t("cloud.loginIncomplete"), "error");
    return;
  }
  busy.value = "login";
  try {
    status.value = normalizeStatus(await api.cloudLogin(email.value.trim(), password.value));
    password.value = "";
    app.toast(t("cloud.loggedIn"), "success");
    await syncNow(true);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
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
    <div class="page-header">
      <h1 class="page-title">{{ t("cloud.title") }}</h1>
      <p class="page-desc">{{ t("cloud.desc") }}</p>
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
          <button class="btn btn-primary" type="button" :disabled="busy !== ''" @click="verifyEmail"><Icon name="check" :size="15" />{{ busy === "verify" ? t("cloud.verifying") : t("cloud.verify") }}</button>
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
          <button class="btn btn-primary form-submit" data-test="cloud-login" type="button" :disabled="busy !== ''" @click="login"><Icon name="key" :size="15" />{{ busy === "login" ? t("cloud.loggingIn") : t("cloud.login") }}</button>
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
    </template>

    <template v-else-if="status && authenticated">
      <section class="cloud-panel account-summary">
        <div class="account-identity">
          <span class="cloud-avatar"><Icon name="cloud" :size="22" /></span>
          <div><h2>{{ status.email }}</h2><p>{{ t(status.role === "admin" ? "cloud.roleAdmin" : "cloud.roleUser") }}</p></div>
        </div>
        <div class="cloud-actions">
          <button v-if="status.role === 'admin'" class="btn btn-ghost" data-test="cloud-admin-open" type="button" @click="adminOpen ? closeAdmin() : adminOpen = true"><Icon name="settings" :size="15" />{{ t(adminOpen ? "cloud.adminClose" : "cloud.adminOpen") }}</button>
          <button class="btn btn-primary" data-test="cloud-sync" type="button" :disabled="busy !== ''" @click="syncNow(false)"><Icon name="refresh" :size="15" />{{ busy === "sync" ? t("cloud.syncing") : t("cloud.syncNow") }}</button>
          <button class="btn btn-ghost" data-test="cloud-logout" type="button" :disabled="busy !== ''" @click="logout"><Icon name="power" :size="15" />{{ t("cloud.logout") }}</button>
        </div>
      </section>

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

      <section class="sync-strip" aria-label="sync status">
        <div><span>{{ t("cloud.lastSync") }}</span><strong>{{ formatTime(status.last_sync_at) }}</strong></div>
        <div><span>{{ t("cloud.pendingItems") }}</span><strong>{{ status.pending_items }}</strong></div>
        <div><span>{{ t("cloud.conflictCount") }}</span><strong>{{ status.conflicts.length }}</strong></div>
      </section>

      <section v-if="status.last_error" class="cloud-panel cloud-notice danger" role="alert">
        <Icon name="warn" :size="20" />
        <div class="sync-error-content">
          <strong>{{ t("cloud.syncFailed") }}</strong>
          <p>{{ syncErrorMessage }}</p>
          <div class="sync-error-meta">
            <span v-if="status.last_attempt_at">{{ t("cloud.syncAttempt", { time: formatTime(status.last_attempt_at) }) }}</span>
            <span v-if="status.consecutive_failures">{{ t("cloud.syncFailures", { count: status.consecutive_failures }) }}</span>
            <span v-if="retrySeconds > 0">{{ t("cloud.syncRetryIn", { seconds: retrySeconds }) }}</span>
          </div>
          <button class="btn btn-ghost btn-sm sync-retry" data-test="cloud-sync-retry" type="button" :disabled="busy !== ''" @click="syncNow()">
            <Icon name="refresh" :size="14" />{{ t("cloud.syncRetry") }}
          </button>
        </div>
      </section>

      <section class="cloud-section">
        <div class="section-toolbar"><div><h2>{{ t("cloud.securityTitle") }}</h2><p>{{ t("cloud.securityDesc") }}</p></div><button class="btn btn-ghost btn-sm" type="button" @click="passwordOpen = !passwordOpen"><Icon name="key" :size="14" />{{ t("cloud.changePassword") }}</button></div>
        <div v-if="passwordOpen" class="cloud-panel password-panel">
          <div class="cloud-form password-form">
            <label class="field"><span class="field-label">{{ t("cloud.currentPassword") }}</span><input v-model="currentPassword" class="input" type="password" autocomplete="current-password" /></label>
            <label class="field"><span class="field-label">{{ t("cloud.newPassword") }}</span><input v-model="newPassword" class="input" type="password" autocomplete="new-password" /></label>
            <label class="field"><span class="field-label">{{ t("cloud.confirmPassword") }}</span><input v-model="newPasswordConfirm" class="input" type="password" autocomplete="new-password" /></label>
            <button class="btn btn-primary" type="button" :disabled="busy !== ''" @click="changePassword"><Icon name="check" :size="14" />{{ busy === "password" ? t("cloud.changingPassword") : t("common.save") }}</button>
          </div>
        </div>
      </section>

      <section class="cloud-section">
        <div class="section-toolbar"><div><h2>{{ t("cloud.conflictsTitle") }}</h2><p>{{ t("cloud.conflictsDesc") }}</p></div></div>
        <div v-if="status.conflicts.length === 0" class="cloud-empty"><Icon name="check" :size="24" /><strong>{{ t("cloud.noConflicts") }}</strong></div>
        <div v-else class="conflict-list">
          <article v-for="conflict in status.conflicts" :key="conflict.id" class="conflict-row">
            <span class="badge badge-neutral">{{ t(`cloud.kind.${conflict.kind}`) }}</span>
            <div><strong>{{ conflict.display_name || t(`cloud.kind.${conflict.kind}`) }}</strong><span>{{ t(conflict.resolution === "local_won" ? "cloud.localWon" : "cloud.remoteWon") }}</span><small>{{ formatTime(conflict.created_at) }}</small></div>
          </article>
        </div>
      </section>
    </template>

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
.cloud-page { width: 100%; max-width: 980px; margin: 0 auto; }
.cloud-panel { min-width: 0; padding: 20px; border: 1px solid var(--border-soft); border-radius: 8px; background: var(--bg-card); }
.cloud-skeleton { display: grid; gap: 12px; }
.cloud-skeleton span { display: block; height: 86px; border-radius: 8px; background: linear-gradient(90deg, var(--bg-elev), var(--bg-card), var(--bg-elev)); background-size: 200% 100%; animation: cloud-shimmer 1.4s ease-in-out infinite; }
.cloud-notice { min-height: 160px; display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 16px; }
.cloud-notice.danger { color: var(--danger); }
.cloud-notice p, .section-heading p, .section-toolbar p { margin: 4px 0 0; color: var(--text-dim); }
.sync-error-content { min-width: 0; }
.sync-error-meta { display: flex; flex-wrap: wrap; gap: 6px 14px; margin-top: 8px; color: var(--text-faint); font-size: 11px; }
.sync-retry { margin-top: 12px; color: var(--danger); }
.cloud-auth-tabs { width: fit-content; display: grid; grid-template-columns: repeat(2, minmax(132px, 1fr)); gap: 3px; padding: 3px; margin-bottom: 16px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-elev); }
.cloud-auth-tabs button { min-height: 38px; display: inline-flex; align-items: center; justify-content: center; gap: 7px; padding: 7px 16px; border: 0; border-radius: 6px; background: transparent; color: var(--text-dim); font-weight: 600; cursor: pointer; }
.cloud-auth-tabs button.active { background: var(--bg-card); color: var(--text); box-shadow: 0 1px 4px rgba(50, 43, 34, .1); }
.auth-panel { max-width: 760px; }
.section-heading, .account-identity { display: flex; align-items: center; gap: 12px; }
.section-heading h2, .account-identity h2, .section-toolbar h2 { margin: 0; font-size: 15px; }
.cloud-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-top: 20px; }
.cloud-form .field { margin: 0; }
.cloud-form small { color: var(--text-faint); }
.form-submit { align-self: end; justify-self: start; min-width: 130px; }
.compact-form { grid-template-columns: minmax(180px, 1fr) auto; align-items: end; }
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
@keyframes cloud-shimmer { to { background-position: -200% 0; } }
@media (prefers-reduced-motion: reduce) { .cloud-skeleton span { animation: none; } }
@media (max-width: 720px) {
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
