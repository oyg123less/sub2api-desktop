<script setup lang="ts">
import { BookOpen, CircleHelp, Download, Home, MoveRight } from "lucide-vue-next";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";

const destinations = [
  {
    title: "返回首页",
    description: "重新了解 Amber 的核心用途与当前稳定版本。",
    to: "/",
    icon: Home,
  },
  {
    title: "下载 Amber",
    description: "获取当前 Windows x64 稳定版与安装校验信息。",
    to: "/download",
    icon: Download,
  },
  {
    title: "查看文档",
    description: "查找安装、账号导入、Codex、SSH、云同步与共享说明。",
    to: "/docs",
    icon: BookOpen,
  },
  {
    title: "常见问题",
    description: "排查连接失败、502、代理、端口和共享设备离线等问题。",
    to: "/faq",
    icon: CircleHelp,
  },
];
</script>

<template>
  <PageIntro
    eyebrow="404"
    title="页面没有找到"
    description="这个地址可能已经更改、被移除，或者输入有误。请从下面的常用入口继续。"
  >
    <div class="action-row intro-actions">
      <RouterLink class="button button-primary" to="/">
        <Home :size="18" aria-hidden="true" />
        返回首页
      </RouterLink>
      <RouterLink class="button button-secondary" to="/docs">
        <BookOpen :size="18" aria-hidden="true" />
        查看文档
      </RouterLink>
    </div>
  </PageIntro>

  <section class="section" aria-labelledby="destinations-heading">
    <div class="container">
      <div class="section-header not-found-header">
        <div>
          <h2 id="destinations-heading">常用入口</h2>
          <p>选择最接近原来目标的页面。</p>
        </div>
      </div>

      <nav class="destination-grid" aria-label="页面恢复导航">
        <RouterLink v-for="item in destinations" :key="item.to" :to="item.to">
          <component :is="item.icon" :size="22" aria-hidden="true" />
          <div>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </div>
          <MoveRight class="destination-arrow" :size="19" aria-hidden="true" />
        </RouterLink>
      </nav>
    </div>
  </section>
</template>

<style scoped>
.intro-actions {
  margin-top: 28px;
}

.not-found-header {
  margin-bottom: 30px;
}

.destination-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  border: 1px solid var(--border);
  background: var(--border);
}

.destination-grid > a {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 22px;
  gap: 15px;
  align-items: start;
  min-width: 0;
  padding: 28px;
  background: var(--surface);
  color: var(--ink);
  text-decoration: none;
}

.destination-grid > a:hover {
  background: var(--surface-muted);
  color: var(--ink);
}

.destination-grid > a > svg:first-child {
  margin-top: 1px;
  color: var(--amber);
}

.destination-grid h3 {
  margin-bottom: 7px;
}

.destination-grid p {
  margin: 0;
  color: var(--ink-soft);
}

.destination-arrow {
  align-self: center;
  color: var(--ink-soft);
}

.destination-grid > a:hover .destination-arrow {
  color: var(--amber-dark);
}

@media (max-width: 720px) {
  .destination-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 480px) {
  .intro-actions .button {
    width: 100%;
  }

  .destination-grid > a {
    grid-template-columns: 25px minmax(0, 1fr);
    padding: 22px;
  }

  .destination-arrow {
    display: none;
  }
}
</style>
