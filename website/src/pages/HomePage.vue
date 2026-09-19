<script setup lang="ts">
import {
  ArrowRight,
  Cable,
  ChevronDown,
  Download,
  Gauge,
  Github,
  Share2,
} from "lucide-vue-next";
import type { Component } from "vue";
import { RouterLink } from "vue-router";
import InteractiveCard from "../components/InteractiveCard.vue";
import ProductScreenshot from "../components/ProductScreenshot.vue";
import ProductShowcase from "../components/ProductShowcase.vue";
import RevealOnScroll from "../components/RevealOnScroll.vue";
import SectionHeading from "../components/SectionHeading.vue";
import { stableRelease } from "../config/releases";
import { homeFaqs } from "../data/faq";
import { capabilityItems, trustItems } from "../data/home";

const capabilityIcons: Record<string, Component> = {
  routing: Gauge,
  sharing: Share2,
  codex: Cable,
};
</script>

<template>
  <section class="home-hero" aria-labelledby="home-title">
    <div class="hero-glow" aria-hidden="true"></div>
    <div class="hero-shell">
      <RevealOnScroll class="hero-copy">
        <span class="hero-version">
          <span class="pulse-dot" aria-hidden="true"></span>
          v{{ stableRelease.version }} 已发布 · 原生动态联动
        </span>
        <h1 id="home-title">Amber</h1>
        <p class="hero-lede">把多个账号，变成可调度、可共享的 Codex 网关</p>
        <p class="hero-description">在 Windows 上统一管理账号池、网络出口与受控授权，本地优先无损接入。</p>

        <div class="hero-actions">
          <a class="button button-primary hero-download" :href="stableRelease.downloadUrl">
            <Download :size="19" aria-hidden="true" />
            下载 Windows
          </a>
          <RouterLink class="button button-secondary" to="/docs">
            查看使用文档
            <ArrowRight :size="17" aria-hidden="true" />
          </RouterLink>
        </div>

        <p class="hero-meta">Windows 10 / 11 · x64 · 免费开源 · 本地优先</p>
      </RevealOnScroll>

      <RevealOnScroll class="hero-product" :delay="90">
        <div class="hero-media-wrapper">
          <div class="border-beam" aria-hidden="true"></div>
          <div class="border-beam-runner" aria-hidden="true"></div>
          <ProductScreenshot
            src="/gateway-topology.svg"
            mobile-src="/gateway-topology.svg"
            alt="Amber 多账号智能网关与 Codex 原生联动拓扑架构"
            :width="1200"
            :height="520"
            loading="eager"
            fetch-priority="high"
          />
        </div>
      </RevealOnScroll>
    </div>
  </section>

  <section class="home-trust" aria-label="Amber 产品信息">
    <div class="container trust-list">
      <span v-for="item in trustItems" :key="item" class="trust-badge">
        <span class="trust-dot"></span>
        {{ item }}
      </span>
    </div>
  </section>

  <section class="section section-spacious" aria-labelledby="capability-title">
    <div class="container">
      <SectionHeading
        eyebrow="核心特性"
        heading-id="capability-title"
        title="一个入口，打通账号池、共享与 Codex"
        description="将零散的配置整合为可观测、可自愈的本地网关，每项功能都有清晰的边界与状态。"
      />

      <div class="capability-grid">
        <RevealOnScroll
          v-for="(item, index) in capabilityItems"
          :key="item.id"
          :delay="index * 70"
        >
          <InteractiveCard class="capability-card" :tone="item.tone">
            <span class="card-icon capability-icon">
              <component :is="capabilityIcons[item.id]" :size="24" aria-hidden="true" />
            </span>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
            <RouterLink class="card-link" :to="item.to">
              了解更多
              <ArrowRight class="card-arrow" :size="17" aria-hidden="true" />
            </RouterLink>
          </InteractiveCard>
        </RevealOnScroll>
      </div>
    </div>
  </section>

  <section id="product-showcase" class="section section-muted product-section" aria-labelledby="showcase-title">
    <div class="container">
      <SectionHeading
        eyebrow="产品全景"
        heading-id="showcase-title"
        title="四个核心视图，一览无余"
        description="调度状态、网络出口、受控共享与原生接入。均为真实客户端界面，敏感数据已做脱敏处理。"
      />
      <ProductShowcase />
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="home-faq-title">
    <div class="container home-faq-layout">
      <SectionHeading
        eyebrow="常见问题"
        heading-id="home-faq-title"
        title="开始之前，理清关键细节"
        description="这里汇总了最受关注的核心疑问，更多深度解答请查阅完整常见问题页。"
      />

      <div class="home-faq-list">
        <details v-for="faq in homeFaqs" :key="faq.id" class="home-faq-item" :data-faq-id="faq.id">
          <summary>
            <span>{{ faq.question }}</span>
            <ChevronDown :size="19" aria-hidden="true" />
          </summary>
          <div class="home-faq-answer">
            <p v-for="paragraph in faq.paragraphs" :key="paragraph">{{ paragraph }}</p>
          </div>
        </details>
      </div>

      <RouterLink class="button button-secondary faq-more" to="/faq">
        查看全部常见问题
        <ArrowRight :size="17" aria-hidden="true" />
      </RouterLink>
    </div>
  </section>

  <section class="section-compact home-cta" aria-labelledby="cta-title">
    <div class="container cta-layout">
      <div>
        <p class="eyebrow">Amber v{{ stableRelease.version }}</p>
        <h2 id="cta-title">准备好让 Codex 连接到你的账号池了吗？</h2>
        <p>面向 Windows 10 / 11 · x64 · 免费开源，即刻体验</p>
      </div>
      <div class="cta-actions">
        <a class="button button-primary" :href="stableRelease.downloadUrl">
          <Download :size="18" aria-hidden="true" />
          下载 Windows
        </a>
        <RouterLink class="button button-secondary cta-secondary" to="/docs">查看使用文档</RouterLink>
        <a class="cta-github" href="https://github.com/oyg123less/sub2api-desktop" target="_blank" rel="noreferrer">
          <Github :size="17" aria-hidden="true" />
          GitHub
        </a>
      </div>
    </div>
  </section>
