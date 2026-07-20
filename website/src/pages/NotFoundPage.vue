<script setup lang="ts">
import { BookOpen, Download, Home } from "lucide-vue-next";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";

const destinations = [
  {
    title: "返回首页",
    to: "/",
    icon: Home,
  },
  {
    title: "查看文档",
    to: "/docs",
    icon: BookOpen,
  },
  {
    title: "下载 Amber",
    to: "/download",
    icon: Download,
  },
];
</script>

<template>
  <PageIntro
    eyebrow="404"
    title="页面没有找到"
    description="这个地址可能已经更改、被移除，或者输入有误。请选择一个入口继续。"
  >
    <nav class="recovery-links" aria-label="页面恢复导航">
      <RouterLink
        v-for="(item, index) in destinations"
        :key="item.to"
        :class="['button', index === 0 ? 'button-primary' : 'button-secondary']"
        :to="item.to"
      >
        <component :is="item.icon" :size="18" aria-hidden="true" />
        {{ item.title }}
      </RouterLink>
    </nav>
  </PageIntro>
</template>

<style scoped>
.recovery-links {
  display: flex;
  flex-wrap: wrap;
  gap: 11px;
  margin-top: 28px;
}

@media (max-width: 480px) {
  .recovery-links {
    display: grid;
  }

  .recovery-links .button {
    width: 100%;
  }
}
</style>
