<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  open: boolean;
  title: string;
  desc?: string;
  danger?: boolean;
  confirmText?: string;
  loading?: boolean;
}>();
const emit = defineEmits<{ (e: "confirm"): void; (e: "cancel"): void }>();
const { t } = useI18n();
const dialog = ref<HTMLElement | null>(null);
let returnFocus: HTMLElement | null = null;

function focusable(): HTMLElement[] {
  if (!dialog.value) return [];
  return Array.from(dialog.value.querySelectorAll<HTMLElement>("button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])"));
}

function onKeydown(event: KeyboardEvent) {
  if (!props.open) return;
  if (event.key === "Escape" && !props.loading) {
    event.preventDefault();
    emit("cancel");
    return;
  }
  if (event.key !== "Tab") return;
  const items = focusable();
  if (items.length === 0) {
    event.preventDefault();
    dialog.value?.focus();
    return;
  }
  const first = items[0];
  const last = items[items.length - 1];
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

watch(() => props.open, async (open) => {
  if (open) {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.addEventListener("keydown", onKeydown);
    await nextTick();
    (focusable()[0] || dialog.value)?.focus();
  } else {
    document.removeEventListener("keydown", onKeydown);
    returnFocus?.focus();
    returnFocus = null;
  }
}, { immediate: true });

onBeforeUnmount(() => document.removeEventListener("keydown", onKeydown));
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal-backdrop" @click.self="emit('cancel')">
      <div ref="dialog" class="modal" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1">
        <h3 class="modal-title">{{ title }}</h3>
        <p v-if="desc" class="modal-desc">{{ desc }}</p>
        <slot />
        <div class="modal-actions">
          <button class="btn btn-ghost" @click="emit('cancel')" :disabled="loading">
            {{ t("common.cancel") }}
          </button>
          <button
            class="btn"
            :class="danger ? 'btn-danger' : 'btn-primary'"
            @click="emit('confirm')"
            :disabled="loading"
          >
            {{ confirmText || t("common.confirm") }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
