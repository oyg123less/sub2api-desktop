<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef } from "vue";

const props = withDefaults(
  defineProps<{
    as?: string;
    delay?: number;
  }>(),
  {
    as: "div",
    delay: 0,
  },
);

const root = useTemplateRef<HTMLElement>("root");
const ready = ref(false);
const visible = ref(true);
let observer: IntersectionObserver | undefined;

onMounted(() => {
  const element = root.value;
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (!element || reduceMotion || !("IntersectionObserver" in window)) return;

  visible.value = false;
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      visible.value = true;
      observer?.disconnect();
    },
    { rootMargin: "0px 0px -8%", threshold: 0.08 },
  );
  observer.observe(element);
  requestAnimationFrame(() => {
    ready.value = true;
  });
});

onBeforeUnmount(() => observer?.disconnect());
</script>

<template>
  <component
    :is="props.as"
    ref="root"
    class="reveal"
    :class="{ 'reveal-ready': ready, 'is-visible': visible }"
    :style="{ '--reveal-delay': `${props.delay}ms` }"
  >
    <slot />
  </component>
</template>
