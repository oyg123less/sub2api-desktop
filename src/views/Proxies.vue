<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import EmptyState from "../components/EmptyState.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import { api, type AccountProxySummary, type Proxy } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const router = useRouter();
const app = useAppStore();

const proxies = ref<Proxy[]>([]);
const loading = ref(true);
const testing = ref<Record<number, boolean>>({});
type ProxyTest = Awaited<ReturnType<typeof api.testProxy>>;
const testResults = ref<Record<number, ProxyTest>>({});
const accountProxySummary = ref<AccountProxySummary>({ total: 0, bound: 0, unbound: 0, mixed: false, bindings: [] });
const globalProxyID = ref<number | null>(null);
const globalApplyOpen = ref(false);
const globalApplyTarget = ref<number | null>(null);
const globalApplying = ref(false);
const startingAccountTest = ref(false);

const globalProxyLabel = computed(() => {
  const summary = accountProxySummary.value;
  if (!summary.total || summary.unbound === summary.total) return t("proxies.allDirect");
  if (!summary.mixed && summary.uniform_proxy_id) return proxies.value.find((proxy) => proxy.id === summary.uniform_proxy_id)?.name || t("proxies.mixedAccounts");
  return t("proxies.mixedAccounts");
});

const addOpen = ref(false);
const saving = ref(false);
const editId = ref<number | null>(null);
const clearPassword = ref(false);
const form = reactive({
  name: "",
  type: "http" as Proxy["type"],
  host: "",
  port: 1080,
  username: "",
  password: "",
});

const deleteTarget = ref<Proxy | null>(null);

function normalizeProxySummary(summary: AccountProxySummary): AccountProxySummary {
  return {
    ...summary,
    bindings: Array.isArray(summary.bindings) ? summary.bindings : [],
  };
}

async function load(notify = true): Promise<boolean> {
  try {
    const [proxyResult, summary] = await Promise.all([api.listProxies(), api.accountProxySummary()]);
    proxies.value = proxyResult.proxies || [];
    accountProxySummary.value = normalizeProxySummary(summary);
    globalProxyID.value = !accountProxySummary.value.mixed ? accountProxySummary.value.uniform_proxy_id ?? null : null;
    return true;
  } catch (e) {
    if (notify) app.toast((e as Error).message, "error");
    return false;
  } finally {
    loading.value = false;
  }
}

function proxyAccountCount(id: number) {
  return accountProxySummary.value.bindings.find((binding) => binding.proxy_id === id)?.count ?? 0;
}

function requestGlobalApply(proxyID: number | null) {
  globalApplyTarget.value = proxyID;
  globalApplyOpen.value = true;
}

async function confirmGlobalApply() {
  globalApplying.value = true;
  const target = globalApplyTarget.value;
  try {
    const result = await api.bindAllAccountsProxy(target);
    const refreshed = await load(false);
    const summary = accountProxySummary.value;
    const validResult = result.matched >= 0 && result.updated >= 0 && result.unchanged >= 0
      && result.updated + result.unchanged === result.matched;
    const validSummary = target == null
      ? summary.total === result.matched && summary.bound === 0 && summary.unbound === result.matched && !summary.mixed && summary.bindings.length === 0
      : summary.total === result.matched && summary.bound === result.matched && summary.unbound === 0 && !summary.mixed
        && summary.uniform_proxy_id === target
        && summary.bindings.some((binding) => binding.proxy_id === target && binding.count === result.matched);
    if (!refreshed || !validResult || !validSummary) throw new Error(t("proxies.applyVerifyFailed"));
    globalApplyOpen.value = false;
    app.toast(t("proxies.applyComplete", { updated: result.updated, unchanged: result.unchanged }), "success");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    globalApplying.value = false;
  }
}

async function testAllAccounts() {
  startingAccountTest.value = true;
  try {
    await api.startAccountTestRun({ scope: "all" });
    await router.push("/accounts");
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    startingAccountTest.value = false;
  }
}

function openAdd() {
  editId.value = null;
  Object.assign(form, { name: "", type: "http", host: "", port: 1080, username: "", password: "" });
  clearPassword.value = false;
  addOpen.value = true;
}

function openEdit(p: Proxy) {
  editId.value = p.id;
  Object.assign(form, {
    name: p.name,
    type: p.type,
    host: p.host,
    port: p.port,
    username: p.username || "",
    password: "",
  });
  clearPassword.value = false;
  addOpen.value = true;
}

async function save() {
  if (!form.host || !form.port) {
    app.toast(t("proxies.host") + " / " + t("proxies.port"), "warn");
    return;
  }
  saving.value = true;
  try {
    const payload = {
      name: form.name || `${form.host}:${form.port}`,
      type: form.type,
      host: form.host,
      port: Number(form.port),
      username: form.username || undefined,
      password: form.password || undefined,
      clear_password: editId.value != null ? clearPassword.value : undefined,
    };
    if (editId.value != null) {
      await api.updateProxy(editId.value, payload);
    } else {
      await api.createProxy(payload);
    }
    addOpen.value = false;
    await load();
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    saving.value = false;
  }
}

