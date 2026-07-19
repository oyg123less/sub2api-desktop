<script setup lang="ts">
import { Expand, X } from "lucide-vue-next";
import { nextTick, onBeforeUnmount, ref } from "vue";

const props = defineProps<{
  src: string;
  alt: string;
  caption?: string;
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
  <figure class="image-viewer">
    <button type="button" class="image-button" :aria-label="`放大查看：${alt}`" @click="showImage">
      <img :src="src" :alt="alt" loading="lazy" decoding="async" />
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
      <button ref="closeButton" class="lightbox-close" type="button" aria-label="关闭图片预览" @click="closeImage">
        <X :size="22" aria-hidden="true" />
      </button>
      <img :src="src" :alt="alt" />
      <p v-if="caption">{{ caption }}</p>
    </div>
  </Teleport>
</template>
