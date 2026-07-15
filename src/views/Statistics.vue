<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import LineChart from "../components/LineChart.vue";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import AnimatedNumber from "../components/AnimatedNumber.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import { api, type RequestLog, type StatsResponse } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const days = ref(7);
const stats = ref<StatsResponse | null>(null);
const logs = ref<RequestLog[]>([]);
const loading = ref(true);
const clearOpen = ref(false);
const clearing = ref(false);
const expandedLog = ref<number | null>(null);

const successRate = computed(() => {
  const s = stats.value?.summary;
  if (!s || s.total_requests === 0) return "—";
  return Math.round((s.success_requests / s.total_requests) * 100) + "%";
});

const chartData = computed(() =>
  (stats.value?.daily || []).map((d) => ({
    label: d.date.slice(5),
    value: d.requests,
  })),
);

const hasData = computed(() => (stats.value?.summary.total_requests ?? 0) > 0);

async function load() {
  loading.value = true;
  try {
    [stats.value, logs.value] = [
      await api.stats(days.value),
      (await api.logs(40)).logs || [],
    ];
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
  }
}

function fmtNum(n?: number) {
  return (n ?? 0).toLocaleString();
}
function fmtTime(s: string) {
  return new Date(s).toLocaleString();
}
function statusClass(code: number) {
  return code >= 200 && code < 300 ? "badge-success" : "badge-danger";
}

async function exportLogs(format: "json" | "csv") {
	try {
		const blob = await api.exportLogs(format, days.value);
		const href = URL.createObjectURL(blob);
		const anchor = document.createElement("a");
		anchor.href = href;
		anchor.download = `amber-logs-${new Date().toISOString().slice(0, 10)}.${format}`;
		anchor.click();
		URL.revokeObjectURL(href);
	} catch (e) {
		app.toast((e as Error).message, "error");
	}
}

async function clearLogs() {
	clearing.value = true;
	try {
		const result = await api.clearLogs();
		clearOpen.value = false;
		app.toast(t("logs.cleared", { count: result.deleted }), "success");
		await load();
	} catch (e) {
		app.toast((e as Error).message, "error");
	} finally {
		clearing.value = false;
	}
}

watch(days, load);
onMounted(load);
</script>

