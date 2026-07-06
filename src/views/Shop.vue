<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import { openUrl } from "../platform";
import { useAppStore } from "../store";

const { t } = useI18n();
const app = useAppStore();

const SHOP_URL = "https://pay.ldxp.cn/shop/PZ01GCD3";

async function copyLink() {
  try {
    await navigator.clipboard.writeText(SHOP_URL);
    app.toast(t("common.copied"), "success");
  } catch {
    app.toast(t("shop.copyFailed"), "error");
  }
}
</script>

<template>
  <div class="shop-page">
    <div class="page-header">
      <h1 class="page-title">{{ t("shop.title") }}</h1>
      <p class="page-desc">{{ t("shop.desc") }}</p>
    </div>

    <div class="card shop-hero">
      <div class="shop-hero-icon"><Icon name="cart" :size="30" /></div>
      <h2 class="shop-hero-title">{{ t("shop.heroTitle") }}</h2>
      <p class="shop-hero-sub">{{ t("shop.heroSub") }}</p>

      <button class="btn btn-primary btn-lg" @click="openUrl(SHOP_URL)">
        <Icon name="external" :size="16" /> {{ t("shop.open") }}
      </button>

      <div class="shop-url-row">
        <span class="shop-url">{{ SHOP_URL }}</span>
        <button class="btn btn-ghost btn-sm" @click="copyLink">
          <Icon name="copy" :size="14" /> {{ t("common.copy") }}
        </button>
      </div>
    </div>

    <div class="card shop-steps">
      <h3 class="card-title"><Icon name="docs" :size="16" /> {{ t("shop.stepsTitle") }}</h3>
      <ol class="shop-step-list">
        <li>{{ t("shop.step1") }}</li>
        <li>{{ t("shop.step2") }}</li>
        <li>{{ t("shop.step3") }}</li>
      </ol>
      <div class="shop-note">
        <Icon name="warn" :size="14" /> <span>{{ t("shop.note") }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.shop-page {
  display: flex;
  flex-direction: column;
}
.shop-hero {
  text-align: center;
  padding: 32px 24px 24px;
}
.shop-hero-icon {
  width: 60px;
  height: 60px;
  margin: 0 auto 14px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--primary);
  background: var(--primary-soft);
}
.shop-hero-title {
  margin: 0 0 6px;
  font-size: 20px;
  font-weight: 650;
  color: var(--text);
}
.shop-hero-sub {
  margin: 0 auto 20px;
  max-width: 560px;
  color: var(--text-dim);
  font-size: 14px;
  line-height: 1.7;
}
.btn-lg {
  padding: 11px 22px;
  font-size: 15px;
}
.shop-url-row {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-top: 18px;
  padding: 6px 6px 6px 14px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--bg-elev);
}
.shop-url {
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--text-dim);
}
.shop-steps {
  margin-top: 16px;
}
.shop-step-list {
  margin: 8px 0 0;
  padding-left: 20px;
  color: var(--text-dim);
  font-size: 13.5px;
  line-height: 1.9;
}
.shop-note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--warn-soft);
  color: var(--text-dim);
  font-size: 12.5px;
  line-height: 1.6;
}
</style>
