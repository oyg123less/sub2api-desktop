<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { api, type CloudReceivedShare } from "../../api/control";
import { useAppStore } from "../../store";
import Icon from "../Icon.vue";
import { formatConnectionCode, parseConnectionText } from "./connectionText";

const emit = defineEmits<{ connected: [share: CloudReceivedShare] }>();
const { t } = useI18n();
const app = useAppStore();
const connectionCode = ref("");
const password = ref("");
const busy = ref(false);
const connected = ref<CloudReceivedShare | null>(null);
const claimIdempotencyKey = ref("");
const ready = computed(() => connectionCode.value.replace(/\D/g, "").length === 9 && /^[A-HJ-NP-Z2-9]{6}$/.test(password.value.toUpperCase()));
const connectionReady = computed(() => connected.value?.connection_test?.ok ?? connected.value?.local_enabled ?? false);

function applyText(text: string): boolean {
  const parsed = parseConnectionText(text);
  if (!parsed) return false;
  connectionCode.value = formatConnectionCode(parsed.connectionCode);
  password.value = parsed.password;
  return true;
}

async function pasteDetails() {
  try {
    if (!applyText(await navigator.clipboard.readText())) app.toast(t("cloud.v4.connect.pasteInvalid"), "error");
  } catch {
    app.toast(t("common.copyFailed"), "error");
  }
}

function handlePaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData("text") || "";
  if (applyText(text)) event.preventDefault();
}

async function claim() {
  if (!ready.value) return;
  busy.value = true;
  try {
    if (!claimIdempotencyKey.value) claimIdempotencyKey.value = globalThis.crypto?.randomUUID?.() || `claim-${Date.now()}`;
    const share = await api.cloudConnectClaimAndUse({
      connection_code: connectionCode.value.replace(/\D/g, ""),
      password: password.value.toUpperCase(),
      idempotency_key: claimIdempotencyKey.value,
    });
    connected.value = share;
    claimIdempotencyKey.value = "";
    emit("connected", share);
    app.toast(t(connectionReady.value ? "cloud.v4.connect.connectedSuccess" : "cloud.v4.connect.connectedNeedsAttention"), connectionReady.value ? "success" : "error");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

watch([connectionCode, password], () => {
  if (!busy.value) claimIdempotencyKey.value = "";
});

function reset() {
  connected.value = null;
  connectionCode.value = "";
  password.value = "";
}
</script>

<template>
  <section class="quick-panel join-panel" data-test="quick-share-join" @paste="handlePaste">
    <div class="quick-heading">
      <span class="quick-icon"><Icon name="download" :size="20" /></span>
      <div><h2>{{ t("cloud.v4.connect.joinTitle") }}</h2><p>{{ t("cloud.v4.connect.joinDesc") }}</p></div>
    </div>

    <div v-if="connected" class="connect-success" :class="{ 'needs-attention': !connectionReady }">
      <span><Icon :name="connectionReady ? 'check' : 'warn'" :size="24" /></span>
      <h3>{{ t(connectionReady ? "cloud.v4.connect.readyTitle" : "cloud.v4.connect.attentionTitle") }}</h3>
      <p>{{ t(connectionReady ? "cloud.v4.connect.readyDesc" : "cloud.v4.connect.attentionDesc", { name: connected.owner.display_name }) }}</p>
      <p v-if="!connectionReady && connected.connection_test?.message" class="test-message">{{ connected.connection_test.message }}</p>
      <div><strong>{{ connected.group.name }}</strong><small>{{ connected.rpm_limit }} RPM · {{ connected.concurrency_limit }} {{ t("cloud.v4.connect.concurrentShort") }}</small></div>
      <button class="btn btn-ghost" type="button" @click="reset">{{ t("cloud.v4.connect.connectAnother") }}</button>
    </div>

    <div v-else class="join-form">
      <button class="paste-button" type="button" @click="pasteDetails"><Icon name="copy" :size="15" />{{ t("cloud.v4.connect.pasteDetails") }}</button>
      <label><span>{{ t("cloud.v4.connect.connectionCode") }}</span><input v-model="connectionCode" data-test="connect-code" class="input mono code-field" maxlength="11" inputmode="numeric" placeholder="000 000 000" @input="connectionCode = formatConnectionCode(connectionCode)" /></label>
      <label><span>{{ t("cloud.v4.connect.temporaryPassword") }}</span><input v-model="password" data-test="connect-password" class="input mono password-field" maxlength="6" autocomplete="one-time-code" placeholder="ABC234" @input="password = password.toUpperCase().replace(/[^A-HJ-NP-Z2-9]/g, '')" @keyup.enter="claim" /></label>
      <button class="btn btn-primary connect-button" data-test="connect-and-use" type="button" :disabled="!ready || busy" @click="claim"><Icon name="link" :size="15" />{{ busy ? t("cloud.v4.connect.connecting") : t("cloud.v4.connect.connectAndUse") }}</button>
      <p class="join-note"><Icon name="info" :size="14" />{{ t("cloud.v4.connect.loginAndSafety") }}</p>
    </div>
  </section>
</template>

<style scoped>
.quick-panel { min-width: 0; padding: 22px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); box-shadow: var(--shadow-sm); }
.quick-heading { min-height: 48px; display: grid; grid-template-columns: 42px minmax(0,1fr); align-items: center; gap: 12px; }.quick-heading h2 { margin: 0; font-size: 17px; }.quick-heading p { margin: 4px 0 0; color: var(--text-dim); font-size: 13px; line-height: 1.45; }.quick-icon { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 7px; color: var(--success); background: color-mix(in srgb, var(--success) 12%, transparent); }
.join-form { position: relative; display: grid; grid-template-columns: 1.25fr .8fr; gap: 12px; padding-top: 28px; }.join-form label { display: grid; gap: 6px; }.join-form label > span { color: var(--text-dim); font-size: 12px; }.code-field, .password-field { height: 52px; font-size: 20px; font-weight: 700; letter-spacing: 0; text-align: center; }.paste-button { position: absolute; top: 0; right: 0; display: inline-flex; align-items: center; gap: 5px; padding: 2px 0; border: 0; color: var(--primary); background: transparent; cursor: pointer; }.connect-button { grid-column: 1 / -1; min-height: 42px; justify-content: center; }.join-note { grid-column: 1 / -1; display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--text-faint); font-size: 12px; line-height: 1.5; }
.connect-success { min-height: 230px; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }.connect-success > span { width: 46px; height: 46px; display: grid; place-items: center; border-radius: 50%; color: var(--success); background: color-mix(in srgb, var(--success) 13%, transparent); }.connect-success h3 { margin: 12px 0 4px; }.connect-success p { margin: 0; color: var(--text-dim); }.connect-success > div { display: grid; gap: 3px; margin: 16px 0; }.connect-success small { color: var(--text-faint); }
.connect-success.needs-attention > span { color: var(--warn); background: var(--warn-soft); }.connect-success .test-message { max-width: 420px; margin-top: 8px; color: var(--text-faint); font-size: 12px; overflow-wrap: anywhere; }
@media (max-width: 720px) { .join-form { grid-template-columns: 1fr; }.connect-button, .join-note { grid-column: auto; } }
</style>
