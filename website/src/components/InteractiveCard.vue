<script setup lang="ts">
withDefaults(
  defineProps<{
    as?: string;
    tone?: "amber" | "teal" | "green";
  }>(),
  {
    as: "article",
    tone: "amber",
  },
);
</script>

<template>
  <component :is="as" class="interactive-card" :class="`interactive-card-${tone}`">
    <slot />
  </component>
</template>

<style scoped>
.interactive-card-teal {
  --card-accent: var(--teal);
  --card-accent-soft: var(--teal-soft);
}

.interactive-card-green {
  --card-accent: var(--green);
  --card-accent-soft: var(--green-soft);
}

.interactive-card-amber {
  --card-accent: var(--amber);
  --card-accent-soft: var(--amber-soft);
}
</style>
