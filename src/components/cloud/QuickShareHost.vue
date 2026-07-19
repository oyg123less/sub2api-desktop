<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, type Account, type CloudConnectHost } from "../../api/control";
import { useAppStore } from "../../store";
import Icon from "../Icon.vue";
import { formatConnectionCode, formatConnectionText } from "./connectionText";

const props = defineProps<{ host: CloudConnectHost; accounts: Account[] }>();
const emit = defineEmits<{ updated: [host?: CloudConnectHost] }>();
const { t } = useI18n();
const app = useAppStore();
const busy = ref("");
const editingAccounts = ref(false);
const selected = ref<number[]>([]);
const relayModes = ref<Record<number, "owner_device" | "worker_direct">>({});
const maxClaims = ref(1);
const durationMinutes = ref(30);
const optionsOpen = ref(false);
const startIdempotencyKey = ref("");

const availableAccounts = computed(() => props.accounts.filter((account) => account.status === "active" && account.client_uid));
const activeWindow = computed(() => Boolean(props.host.window && new Date(props.host.window.expires_at).getTime() > Date.now()));
const canCopy = computed(() => Boolean(props.host.endpoint?.connection_code && props.host.temporary_password && activeWindow.value));

function openAccounts() {
  selected.value = availableAccounts.value
    .filter((account) => props.host.accounts.some((item) => item.account_uid === account.client_uid))
    .map((account) => account.id);
  if (!selected.value.length) selected.value = availableAccounts.value.map((account) => account.id);
  for (const account of availableAccounts.value) {
    const existing = props.host.accounts.find((item) => item.account_uid === account.client_uid);
    relayModes.value[account.id] = existing?.relay_mode || "owner_device";
  }
  editingAccounts.value = true;
}

