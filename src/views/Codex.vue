<script setup lang="ts">
import { onMounted, ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import CopyField from "../components/CopyField.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type CodexStatus } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const st = ref<CodexStatus | null>(null);
const loading = ref(true);
const busy = ref(false);
const restoreOpen = ref(false);

const applied = computed(() => st.value?.applied ?? false);

async function load() {
  try {
    st.value = await api.codexStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

async function apply() {
  busy.value = true;
  try {
    st.value = await api.codexApply();
    app.toast(t("codex.applied"), "success");
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    busy.value = false;
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
            <CopyField :value="st.model" />
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
            <button class="btn btn-ghost btn-sm" @click="copyText(st.config_preview)">
              <Icon name="copy" :size="13" /> {{ t("common.copy") }}
            </button>
          </div>
          <pre>{{ st.config_preview }}</pre>
        </div>

        <div class="code-block">
          <div class="code-head">
            <span class="code-label">~/.codex/auth.json</span>
            <button class="btn btn-ghost btn-sm" @click="copyText(st.auth_preview)">
              <Icon name="copy" :size="13" /> {{ t("common.copy") }}
            </button>
          </div>
          <pre>{{ st.auth_preview }}</pre>
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
