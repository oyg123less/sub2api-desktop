<script setup lang="ts">
import { Download, Github, Menu, X } from "lucide-vue-next";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { stableRelease } from "../config/releases";

const route = useRoute();
const menuOpen = ref(false);
const menuToggle = ref<HTMLButtonElement | null>(null);

function closeMenu(restoreFocus = false) {
  if (!menuOpen.value) return;
  menuOpen.value = false;
  if (restoreFocus) menuToggle.value?.focus();
}

function trapMenuFocus(event: KeyboardEvent) {
  if (event.key !== "Tab" || !menuOpen.value) return;
  const navigation = document.getElementById("primary-navigation");
  const links = navigation ? Array.from(navigation.querySelectorAll<HTMLAnchorElement>("a[href]")) : [];
  const focusable = [menuToggle.value, ...links].filter(
    (element): element is HTMLButtonElement | HTMLAnchorElement => element !== null,
  );
  if (focusable.length === 0) return;

  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!first || !last) return;
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

function onWindowKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    closeMenu(true);
    return;
  }
  trapMenuFocus(event);
}

function onWindowResize() {
  if (window.innerWidth > 900) closeMenu();
}

onMounted(() => {
  window.addEventListener("keydown", onWindowKeydown);
  window.addEventListener("resize", onWindowResize);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onWindowKeydown);
  window.removeEventListener("resize", onWindowResize);
  document.body.classList.remove("menu-open");
  document.querySelectorAll(".skip-link, #main-content, .site-footer").forEach((element) => element.removeAttribute("inert"));
});

watch(
  () => route.fullPath,
  () => {
    menuOpen.value = false;
  },
);

watch(menuOpen, (open) => {
  document.body.classList.toggle("menu-open", open);
  document.querySelectorAll(".skip-link, #main-content, .site-footer").forEach((element) => {
    element.toggleAttribute("inert", open);
  });
});
</script>

<template>
  <header class="site-header">
    <div class="header-inner">
      <RouterLink class="brand" to="/" aria-label="Amber 首页" @click="closeMenu()">
        <img src="/amber-mark.svg" alt="" width="34" height="34" />
        <span>Amber</span>
      </RouterLink>

      <button
        ref="menuToggle"
        class="icon-button menu-toggle"
        type="button"
        :aria-expanded="menuOpen"
        aria-controls="primary-navigation"
        :aria-label="menuOpen ? '关闭导航' : '打开导航'"
        :title="menuOpen ? '关闭导航' : '打开导航'"
        @click="menuOpen = !menuOpen"
      >
        <X v-if="menuOpen" :size="20" aria-hidden="true" />
        <Menu v-else :size="20" aria-hidden="true" />
      </button>

      <nav id="primary-navigation" class="primary-nav" :class="{ open: menuOpen }" aria-label="主导航" @click="closeMenu()">
        <RouterLink class="mobile-only-nav" to="/">首页</RouterLink>
        <a href="/#product-showcase">产品</a>
        <RouterLink to="/docs">文档</RouterLink>
        <RouterLink to="/changelog">更新</RouterLink>
        <RouterLink class="mobile-only-nav" to="/faq">FAQ</RouterLink>
        <RouterLink class="mobile-only-nav" to="/security">安全</RouterLink>
        <RouterLink class="mobile-only-nav" to="/status">状态</RouterLink>
        <a href="https://github.com/oyg123less/sub2api-desktop" target="_blank" rel="noreferrer">
          <Github class="nav-icon" :size="16" aria-hidden="true" />
          GitHub
        </a>
        <RouterLink class="nav-download" to="/download">
          <Download class="nav-icon" :size="16" aria-hidden="true" />
          下载 v{{ stableRelease.version }}
        </RouterLink>
      </nav>
    </div>
  </header>
</template>