<template>
  <div>
    <div class="page-header row-between">
      <div>
        <h1 class="page-title">{{ t("statistics.title") }}</h1>
        <p class="page-desc">{{ t("statistics.desc") }}</p>
      </div>
		<div class="row-gap">
			<button class="btn btn-sm" @click="exportLogs('json')"><Icon name="download" :size="14" /> JSON</button>
			<button class="btn btn-sm" @click="exportLogs('csv')"><Icon name="download" :size="14" /> CSV</button>
			<button class="btn btn-danger btn-sm" @click="clearOpen = true"><Icon name="trash" :size="14" /> {{ t("logs.clear") }}</button>
			<select v-model.number="days" class="select" style="width: 140px">
				<option :value="7">{{ t("statistics.days7") }}</option>
				<option :value="30">{{ t("statistics.days30") }}</option>
			</select>
		</div>
    </div>

    <SkeletonBlock v-if="loading" :cards="4" :rows="5" />
    <div v-show="!loading" class="grid grid-4">
      <div class="stat">
        <div class="stat-label">{{ t("statistics.totalRequests") }}</div>
        <div class="stat-value"><AnimatedNumber :value="stats?.summary.total_requests || 0" /></div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.successRate") }}</div>
        <div class="stat-value">{{ successRate }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.totalTokens") }}</div>
        <div class="stat-value"><AnimatedNumber :value="stats?.summary.total_tokens || 0" /></div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.avgLatency") }}</div>
        <div class="stat-value"><AnimatedNumber :value="stats?.summary.avg_latency_ms || 0" /><span style="font-size: 14px" class="faint"> ms</span></div>
      </div>
    </div>

    <div v-show="!loading" class="card" style="margin-top: 16px">
      <h3 class="card-title">{{ t("statistics.dailyTrend") }}</h3>
      <div v-if="!hasData" class="empty">{{ t("statistics.noData") }}</div>
      <LineChart v-else :data="chartData" />
    </div>
		<p v-show="!loading" class="faint text-sm" style="margin-top: 10px">
			{{ t("logs.retentionScope", { rows: fmtNum(stats?.retention.retained_rows), days: stats?.retention.days === 0 ? t("logs.forever") : stats?.retention.days }) }}
		</p>

    <div v-show="!loading" class="grid grid-2" style="margin-top: 16px">
      <div class="card">
        <h3 class="card-title">{{ t("statistics.byModel") }}</h3>
        <div v-if="!(stats?.by_model?.length)" class="empty">{{ t("statistics.noData") }}</div>
        <div v-else class="list">
          <div v-for="m in stats!.by_model" :key="m.model" class="list-row">
            <span class="mono" style="flex: 1">{{ m.model }}</span>
            <span class="faint text-sm">{{ fmtNum(m.requests) }} {{ t("statistics.requests") }}</span>
            <span class="faint text-sm" style="width: 90px; text-align: right">{{ fmtNum(m.total_tokens) }} tok</span>
          </div>
        </div>
      </div>

      <div class="card">
        <h3 class="card-title">{{ t("logs.title") }}</h3>
        <div v-if="logs.length === 0" class="empty">{{ t("statistics.noData") }}</div>
        <div v-else class="list" style="max-height: 320px; overflow-y: auto">
          <div v-for="l in logs" :key="l.id" class="statistics-log-entry">
          <button class="list-row statistics-log-row" type="button" :class="{ expandable: !!l.error }" :aria-expanded="expandedLog === l.id" @click="l.error && (expandedLog = expandedLog === l.id ? null : l.id)">
            <span class="badge" :class="statusClass(l.status_code)" style="min-width: 48px; justify-content: center">
              {{ l.status_code }}
            </span>
					<div style="flex: 1; min-width: 0">
						<div class="mono text-sm">{{ l.resolved_model || l.model }}</div>
						<div v-if="l.error_kind" class="faint text-xs">{{ l.error_kind }} · {{ t("logs.attempts", { count: l.attempt_count }) }}</div>
					</div>
            <span class="faint text-sm" style="width: 58px; text-align: right">{{ l.latency_ms }}ms</span>
            <span class="faint text-sm" style="width: 130px; text-align: right">{{ fmtTime(l.created_at) }}</span>
            <Icon v-if="l.error" name="chevron-down" :size="14" class="log-chevron" :class="{ open: expandedLog === l.id }" />
          </button>
          <div v-if="l.error && expandedLog === l.id" class="log-error-detail">{{ l.error }}</div>
          </div>
        </div>
      </div>
    </div>

		<ConfirmModal
			:open="clearOpen"
			:title="t('logs.clearConfirm')"
			:desc="t('logs.clearDesc')"
			danger
			:loading="clearing"
			@confirm="clearLogs"
			@cancel="clearOpen = false"
		/>
  </div>
</template>

<style scoped>
.statistics-log-entry { border-bottom: 1px solid var(--border-soft); }
.statistics-log-entry:last-child { border-bottom: 0; }
.statistics-log-row { width: 100%; border: 0; border-bottom: 0; background: transparent; color: var(--text); text-align: left; }
.statistics-log-row.expandable { cursor: pointer; }
.statistics-log-row.expandable:hover { background: var(--bg-hover); }
.log-chevron { transition: transform var(--motion-fast) var(--motion-ease); }
.log-chevron.open { transform: rotate(180deg); }
.log-error-detail { padding: 0 10px 12px 60px; color: var(--danger); font-family: var(--mono); font-size: 12px; overflow-wrap: anywhere; }
</style>
