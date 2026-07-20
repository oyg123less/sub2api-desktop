<script setup lang="ts">
import {
  ArrowRight,
  Cable,
  ChevronDown,
  Cloud,
  Download,
  Gauge,
  Github,
  KeyRound,
  Laptop,
  Network,
  Server,
  Share2,
  ShieldCheck,
  UsersRound,
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
import { capabilityItems, trustItems, useCases, workflowItems } from "../data/home";

const capabilityIcons: Record<string, Component> = {
  routing: Gauge,
  sharing: Share2,
  codex: Cable,
};

const scenarioIcons: Component[] = [UsersRound, Server, ShieldCheck];
</script>

<template>
  <section class="home-hero" aria-labelledby="home-title">
    <div class="hero-shell">
      <RevealOnScroll class="hero-copy">
        <span class="hero-version">
          <span class="status-dot" aria-hidden="true"></span>
          当前稳定版 v{{ stableRelease.version }}
        </span>
        <h1 id="home-title">Amber</h1>
        <p class="hero-lede">把多个账号变成可调度、可共享的 Codex 网关。</p>
        <p class="hero-description">在 Windows 上统一管理账号、代理、共享和本地/远程 Codex 接入。</p>

        <div class="hero-actions">
          <a class="button button-primary hero-download" :href="stableRelease.downloadUrl">
            <Download :size="19" aria-hidden="true" />
            下载 Windows
          </a>
          <RouterLink class="button button-secondary" to="/docs">
            查看文档
            <ArrowRight :size="17" aria-hidden="true" />
          </RouterLink>
        </div>

        <p class="hero-meta">Windows 10 / 11 · x64 · 开源</p>
      </RevealOnScroll>

      <RevealOnScroll class="hero-product" :delay="90">
        <ProductScreenshot
          src="/screenshots/v044/hero-cover-v044.png"
          mobile-src="/screenshots/v044/hero-cover-v044-mobile.png"
          alt="Amber v0.4.4 仪表盘真实界面封面"
          :width="1920"
          :height="600"
          loading="eager"
          fetch-priority="high"
        />
      </RevealOnScroll>
    </div>
  </section>

  <section class="home-trust" aria-label="Amber 产品信息">
    <div class="container trust-list">
      <span v-for="item in trustItems" :key="item">{{ item }}</span>
    </div>
  </section>

  <section class="section" aria-labelledby="capability-title">
    <div class="container">
      <SectionHeading
        eyebrow="核心能力"
        heading-id="capability-title"
        title="一个入口，接住账号、共享与 Codex"
        description="Amber 把高频操作放进同一套可观察流程，每项能力都有明确状态和退出路径。"
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
        eyebrow="产品演示"
        heading-id="showcase-title"
        title="四个视图，看清 Amber 如何工作"
        description="切换账号、网络、共享和 Codex 接入。以下均为 Amber v0.4.4 真实界面，敏感信息已做不可逆脱敏。"
      />
      <ProductShowcase />
    </div>
  </section>

  <section class="section" aria-labelledby="workflow-title">
    <div class="container">
      <SectionHeading
        eyebrow="三步工作流"
        heading-id="workflow-title"
        title="从导入到调用，不在工具之间来回切换"
        description="先确认账号与网络，再启动网关；共享是可选动作，不影响本地使用。"
      />

      <ol class="workflow-list">
        <RevealOnScroll
          v-for="(item, index) in workflowItems"
          :key="item.number"
          as="li"
          :delay="index * 70"
        >
          <span class="workflow-number">{{ item.number }}</span>
          <div class="workflow-copy">
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </div>
          <div class="workflow-image" aria-hidden="true">
            <img :src="item.image" alt="" width="2800" height="1750" loading="lazy" decoding="async" />
          </div>
        </RevealOnScroll>
      </ol>
    </div>
  </section>

  <section class="section feature-band network-band" aria-labelledby="network-title">
    <div class="container feature-layout">
      <RevealOnScroll class="feature-copy">
        <p class="eyebrow">账号、代理与网络出口</p>
        <h2 id="network-title">每个账号的网络模式都明确可见</h2>
        <p>直连不会意外继承代理；系统代理与指定代理分开测试。账号测试通过后，再交给调度器参与请求。</p>
        <div class="mode-list" aria-label="网络模式">
          <span><Network :size="17" aria-hidden="true" />direct</span>
          <span><Laptop :size="17" aria-hidden="true" />system</span>
          <span><KeyRound :size="17" aria-hidden="true" />proxy</span>
        </div>
        <RouterLink class="inline-action" to="/docs#proxies">
          配置账号与代理
          <ArrowRight :size="17" aria-hidden="true" />
        </RouterLink>
      </RevealOnScroll>

      <RevealOnScroll class="feature-media" :delay="80">
        <ProductScreenshot
          src="/screenshots/v044/network-v044.png"
          mobile-src="/screenshots/v044/network-v044-compact.png"
          alt="Amber v0.4.4 代理与网络出口真实界面"
        />
      </RevealOnScroll>
    </div>
  </section>

  <section class="section section-muted feature-band sharing-band" aria-labelledby="sharing-title">
    <div class="container feature-layout feature-layout-reverse">
      <RevealOnScroll class="feature-media">
        <ProductScreenshot
          src="/screenshots/v044/cloud-sharing-v044.png"
          mobile-src="/screenshots/v044/cloud-sharing-v044-compact.png"
          alt="Amber v0.4.4 连接码受控共享真实界面"
        />
      </RevealOnScroll>

      <RevealOnScroll class="feature-copy" :delay="80">
        <p class="eyebrow">云共享与远程 Codex</p>
        <h2 id="sharing-title">共享的是受控入口，不是裸露账号</h2>
        <p>连接码建立独立授权；需要本地回流的请求固定由指定设备承载。远程 Codex 则通过已验证的 SSH 反向隧道回到 Amber。</p>
        <div class="request-path" aria-label="远程 Codex 请求路径">
          <span>远程 Codex</span>
          <ArrowRight :size="16" aria-hidden="true" />
          <span>SSH 隧道</span>
          <ArrowRight :size="16" aria-hidden="true" />
          <span>Amber</span>
        </div>
        <div class="feature-actions">
          <RouterLink class="inline-action" to="/docs#sharing">查看共享说明 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
          <RouterLink class="inline-action" to="/docs#ssh-reverse">查看远程接入 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
        </div>
      </RevealOnScroll>
    </div>
  </section>

  <section class="section" aria-labelledby="scenarios-title">
    <div class="container">
      <SectionHeading
        eyebrow="适用场景"
        heading-id="scenarios-title"
        title="为重复调用和明确边界设计"
        description="围绕个人账号池、远程开发和固定范围共享，保持清晰的使用边界。"
      />
      <div class="scenario-grid">
        <RevealOnScroll v-for="(item, index) in useCases" :key="item.title" :delay="index * 70">
          <article
            class="scenario-card"
            :class="`scenario-card-${index === 1 ? 'teal' : index === 2 ? 'green' : 'amber'}`"
          >
            <span class="card-icon scenario-icon">
              <component :is="scenarioIcons[index]" :size="23" aria-hidden="true" />
            </span>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </article>
        </RevealOnScroll>
      </div>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="home-faq-title">
    <div class="container home-faq-layout">
      <SectionHeading
        eyebrow="常见问题"
        heading-id="home-faq-title"
        title="开始前，先把边界说清楚"
        description="这里保留五个最高频问题，完整排查路径仍在常见问题页。"
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
        <p>Windows 10 / 11 · x64 · 开源项目</p>
      </div>
      <div class="cta-actions">
        <a class="button button-dark" :href="stableRelease.downloadUrl">
          <Download :size="18" aria-hidden="true" />
          下载 Windows 版
        </a>
        <RouterLink class="button cta-secondary" to="/docs">查看使用文档</RouterLink>
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
  border-bottom: 1px solid var(--border);
  background: var(--bg);
}

