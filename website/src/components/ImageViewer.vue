<script setup lang="ts">
import { Expand, X } from "lucide-vue-next";
import { nextTick, onBeforeUnmount, ref } from "vue";

const props = defineProps<{
  src: string;
  mobileSrc?: string;
  alt: string;
  caption?: string;
  width?: number;
  height?: number;
  loading?: "eager" | "lazy";
  fetchPriority?: "high" | "low" | "auto";
  variant?: "default" | "product";
}>();

const open = ref(false);
const closeButton = ref<HTMLButtonElement | null>(null);
let previousFocus: HTMLElement | null = null;

async function showImage() {
  previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  open.value = true;
  document.body.classList.add("modal-open");
  await nextTick();
  closeButton.value?.focus();
}

function closeImage() {
  open.value = false;
  document.body.classList.remove("modal-open");
  previousFocus?.focus();
}

function onDialogKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") closeImage();
  if (event.key === "Tab") {
    event.preventDefault();
    closeButton.value?.focus();
  }
}

onBeforeUnmount(() => document.body.classList.remove("modal-open"));
</script>

<template>
  <figure class="image-viewer" :class="{ 'image-viewer-product': props.variant === 'product' }">
    <button type="button" class="image-button" :aria-label="`放大查看：${alt}`" :title="`放大查看：${alt}`" @click="showImage">
      <picture>
        <source v-if="mobileSrc" media="(max-width: 640px)" :srcset="mobileSrc" />
        <img
          :src="src"
          :alt="alt"
          :width="width"
          :height="height"
          :loading="loading ?? 'lazy'"
          :fetchpriority="fetchPriority ?? 'auto'"
          decoding="async"
        />
      </picture>
      <span class="image-expand" aria-hidden="true"><Expand :size="18" /></span>
    </button>
    <figcaption v-if="caption">{{ caption }}</figcaption>
  </figure>

  <Teleport to="body">
    <div
      v-if="open"
      class="lightbox"
      role="dialog"
      aria-modal="true"
      :aria-label="`图片预览：${props.alt}`"
      tabindex="-1"
      @keydown="onDialogKeydown"
      @click.self="closeImage"
    >
      <button ref="closeButton" class="lightbox-close" type="button" aria-label="关闭图片预览" title="关闭图片预览" @click="closeImage">
        <X :size="22" aria-hidden="true" />
      </button>
      <picture>
        <source v-if="mobileSrc" media="(max-width: 640px)" :srcset="mobileSrc" />
        <img :src="src" :alt="alt" />
      </picture>
      <p v-if="caption">{{ caption }}</p>
    </div>
  </Teleport>
</template>
