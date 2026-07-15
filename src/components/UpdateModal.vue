<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { ReleaseInfo } from "../api/control";
import { openUrl } from "../platform";
import Icon from "./Icon.vue";

const props = defineProps<{ open: boolean; release: ReleaseInfo | null; currentVersion: string }>();
const emit = defineEmits<{ (event: "close"): void }>();
const { t } = useI18n();
const dialog = ref<HTMLElement | null>(null);
let returnFocus: HTMLElement | null = null;

function focusable(): HTMLElement[] {
  if (!dialog.value) return [];
  return Array.from(dialog.value.querySelectorAll<HTMLElement>("button:not([disabled]), [href], [tabindex]:not([tabindex='-1'])"));
}

function onKeydown(event: KeyboardEvent) {
  if (!props.open) return;
  if (event.key === "Escape") {
    event.preventDefault();
    emit("close");
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

function download() {
  if (!props.release?.html_url) return;
  openUrl(props.release.html_url);
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
    <div v-if="open && release" class="modal-backdrop" @click.self="emit('close')">
      <div ref="dialog" class="modal update-modal" role="dialog" aria-modal="true" :aria-label="t('updates.title')" tabindex="-1">
        <h3 class="modal-title">{{ release.name || t("updates.title") }}</h3>
        <div class="update-version-row">
          <span>{{ t("updates.current", { version: currentVersion }) }}</span>
          <Icon name="external" :size="14" />
          <strong>{{ release.tag_name }}</strong>
        </div>
        <h4 class="update-notes-title">{{ t("updates.notes") }}</h4>
        <div class="update-notes">{{ release.body || t("updates.noNotes") }}</div>
        <div class="modal-actions">
          <button class="btn btn-ghost" type="button" @click="emit('close')">{{ t("common.close") }}</button>
          <button class="btn btn-primary" type="button" @click="download">
            <Icon name="download" :size="15" /> {{ t("updates.download") }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.update-modal {
  max-width: 620px;
}
.update-version-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border-soft);
  color: var(--text-dim);
  font-size: 12px;
}
.update-version-row strong {
  color: var(--primary-hover);
}
.update-notes-title {
  margin: 16px 0 8px;
  font-size: 13px;
  font-weight: 600;
}
.update-notes {
  max-height: min(360px, 48vh);
  overflow: auto;
  padding-right: 8px;
  color: var(--text-dim);
  font-size: 12.5px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
