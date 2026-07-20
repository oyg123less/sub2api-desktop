<script setup lang="ts">
import { ArrowRight, ChevronDown, Search, SearchX, X } from "lucide-vue-next";
import { computed, nextTick, ref } from "vue";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";
import { stableRelease, upcomingRelease } from "../config/releases";
import { faqCategories, faqs, type FaqCategory } from "../data/faq";

const currentVersion = `v${stableRelease.version}`;
const nextVersion = `v${upcomingRelease.version}`;
const categories: readonly ("全部" | FaqCategory)[] = ["全部", ...faqCategories];

const query = ref("");
const activeCategory = ref<"全部" | FaqCategory>("全部");
const searchInput = ref<HTMLInputElement | null>(null);

async function clearSearch() {
  query.value = "";
  await nextTick();
  searchInput.value?.focus();
}

const filteredFaqs = computed(() => {
  const term = query.value.trim().toLocaleLowerCase();

  return faqs.filter((faq) => {
    if (activeCategory.value !== "全部" && faq.category !== activeCategory.value) return false;
    if (!term) return true;

    const searchable = [faq.question, faq.category, ...faq.paragraphs, ...(faq.steps ?? []), faq.note ?? "", ...faq.keywords]
      .join(" ")
      .toLocaleLowerCase();
    return searchable.includes(term);
  });
});
</script>

<template>
  <div class="faq-page">
    <PageIntro
      eyebrow="FAQ · 排障"
      title="常见问题与排障"
      description="先确认问题发生在哪一段链路，再修改配置。页面同时标明当前稳定版与即将发布行为，避免把计划中的修复当成现有功能。"
    >
      <div class="version-key" aria-label="版本说明">
        <span class="status-pill stable"><span class="status-dot" />{{ currentVersion }} 当前稳定版</span>
        <span class="status-pill upcoming"><span class="status-dot" />{{ nextVersion }} 即将发布</span>
      </div>
    </PageIntro>

    <section class="section-compact" aria-labelledby="faq-results-heading">
      <div class="container faq-layout">
        <div class="faq-controls" role="search" aria-label="筛选常见问题">
          <label class="sr-only" for="faq-search">搜索问题、错误码或关键词</label>
          <div class="search-field">
            <Search :size="19" aria-hidden="true" />
            <input id="faq-search" ref="searchInput" v-model="query" type="search" placeholder="搜索 502、端口、SSH、同步..." autocomplete="off" />
            <button v-if="query" type="button" aria-label="清除搜索" title="清除搜索" @click="clearSearch">
              <X :size="18" aria-hidden="true" />
            </button>
          </div>

          <label class="category-field">
            <span>问题分类</span>
            <select v-model="activeCategory">
              <option v-for="category in categories" :key="category" :value="category">{{ category }}</option>
            </select>
          </label>
        </div>

        <div class="faq-results-heading">
          <h2 id="faq-results-heading">排障答案</h2>
          <p aria-live="polite">找到 {{ filteredFaqs.length }} 个问题</p>
        </div>

        <div v-if="filteredFaqs.length" class="faq-list">
          <details v-for="faq in filteredFaqs" :key="faq.id" :id="faq.id" :open="faq.id === 'bad-gateway'">
            <summary>
              <span class="faq-summary-copy">
                <span class="faq-category">{{ faq.category }}</span>
                <span class="faq-question">{{ faq.question }}</span>
              </span>
              <span class="faq-summary-meta">
                <span v-if="faq.version === 'mixed'" class="status-pill warning">版本差异</span>
                <ChevronDown class="faq-chevron" :size="20" aria-hidden="true" />
              </span>
            </summary>
            <div class="faq-answer">
              <p v-for="paragraph in faq.paragraphs" :key="paragraph">{{ paragraph }}</p>
              <ol v-if="faq.steps" class="answer-steps">
                <li v-for="step in faq.steps" :key="step">{{ step }}</li>
              </ol>
              <p v-if="faq.note" class="answer-note"><strong>注意：</strong>{{ faq.note }}</p>
            </div>
          </details>
        </div>

        <div v-else class="empty-state">
          <SearchX :size="30" aria-hidden="true" />
          <h3>没有匹配的问题</h3>
          <p>换一个关键词，或将分类切回“全部”。</p>
        </div>
      </div>
    </section>

    <section class="section-compact section-muted" aria-labelledby="faq-next-heading">
      <div class="container faq-next">
        <div>
          <h2 id="faq-next-heading">还需要上下文？</h2>
          <p>使用文档提供完整操作路径；安全页说明本地、云端与共享链路分别处理哪些数据。</p>
        </div>
        <div class="action-row">
          <RouterLink class="button button-secondary" to="/docs">查看使用文档 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
          <RouterLink class="button button-secondary" to="/security">查看安全边界 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.version-key {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
  margin-top: 24px;
}

.faq-layout {
  max-width: 980px;
}

.faq-controls {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 230px;
  gap: 14px;
  margin-bottom: 42px;
}

