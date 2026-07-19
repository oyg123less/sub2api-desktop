<script setup lang="ts">
defineProps<{
  eyebrow?: string;
  title: string;
  description: string;
}>();
</script>

<template>
  <header class="page-intro">
    <div class="container page-intro-inner">
      <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
      <h1>{{ title }}</h1>
      <p class="page-lede">{{ description }}</p>
      <slot />
    </div>
  </header>
</template>
