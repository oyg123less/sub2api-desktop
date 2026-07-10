<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import ConfirmModal from "../components/ConfirmModal.vue";
import { api, type Proxy } from "../api/control";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const proxies = ref<Proxy[]>([]);
const loading = ref(true);
const testing = ref<Record<number, boolean>>({});
type ProxyTest = Awaited<ReturnType<typeof api.testProxy>>;
const testResults = ref<Record<number, ProxyTest>>({});

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

async function load() {
  try {
    proxies.value = (await api.listProxies()).proxies || [];
  } catch (e) {
    app.toast((e as Error).message, "error");
  } finally {
    loading.value = false;
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

    <div v-if="loading" class="empty">{{ t("common.loading") }}</div>

    <div v-else-if="proxies.length === 0" class="card empty">
      <div class="empty-icon">🌐</div>
      <div class="empty-title">{{ t("proxies.empty") }}</div>
      <div class="faint">{{ t("proxies.emptyDesc") }}</div>
      <button class="btn btn-primary mt-16" @click="openAdd">
        <Icon name="plus" :size="16" /> {{ t("proxies.add") }}
      </button>
    </div>

    <div v-else class="card">
      <div class="list">
        <div v-for="p in proxies" :key="p.id" class="list-row">
          <span class="badge badge-neutral" style="text-transform: uppercase">{{ p.type }}</span>
          <div style="flex: 1">
            <div style="font-weight: 550">{{ p.name }}</div>
            <div class="faint mono text-sm">{{ p.host }}:{{ p.port }}</div>
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
  </div>
</template>

<style scoped>
.proxy-stages { display: flex; gap: 5px; flex-wrap: wrap; margin-top: 6px; }
.proxy-stage { font-size: 11px; line-height: 18px; padding: 0 6px; border: 1px solid var(--border); border-radius: 4px; color: var(--text-muted); }
.stage-ok { color: var(--success); border-color: color-mix(in srgb, var(--success) 35%, var(--border)); }
.stage-failed { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, var(--border)); }
.stage-not_run, .stage-skipped { opacity: .55; }
</style>
