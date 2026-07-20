<script setup lang="ts">
import { ArrowRight, CheckCircle2 } from "lucide-vue-next";
import { computed, ref } from "vue";
import { RouterLink } from "vue-router";
import { showcaseItems } from "../data/home";
import ProductScreenshot from "./ProductScreenshot.vue";

const activeIndex = ref(0);
const tabButtons = ref<HTMLButtonElement[]>([]);
const activeItem = computed(() => showcaseItems[activeIndex.value]);

function registerTab(element: unknown, index: number) {
  if (element instanceof HTMLButtonElement) tabButtons.value[index] = element;
}

function selectTab(index: number, moveFocus = false) {
  activeIndex.value = (index + showcaseItems.length) % showcaseItems.length;
  if (moveFocus) tabButtons.value[activeIndex.value]?.focus();
}

function onTabKeydown(event: KeyboardEvent, index: number) {
  const keyTargets: Record<string, number> = {
    ArrowRight: index + 1,
    ArrowDown: index + 1,
    ArrowLeft: index - 1,
    ArrowUp: index - 1,
    Home: 0,
    End: showcaseItems.length - 1,
  };
  if (!(event.key in keyTargets)) return;
  event.preventDefault();
  selectTab(keyTargets[event.key], true);
}
</script>

<template>
  <div class="product-showcase">
    <div class="showcase-tabs" role="tablist" aria-label="Amber 产品演示">
      <button
        v-for="(item, index) in showcaseItems"
        :id="`showcase-tab-${item.id}`"
        :key="item.id"
        :ref="(element) => registerTab(element, index)"
        type="button"
        role="tab"
        :aria-selected="activeIndex === index"
        :aria-controls="`showcase-panel-${item.id}`"
        :tabindex="activeIndex === index ? 0 : -1"
        @click="selectTab(index)"
        @keydown="onTabKeydown($event, index)"
      >
        <span class="tab-index">0{{ index + 1 }}</span>
        {{ item.label }}
      </button>
    </div>

    <div class="showcase-layout">
      <Transition name="showcase-copy" mode="out-in">
        <div :key="activeItem.id" class="showcase-copy">
          <p class="eyebrow">{{ activeItem.label }}</p>
          <h3>{{ activeItem.title }}</h3>
          <p class="showcase-description">{{ activeItem.description }}</p>
          <ul>
            <li v-for="point in activeItem.points" :key="point">
              <CheckCircle2 :size="18" aria-hidden="true" />
              <span>{{ point }}</span>
            </li>
          </ul>
          <RouterLink class="showcase-link" :to="activeItem.docsLink">
            查看对应文档
            <ArrowRight class="card-arrow" :size="17" aria-hidden="true" />
          </RouterLink>
          <p class="showcase-caption">{{ activeItem.caption }}</p>
        </div>
      </Transition>

      <div class="showcase-stage" data-testid="showcase-stage">
        <div
          v-for="(item, index) in showcaseItems"
          :id="`showcase-panel-${item.id}`"
          :key="item.id"
          class="showcase-panel"
          :class="{ 'is-active': activeIndex === index }"
          role="tabpanel"
          :aria-labelledby="`showcase-tab-${item.id}`"
          :aria-hidden="activeIndex !== index"
          :inert="activeIndex !== index"
        >
          <ProductScreenshot :src="item.image" :mobile-src="item.mobileImage" :alt="item.alt" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.product-showcase {
  min-width: 0;
}

.showcase-tabs {
  display: flex;
  width: fit-content;
  max-width: 100%;
  margin-bottom: 30px;
  padding: 4px;
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-card);
  background: var(--surface-muted);
  scrollbar-width: thin;
}

.showcase-tabs button {
  display: inline-flex;
  min-width: max-content;
  min-height: 42px;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border: 0;
  border-radius: var(--radius-control);
  background: transparent;
  color: var(--ink-soft);
  font-size: 14px;
  font-weight: 700;
  cursor: pointer;
  transition:
    background-color var(--motion-fast) ease,
    color var(--motion-fast) ease,
    box-shadow var(--motion-fast) ease;
}

.showcase-tabs button[aria-selected="true"] {
  background: var(--surface);
  color: var(--amber-dark);
  box-shadow: var(--shadow-xs);
}

.tab-index {
  color: var(--ink-faint);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 11px;
}

.showcase-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.34fr) minmax(0, 1fr);
  gap: 56px;
  align-items: center;
}

.showcase-copy {
  min-width: 0;
}

.showcase-copy h3 {
  margin-bottom: 14px;
  font-size: 30px;
  line-height: 1.25;
}

.showcase-description {
  margin-bottom: 22px;
  color: var(--ink-soft);
  font-size: 17px;
}

.showcase-copy ul {
  display: grid;
  gap: 11px;
  margin: 0 0 24px;
  padding: 0;
  list-style: none;
}

.showcase-copy li {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  gap: 9px;
  color: var(--ink);
  font-size: 14px;
}

.showcase-copy li svg {
  margin-top: 3px;
  color: var(--green);
}

.showcase-link {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--amber-dark);
  font-weight: 740;
  text-decoration: none;
}

.showcase-link:hover {
  color: var(--amber);
}

.showcase-link svg {
  transition: transform var(--motion-normal) var(--ease-out);
}

.showcase-link:hover svg {
  transform: translateX(4px);
}

.showcase-caption {
  margin: 16px 0 0;
  color: var(--ink-faint);
  font-size: 12px;
  line-height: 1.55;
}

.showcase-stage {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 10;
  min-width: 0;
}

.showcase-panel {
  position: absolute;
  inset: 0;
  visibility: hidden;
  opacity: 0;
  pointer-events: none;
  transform: translateY(5px);
  transition:
    opacity 220ms ease,
    transform 220ms var(--ease-out),
    visibility 0s linear 220ms;
}

.showcase-panel.is-active {
  visibility: visible;
  opacity: 1;
  pointer-events: auto;
  transform: translateY(0);
  transition-delay: 0s;
}

.showcase-copy-enter-active,
.showcase-copy-leave-active {
  transition:
    opacity 220ms ease,
    transform 220ms var(--ease-out);
}

.showcase-copy-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.showcase-copy-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (max-width: 900px) {
  .showcase-layout {
    grid-template-columns: 1fr;
    gap: 30px;
  }

  .showcase-copy {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(240px, 0.8fr);
    gap: 0 30px;
  }

  .showcase-copy .eyebrow,
  .showcase-copy h3,
  .showcase-description {
    grid-column: 1;
  }

  .showcase-copy ul {
    grid-column: 2;
    grid-row: 1 / span 4;
    align-self: center;
  }

  .showcase-link,
  .showcase-caption {
    grid-column: 1;
  }
}

@media (max-width: 640px) {
  .showcase-tabs {
    width: calc(100% + 14px);
    margin-right: -14px;
    margin-bottom: 24px;
    border-right: 0;
    border-radius: var(--radius-card) 0 0 var(--radius-card);
  }

  .showcase-tabs button {
    padding-inline: 12px;
  }

  .showcase-layout {
    gap: 24px;
  }

  .showcase-copy {
    display: block;
  }

  .showcase-copy h3 {
    font-size: 24px;
  }

  .showcase-description {
    font-size: 15px;
  }

  .showcase-caption {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .showcase-panel,
  .showcase-copy-enter-active,
  .showcase-copy-leave-active {
    transition: none;
  }

  .showcase-panel {
    transform: none;
  }
}
</style>
