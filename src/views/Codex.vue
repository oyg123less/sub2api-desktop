<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import Collapsible from "../components/Collapsible.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import CopyField from "../components/CopyField.vue";
import Icon from "../components/Icon.vue";
import {
  api,
  type CodexFiles,
  type CodexRemoteProbe,
  type CodexRemoteTarget,
  type CodexStatus,
} from "../api/control";
import { useAppStore } from "../store";
import {
  codexActiveTab,
  hostKeyAccepted,
  isValidCodexModel,
  remoteForm,
  remoteModelInitialized,
  remoteProbe,
  testedSignature,
  validateCodexRemoteForm,
  type CodexRemoteFormErrors,
  type CodexRemoteFormField,
} from "./codexRemote";

const { t } = useI18n();
const app = useAppStore();

const activeTab = codexActiveTab;
const st = ref<CodexStatus | null>(null);
const files = ref<CodexFiles | null>(null);
const loading = ref(true);
const busy = ref(false);
const savingFiles = ref(false);
const restoreOpen = ref(false);
const configOpen = ref(false);
const authOpen = ref(false);
const modelDraft = ref("");
const configDraft = ref("");
const authDraft = ref("");

const targets = ref<CodexRemoteTarget[]>([]);
const remoteBusy = ref<string | null>(null);
const remoteErrors = ref<CodexRemoteFormErrors>({});
const hostKeyOpen = ref(false);
const targetFilesOpen = ref<Record<number, boolean>>({});
const restoreTarget = ref<CodexRemoteTarget | null>(null);
const deleteTarget = ref<CodexRemoteTarget | null>(null);

let targetPoll: ReturnType<typeof setInterval> | undefined;

const applied = computed(() => st.value?.applied ?? false);
const modelValid = computed(() => isValidCodexModel(modelDraft.value));
const remoteSignature = computed(() => JSON.stringify({
  host: remoteForm.value.host.trim(),
  port: Number(remoteForm.value.port),
  user: remoteForm.value.user.trim(),
  password: remoteForm.value.password,
}));
const configPreview = computed(() => {
  const base = st.value?.config_preview || "";
  if (!modelValid.value) return base;
  return base.replace(/^model = ".*"$/m, `model = "${modelDraft.value.trim()}"`);
});
const authPreview = computed(() => st.value?.auth_preview || "");

watch(remoteSignature, () => {
  if (testedSignature.value && testedSignature.value !== remoteSignature.value) {
    remoteProbe.value = null;
    testedSignature.value = "";
    hostKeyAccepted.value = false;
  }
});

