<script setup lang="ts">
import { ArrowRight, Download, Laptop, Share2, UsersRound } from "lucide-vue-next";
import { RouterLink } from "vue-router";
import { stableRelease } from "../config/releases";

const highlights = [
  {
    icon: UsersRound,
    title: "多账号调度",
    text: "额度感知与自动切换",
  },
  {
    icon: Share2,
    title: "受控共享",
    text: "独立密钥、限流与额度",
  },
  {
    icon: Laptop,
    title: "一键注入",
    text: "连接本机与远程 Codex",
  },
];
</script>

<template>
  <section class="home-hero" aria-labelledby="home-title">
    <div class="container hero-inner">
      <div class="hero-copy">
        <h1 id="home-title">Amber</h1>
        <p class="hero-lede">让多个账号，成为可调度、可共享的 Codex 额度池。</p>
        <p class="hero-description">基于 sub2api 网关能力，借鉴 CCSwitch 使用体验，支持代理配置、加密云同步，以及 Codex 本地/远程一键注入。</p>

        <div class="hero-actions">
          <a class="button button-primary hero-download" :href="stableRelease.downloadUrl">
            <Download :size="20" aria-hidden="true" />
            下载最新版 v{{ stableRelease.version }}
          </a>
          <RouterLink class="hero-doc-link" to="/docs">
            查看使用文档
            <ArrowRight :size="17" aria-hidden="true" />
          </RouterLink>
        </div>

        <p class="hero-meta">Windows 10 / 11 · x64</p>
      </div>

      <picture class="hero-cover">
        <source media="(max-width: 640px)" srcset="/hero-cover-mobile.png" />
        <img
          class="hero-cover-image"
          src="/hero-cover.png"
          alt="Amber 产品封面，三条线路汇入本地网关并统一输出"
          width="2880"
          height="1280"
          fetchpriority="high"
          decoding="async"
        />
      </picture>
    </div>
  </section>

  <section class="home-highlights" aria-label="Amber 核心能力">
    <div class="container highlights-grid">
      <div v-for="item in highlights" :key="item.title" class="highlight-item">
        <component :is="item.icon" :size="20" aria-hidden="true" />
        <span>
          <strong>{{ item.title }}</strong>
          {{ item.text }}
        </span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.home-hero {
  border-bottom: 1px solid var(--border);
  background: var(--bg);
}

.hero-inner {
  display: grid;
  justify-items: center;
  padding-block: 48px 22px;
}

.hero-copy {
  width: min(100%, 760px);
  text-align: center;
}

.hero-copy h1 {
  margin-bottom: 10px;
  font-size: 64px;
}

.hero-lede {
  margin-bottom: 8px;
  color: var(--ink);
  font-size: 25px;
  font-weight: 680;
  line-height: 1.45;
}

.hero-description {
  max-width: 720px;
  margin: 0 auto 20px;
  color: var(--ink-soft);
  font-size: 14px;
  line-height: 1.7;
}

.hero-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 20px;
}

.hero-download {
  min-height: 54px;
  padding-inline: 24px;
  box-shadow: 0 12px 26px rgba(148, 59, 24, 0.18);
  font-size: 16px;
}

.hero-doc-link {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 7px;
  color: var(--ink-soft);
  font-size: 14px;
  font-weight: 680;
  text-decoration: none;
}

.hero-doc-link:hover {
  color: var(--ink);
  text-decoration: underline;
}

.hero-meta {
  margin: 13px 0 0;
  color: var(--ink-soft);
  font-size: 13px;
}

.hero-cover {
  display: block;
  width: 100%;
  height: clamp(250px, calc(100svh - 460px), 500px);
  margin-top: 30px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #f7f7f4;
}

.hero-cover-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
}

.home-highlights {
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.highlights-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.highlight-item {
  display: grid;
  min-width: 0;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 11px;
  padding: 22px 28px;
  border-left: 1px solid var(--border);
}

.highlight-item:last-child {
  border-right: 1px solid var(--border);
}

.highlight-item svg {
  margin-top: 2px;
  color: var(--teal);
}

.highlight-item span {
  color: var(--ink-soft);
  font-size: 13px;
}

.highlight-item strong {
  display: block;
  color: var(--ink);
  font-size: 14px;
}

@media (max-width: 760px) {
  .hero-inner {
    padding-top: 38px;
  }

  .hero-copy h1 {
    font-size: 52px;
  }

  .hero-lede {
    font-size: 22px;
  }

  .hero-cover {
    height: clamp(220px, calc(100svh - 460px), 350px);
  }

  .highlight-item {
    padding-inline: 18px;
  }
}

@media (max-width: 900px) {
  .hero-description {
    display: none;
  }
}

@media (max-width: 640px) {
  .hero-inner {
    padding-block: 32px 18px;
  }

  .hero-copy h1 {
    margin-bottom: 8px;
    font-size: 48px;
  }

  .hero-lede {
    margin-bottom: 20px;
    font-size: 20px;
    line-height: 1.45;
  }

  .hero-actions {
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }

  .hero-download {
    width: 100%;
  }

  .hero-doc-link {
    justify-content: center;
  }

  .hero-cover {
    margin-top: 24px;
  }

  .highlights-grid {
    grid-template-columns: 1fr;
  }

  .highlight-item,
  .highlight-item:last-child {
    padding-block: 15px;
    border-right: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  .highlight-item:last-child {
    border-bottom: 0;
  }
}

@media (max-width: 360px) {
  .hero-meta {
    display: none;
  }

  .hero-inner {
    padding-top: 25px;
  }

  .hero-cover {
    height: 180px;
    margin-top: 18px;
  }
}
</style>
