<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import { api, type PricingResponse } from "../api/control";
import { openUrl } from "../platform";
import { useAppStore } from "../store";

type PricedModel = PricingResponse["models"][number];

const { t } = useI18n();
const app = useAppStore();
const pricing = ref<PricingResponse | null>(null);
const loading = ref(true);
const copiedModel = ref("");
let copiedTimer = 0;

function money(value?: number): string {
  if (value === undefined) return "—";
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 3 })}`;
}

function longPrice(model: PricedModel): string {
  return `${money(model.long_input_per_m)} / ${money(model.long_cached_per_m)} / ${money(model.long_output_per_m)}`;
}

async function copyModel(name: string) {
  try {
    await navigator.clipboard.writeText(name);
    copiedModel.value = name;
    window.clearTimeout(copiedTimer);
    copiedTimer = window.setTimeout(() => (copiedModel.value = ""), 1600);
  } catch {
    app.toast(t("models.copyFailed"), "error");
  }
}

onMounted(async () => {
  try {
    pricing.value = await api.pricing();
  } catch (error) {
    app.toast((error as Error).message, "error");
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <div class="models-page">
    <div class="page-header">
      <h1 class="page-title">{{ t("models.title") }}</h1>
      <p class="page-desc">{{ t("models.desc") }}</p>
    </div>

    <section class="pricing-notice">
      <Icon name="info" :size="18" />
      <div><strong>{{ t("models.standardTitle") }}</strong><p>{{ t("models.standardDesc") }}</p></div>
    </section>

    <SkeletonBlock v-if="loading" :cards="3" :rows="4" />
    <section v-else-if="pricing" class="pricing-section">
      <div class="model-grid">
        <article v-for="model in pricing.models" :key="model.model" class="model-card">
          <header class="model-card-head">
            <span class="model-icon"><Icon name="database" :size="18" /></span>
            <strong class="mono model-name">{{ model.model }}</strong>
            <button
              class="copy-btn"
              type="button"
              :title="t('models.copyModel')"
              :aria-label="t('models.copyModel')"
              @click="copyModel(model.model)"
            >
              <Icon :name="copiedModel === model.model ? 'check' : 'copy'" :size="14" />
            </button>
          </header>

          <dl class="price-lines">
            <div class="price-line">
              <dt>{{ t("models.inputPrice") }}</dt>
              <dd class="mono">{{ money(model.input_per_m) }} / 1M Tokens</dd>
            </div>
            <div class="price-line">
              <dt>{{ t("models.outputPrice") }}</dt>
              <dd class="mono">{{ money(model.output_per_m) }} / 1M Tokens</dd>
            </div>
            <div class="price-line">
              <dt>{{ t("models.cachedPrice") }}</dt>
              <dd class="mono">{{ model.cached_per_m === undefined ? "—" : `${money(model.cached_per_m)} / 1M Tokens` }}</dd>
            </div>
            <div v-if="model.long_context_threshold" class="price-line">
              <dt>{{ t("models.longPrice") }}</dt>
              <dd class="mono long-price">{{ longPrice(model) }}</dd>
            </div>
          </dl>

          <footer class="model-badges">
            <span class="badge-tag">{{ t("models.standardBadge") }}</span>
            <span class="badge-tag badge-match">{{ t("models.priceMatch") }}</span>
            <span v-if="model.long_context_threshold" class="badge-tag badge-long">272K+</span>
          </footer>
        </article>
      </div>

      <footer class="pricing-notes">
        <p>{{ t("models.unit") }}</p>
        <p>{{ t("models.longThreshold", { count: "272k" }) }}</p>
        <p>{{ t("models.cacheNote") }}</p>
        <p>{{ t("models.subscriptionNote") }}</p>
        <button class="source-link" type="button" @click="openUrl(pricing.source_url)">
          {{ t("models.source", { date: pricing.price_version }) }}: <span class="mono">{{ pricing.source_url }}</span> <Icon name="external" :size="13" />
        </button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.models-page { width: 100%; }
.pricing-notice { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 10px; margin-bottom: 16px; padding: 13px 15px; border-block: 1px solid var(--border-soft); color: var(--primary); }
.pricing-notice strong { font-size: 13px; }
.pricing-notice p { margin: 3px 0 0; color: var(--text-dim); font-size: 12.5px; line-height: 1.55; }
.pricing-section { min-width: 0; }
.model-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.model-card { position: relative; display: flex; flex-direction: column; gap: 13px; padding: 17px 18px 15px; border: 1px solid var(--border-soft); border-radius: var(--radius); background: var(--bg-card); box-shadow: var(--shadow-xs); overflow: hidden; transition: transform var(--motion-normal) var(--motion-ease), box-shadow var(--motion-normal) var(--motion-ease), border-color var(--motion-fast) var(--motion-ease); }
.model-card::before { content: ""; position: absolute; inset: 0 0 auto; height: 2px; background: linear-gradient(90deg, var(--primary), transparent 70%); opacity: 0.55; }
.model-card:hover { transform: translateY(-2px); box-shadow: var(--shadow-hover); border-color: var(--border); }
.model-card-head { display: flex; align-items: center; gap: 10px; padding-bottom: 12px; border-bottom: 1px solid var(--border-soft); }
.model-icon { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 9px; background: var(--primary-soft); color: var(--primary); flex-shrink: 0; }
.model-name { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 14.5px; letter-spacing: -0.01em; }
.copy-btn { display: grid; place-items: center; width: 27px; height: 27px; padding: 0; border: 1px solid transparent; border-radius: 7px; background: transparent; color: var(--text-faint); cursor: pointer; transition: background var(--motion-fast) var(--motion-ease), color var(--motion-fast) var(--motion-ease); }
.copy-btn:hover { background: var(--bg-elev); border-color: var(--border-soft); color: var(--text); }
.price-lines { display: grid; margin: 0; }
.price-line { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; padding: 5px 0; }
.price-line + .price-line { border-top: 1px dashed var(--border-soft); }
.price-line dt { color: var(--text-faint); font-size: 12px; }
.price-line dd { margin: 0; font-size: 13px; font-weight: 600; white-space: nowrap; }
.long-price { color: var(--text-dim); font-size: 11.5px; font-weight: 500; }
.model-badges { display: flex; flex-wrap: wrap; gap: 6px; margin-top: auto; padding-top: 11px; border-top: 1px solid var(--border-soft); }
.badge-tag { padding: 3.5px 9px; border: 1px solid var(--border-soft); border-radius: 999px; background: var(--bg-elev); color: var(--text-dim); font-size: 10.5px; font-weight: 600; letter-spacing: 0.02em; }
.badge-match { border-color: transparent; background: var(--success-soft); color: var(--success); }
.badge-long { border-color: transparent; background: var(--primary-soft); color: var(--primary); }
.pricing-notes { display: grid; gap: 5px; padding: 14px 2px; color: var(--text-faint); font-size: 12px; line-height: 1.55; }
.pricing-notes p { margin: 0; }
.source-link { max-width: 100%; width: fit-content; display: inline-flex; align-items: center; gap: 5px; padding: 0; border: 0; background: transparent; color: var(--primary); font: inherit; cursor: pointer; overflow-wrap: anywhere; text-align: left; }
@media (max-width: 1100px) { .model-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) { .model-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
