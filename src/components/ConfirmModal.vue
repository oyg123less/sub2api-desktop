<script setup lang="ts">
import { useI18n } from "vue-i18n";

defineProps<{
  open: boolean;
  title: string;
  desc?: string;
  danger?: boolean;
  confirmText?: string;
  loading?: boolean;
}>();
const emit = defineEmits<{ (e: "confirm"): void; (e: "cancel"): void }>();
const { t } = useI18n();
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal-backdrop" @click.self="emit('cancel')">
      <div class="modal">
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
