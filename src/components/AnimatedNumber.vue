<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";

const props = defineProps<{ value: number }>();
const display = ref(0);
let frame = 0;

function animate(target: number) {
  cancelAnimationFrame(frame);
  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    display.value = target;
    return;
  }
  const start = display.value;
  const started = performance.now();
  const tick = (now: number) => {
    const progress = Math.min(1, (now - started) / 250);
    display.value = Math.round(start + (target - start) * (1 - Math.pow(1 - progress, 3)));
    if (progress < 1) frame = requestAnimationFrame(tick);
  };
  frame = requestAnimationFrame(tick);
}

watch(() => props.value, (value) => animate(Number.isFinite(value) ? value : 0), { immediate: true });
onBeforeUnmount(() => cancelAnimationFrame(frame));
</script>

<template>{{ display.toLocaleString() }}</template>