async function test(p: Proxy) {
  testing.value[p.id] = true;
  try {
    const r = await api.testProxy(p.id);
		testResults.value[p.id] = r;
    if (r.ok) {
      app.toast(`${t("proxies.testOk")} · ${r.latency_ms}ms`, "success");
    } else {
      app.toast(`${t("proxies.testFailed")}: ${r.error}`, "error");
    }
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    testing.value[p.id] = false;
  }
}

async function confirmDelete() {
  if (!deleteTarget.value) return;
  try {
    await api.deleteProxy(deleteTarget.value.id);
    deleteTarget.value = null;
    await load();
  } catch (e) {
    app.toast((e as Error).message, "error");
  }
}

onMounted(load);
</script>

<template>
  <div>
    <div class="page-header row-between">
      <div>
        <h1 class="page-title">{{ t("proxies.title") }}</h1>
        <p class="page-desc">{{ t("proxies.desc") }}</p>
      </div>
      <button class="btn btn-primary" @click="openAdd">
        <Icon name="plus" :size="16" /> {{ t("proxies.add") }}
      </button>
    </div>

    <SkeletonBlock v-if="loading" :cards="2" :rows="3" />

    <section v-else class="proxy-account-panel" data-test="proxy-account-panel">
      <div class="proxy-account-summary">
        <span class="proxy-account-icon"><Icon name="accounts" :size="17" /></span>
        <div>
          <span>{{ t("proxies.accountProxyConfig") }}</span>
          <strong>{{ globalProxyLabel }} · {{ t("proxies.boundSummary", { bound: accountProxySummary.bound, total: accountProxySummary.total }) }}</strong>
        </div>
      </div>
      <div class="proxy-account-controls">
        <select v-model.number="globalProxyID" class="select select-sm" data-test="proxy-global-select" :aria-label="t('proxies.accountProxyConfig')" :disabled="!accountProxySummary.total || globalApplying">
          <option :value="null">{{ t("proxies.allDirect") }}</option>
          <option v-for="proxy in proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }} ({{ proxy.type.toUpperCase() }})</option>
        </select>
        <button class="btn btn-primary btn-sm" data-test="proxy-global-apply" :disabled="!accountProxySummary.total || globalApplying" @click="requestGlobalApply(globalProxyID)"><Icon name="accounts" :size="14" />{{ t("proxies.apply") }}</button>
        <button class="icon-button proxy-tool-button" type="button" :disabled="!accountProxySummary.total || accountProxySummary.unbound === accountProxySummary.total || globalApplying" :title="t('proxies.clearAll')" :aria-label="t('proxies.clearAll')" @click="requestGlobalApply(null)"><Icon name="proxies" :size="15" /></button>
        <button class="icon-button proxy-tool-button" type="button" :disabled="!accountProxySummary.total || startingAccountTest" :title="t('proxies.testAllAccounts')" :aria-label="t('proxies.testAllAccounts')" @click="testAllAccounts"><Icon name="activity" :size="15" /></button>
      </div>
    </section>

    <div v-if="!loading && proxies.length === 0" class="card">
      <EmptyState icon="proxies" :title="t('proxies.empty')" :description="t('proxies.emptyDesc')"><button class="btn btn-primary" @click="openAdd"><Icon name="plus" :size="16" /> {{ t("proxies.add") }}</button></EmptyState>
    </div>

    <div v-else-if="!loading" class="card">
      <div class="list">
        <div v-for="p in proxies" :key="p.id" class="list-row">
          <span class="badge badge-neutral" style="text-transform: uppercase">{{ p.type }}</span>
          <div style="flex: 1">
            <div style="font-weight: 550">{{ p.name }}</div>
            <div class="faint mono text-sm">{{ p.host }}:{{ p.port }}</div>
					<div class="proxy-bound-count">{{ t("proxies.boundAccounts", { count: proxyAccountCount(p.id) }) }}</div>
					<div v-if="testResults[p.id]" class="proxy-stages">
						<span v-for="stage in testResults[p.id].stages" :key="stage.id" class="proxy-stage" :class="`stage-${stage.status}`">
							{{ t(`proxies.stage.${stage.id}`) }}
						</span>
					</div>
          </div>
          <button class="btn btn-ghost btn-sm" :disabled="testing[p.id]" @click="test(p)">
            <Icon name="refresh" :size="14" :class="testing[p.id] ? 'spin' : ''" />
            {{ testing[p.id] ? t("proxies.testing") : t("common.test") }}
          </button>
          <button class="btn btn-ghost btn-sm" @click="openEdit(p)">
            <Icon name="edit" :size="14" /> {{ t("common.edit") }}
          </button>
          <button class="btn btn-ghost btn-sm" :disabled="!accountProxySummary.total || globalApplying" :title="t('proxies.applyAll')" @click="requestGlobalApply(p.id)"><Icon name="accounts" :size="14" /></button>
          <button class="btn btn-danger btn-sm" @click="deleteTarget = p">
            <Icon name="trash" :size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- Add modal -->
    <Teleport to="body">
      <div v-if="addOpen" class="modal-backdrop" @click.self="addOpen = false">
        <div class="modal">
          <h3 class="modal-title">{{ editId != null ? t("proxies.edit") : t("proxies.add") }}</h3>
          <div class="field">
            <label class="field-label">{{ t("proxies.name") }}</label>
            <input v-model="form.name" class="input" :placeholder="t('proxies.namePlaceholder')" />
          </div>
          <div class="grid grid-2">
            <div class="field">
              <label class="field-label">{{ t("proxies.type") }}</label>
              <select v-model="form.type" class="select">
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
                <option value="socks5">SOCKS5</option>
              </select>
            </div>
            <div class="field">
              <label class="field-label">{{ t("proxies.port") }}</label>
              <input v-model.number="form.port" type="number" class="input" />
            </div>
          </div>
          <div class="field">
            <label class="field-label">{{ t("proxies.host") }}</label>
            <input v-model="form.host" class="input" :placeholder="t('proxies.hostPlaceholder')" />
          </div>
          <div class="grid grid-2">
            <div class="field">
              <label class="field-label">{{ t("proxies.username") }}</label>
              <input v-model="form.username" class="input" />
            </div>
            <div class="field">
              <label class="field-label">{{ t("proxies.password") }}</label>
              <input v-model="form.password" type="password" class="input" :disabled="clearPassword" :placeholder="editId != null ? t('proxies.passwordKeep') : ''" />
            </div>
          </div>
          <label v-if="editId != null" class="import-validate">
            <input v-model="clearPassword" type="checkbox" />
            <span>{{ t("proxies.passwordClear") }}</span>
          </label>
          <div class="modal-actions">
            <button class="btn btn-ghost" @click="addOpen = false">{{ t("common.cancel") }}</button>
            <button class="btn btn-primary" :disabled="saving" @click="save">{{ t("common.save") }}</button>
          </div>
        </div>
      </div>
    </Teleport>

    <ConfirmModal
      :open="!!deleteTarget"
      :title="t('proxies.deleteConfirm')"
      :desc="t('proxies.deleteDesc')"
      danger
      :confirm-text="t('common.delete')"
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
    <ConfirmModal
      :open="globalApplyOpen"
      :title="t('proxies.applyAllTitle')"
      :desc="t('proxies.applyAllDesc', { proxy: globalApplyTarget ? proxies.find((proxy) => proxy.id === globalApplyTarget)?.name || t('common.unknown') : t('proxies.allDirect'), count: accountProxySummary.total })"
      :confirm-text="t('proxies.applyAll')"
      :loading="globalApplying"
      @confirm="confirmGlobalApply"
      @cancel="globalApplyOpen = false"
    />
  </div>