</template>

<style scoped>
.home-hero {
  position: relative;
  overflow: hidden;
  border-bottom: 1px solid var(--border);
  background:
    radial-gradient(ellipse 70% 40% at 50% -5%, rgba(245, 158, 11, 0.12) 0%, transparent 65%),
    var(--bg);
}

.hero-glow {
  position: absolute;
  top: -160px;
  left: 50%;
  transform: translateX(-50%);
  width: 800px;
  height: 400px;
  background: radial-gradient(circle, rgba(245, 158, 11, 0.15) 0%, transparent 70%);
  filter: blur(80px);
  pointer-events: none;
  z-index: 0;
}

.hero-shell {
  position: relative;
  z-index: 1;
  width: min(calc(100% - 40px), var(--hero-container));
  margin-inline: auto;
  padding-block: 56px 36px;
}

.hero-copy {
  width: min(100%, 860px);
  margin-inline: auto;
  text-align: center;
}

.hero-version {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  padding: 4px 14px;
  border: 1px solid rgba(245, 158, 11, 0.3);
  border-radius: 999px;
  background: rgba(245, 158, 11, 0.08);
  color: #fbbf24;
  font-size: 13px;
  font-weight: 650;
  box-shadow: 0 0 16px rgba(245, 158, 11, 0.1);
  margin-bottom: 12px;
}

.pulse-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
  animation: pulse-ring 2.4s infinite ease-in-out;
}

@keyframes pulse-ring {
  0% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.6);
  }
  70% {
    transform: scale(1);
    box-shadow: 0 0 0 6px rgba(16, 185, 129, 0);
  }
  100% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(16, 185, 129, 0);
  }
}