async function saveAccounts() {
  if (!selected.value.length) return app.toast(t("cloud.v4.connect.selectOneAccount"), "error");
  busy.value = "accounts";
  try {
    const host = await api.cloudConnectHostAccounts(selected.value.map((account_id) => ({
      account_id,
      relay_mode: relayModes.value[account_id] || "owner_device",
    })));
    editingAccounts.value = false;
    emit("updated", host);
    app.toast(t("cloud.v4.connect.accountsSaved"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function start(rotate = false) {
  busy.value = rotate ? "rotate" : "start";
  try {
    if (!startIdempotencyKey.value) startIdempotencyKey.value = globalThis.crypto?.randomUUID?.() || `connect-${Date.now()}`;
    const input = { max_claims: maxClaims.value, duration_minutes: durationMinutes.value, idempotency_key: startIdempotencyKey.value };
    const response = rotate ? await api.cloudConnectHostRotatePassword(input) : await api.cloudConnectHostStart(input);
    emit("updated", response.host || response);
    optionsOpen.value = false;
    startIdempotencyKey.value = "";
    app.toast(t(rotate ? "cloud.v4.connect.passwordRotated" : "cloud.v4.connect.shareStarted"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function action(actionName: "pause" | "resume" | "reset-code") {
  busy.value = actionName;
  try {
    const host = await api.cloudConnectHostAction(actionName);
    emit("updated", host);
    app.toast(t(`cloud.v4.connect.${actionName}Done`), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function copyDetails() {
  if (!canCopy.value) return;
  await navigator.clipboard.writeText(formatConnectionText(
    props.host.endpoint!.connection_code,
    props.host.temporary_password!,
    props.host.window?.expires_at,
  ));
  app.toast(t("cloud.v4.connect.detailsCopied"), "success");
}

async function toggleRecipient(id: string, status: "active" | "paused") {
  busy.value = `recipient-${id}`;
  try {
    const host = await api.cloudConnectRecipientUpdate(id, { status: status === "active" ? "paused" : "active" });
    emit("updated", host);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}

async function removeRecipient(id: string) {
  busy.value = `recipient-${id}`;
  try {
    await api.cloudConnectRecipientDelete(id);
    emit("updated");
    app.toast(t("cloud.v4.connect.accessRemoved"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = "";
  }
}
</script>

<template>
  <section class="quick-panel host-panel" data-test="quick-share-host">
    <div class="quick-heading">
      <span class="quick-icon"><Icon name="upload" :size="20" /></span>
      <div><h2>{{ t("cloud.v4.connect.hostTitle") }}</h2><p>{{ t("cloud.v4.connect.hostDesc") }}</p></div>
      <span v-if="host.configured" class="badge" :class="host.endpoint?.status === 'active' ? 'badge-success' : 'badge-neutral'">
        {{ t(`cloud.v4.connect.hostStatus.${host.endpoint?.status || 'paused'}`) }}
      </span>
    </div>

    <div v-if="!host.configured" class="quick-empty">
      <strong>{{ t("cloud.v4.connect.firstShare") }}</strong>
      <p>{{ t("cloud.v4.connect.firstShareDesc") }}</p>
      <button class="btn btn-primary" type="button" @click="openAccounts"><Icon name="plus" :size="15" />{{ t("cloud.v4.connect.selectAccounts") }}</button>
    </div>

    <template v-else>
      <div class="connection-display">
        <div><span>{{ t("cloud.v4.connect.connectionCode") }}</span><strong class="mono">{{ formatConnectionCode(host.endpoint?.connection_code || '') }}</strong></div>
        <div><span>{{ t("cloud.v4.connect.temporaryPassword") }}</span><strong class="mono">{{ host.temporary_password || "------" }}</strong></div>
      </div>
      <p v-if="host.window" class="window-status">
        {{ t("cloud.v4.connect.windowStatus", { used: host.window.claimed_count, total: host.window.max_claims, time: new Date(host.window.expires_at).toLocaleString() }) }}
      </p>
      <p v-else class="window-status warning">{{ t("cloud.v4.connect.noActivePassword") }}</p>

      <div class="quick-actions">
        <button v-if="!activeWindow" class="btn btn-primary" type="button" :disabled="busy !== ''" @click="optionsOpen = true"><Icon name="play" :size="14" />{{ t("cloud.v4.connect.startSharing") }}</button>
        <button v-else class="btn btn-primary" type="button" :disabled="!canCopy" @click="copyDetails"><Icon name="copy" :size="14" />{{ t("cloud.v4.connect.copyDetails") }}</button>
        <button class="btn btn-ghost" type="button" @click="openAccounts"><Icon name="accounts" :size="14" />{{ t("cloud.v4.connect.accountPool") }}</button>
        <button v-if="activeWindow" class="btn btn-ghost" type="button" @click="optionsOpen = true"><Icon name="refresh" :size="14" />{{ t("cloud.v4.connect.refreshPassword") }}</button>
        <button v-if="host.endpoint?.status === 'active'" class="icon-button" type="button" :title="t('cloud.v4.pause')" @click="action('pause')"><Icon name="stop" :size="14" /></button>
        <button v-else-if="activeWindow" class="icon-button" type="button" :title="t('cloud.v4.resume')" @click="action('resume')"><Icon name="play" :size="14" /></button>
      </div>

      <div class="connected-summary">
        <div><strong>{{ t("cloud.v4.connect.connectedUsers") }}</strong><span>{{ host.recipients.length }}</span></div>
        <article v-for="recipient in host.recipients.slice(0, 3)" :key="recipient.public_id">
          <span class="mini-avatar">{{ recipient.display_name.slice(0, 1).toUpperCase() }}</span>
          <div><strong>{{ recipient.display_name }}</strong><small>{{ recipient.used_requests }} / {{ recipient.quota_requests || "∞" }} · {{ recipient.rpm_limit }} RPM</small></div>
          <button class="icon-button" :title="t(recipient.status === 'active' ? 'cloud.v4.pause' : 'cloud.v4.resume')" :disabled="busy !== ''" @click="toggleRecipient(recipient.public_id, recipient.status)"><Icon :name="recipient.status === 'active' ? 'stop' : 'play'" :size="13" /></button>
          <button class="icon-button danger-text" :title="t('common.delete')" :disabled="busy !== ''" @click="removeRecipient(recipient.public_id)"><Icon name="trash" :size="13" /></button>
        </article>
      </div>
    </template>

    <div v-if="editingAccounts" class="quick-subpanel">
      <div class="subpanel-head"><strong>{{ t("cloud.v4.connect.selectAccounts") }}</strong><button class="icon-button" @click="editingAccounts = false"><Icon name="close" :size="14" /></button></div>
      <p>{{ t("cloud.v4.connect.selectAccountsDesc") }}</p>
      <div class="account-choice-list">
        <label v-for="account in availableAccounts" :key="account.id">
          <input v-model="selected" type="checkbox" :value="account.id" />
          <span><strong>{{ account.email || account.base_url }}</strong><small>{{ account.account_type === 'oauth' ? 'ChatGPT OAuth' : account.base_url }}</small></span>
          <select v-if="account.account_type === 'api_key'" v-model="relayModes[account.id]" class="select compact-select">
            <option value="owner_device">{{ t("cloud.v4.relayMode.owner_device") }}</option>
            <option value="worker_direct">{{ t("cloud.v4.relayMode.worker_direct") }}</option>
          </select>
          <span v-else class="badge badge-neutral">{{ t("cloud.v4.relayMode.owner_device") }}</span>
        </label>
      </div>
      <div class="subpanel-actions"><button class="btn btn-ghost" @click="editingAccounts = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="busy !== ''" @click="saveAccounts">{{ t("common.save") }}</button></div>
    </div>

    <div v-if="optionsOpen" class="quick-subpanel">
      <div class="subpanel-head"><strong>{{ activeWindow ? t("cloud.v4.connect.refreshPassword") : t("cloud.v4.connect.startSharing") }}</strong><button class="icon-button" @click="optionsOpen = false"><Icon name="close" :size="14" /></button></div>
      <div class="sharing-options">
        <label><span>{{ t("cloud.v4.connect.allowedPeople") }}</span><input v-model.number="maxClaims" class="input" type="number" min="1" max="20" /></label>
        <label><span>{{ t("cloud.v4.connect.validMinutes") }}</span><select v-model.number="durationMinutes" class="select"><option :value="15">15</option><option :value="30">30</option><option :value="60">60</option><option :value="120">120</option></select></label>
      </div>
      <p class="security-note"><Icon name="info" :size="14" />{{ t("cloud.v4.connect.independentKeys") }}</p>
      <div class="subpanel-actions"><button class="btn btn-ghost" @click="optionsOpen = false">{{ t("common.cancel") }}</button><button class="btn btn-primary" :disabled="busy !== ''" @click="start(activeWindow)">{{ t(activeWindow ? "cloud.v4.connect.confirmRefresh" : "cloud.v4.connect.confirmStart") }}</button></div>
    </div>
  </section>
</template>

<style scoped>
.quick-panel { position: relative; min-width: 0; padding: 22px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); box-shadow: var(--shadow-sm); }
.quick-heading { min-height: 48px; display: grid; grid-template-columns: 42px minmax(0,1fr) auto; align-items: center; gap: 12px; }
.quick-heading h2 { margin: 0; font-size: 17px; }.quick-heading p { margin: 4px 0 0; color: var(--text-dim); font-size: 13px; line-height: 1.45; }
.quick-icon { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 7px; color: var(--primary); background: var(--primary-soft); }
.quick-empty { min-height: 206px; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 9px; padding: 18px 3px 0; }.quick-empty p { max-width: 430px; margin: 0 0 8px; color: var(--text-dim); }
.connection-display { display: grid; grid-template-columns: 1.35fr 1fr; gap: 10px; margin-top: 21px; }.connection-display > div { min-width: 0; padding: 13px 14px; border: 1px solid var(--border-soft); background: var(--bg-elev); border-radius: 6px; }.connection-display span { display: block; margin-bottom: 5px; color: var(--text-faint); font-size: 11px; }.connection-display strong { display: block; overflow: hidden; font-size: 21px; letter-spacing: 0; white-space: nowrap; text-overflow: ellipsis; }
.window-status { min-height: 18px; margin: 9px 2px 0; color: var(--text-dim); font-size: 12px; }.window-status.warning { color: var(--warn); }
.quick-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 7px; margin-top: 15px; }.quick-actions .icon-button { margin-left: auto; }
.connected-summary { margin-top: 18px; border-top: 1px solid var(--border-soft); }.connected-summary > div { display: flex; justify-content: space-between; padding: 12px 2px 8px; }.connected-summary > div span { color: var(--text-faint); }.connected-summary article { min-height: 42px; display: flex; align-items: center; gap: 8px; padding: 6px 1px; }.connected-summary article > div { min-width: 0; display: grid; flex: 1; }.connected-summary small { overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }.mini-avatar { width: 28px; height: 28px; display: grid; place-items: center; border-radius: 50%; color: var(--primary); background: var(--primary-soft); font-weight: 700; }
.quick-subpanel { position: absolute; z-index: 5; inset: 68px 14px 14px; overflow: auto; padding: 16px; border: 1px solid var(--border); border-radius: 7px; background: var(--bg-card); box-shadow: var(--shadow); }.subpanel-head { display: flex; align-items: center; justify-content: space-between; }.quick-subpanel > p { margin: 5px 0 12px; color: var(--text-dim); font-size: 12px; }
.account-choice-list { display: grid; gap: 6px; }.account-choice-list label { min-height: 50px; display: flex; align-items: center; gap: 10px; padding: 8px; border: 1px solid var(--border-soft); border-radius: 6px; }.account-choice-list label > span:nth-of-type(1) { min-width: 0; display: grid; flex: 1; }.account-choice-list small { overflow: hidden; color: var(--text-faint); text-overflow: ellipsis; white-space: nowrap; }.compact-select { width: 128px; }
.subpanel-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }.sharing-options { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 15px; }.sharing-options label { display: grid; gap: 5px; }.sharing-options span { color: var(--text-dim); font-size: 12px; }.security-note { display: flex; gap: 6px; align-items: flex-start; }
@media (max-width: 720px) { .connection-display, .sharing-options { grid-template-columns: 1fr; }.quick-heading { grid-template-columns: 42px minmax(0,1fr); }.quick-heading > .badge { grid-column: 2; justify-self: start; }.quick-subpanel { inset: 62px 8px 8px; }.account-choice-list label { flex-wrap: wrap; }.compact-select { width: 100%; } }
</style>
