<script setup lang="ts">
import { useHead } from "@unhead/vue";
import { computed, nextTick, ref, watch } from "vue";
import { RouterView, useRoute } from "vue-router";
import SiteFooter from "./components/SiteFooter.vue";
import SiteHeader from "./components/SiteHeader.vue";

const route = useRoute();
const canonicalUrl = computed(() => `https://amberapp.asia${route.path === "/" ? "/" : route.path}`);
const routeAnnouncement = ref("");

useHead({
  htmlAttrs: { lang: "zh-CN" },
  title: computed(() => route.meta.title),
  meta: [
    { name: "description", content: computed(() => route.meta.description) },
    { property: "og:title", content: computed(() => route.meta.title) },
    { property: "og:description", content: computed(() => route.meta.description) },
    { property: "og:url", content: canonicalUrl },
    { name: "twitter:title", content: computed(() => route.meta.title) },
    { name: "twitter:description", content: computed(() => route.meta.description) },
  ],
  link: [
    { rel: "canonical", href: canonicalUrl },
    { rel: "alternate", hreflang: "zh-CN", href: canonicalUrl },
    { rel: "alternate", hreflang: "x-default", href: canonicalUrl },
  ],
});

watch(
  () => route.path,
  async (_path, previousPath) => {
    if (previousPath === undefined) return;
    await nextTick();
    document.getElementById("main-content")?.focus({ preventScroll: true });
    routeAnnouncement.value = route.meta.title;
  },
);
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主要内容</a>
  <p class="sr-only" aria-live="polite">{{ routeAnnouncement }}</p>
  <SiteHeader />
  <main id="main-content" tabindex="-1">
    <RouterView />
  </main>
  <SiteFooter />
</template>