.hero-copy h1 {
  margin: 12px 0 10px;
  font-size: 68px;
  font-weight: 850;
  letter-spacing: -0.035em;
  background: linear-gradient(180deg, #ffffff 45%, #94a3b8 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  line-height: 1.05;
}

.hero-lede {
  margin-bottom: 12px;
  font-size: 26px;
  font-weight: 700;
  line-height: 1.4;
  color: #f1f5f9;
  letter-spacing: -0.01em;
}

.hero-description {
  max-width: 680px;
  margin: 0 auto 28px;
  color: var(--ink-soft);
  font-size: 17px;
  line-height: 1.6;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 14px;
}

.hero-actions .button {
  min-height: 50px;
  padding-inline: 24px;
  font-size: 15px;
}

.hero-meta {
  margin: 18px 0 0;
  color: var(--ink-faint);
  font-size: 13px;
  font-weight: 500;
}

/* Hero Product Showcase */
.hero-product {
  position: relative;
  width: min(100%, 1160px);
  margin: 32px auto 0;
}

.hero-media-wrapper {
  position: relative;
  width: 100%;
  aspect-ratio: 1200 / 520;
  border-radius: 16px;
  overflow: hidden;
  border: none;
  background: #080c14;
  box-shadow:
    0 30px 80px -20px rgba(0, 0, 0, 0.95),
    0 0 50px -10px rgba(245, 158, 11, 0.12);
}

/* Static base border - exactly ONE 1.5px frame */
.border-beam {
  position: absolute;
  inset: 0;
  border-radius: inherit;
  border: 1.5px solid rgba(255, 255, 255, 0.08);
  pointer-events: none;
  z-index: 9;
}

/* Moving laser runner - precisely overlaps the 1.5px static border */
.border-beam-runner {
  position: absolute;
  inset: 0;
  border-radius: inherit;
  border: 1.5px solid transparent;
  -webkit-mask: linear-gradient(#fff 0 0) padding-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask: linear-gradient(#fff 0 0) padding-box, linear-gradient(#fff 0 0);
  mask-composite: exclude;
  pointer-events: none;
  z-index: 10;
}

.border-beam-runner::after {
  content: "";
  position: absolute;
  width: 280px;
  height: 280px;
  offset-path: rect(0 100% 100% 0 round 16px);
  offset-anchor: 50% 50%;
  background: radial-gradient(
    circle at center,
    #ffffff 0%,
    #fef08a 18%,
    #fbbf24 38%,
    rgba(245, 158, 11, 0.45) 58%,
    transparent 76%
  );
  animation: border-beam-travel 5.5s linear infinite;
}

@keyframes border-beam-travel {
  0% {
    offset-distance: 0%;
  }
  100% {
    offset-distance: 100%;
  }
}

@supports not (offset-path: rect(0 100% 100% 0 round 16px)) {
  .border-beam-runner::after {
    width: 200%;
    height: 200%;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    background: conic-gradient(
      from 0deg,
      transparent 0deg,
      transparent 280deg,
      rgba(245, 158, 11, 0.08) 310deg,
      #f59e0b 345deg,
      #ffffff 360deg
    );
    animation: border-beam-spin 5s linear infinite;
  }
}

@keyframes border-beam-spin {
  from {
    transform: translate(-50%, -50%) rotate(0deg);
  }
  to {
    transform: translate(-50%, -50%) rotate(360deg);
  }
}

.hero-product :deep(.image-viewer),
.hero-product :deep(.image-button),
.hero-product :deep(picture),
.hero-product :deep(img) {
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  display: block;
  object-fit: cover;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
}

.hero-product :deep(.image-button) {
  aspect-ratio: auto;
  cursor: zoom-in;
}

.hero-product :deep(.image-expand) {
  display: none;
}

/* Trust Badges */
.home-trust {
  border-bottom: 1px solid var(--border);
  background: var(--bg-subtle);
  padding-block: 16px;
}

.trust-list {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 12px;
}

.trust-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 16px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.03);
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 600;
  backdrop-filter: blur(8px);
  transition:
    border-color var(--motion-fast) ease,
    color var(--motion-fast) ease;
}

.trust-badge:hover {
  border-color: rgba(255, 255, 255, 0.18);
  color: var(--ink);
}

.trust-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #fbbf24;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.5);
}

/* Core Capabilities Grid */
.capability-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 24px;
}

.capability-grid > * {
  min-width: 0;
  height: 100%;
}

.capability-card {
  display: flex;
  height: 100%;
  flex-direction: column;
  padding: 36px 32px;
}

.capability-icon {
  display: grid;
  width: 48px;
  height: 48px;
  margin-bottom: 24px;
  place-items: center;
  border-radius: var(--radius-control);
  background: var(--card-accent-soft);
  color: var(--card-accent);
  border: 1px solid rgba(255, 255, 255, 0.06);
  box-shadow: 0 0 16px var(--card-accent-soft);
}

.capability-card h3 {
  margin-bottom: 12px;
  font-size: 21px;
  font-weight: 750;
  color: var(--ink);
}

.capability-card p {
  margin-bottom: 24px;
  color: var(--ink-soft);
  font-size: 15px;
  line-height: 1.65;
}

.card-link {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: #fbbf24;
  font-size: 14px;
  font-weight: 700;
  text-decoration: none;
  margin-top: auto;
}

.card-link:hover {
  color: #fde68a;
}

.product-section {
  border-block: 1px solid var(--border);
}

/* FAQ Section */
.home-faq-layout > .section-heading {
  margin-bottom: 36px;
}

.home-faq-list {
  border-block: 1px solid var(--border);
}

.home-faq-item + .home-faq-item {
  border-top: 1px solid var(--border);
}

.home-faq-item[open] {
  background: rgba(255, 255, 255, 0.02);
}

