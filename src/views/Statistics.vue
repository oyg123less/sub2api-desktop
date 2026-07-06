<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import LineChart from "../components/LineChart.vue";
import { api, type RequestLog, type StatsResponse } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const days = ref(7);
const stats = ref<StatsResponse | null>(null);
const logs = ref<RequestLog[]>([]);
const loading = ref(true);

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
      <select v-model.number="days" class="select" style="width: 140px">
        <option :value="7">{{ t("statistics.days7") }}</option>
        <option :value="30">{{ t("statistics.days30") }}</option>
      </select>
    </div>

    <div class="grid grid-4">
      <div class="stat">
        <div class="stat-label">{{ t("statistics.totalRequests") }}</div>
        <div class="stat-value">{{ fmtNum(stats?.summary.total_requests) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.successRate") }}</div>
        <div class="stat-value">{{ successRate }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.totalTokens") }}</div>
        <div class="stat-value">{{ fmtNum(stats?.summary.total_tokens) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">{{ t("statistics.avgLatency") }}</div>
        <div class="stat-value">{{ fmtNum(stats?.summary.avg_latency_ms) }}<span style="font-size: 14px" class="faint"> ms</span></div>
      </div>
    </div>

    <div class="card" style="margin-top: 16px">
      <h3 class="card-title">{{ t("statistics.dailyTrend") }}</h3>
      <div v-if="!hasData" class="empty">{{ t("statistics.noData") }}</div>
      <LineChart v-else :data="chartData" />
    </div>

    <div class="grid grid-2" style="margin-top: 16px">
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
          <div v-for="l in logs" :key="l.id" class="list-row" :title="l.error || ''">
            <span class="badge" :class="statusClass(l.status_code)" style="min-width: 48px; justify-content: center">
              {{ l.status_code }}
            </span>
            <span class="mono text-sm" style="flex: 1">{{ l.model }}</span>
            <span class="faint text-sm" style="width: 58px; text-align: right">{{ l.latency_ms }}ms</span>
            <span class="faint text-sm" style="width: 130px; text-align: right">{{ fmtTime(l.created_at) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
