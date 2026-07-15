<script setup lang="ts">
import { onErrorCaptured, ref, watch, type Component } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "./Icon.vue";

const props = defineProps<{
  component?: Component;
  resetKey: string;
}>();

const { t } = useI18n();
const crashed = ref(false);
const retryKey = ref(0);

function retry() {
  crashed.value = false;
  retryKey.value += 1;
}

watch(() => props.resetKey, retry);

onErrorCaptured(() => {
  crashed.value = true;
  return false;
});
</script>

<template>
  <section v-if="crashed" class="route-error" role="alert" data-test="route-error">
    <span class="route-error-icon"><Icon name="warn" :size="24" /></span>
    <div>
      <h1>{{ t("common.pageLoadFailed") }}</h1>
      <p>{{ t("common.pageLoadFailedDesc") }}</p>
    </div>
    <button class="btn btn-primary" type="button" data-test="route-error-retry" @click="retry">
      <Icon name="refresh" :size="15" /> {{ t("common.retry") }}
    </button>
  </section>
  <component :is="component" v-else-if="component" :key="`${resetKey}:${retryKey}`" />
</template>

<style scoped>
.route-error {
  width: min(100%, 680px);
  min-height: 260px;
  margin: 40px auto 0;
  padding: 32px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 16px;
  border: 1px solid var(--border-soft);
  border-radius: 8px;
  background: var(--bg-card);
}
.route-error-icon {
  width: 42px;
  height: 42px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: var(--danger-soft);
  color: var(--danger);
}
.route-error h1 {
  margin: 0;
  font-size: 18px;
}
.route-error p {
  margin: 5px 0 0;
  color: var(--text-dim);
}
</style>
