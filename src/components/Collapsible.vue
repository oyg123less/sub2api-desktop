<script setup lang="ts">
import Icon from "./Icon.vue";

const props = withDefaults(defineProps<{ open?: boolean; disabled?: boolean }>(), {
  open: false,
  disabled: false,
});
const emit = defineEmits<{ (event: "update:open", value: boolean): void }>();
</script>

<template>
  <div class="collapsible" :class="{ 'is-open': props.open }">
    <button
      class="collapsible-trigger"
      type="button"
      :aria-expanded="props.open"
      :disabled="props.disabled"
      @click="emit('update:open', !props.open)"
    >
      <span class="collapsible-label"><slot name="trigger" :open="props.open" /></span>
      <Icon class="collapsible-chevron" name="chevron-down" :size="17" />
    </button>
    <div class="collapsible-content" :aria-hidden="!props.open">
      <div class="collapsible-content-inner">
        <slot />
      </div>
    </div>
  </div>
</template>

<style scoped>
.collapsible {
  min-width: 0;
  border: 1px solid var(--border-soft);
  border-radius: 8px;
  background: var(--bg-card);
}
.collapsible-trigger {
  width: 100%;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text);
  cursor: pointer;
  text-align: left;
}
.collapsible-trigger:hover {
  background: var(--bg-hover);
}
.collapsible-trigger:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
.collapsible-label {
  min-width: 0;
  flex: 1;
}
.collapsible-chevron {
  flex: 0 0 auto;
  color: var(--text-dim);
  transition: transform 180ms ease;
}
.is-open .collapsible-chevron {
  transform: rotate(180deg);
}
.collapsible-content {
  display: grid;
  grid-template-rows: 0fr;
  visibility: hidden;
  transition: grid-template-rows 180ms ease, visibility 180ms step-end;
}
.is-open .collapsible-content {
  grid-template-rows: 1fr;
  visibility: visible;
  transition: grid-template-rows 180ms ease, visibility 0s step-start;
}
.collapsible-content-inner {
  min-height: 0;
  min-width: 0;
  overflow: hidden;
}
@media (prefers-reduced-motion: reduce) {
  .collapsible-chevron,
  .collapsible-content {
    transition: none;
  }
}
</style>