.search-field {
  position: relative;
  display: flex;
  min-width: 0;
  height: 48px;
  align-items: center;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  background: var(--surface);
  color: var(--ink-soft);
}

.search-field:focus-within,
.category-field select:focus-visible {
  border-color: var(--teal);
  outline: 3px solid var(--teal);
  outline-offset: 3px;
  box-shadow: none;
}

.search-field > svg {
  flex: 0 0 auto;
  margin-left: 15px;
}

.search-field input {
  width: 100%;
  min-width: 0;
  height: 100%;
  padding: 0 12px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ink);
}

.search-field input::-webkit-search-cancel-button {
  display: none;
}

.search-field button {
  display: grid;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  margin-right: 4px;
  place-items: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--ink-soft);
  cursor: pointer;
}

.search-field button:hover {
  background: var(--surface-muted);
  color: var(--ink);
}

.category-field {
  display: grid;
  gap: 5px;
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.category-field select {
  width: 100%;
  height: 48px;
  padding: 0 38px 0 12px;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  outline: 0;
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
}

.faq-results-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.faq-results-heading h2,
.faq-results-heading p {
  margin-bottom: 0;
}

.faq-results-heading h2 {
  font-size: 28px;
}

.faq-results-heading p {
  color: var(--ink-soft);
  font-size: 14px;
}

.faq-list {
  border-block: 1px solid var(--border);
}

.faq-list details {
  overflow: clip;
  border-inline: 1px solid transparent;
  transform: none;
  transition:
    border-color 200ms ease,
    background-color 200ms ease;
  interpolate-size: allow-keywords;
}

.faq-list details + details {
  border-top: 1px solid var(--border);
}

.faq-list details[open] {
  background: var(--surface);
}

.faq-list details:focus-within {
  position: relative;
  z-index: 1;
  outline: 3px solid var(--teal);
  outline-offset: -3px;
}

@media (hover: hover) and (pointer: fine) {
  .faq-list details:hover,
  .faq-list details:focus-within {
    border-inline-color: var(--border-strong);
    background: var(--surface-muted);
  }
}

.faq-list details::details-content {
  height: 0;
  overflow: clip;
  opacity: 0;
  transition:
    height 200ms var(--ease-out),
    opacity 160ms ease,
    content-visibility 200ms allow-discrete;
  transition-behavior: allow-discrete;
}

.faq-list details[open]::details-content {
  height: auto;
  opacity: 1;
}

.faq-list summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 24px;
  padding: 23px 18px;
  align-items: center;
  cursor: pointer;
  list-style: none;
}

.faq-list summary::-webkit-details-marker {
  display: none;
}

.faq-summary-copy {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.faq-category {
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.faq-question {
  color: var(--ink);
  font-size: 18px;
  font-weight: 740;
  line-height: 1.45;
}

.faq-summary-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.faq-chevron {
  flex: 0 0 auto;
  color: var(--ink-soft);
  transition: transform 200ms var(--ease-out);
}

details[open] .faq-chevron {
  transform: rotate(180deg);
}

.faq-answer {
  max-width: 820px;
  padding: 0 76px 28px 18px;
  color: var(--ink-soft);
}

.faq-answer p {
  margin-bottom: 13px;
}

.answer-steps {
  margin: 18px 0 14px;
  padding-left: 22px;
  color: var(--ink);
}

.answer-steps li + li {
  margin-top: 5px;
}

.answer-note {
  padding: 12px 14px;
  border-left: 3px solid var(--warning);
  background: var(--warning-soft);
  color: var(--ink);
}

.empty-state {
  display: grid;
  min-height: 260px;
  place-items: center;
  align-content: center;
  border-block: 1px solid var(--border);
  color: var(--ink-soft);
  text-align: center;
}

.empty-state h3 {
  margin: 12px 0 5px;
}

.empty-state p {
  margin-bottom: 0;
}

.faq-next {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 36px;
}

.faq-next h2 {
  margin-bottom: 8px;
  font-size: 28px;
}

.faq-next p {
  max-width: 680px;
  margin-bottom: 0;
  color: var(--ink-soft);
}

.faq-next .action-row {
  flex: 0 0 auto;
}

@media (max-width: 760px) {
  .faq-controls {
    grid-template-columns: 1fr;
  }

  .faq-list summary {
    gap: 12px;
    padding-inline: 10px;
  }

  .faq-summary-meta .status-pill {
    display: none;
  }

  .faq-answer {
    padding-inline: 10px 42px;
  }

  .faq-next {
    display: grid;
  }

  .faq-next .action-row {
    align-items: stretch;
  }
}

@media (max-width: 480px) {
  .faq-results-heading {
    display: block;
  }

  .faq-results-heading p {
    margin-top: 6px;
  }

  .faq-question {
    font-size: 16px;
  }

  .faq-next .button {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .faq-list details,
  .faq-list details::details-content,
  .faq-chevron {
    transition: none;
  }
}
</style>
