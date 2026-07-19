<script setup lang="ts">
import { Menu, X } from "lucide-vue-next";
import { ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";

const route = useRoute();
const menuOpen = ref(false);

const navItems = [
  { label: "首页", to: "/" },
  { label: "下载", to: "/download" },
  { label: "文档", to: "/docs" },
  { label: "更新", to: "/changelog" },
  { label: "FAQ", to: "/faq" },
  { label: "安全", to: "/security" },
  { label: "状态", to: "/status" },
];

watch(
  () => route.fullPath,
  () => {
    menuOpen.value = false;
  },
);
</script>

<template>
  <header class="site-header">
    <div class="header-inner">
      <RouterLink class="brand" to="/" aria-label="Amber 首页">
        <img src="/app-icon.png" alt="" width="34" height="34" />
        <span>Amber</span>
      </RouterLink>

      <button
        class="icon-button menu-toggle"
        type="button"
        :aria-expanded="menuOpen"
        aria-controls="primary-navigation"
        :aria-label="menuOpen ? '关闭导航' : '打开导航'"
        @click="menuOpen = !menuOpen"
      >
        <X v-if="menuOpen" :size="20" aria-hidden="true" />
        <Menu v-else :size="20" aria-hidden="true" />
      </button>

      <nav id="primary-navigation" class="primary-nav" :class="{ open: menuOpen }" aria-label="主导航">
        <RouterLink v-for="item in navItems" :key="item.to" :to="item.to">
          {{ item.label }}
        </RouterLink>
      </nav>
    </div>
  </header>
</template>
