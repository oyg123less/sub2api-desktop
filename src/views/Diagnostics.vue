<script setup lang="ts">
import { computed, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import { api, type DiagnosticCheck, type DiagnosticRun } from "../api/control";
import { useAppStore } from "../store";

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

const { t } = useI18n();
const app = useAppStore();
const run = ref<DiagnosticRun | null>(null);
const starting = ref(false);
let pollTimer: number | undefined;

const running = computed(() => run.value?.status === "running");
const checks = computed(() => run.value?.checks ?? []);

function statusIcon(check: DiagnosticCheck) {
  if (check.status === "ok") return "check";
  if (check.status === "failed" || check.status === "warning") return "warn";
  return "bolt";
}

async function poll(runId: string) {
  try {
    run.value = await api.getDiagnostics(runId);
    if (run.value.status === "running") {
      pollTimer = window.setTimeout(() => poll(runId), 400);
    }
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

async function start() {
  if (starting.value || running.value) return;
  starting.value = true;
  clearTimeout(pollTimer);
  try {
    run.value = await api.startDiagnostics();
    await poll(run.value.run_id);
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    starting.value = false;
  }
}

function summaryText() {
  if (!run.value) return "";
  const lines = [
    `Amber ${t("diagnostics.title")}: ${run.value.run_id}`,
    `${t("diagnostics.ok")}: ${run.value.summary.ok}, ${t("diagnostics.warning")}: ${run.value.summary.warning}, ${t("diagnostics.failed")}: ${run.value.summary.failed}`,
    ...run.value.checks.map((check) => `[${check.status}] ${check.title}: ${check.message}`),
  ];
  return lines.join("\n");
}

async function copySummary() {
  try {
    await navigator.clipboard.writeText(summaryText());
    app.toast(t("common.copied"), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

async function download(format: "json" | "text") {
  if (!run.value) return;
  try {
    const blob = await api.diagnosticReport(run.value.run_id, format);
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `amber-diagnostics-${run.value.run_id}.${format === "json" ? "json" : "txt"}`;
    link.click();
    URL.revokeObjectURL(url);
  } catch (error) {
    app.toast((error as Error).message, "error");
  }
}

onUnmounted(() => clearTimeout(pollTimer));
</script>

<template>
  <div>
    <div class="row-between" :class="props.embedded ? 'diagnostic-embedded-header' : 'page-header'">
      <div v-if="!props.embedded">
        <h1 class="page-title">{{ t("diagnostics.title") }}</h1>
        <p class="page-desc">{{ t("diagnostics.desc") }}</p>
      </div>
      <p v-else class="faint text-sm">{{ t("diagnostics.desc") }}</p>
      <div class="flex gap-8">
        <button v-if="run?.status === 'completed'" class="btn btn-ghost" @click="copySummary">
          <Icon name="copy" :size="15" /> {{ t("diagnostics.copy") }}
        </button>
        <button class="btn btn-primary" :disabled="starting || running" @click="start">
          <Icon :name="running ? 'refresh' : 'play'" :class="{ spin: running }" :size="15" />
          {{ running ? t("diagnostics.running") : t("diagnostics.start") }}
        </button>
      </div>
    </div>

    <section v-if="run" class="diagnostic-workspace" aria-live="polite">
      <div class="diagnostic-progress" :aria-label="t('diagnostics.progress')">
        <div class="diagnostic-progress-fill" :style="{ width: `${run.progress}%` }"></div>
      </div>

      <div class="diagnostic-summary">
        <div><strong>{{ run.summary.failed }}</strong><span>{{ t("diagnostics.failed") }}</span></div>
        <div><strong>{{ run.summary.warning }}</strong><span>{{ t("diagnostics.warning") }}</span></div>
        <div><strong>{{ run.summary.ok }}</strong><span>{{ t("diagnostics.ok") }}</span></div>
        <div class="diagnostic-actions">
          <button class="btn btn-ghost btn-sm" :disabled="running" @click="download('json')">
            <Icon name="download" :size="14" /> JSON
          </button>
          <button class="btn btn-ghost btn-sm" :disabled="running" @click="download('text')">
            <Icon name="download" :size="14" /> TXT
          </button>
        </div>
      </div>

      <div class="diagnostic-list">
        <article v-for="check in checks" :key="check.id" class="diagnostic-row" :class="`is-${check.status}`">
          <div class="diagnostic-status"><Icon :name="statusIcon(check)" :size="18" /></div>
          <div class="diagnostic-copy">
            <div class="row-between">
              <strong>{{ check.title }}</strong>
              <span class="faint text-sm">{{ check.duration_ms }} ms</span>
            </div>
            <p>{{ check.message }}</p>
          </div>
        </article>
      </div>
    </section>

    <div v-else class="empty-state diagnostic-empty">
      <Icon name="bolt" :size="30" />
      <h3>{{ t("diagnostics.ready") }}</h3>
    </div>
  </div>
</template>
