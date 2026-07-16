<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import SkeletonBlock from "../components/SkeletonBlock.vue";
import { api, type PricingResponse } from "../api/control";
import { openUrl } from "../platform";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();
const pricing = ref<PricingResponse | null>(null);
const loading = ref(true);

function money(value?: number): string {
  if (value === undefined) return "-";
  return `$${value.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 3 })}`;
}

function longPrice(model: PricingResponse["models"][number]): string {
  if (!model.long_context_threshold) return "-";
  return `${money(model.long_input_per_m)} / ${money(model.long_cached_per_m)} / ${money(model.long_output_per_m)}`;
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

    <SkeletonBlock v-if="loading" :cards="1" :rows="8" />
    <section v-else-if="pricing" class="pricing-section">
      <div class="pricing-table-wrap">
        <table class="pricing-table">
          <thead><tr><th>{{ t("models.model") }}</th><th>{{ t("models.input") }}</th><th>{{ t("models.cached") }}</th><th>{{ t("models.output") }}</th><th>{{ t("models.longContext") }}</th></tr></thead>
          <tbody>
            <tr v-for="model in pricing.models" :key="model.model">
              <td><strong class="mono">{{ model.model }}</strong><span class="price-match">{{ t("models.priceMatch") }}</span></td>
              <td>{{ money(model.input_per_m) }}</td>
              <td>{{ money(model.cached_per_m) }}</td>
              <td>{{ money(model.output_per_m) }}</td>
              <td class="long-price">{{ longPrice(model) }}</td>
            </tr>
          </tbody>
        </table>
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
.pricing-table-wrap { overflow-x: auto; border-block: 1px solid var(--border); }
.pricing-table { width: 100%; min-width: 820px; border-collapse: collapse; font-size: 13px; }
.pricing-table th { padding: 11px 12px; color: var(--text-faint); font-size: 11px; text-align: right; text-transform: uppercase; }
.pricing-table th:first-child { text-align: left; }
.pricing-table td { padding: 13px 12px; border-top: 1px solid var(--border-soft); text-align: right; white-space: nowrap; }
.pricing-table td:first-child { display: flex; align-items: center; justify-content: space-between; gap: 10px; text-align: left; }
.price-match { padding: 3px 6px; border-radius: 4px; background: var(--success-soft); color: var(--success); font-size: 10px; font-weight: 700; }
.long-price { color: var(--text-dim); font-family: var(--mono); font-size: 12px; }
.pricing-notes { display: grid; gap: 5px; padding: 14px 2px; color: var(--text-faint); font-size: 12px; line-height: 1.55; }
.pricing-notes p { margin: 0; }
.source-link { max-width: 100%; width: fit-content; display: inline-flex; align-items: center; gap: 5px; padding: 0; border: 0; background: transparent; color: var(--primary); font: inherit; cursor: pointer; overflow-wrap: anywhere; text-align: left; }
@media (max-width: 760px) { .pricing-table td, .pricing-table th { padding-inline: 9px; } }
</style>
