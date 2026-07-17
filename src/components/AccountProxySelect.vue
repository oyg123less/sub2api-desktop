<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { api, type Proxy } from "../api/control";
import Icon from "./Icon.vue";

const props = defineProps<{ modelValue: number | null; proxies: Proxy[]; disabled?: boolean }>();
const emit = defineEmits<{ "update:modelValue": [value: number | null] }>();
const { t } = useI18n();

const root = ref<HTMLElement | null>(null);
const open = ref(false);
const query = ref("");
const testing = ref<Record<number, boolean>>({});
const results = ref<Record<number, { ok: boolean; latency?: number }>>({});

const selected = computed(() => props.proxies.find((proxy) => proxy.id === props.modelValue) ?? null);
const filtered = computed(() => {
  const term = query.value.trim().toLowerCase();
  if (!term) return props.proxies;
  return props.proxies.filter((proxy) => `${proxy.name} ${proxy.type} ${proxy.host} ${proxy.port}`.toLowerCase().includes(term));
});

function choose(value: number | null) {
  emit("update:modelValue", value);
  open.value = false;
  query.value = "";
}

async function testProxy(proxy: Proxy) {
  if (testing.value[proxy.id]) return;
  testing.value = { ...testing.value, [proxy.id]: true };
  try {
    const result = await api.testProxy(proxy.id);
    results.value = { ...results.value, [proxy.id]: { ok: result.ok, latency: result.latency_ms } };
  } catch {
    results.value = { ...results.value, [proxy.id]: { ok: false } };
  } finally {
    testing.value = { ...testing.value, [proxy.id]: false };
  }
}

function onDocumentClick(event: MouseEvent) {
  if (root.value && !root.value.contains(event.target as Node)) open.value = false;
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") open.value = false;
}

onMounted(() => {
  document.addEventListener("click", onDocumentClick);
  document.addEventListener("keydown", onDocumentKeydown);
});
onUnmounted(() => {
  document.removeEventListener("click", onDocumentClick);
  document.removeEventListener("keydown", onDocumentKeydown);
});
</script>

<template>
  <div ref="root" class="proxy-select">
    <button
      type="button"
      class="select proxy-select-trigger"
      :disabled="disabled"
      data-test="import-proxy-trigger"
      @click="open = !open"
    >
      <span v-if="selected" class="proxy-selected">
        <strong>{{ selected.name }}</strong>
        <small>{{ selected.type.toUpperCase() }} · {{ selected.host }}:{{ selected.port }}</small>
      </span>
      <span v-else>{{ t("accounts.noProxy") }}</span>
      <Icon name="chevron-down" :size="16" :class="{ 'proxy-chevron-open': open }" />
    </button>

    <div v-if="open" class="proxy-select-menu" data-test="import-proxy-menu">
      <label class="proxy-search">
        <Icon name="search" :size="14" />
        <input v-model="query" type="search" :placeholder="t('accounts.searchProxy')" />
      </label>
      <div class="proxy-options">
        <button type="button" class="proxy-option proxy-direct" @click="choose(null)">
          <span>{{ t("accounts.noProxy") }}</span><Icon v-if="modelValue == null" name="check" :size="14" />
        </button>
        <div v-for="proxy in filtered" :key="proxy.id" class="proxy-option">
          <button type="button" class="proxy-option-main" @click="choose(proxy.id)">
            <span><strong>{{ proxy.name }}</strong><small>{{ proxy.type.toUpperCase() }} · {{ proxy.host }}:{{ proxy.port }}</small></span>
            <Icon v-if="modelValue === proxy.id" name="check" :size="14" />
          </button>
          <span v-if="results[proxy.id]" class="proxy-test-result" :class="results[proxy.id].ok ? 'is-ok' : 'is-failed'">
            {{ results[proxy.id].ok ? `${results[proxy.id].latency ?? 0}ms` : t("accounts.proxyTestFailed") }}
          </span>
          <button type="button" class="proxy-test-btn" :title="t('accounts.testProxy')" :disabled="testing[proxy.id]" @click="testProxy(proxy)">
            <Icon name="refresh" :size="14" :class="{ spin: testing[proxy.id] }" />
          </button>
        </div>
        <p v-if="filtered.length === 0" class="proxy-empty">{{ t("accounts.noMatchingProxy") }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.proxy-select { position: relative; min-width: 0; }
.proxy-select-trigger { min-height: 42px; display: flex; align-items: center; justify-content: space-between; gap: 12px; text-align: left; cursor: pointer; }
.proxy-select-trigger:disabled { cursor: not-allowed; opacity: .6; }
.proxy-selected { min-width: 0; display: grid; gap: 2px; }
.proxy-selected strong, .proxy-selected small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.proxy-selected small { color: var(--text-faint); font-size: 11px; font-weight: 400; }
.proxy-select-trigger .icon { flex: 0 0 auto; transition: transform var(--motion-fast) var(--motion-ease); }
.proxy-chevron-open { transform: rotate(180deg); }
.proxy-select-menu { position: absolute; z-index: 80; top: calc(100% + 6px); left: 0; width: 100%; overflow: hidden; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); box-shadow: var(--shadow); }
.proxy-search { display: flex; align-items: center; gap: 8px; padding: 9px 11px; border-bottom: 1px solid var(--border-soft); color: var(--text-faint); }
.proxy-search input { width: 100%; min-width: 0; border: 0; outline: 0; background: transparent; color: var(--text); }
.proxy-options { max-height: 240px; overflow-y: auto; padding: 5px; }
.proxy-option { min-width: 0; display: flex; align-items: center; gap: 6px; border-radius: 6px; }
.proxy-option:hover { background: var(--bg-hover); }
.proxy-option-main, .proxy-direct { min-width: 0; flex: 1; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 9px; border: 0; background: transparent; color: var(--text); text-align: left; cursor: pointer; }
.proxy-option-main > span { min-width: 0; display: grid; gap: 2px; }
.proxy-option-main strong, .proxy-option-main small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.proxy-option-main small { color: var(--text-faint); font-size: 11px; font-weight: 400; }
.proxy-test-btn { flex: 0 0 auto; display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 6px; background: transparent; color: var(--text-dim); cursor: pointer; }
.proxy-test-btn:hover { background: var(--accent-soft); color: var(--accent); }
.proxy-test-btn:disabled { cursor: wait; opacity: .6; }
.proxy-test-result { flex: 0 0 auto; font-size: 10px; }
.proxy-test-result.is-ok { color: var(--success); }
.proxy-test-result.is-failed { color: var(--danger); }
.proxy-empty { margin: 0; padding: 18px 10px; color: var(--text-faint); font-size: 12px; text-align: center; }
</style>