.hero-shell {
  width: min(calc(100% - 40px), var(--hero-container));
  margin-inline: auto;
  padding-block: 38px 20px;
}

.hero-copy {
  width: min(100%, 850px);
  margin-inline: auto;
  text-align: center;
}

.hero-version {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  border: 1px solid rgba(36, 116, 75, 0.18);
  border-radius: 999px;
  background: var(--green-soft);
  color: var(--green);
  font-size: 12px;
  font-weight: 760;
}

.hero-copy h1 {
  margin: 10px 0 5px;
  font-size: 64px;
  line-height: 1.08;
}

.hero-lede {
  margin-bottom: 8px;
  color: var(--ink);
  font-size: 27px;
  font-weight: 720;
  line-height: 1.4;
}

.hero-description {
  margin-bottom: 20px;
  color: var(--ink-soft);
  font-size: 17px;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 10px;
}

.hero-actions .button {
  min-height: 50px;
  padding-inline: 20px;
}

.hero-meta {
  margin: 10px 0 0;
  color: var(--ink-faint);
  font-size: 12px;
}

.hero-product {
  position: relative;
  width: min(100%, 1120px);
  height: auto;
  aspect-ratio: 16 / 5;
  margin: 24px auto 0;
}

.hero-product :deep(.image-viewer),
.hero-product :deep(.image-button) {
  height: 100%;
}

