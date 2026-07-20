<script setup lang="ts">
import ImageViewer from "./ImageViewer.vue";

withDefaults(
  defineProps<{
    src: string;
    mobileSrc?: string;
    alt: string;
    caption?: string;
    width?: number;
    height?: number;
    loading?: "eager" | "lazy";
    fetchPriority?: "high" | "low" | "auto";
  }>(),
  {
    caption: "",
    width: 1920,
    height: 1200,
    loading: "lazy",
    fetchPriority: "auto",
  },
);
</script>

<template>
  <ImageViewer
    :src="src"
    :mobile-src="mobileSrc"
    :alt="alt"
    :caption="caption"
    :loading="loading"
    :fetch-priority="fetchPriority"
    :width="width"
    :height="height"
    variant="product"
  />
</template>