async function load() {
  try {
    const [status, localFiles, remoteTargets] = await Promise.all([
      api.codexStatus(),
      api.codexFiles(),
      api.codexRemoteTargets(),
    ]);
    st.value = status;
    files.value = localFiles;
    targets.value = remoteTargets.targets;
    modelDraft.value = status.model;
    if (!remoteModelInitialized.value) {
      remoteForm.value.model = status.model;
      remoteModelInitialized.value = true;
    }
    configDraft.value = localFiles.config_content || localFiles.config_default;
    authDraft.value = localFiles.auth_content || localFiles.auth_default;
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

async function loadTargets(silent = false) {
  try {
    targets.value = (await api.codexRemoteTargets()).targets;
  } catch (error) {
    if (!silent) app.toast((error as Error).message, "error");
  }
}

async function applyLocal() {
  if (!modelValid.value) {
    app.toast(t("codex.modelInvalid"), "error");
    return;
  }
  busy.value = true;
  try {
    st.value = await api.codexApply(modelDraft.value.trim());
    modelDraft.value = st.value.model;
    files.value = await api.codexFiles();
    configDraft.value = files.value.config_content || files.value.config_default;
    authDraft.value = files.value.auth_content || files.value.auth_default;
    app.toast(t("codex.applied"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

function fillDefaults() {
  if (!files.value) return;
  configDraft.value = files.value.config_default;
  authDraft.value = files.value.auth_default;
}

async function saveFiles() {
  if (authDraft.value.trim()) {
    try {
      JSON.parse(authDraft.value);
    } catch {
      app.toast(t("codex.authInvalid"), "error");
      return;
    }
  }
  savingFiles.value = true;
  try {
    files.value = await api.saveCodexFiles(configDraft.value, authDraft.value);
    st.value = await api.codexStatus();
    app.toast(t("codex.filesSaved"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    savingFiles.value = false;
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    app.toast(t("common.copied"), "success");
  } catch {
    app.toast(t("codex.copyFailed"), "error");
  }
}

async function confirmLocalRestore() {
  busy.value = true;
  try {
    st.value = await api.codexRestore();
    files.value = await api.codexFiles();
    configDraft.value = files.value.config_content || files.value.config_default;
    authDraft.value = files.value.auth_content || files.value.auth_default;
    restoreOpen.value = false;
    app.toast(t("codex.restored"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

function validateRemote(): boolean {
  remoteForm.value.port = Number(remoteForm.value.port);
  remoteForm.value.remotePort = Number(remoteForm.value.remotePort);
  remoteErrors.value = validateCodexRemoteForm(remoteForm.value);
  return Object.keys(remoteErrors.value).length === 0;
}

function remoteFieldError(field: CodexRemoteFormField): string {
  const error = remoteErrors.value[field];
  if (!error) return "";
  if (error === "required") return t("codex.fieldRequired");
  if (field === "model") return t("codex.modelInvalid");
  return t("codex.portInvalid");
}

async function testRemoteConnection() {
  if (!validateRemote()) return;
  if (!remoteForm.value.password) {
    remoteErrors.value = { ...remoteErrors.value, password: "required" };
    return;
  }
  remoteBusy.value = "test";
  try {
    const probe = await api.codexRemoteTest({
      host: remoteForm.value.host.trim(),
      port: remoteForm.value.port,
      user: remoteForm.value.user.trim(),
      password: remoteForm.value.password,
    });
    remoteProbe.value = probe;
    testedSignature.value = remoteSignature.value;
    hostKeyAccepted.value = probe.known;
    if (probe.known) app.toast(t("codex.testSuccess"), "success");
    else hostKeyOpen.value = true;
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    remoteBusy.value = null;
  }
}

function confirmHostKey() {
  hostKeyAccepted.value = true;
  hostKeyOpen.value = false;
  app.toast(t("codex.hostKeyAccepted"), "success");
}

async function injectRemote() {
  if (!validateRemote()) return;
  const canUseSavedCredential = Boolean(remoteForm.value.id && !remoteForm.value.password);
  if (!canUseSavedCredential && (testedSignature.value !== remoteSignature.value || !remoteProbe.value)) {
    await testRemoteConnection();
    return;
  }
  if (!canUseSavedCredential && !hostKeyAccepted.value) {
    hostKeyOpen.value = true;
    return;
  }
  remoteBusy.value = "inject";
  try {
    const target = await api.codexRemoteInject({
      id: remoteForm.value.id,
      host: remoteForm.value.host.trim(),
      port: remoteForm.value.port,
      user: remoteForm.value.user.trim(),
      password: remoteForm.value.password,
      model: remoteForm.value.model.trim(),
      remote_port: remoteForm.value.remotePort,
      save: remoteForm.value.save,
      accept_host_key: hostKeyAccepted.value,
    });
    const index = targets.value.findIndex((item) => item.id === target.id || item.id === remoteForm.value.id);
    if (index >= 0) targets.value.splice(index, 1, target);
    else targets.value.push(target);
    remoteForm.value.id = target.id;
    remoteForm.value.password = "";
    remoteForm.value.save = target.saved;
    remoteProbe.value = null;
    testedSignature.value = "";
    hostKeyAccepted.value = false;
    app.toast(t("codex.injectSuccess"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    remoteBusy.value = null;
  }
}

function newRemoteTarget() {
  remoteForm.value = {
    host: "",
    port: 22,
    user: "",
    password: "",
    model: st.value?.model || "gpt-5.6",
    remotePort: 8080,
    save: true,
  };
  remoteErrors.value = {};
  remoteProbe.value = null;
  testedSignature.value = "";
  hostKeyAccepted.value = false;
}

function reinjectTarget(target: CodexRemoteTarget) {
  remoteForm.value = {
    id: target.id,
    host: target.host,
    port: target.port,
    user: target.user,
    password: "",
    model: target.model,
    remotePort: target.remote_port,
    save: target.saved,
  };
  remoteErrors.value = {};
  remoteProbe.value = null;
  testedSignature.value = "";
  hostKeyAccepted.value = false;
  window.scrollTo({ top: 0, behavior: "smooth" });
}

async function onTunnelChange(target: CodexRemoteTarget, event: Event) {
  const input = event.target as HTMLInputElement;
  remoteBusy.value = `tunnel-${target.id}`;
  try {
    const updated = await api.codexRemoteSetTunnel(target.id, input.checked);
    replaceTarget(updated);
  } catch (error) {
    input.checked = target.tunnel_enabled;
    app.toast((error as Error).message, "error");
  } finally {
    remoteBusy.value = null;
  }
}

async function confirmRemoteRestore() {
  if (!restoreTarget.value) return;
  const target = restoreTarget.value;
  remoteBusy.value = `restore-${target.id}`;
  try {
    replaceTarget(await api.codexRemoteRestore(target.id));
    restoreTarget.value = null;
    app.toast(t("codex.remoteRestored"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    remoteBusy.value = null;
  }
}

async function confirmRemoteDelete() {
  if (!deleteTarget.value) return;
  const target = deleteTarget.value;
  remoteBusy.value = `delete-${target.id}`;
  try {
    await api.codexRemoteDelete(target.id);
    targets.value = targets.value.filter((item) => item.id !== target.id);
    if (remoteForm.value.id === target.id) newRemoteTarget();
    deleteTarget.value = null;
    app.toast(t("codex.remoteDeleted"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    remoteBusy.value = null;
  }
}

function replaceTarget(target: CodexRemoteTarget) {
  const index = targets.value.findIndex((item) => item.id === target.id);
  if (index >= 0) targets.value.splice(index, 1, target);
  else targets.value.push(target);
}

function targetStatusLabel(target: CodexRemoteTarget): string {
  return t(`codex.tunnelStatus.${target.tunnel_status}`);
}

function targetStatusClass(target: CodexRemoteTarget): string {
  if (target.tunnel_status === "connected") return "badge-success";
  if (target.tunnel_status === "down") return "badge-danger";
  return "badge-neutral";
}

onMounted(() => {
  void load();
  targetPoll = setInterval(() => {
    if (activeTab.value === "remote") void loadTargets(true);
  }, 5000);
});

onBeforeUnmount(() => {
  if (targetPoll) clearInterval(targetPoll);
});
</script>

<template>
  <div class="codex-page">
    <div class="page-header">
      <h1 class="page-title">{{ t("codex.title") }}</h1>
      <p class="page-desc">{{ t("codex.desc") }}</p>
    </div>

    <div class="codex-tabs" role="tablist" :aria-label="t('codex.title')">
      <button
        type="button"
        role="tab"
        data-test="tab-local"
        :aria-selected="activeTab === 'local'"
        :class="{ active: activeTab === 'local' }"
        @click="activeTab = 'local'"
      >
        <Icon name="terminal" :size="16" />
        {{ t("codex.localTab") }}
      </button>
      <button
        type="button"
        role="tab"
        data-test="tab-remote"
        :aria-selected="activeTab === 'remote'"
        :class="{ active: activeTab === 'remote' }"
        @click="activeTab = 'remote'"
      >
        <Icon name="server" :size="16" />
        {{ t("codex.remoteTab") }}
      </button>
    </div>

    <div v-if="loading" class="codex-loading">{{ t("common.loading") }}</div>

    <template v-else-if="st && activeTab === 'local'">
      <section class="codex-panel local-summary">
        <div class="local-summary-head">
          <div class="status-stack">
            <span class="badge" :class="applied ? 'badge-success' : 'badge-neutral'">
              <span class="badge-dot"></span>
              {{ applied ? t("codex.statusOn") : t("codex.statusOff") }}
            </span>
            <span class="faint text-sm path-text">{{ st.config_path }}</span>
            <span v-if="st.backup_at" class="faint text-sm">
              {{ t("codex.backupAt", { time: new Date(st.backup_at).toLocaleString() }) }}
            </span>
          </div>
          <div class="action-row">
            <button class="btn btn-primary" :disabled="busy" @click="applyLocal">
              <Icon name="terminal" :size="16" />
              {{ applied ? t("codex.reapply") : t("codex.apply") }}
            </button>
            <button class="btn btn-ghost" :disabled="busy || !st.backup_exists" @click="restoreOpen = true">
              <Icon name="refresh" :size="14" />
              {{ t("codex.restore") }}
            </button>
          </div>
        </div>

        <div v-if="st.stale" class="stale-warning" role="status">
          <Icon name="warn" :size="16" />
          <div>
            <strong>{{ t("codex.configStale") }}</strong>
            <span>{{ t("codex.configStaleDesc") }}</span>
          </div>
        </div>

        <div class="local-fields">
          <div class="field no-margin">
            <label class="field-label">{{ t("codex.baseUrl") }}</label>
            <CopyField :value="st.base_url" />
          </div>
          <div class="field no-margin">
            <label class="field-label" for="codex-local-model">{{ t("codex.model") }}</label>
            <input
              id="codex-local-model"
              v-model="modelDraft"
              class="input mono"
              :class="{ 'input-error': !modelValid }"
              list="codex-model-options"
              placeholder="gpt-5.6"
            />
            <datalist id="codex-model-options">
              <option v-for="model in st.models" :key="model" :value="model" />
            </datalist>
            <p class="field-help" :class="{ 'text-danger': !modelValid }">
              {{ modelValid ? t("codex.modelHint") : t("codex.modelInvalid") }}
            </p>
          </div>
        </div>
      </section>

      <div v-if="files" class="file-sections">
        <div class="section-toolbar">
          <div>
            <h2>{{ t("codex.filesTitle") }}</h2>
            <p>{{ t("codex.filesDesc") }}</p>
          </div>
          <div class="action-row">
            <button class="btn btn-ghost btn-sm" :disabled="savingFiles" @click="fillDefaults">
              {{ t("codex.fillDefault") }}
            </button>
            <button class="btn btn-primary btn-sm" :disabled="savingFiles" @click="saveFiles">
              <Icon name="check" :size="14" />
              {{ t("codex.saveFiles") }}
            </button>
          </div>
        </div>

        <Collapsible v-model:open="configOpen">
          <template #trigger>
            <span class="file-trigger">
              <span>config.toml</span>
              <span>{{ files.config_path }}</span>
            </span>
          </template>
          <div class="file-editor">
            <div class="code-head">
              <span class="code-label">{{ files.config_path }}</span>
              <button class="btn btn-ghost btn-sm" @click="copyText(configDraft)">
                <Icon name="copy" :size="13" /> {{ t("common.copy") }}
              </button>
            </div>
            <textarea v-model="configDraft" rows="11" spellcheck="false"></textarea>
          </div>
        </Collapsible>

        <Collapsible v-model:open="authOpen">
          <template #trigger>
            <span class="file-trigger">
              <span>auth.json</span>
              <span>{{ files.auth_path }}</span>
            </span>
          </template>
          <div class="file-editor">
            <div class="code-head">
              <span class="code-label">{{ files.auth_path }}</span>
              <button class="btn btn-ghost btn-sm" @click="copyText(authDraft)">
                <Icon name="copy" :size="13" /> {{ t("common.copy") }}
              </button>
            </div>
            <textarea v-model="authDraft" rows="6" spellcheck="false"></textarea>
          </div>
        </Collapsible>
      </div>

      <section class="how-strip">
        <div><Icon name="docs" :size="17" /><strong>{{ t("codex.howTitle") }}</strong></div>
        <p>{{ t("codex.how1") }}</p>
        <p>{{ t("codex.how2") }}</p>
        <p>{{ t("codex.backupNote") }}</p>
      </section>
    </template>

    <template v-else-if="st && activeTab === 'remote'">
      <section class="codex-panel remote-form-panel">
        <div class="section-toolbar">
          <div>
            <h2>{{ remoteForm.id ? t("codex.reinjectTitle") : t("codex.remoteConnectTitle") }}</h2>
            <p>{{ t("codex.remoteConnectDesc") }}</p>
          </div>
          <button v-if="remoteForm.id" class="btn btn-ghost btn-sm" @click="newRemoteTarget">
            <Icon name="plus" :size="14" /> {{ t("codex.newTarget") }}
          </button>
        </div>

        <div class="remote-fields">
          <div class="field">
            <label class="field-label" for="remote-host">{{ t("codex.host") }}</label>
            <input id="remote-host" v-model="remoteForm.host" data-test="remote-host" class="input mono" :class="{ 'input-error': remoteErrors.host }" placeholder="user@example.com" />
            <p v-if="remoteErrors.host" class="field-error">{{ remoteFieldError("host") }}</p>
          </div>
          <div class="field compact-field">
            <label class="field-label" for="remote-port">{{ t("codex.port") }}</label>
            <input id="remote-port" v-model.number="remoteForm.port" data-test="remote-port" class="input mono" :class="{ 'input-error': remoteErrors.port }" type="number" min="1" max="65535" />
            <p v-if="remoteErrors.port" class="field-error">{{ remoteFieldError("port") }}</p>
          </div>
          <div class="field">
            <label class="field-label" for="remote-user">{{ t("codex.user") }}</label>
            <input id="remote-user" v-model="remoteForm.user" data-test="remote-user" class="input mono" :class="{ 'input-error': remoteErrors.user }" autocomplete="username" placeholder="deploy" />
            <p v-if="remoteErrors.user" class="field-error">{{ remoteFieldError("user") }}</p>
          </div>
          <div class="field">
            <label class="field-label" for="remote-password">{{ t("codex.password") }}</label>
            <input id="remote-password" v-model="remoteForm.password" data-test="remote-password" class="input" :class="{ 'input-error': remoteErrors.password }" type="password" autocomplete="current-password" :placeholder="remoteForm.id ? t('codex.savedPasswordPlaceholder') : ''" />
            <p v-if="remoteErrors.password" class="field-error">{{ remoteFieldError("password") }}</p>
          </div>
          <div class="field">
            <label class="field-label" for="remote-model">{{ t("codex.model") }}</label>
            <input id="remote-model" v-model="remoteForm.model" data-test="remote-model" class="input mono" :class="{ 'input-error': remoteErrors.model }" list="codex-model-options" />
            <p v-if="remoteErrors.model" class="field-error">{{ remoteFieldError("model") }}</p>
          </div>
          <div class="field compact-field">
            <label class="field-label" for="remote-forward-port">{{ t("codex.remotePort") }}</label>
            <input id="remote-forward-port" v-model.number="remoteForm.remotePort" data-test="remote-forward-port" class="input mono" :class="{ 'input-error': remoteErrors.remotePort }" type="number" min="1" max="65535" />
            <p v-if="remoteErrors.remotePort" class="field-error">{{ remoteFieldError("remotePort") }}</p>
          </div>
        </div>

        <div class="remote-form-footer">
          <label class="remember-check">
            <input v-model="remoteForm.save" type="checkbox" />
            <span>{{ t("codex.rememberTarget") }}</span>
          </label>
          <div class="action-row">
            <button class="btn btn-ghost" data-test="remote-test" :disabled="remoteBusy !== null" @click="testRemoteConnection">
              <Icon name="link" :size="15" />
              {{ remoteBusy === "test" ? t("codex.testing") : t("codex.testConn") }}
            </button>
            <button class="btn btn-primary" data-test="remote-inject" :disabled="remoteBusy !== null" @click="injectRemote">
              <Icon name="bolt" :size="15" />
              {{ remoteBusy === "inject" ? t("codex.injecting") : t("codex.injectNow") }}
            </button>
          </div>
        </div>

        <div v-if="remoteProbe" class="probe-result">
          <Icon name="check" :size="16" />
          <span>{{ remoteProbe.os }} · {{ remoteProbe.codex_dir }}</span>
          <span>{{ remoteProbe.known ? t("codex.hostKeyKnown") : t("codex.hostKeyPending") }}</span>
        </div>
      </section>

      <section class="targets-section" data-test="remote-targets">
        <div class="section-toolbar">
          <div>
            <h2>{{ t("codex.targetsTitle") }}</h2>
            <p>{{ t("codex.targetsDesc") }}</p>
          </div>
          <button class="icon-action" type="button" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="loadTargets()">
            <Icon name="refresh" :size="16" />
          </button>
        </div>

        <div v-if="targets.length === 0" class="target-empty">
          <Icon name="server" :size="28" />
          <strong>{{ t("codex.noTargets") }}</strong>
          <span>{{ t("codex.noTargetsDesc") }}</span>
        </div>

        <article v-for="target in targets" :key="target.id" class="target-card" :data-target-id="target.id">
          <div class="target-head">
            <div class="target-identity">
              <div class="target-icon"><Icon name="server" :size="18" /></div>
              <div>
                <h3>{{ target.name || `${target.user}@${target.host}` }}</h3>
                <p>{{ target.user }}@{{ target.host }}:{{ target.port }} · {{ target.model }}</p>
              </div>
            </div>
            <span class="badge" :class="targetStatusClass(target)">
              <span class="badge-dot"></span>{{ targetStatusLabel(target) }}
            </span>
          </div>

          <div class="target-controls">
            <div class="route-control">
              <label class="switch">
                <input
                  type="checkbox"
                  :checked="target.tunnel_enabled"
                  :disabled="remoteBusy !== null || !target.injected"
                  :aria-label="t('codex.routeToggle')"
                  @change="onTunnelChange(target, $event)"
                />
                <span class="slider"></span>
              </label>
              <div>
                <strong>{{ t("codex.routeToggle") }}</strong>
                <span>{{ t("codex.routeToggleDesc", { port: target.remote_port }) }}</span>
              </div>
            </div>
            <div class="target-actions">
              <button class="btn btn-ghost btn-sm" @click="reinjectTarget(target)">
                <Icon name="refresh" :size="13" /> {{ t("codex.reinject") }}
              </button>
              <button class="btn btn-ghost btn-sm" :disabled="remoteBusy !== null || !target.injected" @click="restoreTarget = target">
                {{ t("codex.restore") }}
              </button>
              <button class="icon-action danger" type="button" :title="t('common.delete')" :aria-label="t('common.delete')" :disabled="remoteBusy !== null" @click="deleteTarget = target">
                <Icon name="trash" :size="15" />
              </button>
            </div>
          </div>

          <p v-if="target.last_error" class="target-error"><Icon name="warn" :size="14" />{{ target.last_error }}</p>

          <Collapsible v-model:open="targetFilesOpen[target.id]">
            <template #trigger>
              <span class="target-files-trigger">{{ t("codex.remoteFiles") }} <span>config.toml · auth.json</span></span>
            </template>
            <div class="remote-previews">
              <div class="preview-block">
                <div class="code-head"><span class="code-label">~/.codex/config.toml</span></div>
                <pre>{{ target.config_preview }}</pre>
              </div>
              <div class="preview-block">
                <div class="code-head"><span class="code-label">~/.codex/auth.json</span></div>
                <pre>{{ target.auth_preview }}</pre>
              </div>
            </div>
          </Collapsible>
        </article>
      </section>

      <section class="how-strip remote-how">
        <div><Icon name="link" :size="17" /><strong>{{ t("codex.remoteHowTitle") }}</strong></div>
        <p>{{ t("codex.remoteHow1") }}</p>
        <p>{{ t("codex.remoteHow2") }}</p>
      </section>
    </template>

    <ConfirmModal
      :open="restoreOpen"
      :title="t('codex.restoreConfirm')"
      :desc="t('codex.restoreDesc')"
      :confirm-text="t('codex.restore')"
      :loading="busy"
      @confirm="confirmLocalRestore"
      @cancel="restoreOpen = false"
    />

    <ConfirmModal
      :open="hostKeyOpen"
      :title="t('codex.hostKeyConfirm')"
      :desc="t('codex.hostKeyConfirmDesc')"
      :confirm-text="t('codex.trustHostKey')"
      @confirm="confirmHostKey"
      @cancel="hostKeyOpen = false"
    >
      <div v-if="remoteProbe" class="fingerprint-box">
        <span>{{ t("codex.hostKeyFingerprint") }}</span>
        <code>{{ remoteProbe.host_key_fingerprint }}</code>
        <small>{{ remoteProbe.os }} · {{ remoteProbe.home }}</small>
      </div>
    </ConfirmModal>

    <ConfirmModal
      :open="Boolean(restoreTarget)"
      :title="t('codex.remoteRestoreConfirm')"
      :desc="t('codex.remoteRestoreDesc', { target: restoreTarget?.name || '' })"
      :confirm-text="t('codex.restore')"
      :loading="remoteBusy?.startsWith('restore-')"
      @confirm="confirmRemoteRestore"
      @cancel="restoreTarget = null"
    />

    <ConfirmModal
      :open="Boolean(deleteTarget)"
      :title="t('codex.remoteDeleteConfirm')"
      :desc="t('codex.remoteDeleteDesc', { target: deleteTarget?.name || '' })"
      :confirm-text="t('common.delete')"
      :loading="remoteBusy?.startsWith('delete-')"
      danger
      @confirm="confirmRemoteDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<style scoped>
.codex-page {
  width: 100%;
  min-width: 0;
  max-width: 1120px;
  margin: 0 auto;
}
.codex-tabs {
  width: fit-content;
  display: grid;
  grid-template-columns: repeat(2, minmax(132px, 1fr));
  gap: 3px;
  padding: 3px;
  margin-bottom: 16px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-elev);
}
.codex-tabs button {
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 7px 14px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-dim);
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}
.codex-tabs button.active {
  background: var(--bg-card);
  color: var(--text);
  box-shadow: 0 1px 4px rgba(50, 43, 34, 0.1);
}
.codex-loading {
  min-height: 240px;
  display: grid;
  place-items: center;
  color: var(--text-faint);
}
.codex-panel {
  min-width: 0;
  padding: 20px;
  border: 1px solid var(--border-soft);
  border-radius: 8px;
  background: var(--bg-card);
}
.local-summary-head,
.section-toolbar,
.remote-form-footer,
.target-head,
.target-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.status-stack {
  min-width: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}
.path-text {
  min-width: 0;
  overflow-wrap: anywhere;
}
.action-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.local-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 0.8fr);
  gap: 18px;
  align-items: start;
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid var(--border-soft);
}
.stale-warning {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 16px;
  padding: 10px 12px;
  border: 1px solid var(--warn);
  border-radius: 6px;
  background: var(--warn-soft);
  color: var(--warn);
}
.stale-warning > div {
  min-width: 0;
  display: grid;
  gap: 2px;
}
.stale-warning strong {
  font-size: 12.5px;
}
.stale-warning span {
  color: var(--text-dim);
  font-size: 11.5px;
}
.no-margin {
  margin: 0;
}
.field-help,
.field-error {
  margin: 5px 0 0;
  font-size: 11.5px;
  color: var(--text-faint);
}
.field-error {
  color: var(--danger);
}
.input-error {
  border-color: var(--danger);
}
.file-sections,
.targets-section {
  display: grid;
  gap: 10px;
  margin-top: 22px;
}
.section-toolbar {
  align-items: flex-end;
  margin-bottom: 2px;
}
.section-toolbar h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 650;
}
.section-toolbar p {
  margin: 4px 0 0;
  color: var(--text-dim);
  font-size: 12.5px;
}
.file-trigger,
.target-files-trigger {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-weight: 600;
}
.file-trigger span:last-child,
.target-files-trigger span {
  min-width: 0;
  color: var(--text-faint);
  font-family: var(--mono);
  font-size: 11.5px;
  font-weight: 400;
  overflow-wrap: anywhere;
}
.file-editor {
  border-top: 1px solid var(--border-soft);
  background: var(--bg-elev);
}
.code-head {
  min-height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 8px 6px 12px;
  border-bottom: 1px solid var(--border-soft);
}
.code-label {
  min-width: 0;
  color: var(--text-dim);
  font-family: var(--mono);
  font-size: 11.5px;
  overflow-wrap: anywhere;
}
.file-editor textarea {
  width: 100%;
  display: block;
  margin: 0;
  padding: 12px 14px;
  border: 0;
  outline: 0;
  resize: vertical;
  background: transparent;
  color: var(--text);
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.6;
}
.how-strip {
  margin-top: 22px;
  padding: 16px 2px 2px;
  border-top: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 12.5px;
}
.how-strip > div {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text);
}
.how-strip p {
  margin: 7px 0 0;
  line-height: 1.65;
}
.remote-form-panel {
  padding-bottom: 16px;
}
.remote-fields {
  display: grid;
  grid-template-columns: minmax(180px, 1.3fr) minmax(100px, 0.45fr) minmax(150px, 0.8fr);
  gap: 0 14px;
  margin-top: 18px;
}
.remote-fields .field {
  min-width: 0;
  margin-bottom: 14px;
}
.remote-form-footer {
  padding-top: 14px;
  border-top: 1px solid var(--border-soft);
}
.remember-check {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-dim);
  cursor: pointer;
  font-size: 12.5px;
}
.remember-check input {
  width: 16px;
  height: 16px;
  accent-color: var(--primary);
}
.probe-result {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
  padding: 9px 11px;
  border-radius: 6px;
  background: var(--success-soft);
  color: var(--success);
  font-size: 12px;
}
.probe-result span:nth-child(2) {
  min-width: 0;
  overflow-wrap: anywhere;
}
.probe-result span:last-child {
  margin-left: auto;
}
.icon-action {
  width: 32px;
  height: 32px;
  display: inline-grid;
  place-items: center;
  flex: 0 0 auto;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: transparent;
  color: var(--text-dim);
  cursor: pointer;
}
.icon-action:hover {
  background: var(--bg-hover);
  color: var(--text);
}
.icon-action.danger {
  color: var(--danger);
}
.target-empty {
  min-height: 190px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 6px;
  border: 1px dashed var(--border);
  border-radius: 8px;
  color: var(--text-faint);
  text-align: center;
}
.target-empty strong {
  color: var(--text-dim);
}
.target-card {
  min-width: 0;
  padding: 16px;
  border: 1px solid var(--border-soft);
  border-radius: 8px;
  background: var(--bg-card);
}
.target-identity {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 11px;
}
.target-icon {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  border-radius: 7px;
  background: var(--primary-soft);
  color: var(--primary);
}
.target-identity > div:last-child {
  min-width: 0;
}
.target-identity h3 {
  margin: 0;
  font-size: 14px;
}
.target-identity p {
  margin: 3px 0 0;
  color: var(--text-faint);
  font-family: var(--mono);
  font-size: 11.5px;
  overflow-wrap: anywhere;
}
.target-controls {
  margin: 14px 0 10px;
  padding: 12px 0;
  border-top: 1px solid var(--border-soft);
  border-bottom: 1px solid var(--border-soft);
}
.route-control {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 10px;
}
.route-control > div {
  min-width: 0;
  display: grid;
}
.route-control strong {
  font-size: 12.5px;
}
.route-control span {
  color: var(--text-faint);
  font-size: 11.5px;
  overflow-wrap: anywhere;
}
.target-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 7px;
}
.target-error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 0 0 10px;
  color: var(--danger);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.target-card :deep(.collapsible) {
  background: var(--bg-elev);
}
.remote-previews {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-top: 1px solid var(--border-soft);
}
.preview-block {
  min-width: 0;
}
.preview-block + .preview-block {
  border-left: 1px solid var(--border-soft);
}
.preview-block pre {
  min-height: 112px;
  margin: 0;
  padding: 11px 12px;
  color: var(--text);
  font-family: var(--mono);
  font-size: 11.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.remote-how {
  margin-top: 24px;
}
.fingerprint-box {
  display: grid;
  gap: 6px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg-elev);
}
.fingerprint-box span,
.fingerprint-box small {
  color: var(--text-dim);
  font-size: 12px;
}
.fingerprint-box code {
  overflow-wrap: anywhere;
  color: var(--text);
  font-size: 12px;
}
@media (max-width: 900px) {
  .remote-fields {
    grid-template-columns: minmax(0, 1fr) minmax(120px, 0.45fr);
  }
  .remote-fields .field:nth-child(3),
  .remote-fields .field:nth-child(4),
  .remote-fields .field:nth-child(5) {
    grid-column: span 1;
  }
  .remote-form-footer {
    align-items: flex-start;
    flex-direction: column;
  }
}
@media (max-width: 720px) {
  .codex-tabs {
    width: 100%;
  }
  .local-summary-head,
  .section-toolbar,
  .remote-form-footer,
  .target-head,
  .target-controls {
    align-items: stretch;
    flex-direction: column;
  }
  .local-fields,
  .remote-fields,
  .remote-previews {
    grid-template-columns: minmax(0, 1fr);
  }
  .remote-fields .field {
    grid-column: 1 !important;
  }
  .action-row,
  .target-actions {
    justify-content: flex-start;
  }
  .action-row .btn {
    flex: 1 1 auto;
  }
  .target-head .badge {
    align-self: flex-start;
  }
  .target-actions {
    justify-content: flex-start;
  }
  .preview-block + .preview-block {
    border-top: 1px solid var(--border-soft);
    border-left: 0;
  }
  .file-trigger {
    align-items: flex-start;
    flex-direction: column;
    gap: 2px;
  }
}
</style>