.home-faq-item summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 20px;
  align-items: center;
  padding: 22px 18px;
  color: var(--ink);
  font-size: 17px;
  font-weight: 700;
  cursor: pointer;
  list-style: none;
  transition:
    background-color var(--motion-fast) ease,
    color var(--motion-fast) ease;
}

.home-faq-item summary::-webkit-details-marker {
  display: none;
}

.home-faq-item summary:hover {
  background: rgba(255, 255, 255, 0.04);
  color: #fbbf24;
}

.home-faq-item summary svg {
  color: var(--ink-faint);
  transition: transform var(--motion-fast) var(--ease-out);
}

.home-faq-item[open] summary svg {
  transform: rotate(180deg);
  color: #fbbf24;
}

.home-faq-answer {
  max-width: 880px;
  padding: 0 52px 24px 18px;
  color: var(--ink-soft);
  font-size: 15px;
  line-height: 1.65;
}

.home-faq-answer p:last-child {
  margin-bottom: 0;
}

.faq-more {
  margin-top: 32px;
}

/* Bottom CTA */
.home-cta {
  position: relative;
  overflow: hidden;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  background:
    radial-gradient(circle at 75% 50%, rgba(245, 158, 11, 0.12) 0%, transparent 60%),
    #070a0f;
  color: var(--ink);
}

.cta-layout {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 32px;
}

.home-cta .eyebrow {
  color: #fbbf24;
}

.home-cta h2 {
  max-width: 720px;
  margin-bottom: 10px;
  color: #fff;
  font-size: 36px;
  letter-spacing: -0.02em;
}

.home-cta p:last-child {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.cta-actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}

.cta-github {
  display: inline-flex;
  min-height: 46px;
  align-items: center;
  gap: 8px;
  padding-inline: 14px;
  color: var(--ink-soft);
  font-size: 14px;
  font-weight: 700;
  text-decoration: none;
  border-radius: var(--radius-control);
  transition:
    color var(--motion-fast) ease,
    background-color var(--motion-fast) ease;
}

.cta-github:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.05);
}

@media (max-width: 900px) {
  .hero-shell {
    padding-top: 30px;
  }

  .hero-copy h1 {
    font-size: 56px;
  }

  .hero-lede {
    font-size: 24px;
  }

  .capability-grid {
    grid-template-columns: 1fr;
  }

  .cta-layout {
    flex-direction: column;
    align-items: flex-start;
  }
}

@media (max-width: 640px) {
  .hero-shell {
    width: min(calc(100% - 28px), var(--hero-container));
    padding-block: 18px 10px;
  }

  .hero-version {
    min-height: 24px;
    padding-block: 2px;
    font-size: 11px;
  }

  .hero-copy h1 {
    margin-block: 5px 3px;
    font-size: 46px;
  }

  .hero-lede {
    margin-bottom: 5px;
    font-size: 20px;
    line-height: 1.38;
  }

  .hero-description {
    max-width: 340px;
    margin: 0 auto 12px;
    font-size: 13px;
    line-height: 1.5;
  }

  .hero-actions {
    flex-wrap: nowrap;
    gap: 7px;
  }

  .hero-actions .button {
    min-height: 44px;
    padding-inline: 12px;
    font-size: 13px;
  }

  .hero-meta {
    margin-top: 6px;
  }

  .hero-product {
    height: auto;
    aspect-ratio: 3 / 2;
    margin-top: 13px;
  }

  .hero-media-wrapper {
    border-radius: 12px;
  }

  .border-beam::after {
    offset-path: rect(0 100% 100% 0 round 12px);
  }

  .capability-grid {
    grid-template-columns: 1fr;
  }

  .capability-card {
    padding: 24px;
  }

  .cta-actions {
    width: 100%;
  }

  .cta-actions .button {
    width: 100%;
  }
}

@media (min-width: 641px) and (max-height: 760px) {
  .hero-shell {
    padding-block: 22px 10px;
  }

  .hero-copy h1 {
    margin-block: 6px 3px;
    font-size: 52px;
  }

  .hero-lede {
    margin-bottom: 4px;
    font-size: 22px;
  }

  .hero-description {
    margin-bottom: 12px;
    font-size: 15px;
  }

  .hero-actions .button {
    min-height: 44px;
  }

  .hero-meta {
    display: none;
  }

  .hero-product {
    width: min(100%, calc((100svh - 400px) * 3.2));
    margin-top: 16px;
  }
}

@media (max-width: 360px) {
  .hero-description {
    font-size: 12px;
  }

  .hero-meta {
    display: none;
  }

  .hero-actions .button {
    padding-inline: 9px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-faq-item summary svg {
    transition: none;
  }
}
</style>
