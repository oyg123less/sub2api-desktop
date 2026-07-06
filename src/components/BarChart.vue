<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  data: { label: string; value: number }[];
}>();

const max = computed(() => Math.max(1, ...props.data.map((d) => d.value)));
</script>

<template>
  <div class="chart">
    <div v-for="(d, i) in data" :key="i" class="chart-col">
      <div class="chart-bar-wrap">
        <div class="chart-val">{{ d.value > 0 ? d.value : "" }}</div>
        <div
          class="chart-bar"
          :style="{ height: (d.value / max) * 100 + '%' }"
        ></div>
      </div>
      <div class="chart-label">{{ d.label }}</div>
    </div>
  </div>
</template>

<style scoped>
.chart {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  height: 200px;
  padding-top: 20px;
}
.chart-col {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
}
.chart-bar-wrap {
  flex: 1;
  width: 100%;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  align-items: center;
  position: relative;
}
.chart-bar {
  width: 62%;
  min-height: 3px;
  border-radius: 6px 6px 0 0;
  background: linear-gradient(180deg, var(--primary), rgba(217, 119, 87, 0.35));
  transition: height 0.4s ease;
}
.chart-val {
  font-size: 11px;
  color: var(--text-dim);
  margin-bottom: 5px;
  font-variant-numeric: tabular-nums;
}
.chart-label {
  font-size: 11px;
  color: var(--text-faint);
  margin-top: 8px;
  white-space: nowrap;
}
</style>