</template>

<style scoped>
.proxy-stages { display: flex; gap: 5px; flex-wrap: wrap; margin-top: 6px; }
.proxy-account-panel { min-height: 58px; display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-bottom: 12px; padding: 9px 0; border-block: 1px solid var(--border-soft); }
.proxy-account-summary { min-width: 180px; display: flex; align-items: center; gap: 9px; }.proxy-account-summary > div { min-width: 0; display: grid; gap: 2px; }.proxy-account-summary span { color: var(--text-faint); font-size: 10.5px; }.proxy-account-summary strong { overflow: hidden; color: var(--text); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }.proxy-account-icon { width: 30px; height: 30px; display: grid; place-items: center; flex: 0 0 auto; border-radius: 7px; background: var(--primary-soft); color: var(--primary) !important; }
.proxy-account-controls { min-width: 0; display: flex; align-items: center; justify-content: flex-end; gap: 6px; flex-wrap: nowrap; }.proxy-account-controls .select { width: min(200px, 100%); }.proxy-tool-button { width: 34px; height: 34px; flex: 0 0 auto; }.proxy-tool-button:disabled { opacity: .42; cursor: default; }
.proxy-bound-count { margin-top: 4px; color: var(--text-faint); font-size: 10.5px; }
.proxy-stage { font-size: 11px; line-height: 18px; padding: 0 6px; border: 1px solid var(--border); border-radius: 4px; color: var(--text-muted); }
.stage-ok { color: var(--success); border-color: color-mix(in srgb, var(--success) 35%, var(--border)); }
.stage-failed { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, var(--border)); }
.stage-not_run, .stage-skipped { opacity: .55; }
@media (max-width: 760px) { .proxy-account-panel { align-items: stretch; flex-direction: column; }.proxy-account-controls { justify-content: flex-start; }.proxy-account-controls .select { flex: 1 1 180px; } }
</style>