.hero-product :deep(.image-button) {
  aspect-ratio: auto;
}

.hero-product :deep(.image-expand) {
  display: none;
}

.hero-product :deep(img) {
  object-position: center;
}

.home-trust {
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.trust-list {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.trust-list span {
  display: grid;
  min-height: 58px;
  place-items: center;
  padding: 12px;
  border-left: 1px solid var(--border);
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 700;
  text-align: center;
}

.trust-list span:last-child {
  border-right: 1px solid var(--border);
}

.capability-grid,
.scenario-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.capability-grid > *,
.scenario-grid > * {
  min-width: 0;
  height: 100%;
}

.capability-card,
.scenario-card {
  display: flex;
  height: 100%;
  flex-direction: column;
  padding: 28px;
}

.scenario-card {
  min-width: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface);
  box-shadow: var(--shadow-xs);
}

.scenario-card-amber {
  --card-accent: var(--amber);
  --card-accent-soft: var(--amber-soft);
}

.scenario-card-teal {
  --card-accent: var(--teal);
  --card-accent-soft: var(--teal-soft);
}

.scenario-card-green {
  --card-accent: var(--green);
  --card-accent-soft: var(--green-soft);
}

.capability-icon,
.scenario-icon {
  display: grid;
  width: 46px;
  height: 46px;
  margin-bottom: 22px;
  place-items: center;
  border-radius: var(--radius-control);
  background: var(--card-accent-soft);
  color: var(--card-accent);
}

.capability-card h3,
.scenario-card h3 {
  margin-bottom: 9px;
}

.capability-card p,
.scenario-card p {
  margin-bottom: 22px;
  color: var(--ink-soft);
}

.card-link,
.inline-action {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--amber-dark);
  font-size: 14px;
  font-weight: 740;
  text-decoration: none;
}

.card-link {
  margin-top: auto;
}

.card-link:hover,
.inline-action:hover {
  color: var(--amber);
}

.product-section {
  border-block: 1px solid var(--border);
}

.workflow-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 0;
  padding: 0;
  border-block: 1px solid var(--border);
  list-style: none;
}

.workflow-list > li {
  display: grid;
  min-width: 0;
  grid-template-columns: 42px minmax(0, 1fr);
  gap: 15px;
  padding: 28px;
  border-right: 1px solid var(--border);
}

.workflow-list > li:first-child {
  padding-left: 0;
}

.workflow-list > li:last-child {
  padding-right: 0;
  border-right: 0;
}

.workflow-number {
  color: var(--amber-dark);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 15px;
  font-weight: 800;
}

.workflow-copy h3 {
  margin-bottom: 7px;
}

.workflow-copy p {
  margin-bottom: 18px;
  color: var(--ink-soft);
  font-size: 14px;
}

.workflow-image {
  grid-column: 1 / -1;
  aspect-ratio: 16 / 7;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface-muted);
}

.workflow-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center 18%;
}

.network-band {
  border-block: 1px solid var(--border);
  background: var(--surface);
}

.feature-layout {
  display: grid;
  grid-template-columns: minmax(280px, 0.76fr) minmax(0, 1.34fr);
  gap: 72px;
  align-items: center;
}

.feature-layout-reverse {
  grid-template-columns: minmax(0, 1.34fr) minmax(280px, 0.76fr);
}

.feature-copy h2 {
  margin-bottom: 16px;
}

.feature-copy > p:not(.eyebrow) {
  margin-bottom: 24px;
  color: var(--ink-soft);
  font-size: 17px;
}

.feature-media {
  min-width: 0;
}

.mode-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 24px;
}

.mode-list span {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  gap: 7px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface-muted);
  color: var(--ink);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 12px;
}

.request-path {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 24px;
}

.request-path span {
  padding: 7px 9px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-control);
  background: var(--surface);
  color: var(--ink);
  font-size: 12px;
  font-weight: 720;
}

.request-path svg {
  color: var(--teal);
}

.feature-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 18px;
}

.scenario-card {
  min-height: 250px;
}

.scenario-card p {
  margin-bottom: 0;
}

.home-faq-layout > .section-heading {
  margin-bottom: 30px;
}

