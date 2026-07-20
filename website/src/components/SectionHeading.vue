<script setup lang="ts">
withDefaults(
  defineProps<{
    eyebrow?: string;
    title: string;
    description?: string;
    align?: "left" | "center";
    headingId?: string;
  }>(),
  {
    eyebrow: "",
    description: "",
    align: "left",
    headingId: undefined,
  },
);
</script>

<template>
  <header class="section-heading" :class="`section-heading-${align}`">
    <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
    <h2 :id="headingId">{{ title }}</h2>
    <p v-if="description" class="section-heading-description">{{ description }}</p>
    <slot />
  </header>
</template>

<style scoped>
.section-heading {
  max-width: 760px;
  margin-bottom: 42px;
}

.section-heading h2 {
  margin-bottom: 12px;
}

.section-heading-description {
  margin-bottom: 0;
  color: var(--ink-soft);
  font-size: 18px;
  line-height: 1.7;
}

.section-heading-center {
  margin-inline: auto;
  text-align: center;
}

@media (max-width: 640px) {
  .section-heading {
    margin-bottom: 30px;
  }

  .section-heading-description {
    font-size: 16px;
  }
}
</style>
