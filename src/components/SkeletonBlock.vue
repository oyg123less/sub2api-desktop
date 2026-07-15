<script setup lang="ts">
withDefaults(defineProps<{ rows?: number; cards?: number }>(), { rows: 3, cards: 3 });
</script>

<template>
  <div class="skeleton-block" aria-hidden="true">
    <div class="skeleton-cards" :style="{ gridTemplateColumns: `repeat(${cards}, minmax(0, 1fr))` }">
      <span v-for="index in cards" :key="`card-${index}`" class="skeleton-card"></span>
    </div>
    <span v-for="index in rows" :key="`row-${index}`" class="skeleton-row" :style="{ width: `${100 - (index - 1) * 8}%` }"></span>
  </div>
</template>

<style scoped>
.skeleton-block { display: grid; gap: 12px; width: 100%; }
.skeleton-cards { display: grid; gap: 12px; }
.skeleton-card, .skeleton-row { display: block; border-radius: 8px; background: linear-gradient(90deg, var(--bg-elev), var(--bg-card), var(--bg-elev)); background-size: 200% 100%; animation: skeleton-shimmer 1.4s ease-in-out infinite; }
.skeleton-card { min-height: 92px; }
.skeleton-row { height: 48px; }
@keyframes skeleton-shimmer { to { background-position: -200% 0; } }
@media (max-width: 720px) { .skeleton-cards { grid-template-columns: minmax(0, 1fr) !important; } }
@media (prefers-reduced-motion: reduce) { .skeleton-card, .skeleton-row { animation: none; } }
</style>