.home-faq-list {
  border-block: 1px solid var(--border);
}

.home-faq-item + .home-faq-item {
  border-top: 1px solid var(--border);
}

.home-faq-item[open] {
  background: var(--surface);
}

.home-faq-item summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 20px;
  align-items: center;
  padding: 20px 16px;
  color: var(--ink);
  font-size: 17px;
  font-weight: 740;
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
  background: var(--surface-emphasis);
  color: var(--amber-dark);
}

.home-faq-item summary svg {
  color: var(--ink-faint);
  transition: transform 200ms var(--ease-out);
}

.home-faq-item[open] summary svg {
  transform: rotate(180deg);
}

.home-faq-answer {
  max-width: 880px;
  padding: 0 52px 22px 16px;
  color: var(--ink-soft);
  font-size: 14px;
}

.home-faq-answer p:last-child {
  margin-bottom: 0;
}

.faq-more {
  margin-top: 28px;
}

.home-cta {
  background: #232724;
  color: #d5d9d4;
}

.cta-layout {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 48px;
}

.home-cta .eyebrow {
  color: #e7a07c;
}

.home-cta h2 {
  max-width: 720px;
  margin-bottom: 8px;
  color: #fff;
}

.home-cta p:last-child {
  margin-bottom: 0;
  color: #b9c0bb;
}

.cta-actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 9px;
}

.cta-secondary {
  border-color: #59605b;
  background: transparent;
  color: #fff;
}

.cta-secondary:hover {
  border-color: #858d87;
  background: #343936;
  color: #fff;
}

.cta-github {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 7px;
  padding-inline: 10px;
  color: #d5d9d4;
  font-size: 13px;
  font-weight: 700;
  text-decoration: none;
}

.cta-github:hover {
  color: #fff;
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

  .capability-grid,
  .scenario-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .capability-grid > :last-child,
  .scenario-grid > :last-child {
    grid-column: 1 / -1;
  }

  .workflow-list {
    grid-template-columns: 1fr;
  }

  .workflow-list > li,
  .workflow-list > li:first-child,
  .workflow-list > li:last-child {
    grid-template-columns: 42px minmax(180px, 0.65fr) minmax(220px, 1fr);
    align-items: center;
    padding: 22px 0;
    border-right: 0;
    border-bottom: 1px solid var(--border);
  }

  .workflow-list > li:last-child {
    border-bottom: 0;
  }

  .workflow-copy p {
    margin-bottom: 0;
  }

  .workflow-image {
    grid-column: 3;
    aspect-ratio: 16 / 6;
  }

  .feature-layout,
  .feature-layout-reverse {
    grid-template-columns: 1fr;
    gap: 42px;
  }

  .feature-layout-reverse .feature-media {
    order: 2;
  }

  .feature-layout-reverse .feature-copy {
    order: 1;
  }

  .cta-layout {
    align-items: flex-start;
    flex-direction: column;
  }

  .cta-actions {
    justify-content: flex-start;
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

  .hero-product :deep(img) {
    object-position: center;
  }

  .trust-list {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    padding-block: 9px;
  }

  .trust-list span,
  .trust-list span:last-child {
    min-height: 30px;
    padding: 5px 10px;
    border: 0;
    font-size: 11px;
  }

  .trust-list span + span::before {
    margin-right: 10px;
    color: var(--border-strong);
    content: "·";
  }

  .capability-grid,
  .scenario-grid {
    grid-template-columns: 1fr;
  }

  .capability-grid > :last-child,
  .scenario-grid > :last-child {
    grid-column: auto;
  }

  .capability-card,
  .scenario-card {
    min-height: 0;
    padding: 23px;
  }

  .workflow-list > li,
  .workflow-list > li:first-child,
  .workflow-list > li:last-child {
    grid-template-columns: 36px minmax(0, 1fr);
    padding-block: 20px;
  }

  .workflow-image {
    grid-column: 1 / -1;
    aspect-ratio: 16 / 7;
  }

  .feature-copy > p:not(.eyebrow) {
    font-size: 15px;
  }

  .request-path {
    gap: 6px;
  }

  .home-faq-item summary {
    padding-inline: 8px;
    font-size: 15px;
  }

  .home-faq-answer {
    padding-inline: 8px 34px;
  }

  .cta-actions,
  .cta-actions .button {
    width: 100%;
  }

  .cta-github {
    justify-content: center;
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

  .hero-actions .button svg {
    width: 16px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-faq-item summary svg {
    transition: none;
  }
}
</style>
