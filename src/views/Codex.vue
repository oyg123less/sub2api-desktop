<script setup lang="ts">
import { onMounted, ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import CopyField from "../components/CopyField.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type CodexStatus, type CodexFiles } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const st = ref<CodexStatus | null>(null);
const loading = ref(true);
const busy = ref(false);
const restoreOpen = ref(false);

const modelDraft = ref("");
const files = ref<CodexFiles | null>(null);
const configDraft = ref("");
const authDraft = ref("");
const savingFiles = ref(false);

const applied = computed(() => st.value?.applied ?? false);

function isValidModel(m: string): boolean {
  const v = m.trim().toLowerCase();
  return v.startsWith("gpt-5") || v.includes("codex");
}
const modelValid = computed(() => isValidModel(modelDraft.value));

// Live previews: swap the model line so edits are reflected before applying.
const configPreview = computed(() => {
  const base = st.value?.config_preview || "";
  if (!modelValid.value) return base;
  return base.replace(/^model = ".*"$/m, `model = "${modelDraft.value.trim()}"`);
});
const authPreview = computed(() => st.value?.auth_preview || "");

async function load() {
  try {
    st.value = await api.codexStatus();
    modelDraft.value = st.value.model;
    files.value = await api.codexFiles();
    configDraft.value = files.value.config_content || files.value.config_default;
    authDraft.value = files.value.auth_content || files.value.auth_default;
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

async function apply() {
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
  } catch (e) {
    app.toast((e as Error).message, "error");
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
  } catch (e) {
    app.toast((e as Error).message, "error");
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

async function confirmRestore() {
  busy.value = true;
  try {
    st.value = await api.codexRestore();
    restoreOpen.value = false;
    app.toast(t("codex.restored"), "success");
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    busy.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ t("codex.title") }}</h1>
      <p class="page-desc">{{ t("codex.desc") }}</p>
    </div>

    <div v-if="!loading && st">
      <!-- Status + actions -->
      <div class="card">
        <div class="row-between" style="margin-bottom: 16px">
          <div class="flex items-center gap-12">
            <span class="badge" :class="applied ? 'badge-success' : 'badge-neutral'">
              <span class="badge-dot"></span>
              {{ applied ? t("codex.statusOn") : t("codex.statusOff") }}
            </span>
            <span class="faint text-sm">{{ st.config_path }}</span>
						<span v-if="st.backup_at" class="faint text-sm">{{ t("codex.backupAt", { time: new Date(st.backup_at).toLocaleString() }) }}</span>
          </div>
          <div class="flex items-center gap-8">
            <button class="btn btn-primary" :disabled="busy" @click="apply">
              <Icon name="terminal" :size="16" />
              {{ applied ? t("codex.reapply") : t("codex.apply") }}
            </button>
            <button
              class="btn btn-ghost"
              :disabled="busy || !st.backup_exists"
              @click="restoreOpen = true"
            >
              <Icon name="refresh" :size="14" /> {{ t("codex.restore") }}
            </button>
          </div>
        </div>

        <div class="grid grid-2">
          <div class="field" style="margin: 0">
            <label class="field-label">{{ t("codex.baseUrl") }}</label>
            <CopyField :value="st.base_url" />
          </div>
          <div class="field" style="margin: 0">
            <label class="field-label">{{ t("codex.model") }}</label>
            <input
              v-model="modelDraft"
              class="input mono"
              :style="modelValid ? '' : 'border-color: var(--danger)'"
              list="codex-model-options"
              placeholder="gpt-5.6-sol"
            />
            <datalist id="codex-model-options">
              <option v-for="m in st.models" :key="m" :value="m" />
            </datalist>
            <p class="faint text-sm" style="margin: 6px 0 0" :style="modelValid ? '' : 'color: var(--danger)'">
              {{ modelValid ? t("codex.modelHint") : t("codex.modelInvalid") }}
            </p>
          </div>
        </div>
      </div>

      <!-- Remote config (copy/paste to a remote server) -->
      <div class="card" style="margin-top: 16px">
        <h3 class="card-title"><Icon name="link" :size="16" /> {{ t("codex.remoteTitle") }}</h3>
        <p class="doc-line">{{ t("codex.remote1") }}</p>
        <p class="doc-line">{{ t("codex.remote2") }}</p>

        <div class="code-block">
          <div class="code-head">
            <span class="code-label">~/.codex/config.toml</span>
            <button class="btn btn-ghost btn-sm" @click="copyText(configPreview)">
              <Icon name="copy" :size="13" /> {{ t("common.copy") }}
            </button>
          </div>
          <pre>{{ configPreview }}</pre>
        </div>

        <div class="code-block">
          <div class="code-head">
            <span class="code-label">~/.codex/auth.json</span>
            <button class="btn btn-ghost btn-sm" @click="copyText(authPreview)">
              <Icon name="copy" :size="13" /> {{ t("common.copy") }}
            </button>
          </div>
          <pre>{{ authPreview }}</pre>
        </div>
      </div>

      <!-- Manual editing of local config files -->
      <div class="card" style="margin-top: 16px" v-if="files">
        <div class="row-between" style="margin-bottom: 4px">
          <h3 class="card-title" style="margin: 0"><Icon name="edit" :size="16" /> {{ t("codex.filesTitle") }}</h3>
          <div class="flex items-center gap-8">
            <button class="btn btn-ghost btn-sm" :disabled="savingFiles" @click="fillDefaults">
              {{ t("codex.fillDefault") }}
            </button>
            <button class="btn btn-primary btn-sm" :disabled="savingFiles" @click="saveFiles">
              {{ t("codex.saveFiles") }}
            </button>
          </div>
        </div>
        <p class="doc-line faint">{{ t("codex.filesDesc") }}</p>

        <div class="code-block">
          <div class="code-head">
            <span class="code-label">{{ files.config_path }}</span>
          </div>
          <textarea v-model="configDraft" class="file-edit" rows="10" spellcheck="false"></textarea>
        </div>

        <div class="code-block">
          <div class="code-head">
            <span class="code-label">{{ files.auth_path }}</span>
          </div>
          <textarea v-model="authDraft" class="file-edit" rows="4" spellcheck="false"></textarea>
        </div>
      </div>

      <!-- Explanation -->
      <div class="card" style="margin-top: 16px">
        <h3 class="card-title"><Icon name="docs" :size="16" /> {{ t("codex.howTitle") }}</h3>
        <p class="doc-line">{{ t("codex.how1") }}</p>
        <p class="doc-line">{{ t("codex.how2") }}</p>
        <p class="doc-line">{{ t("codex.how3") }}</p>
        <div
          class="card"
          style="border-color: var(--warn); background: var(--warn-soft); margin: 8px 0 0"
        >
          <div class="flex items-center gap-8">
            <Icon name="warn" :size="16" style="color: var(--warn); flex-shrink: 0" />
            <span class="text-sm">{{ t("codex.backupNote") }}</span>
          </div>
        </div>
      </div>
    </div>

    <ConfirmModal
      :open="restoreOpen"
      :title="t('codex.restoreConfirm')"
      :desc="t('codex.restoreDesc')"
      :confirm-text="t('codex.restore')"
      @confirm="confirmRestore"
      @cancel="restoreOpen = false"
    />
  </div>
</template>

<style scoped>
.doc-line {
  margin: 0 0 8px;
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--text);
}
.code-block {
  margin: 10px 0 0;
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  background: var(--bg-elev);
}
.code-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px 6px 12px;
  border-bottom: 1px solid var(--border-soft);
}
.code-label {
  font-size: 12px;
  color: var(--text-dim);
  font-weight: 550;
  font-family: var(--mono);
}
.file-edit {
  display: block;
  width: 100%;
  border: none;
  outline: none;
  resize: vertical;
  background: transparent;
  margin: 0;
  padding: 12px 14px;
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--text);
  line-height: 1.6;
  box-sizing: border-box;
}
.code-block pre {
  margin: 0;
  padding: 12px 14px;
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
}
</style>
