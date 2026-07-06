<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  data: { label: string; value: number }[];
}>();

const W = 640;
const H = 200;
const PAD_X = 26;
const PAD_TOP = 26;
const PAD_BOTTOM = 8;

const max = computed(() => Math.max(1, ...props.data.map((d) => d.value)));

const points = computed(() => {
  const n = props.data.length;
  if (n === 0) return [] as { x: number; y: number; v: number }[];
  const innerW = W - PAD_X * 2;
  const innerH = H - PAD_TOP - PAD_BOTTOM;
  return props.data.map((d, i) => ({
    x: PAD_X + (n === 1 ? innerW / 2 : (i / (n - 1)) * innerW),
    y: PAD_TOP + innerH - (d.value / max.value) * innerH,
    v: d.value,
  }));
});

// Catmull-Rom → cubic Bézier for a smooth curve through all points.
const linePath = computed(() => {
  const pts = points.value;
  if (pts.length === 0) return "";
  if (pts.length === 1) return `M ${pts[0].x} ${pts[0].y}`;
  let d = `M ${pts[0].x} ${pts[0].y}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] ?? p2;
    const c1x = p1.x + (p2.x - p0.x) / 6;
    const c1y = p1.y + (p2.y - p0.y) / 6;
    const c2x = p2.x - (p3.x - p1.x) / 6;
    const c2y = p2.y - (p3.y - p1.y) / 6;
    d += ` C ${c1x} ${c1y}, ${c2x} ${c2y}, ${p2.x} ${p2.y}`;
  }
  return d;
});

const areaPath = computed(() => {
  const pts = points.value;
  if (pts.length < 2) return "";
  const bottom = H - PAD_BOTTOM;
  return `${linePath.value} L ${pts[pts.length - 1].x} ${bottom} L ${pts[0].x} ${bottom} Z`;
});

const gridYs = computed(() => {
  const innerH = H - PAD_TOP - PAD_BOTTOM;
  return [0, 0.25, 0.5, 0.75, 1].map((r) => PAD_TOP + innerH * r);
});
</script>

<template>
  <div class="line-chart">
    <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="xMidYMid meet">
      <defs>
        <linearGradient id="lc-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="var(--primary, #d97757)" stop-opacity="0.35" />
          <stop offset="100%" stop-color="var(--primary, #d97757)" stop-opacity="0.02" />
        </linearGradient>
      </defs>

      <line
        v-for="(y, i) in gridYs"
        :key="i"
        :x1="PAD_X"
        :x2="W - PAD_X"
        :y1="y"
        :y2="y"
        stroke="currentColor"
        stroke-opacity="0.08"
        stroke-width="1"
      />

      <path v-if="areaPath" :d="areaPath" fill="url(#lc-fill)" />
      <path
        v-if="linePath"
        :d="linePath"
        fill="none"
        stroke="var(--primary, #d97757)"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      />

      <g v-for="(p, i) in points" :key="i">
        <circle :cx="p.x" :cy="p.y" r="3.5" fill="var(--primary, #d97757)" stroke="var(--bg-card, #fff)" stroke-width="1.5" />
        <text v-if="p.v > 0" :x="p.x" :y="p.y - 10" text-anchor="middle" class="lc-val">{{ p.v }}</text>
      </g>
    </svg>
    <div class="lc-labels">
      <span v-for="(d, i) in data" :key="i" class="lc-label">{{ d.label }}</span>
    </div>
  </div>
</template>

<style scoped>
.line-chart {
  width: 100%;
}
.line-chart svg {
  width: 100%;
  height: auto;
  display: block;
  color: var(--text-faint);
}
.lc-val {
  font-size: 11px;
  fill: var(--text-dim);
  font-variant-numeric: tabular-nums;
}
.lc-labels {
  display: flex;
  justify-content: space-between;
  padding: 6px 18px 0;
}
.lc-label {
  font-size: 11px;
  color: var(--text-faint);
  white-space: nowrap;
}
</style>
