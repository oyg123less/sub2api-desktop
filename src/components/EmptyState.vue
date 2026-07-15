<script setup lang="ts">
import Icon from "./Icon.vue";
defineProps<{ icon: string; title: string; description?: string }>();
</script>

<template>
  <div class="empty-state">
    <span class="empty-state-icon"><Icon :name="icon" :size="24" /></span>
    <strong>{{ title }}</strong>
    <p v-if="description">{{ description }}</p>
    <div v-if="$slots.default" class="empty-state-action"><slot /></div>
  </div>
</template>

<style scoped>
.empty-state { min-height: 190px; display: grid; place-items: center; align-content: center; gap: 7px; padding: 28px 20px; color: var(--text-dim); text-align: center; }
.empty-state-icon { width: 46px; height: 46px; display: grid; place-items: center; border-radius: 8px; background: var(--primary-soft); color: var(--primary); }
.empty-state p { max-width: 440px; margin: 0; color: var(--text-faint); }
.empty-state-action { margin-top: 8px; }
</style>
