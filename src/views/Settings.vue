<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import CopyField from "../components/CopyField.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type Settings } from "../api/control";
import { useAppStore } from "../store";
import {
  isTauri,
  getDataDir,
  setDataDir,
  openDataDir,
  pickDirectory,
  type DataDirInfo,
} from "../tauri";

const { t, locale } = useI18n();
const app = useAppStore();

const s = ref<Settings | null>(null);
const loading = ref(true);
const saving = ref(false);
const regenOpen = ref(false);

const inTauri = isTauri();
const dataDir = ref<DataDirInfo | null>(null);
const movingDir = ref(false);
const dirConfirmOpen = ref(false);
const pendingDir = ref("");

async function loadDataDir() {
  if (!inTauri) return;
  try {
    dataDir.value = await getDataDir();
  } catch {
    /* ignore */
  }
}

async function chooseDir() {
  try {
    const picked = await pickDirectory(dataDir.value?.current);
    if (!picked || picked === dataDir.value?.current) return;
    pendingDir.value = picked;
    dirConfirmOpen.value = true;
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

async function confirmMoveDir() {
  dirConfirmOpen.value = false;
  if (!pendingDir.value) return;
  movingDir.value = true;
  try {
    dataDir.value = await setDataDir(pendingDir.value);
    app.toast(t("settings.dataDirMoved"), "success");
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    movingDir.value = false;
    pendingDir.value = "";
  }
}

function resetDir() {
  if (!dataDir.value || !dataDir.value.is_custom) return;
  pendingDir.value = dataDir.value.default;
  dirConfirmOpen.value = true;
}

async function openDir() {
  try {
    await openDataDir();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

const models = ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex-spark", "gpt-5.2"];

async function load() {
  try {
    s.value = await api.getSettings();
    s.value.language = normLang(s.value.language || locale.value);
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!s.value) return;
  saving.value = true;
  try {
    s.value = await api.saveSettings(s.value);
    applyLanguage(s.value.language);
    app.toast(t("settings.saved"), "success");
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    saving.value = false;
  }
}

function normLang(lang: string): string {
  return lang && lang.toLowerCase().startsWith("en") ? "en" : "zh";
}

function applyLanguage(lang: string) {
  const l = normLang(lang);
  locale.value = l;
  localStorage.setItem("s2a_lang", l);
}

async function confirmRegen() {
  try {
    const r = await api.regenerateKey();
    if (s.value) s.value.local_api_key = r.local_api_key;
    regenOpen.value = false;
    app.toast(t("settings.saved"), "success");
    await app.refreshStatus();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

onMounted(() => {
  load();
  loadDataDir();
});
</script>

<template>
  <div>
    <div class="page-header row-between">
      <div>
        <h1 class="page-title">{{ t("settings.title") }}</h1>
        <p class="page-desc">{{ t("settings.desc") }}</p>
      </div>
      <button class="btn btn-primary" :disabled="saving || !s" @click="save">
        <Icon name="check" :size="16" /> {{ t("common.save") }}
      </button>
    </div>

    <div v-if="loading || !s" class="empty">{{ t("common.loading") }}</div>

    <template v-else>
      <!-- Service -->
      <div class="card">
        <h3 class="card-title"><Icon name="power" :size="16" /> {{ t("settings.server") }}</h3>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.port") }}</h4>
            <p>{{ t("settings.portDesc") }}</p>
          </div>
          <input v-model.number="s.listen_port" type="number" class="input" style="width: 130px" />
        </div>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.autoStart") }}</h4>
            <p>{{ t("settings.autoStartDesc") }}</p>
          </div>
          <label class="switch">
            <input type="checkbox" v-model="s.auto_start_server" />
            <span class="slider"></span>
          </label>
        </div>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.allowLan") }}</h4>
            <p>{{ t("settings.allowLanDesc") }}</p>
          </div>
          <label class="switch">
            <input type="checkbox" v-model="s.allow_lan" />
            <span class="slider"></span>
          </label>
        </div>
      </div>

      <!-- API key -->
      <div class="card">
        <h3 class="card-title"><Icon name="key" :size="16" /> {{ t("settings.apiKey") }}</h3>
        <p class="faint text-sm" style="margin-top: -6px; margin-bottom: 12px">{{ t("settings.apiKeyDesc") }}</p>
        <CopyField :value="s.local_api_key" mask />
        <button class="btn btn-danger btn-sm mt-16" @click="regenOpen = true">
          <Icon name="refresh" :size="14" /> {{ t("settings.regenerate") }}
        </button>
      </div>

      <!-- Anti-ban -->
      <div class="card">
        <h3 class="card-title"><Icon name="power" :size="16" /> {{ t("settings.antiban") }}</h3>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.injectInstr") }}</h4>
            <p>{{ t("settings.injectInstrDesc") }}</p>
          </div>
          <label class="switch">
            <input type="checkbox" v-model="s.inject_instructions" />
            <span class="slider"></span>
          </label>
        </div>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.tlsFingerprint") }}</h4>
            <p>{{ t("settings.tlsFingerprintDesc") }}</p>
          </div>
          <label class="switch">
            <input type="checkbox" v-model="s.tls_fingerprint" />
            <span class="slider"></span>
          </label>
        </div>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.defaultModel") }}</h4>
            <p>{{ t("settings.defaultModelDesc") }}</p>
          </div>
          <select v-model="s.default_model" class="select" style="width: 200px">
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>
      </div>

      <!-- Appearance -->
      <div class="card">
        <h3 class="card-title"><Icon name="settings" :size="16" /> {{ t("settings.appearance") }}</h3>
        <div class="setting-row">
          <div class="setting-info">
            <h4>{{ t("settings.language") }}</h4>
          </div>
          <select v-model="s.language" class="select" style="width: 160px" @change="applyLanguage(s!.language)">
            <option value="zh">简体中文</option>
            <option value="en">English</option>
          </select>
        </div>
      </div>

      <!-- Data storage -->
      <div v-if="inTauri" class="card">
        <h3 class="card-title"><Icon name="database" :size="16" /> {{ t("settings.dataStorage") }}</h3>
        <p class="faint text-sm" style="margin-top: -6px; margin-bottom: 12px">{{ t("settings.dataDirDesc") }}</p>
        <div class="datadir-path">{{ dataDir?.current || "—" }}</div>
        <div class="row-gap mt-16">
          <button class="btn btn-sm" :disabled="movingDir" @click="chooseDir">
            <Icon name="folder" :size="14" /> {{ t("settings.changeDataDir") }}
          </button>
          <button class="btn btn-sm" @click="openDir">
            <Icon name="external" :size="14" /> {{ t("settings.openDataDir") }}
          </button>
          <button
            v-if="dataDir?.is_custom"
            class="btn btn-sm"
            :disabled="movingDir"
            @click="resetDir"
          >
            <Icon name="refresh" :size="14" /> {{ t("settings.resetDataDir") }}
          </button>
        </div>
        <p v-if="movingDir" class="faint text-sm mt-16">{{ t("settings.dataDirMoving") }}</p>
      </div>
    </template>

    <ConfirmModal
      :open="regenOpen"
      :title="t('settings.regenKeyConfirm')"
      :desc="t('settings.regenKeyDesc')"
      danger
      :confirm-text="t('settings.regenerate')"
      @confirm="confirmRegen"
      @cancel="regenOpen = false"
    />

    <ConfirmModal
      :open="dirConfirmOpen"
      :title="t('settings.dataDirConfirm')"
      :desc="t('settings.dataDirConfirmDesc', { path: pendingDir })"
      :confirm-text="t('settings.changeDataDir')"
      @confirm="confirmMoveDir"
      @cancel="dirConfirmOpen = false; pendingDir = ''"
    />
  </div>
</template>

<style scoped>
.datadir-path {
  font-family: var(--font-mono, monospace);
  font-size: 13px;
  padding: 10px 12px;
  background: var(--surface-2, rgba(0, 0, 0, 0.04));
  border-radius: 8px;
  word-break: break-all;
}
.row-gap {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
