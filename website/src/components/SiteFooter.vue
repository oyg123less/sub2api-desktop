<script setup lang="ts">
import { ExternalLink, Github } from "lucide-vue-next";
import { RouterLink } from "vue-router";
import { stableRelease } from "../config/releases";
</script>

<template>
  <footer class="site-footer">
    <div class="container footer-grid">
      <div class="footer-brand">
        <RouterLink class="brand" to="/">
          <img src="/amber-mark.svg" alt="" width="32" height="32" />
          <span>Amber</span>
        </RouterLink>
        <p>把多个账号变成可调度、可共享的 Windows Codex 网关。</p>
      </div>

      <nav aria-label="页脚导航">
        <RouterLink to="/docs">使用文档</RouterLink>
        <RouterLink to="/download">下载 v{{ stableRelease.version }}</RouterLink>
        <RouterLink to="/changelog">更新日志</RouterLink>
        <RouterLink to="/faq">常见问题</RouterLink>
        <RouterLink to="/security">安全与隐私</RouterLink>
        <RouterLink to="/status">服务状态</RouterLink>
      </nav>

      <div class="footer-external">
        <a href="https://github.com/oyg123less/sub2api-desktop" target="_blank" rel="noreferrer">
          <Github :size="17" aria-hidden="true" />
          GitHub
          <ExternalLink :size="14" aria-hidden="true" />
        </a>
        <a href="https://github.com/oyg123less/sub2api-desktop/blob/main/LICENSE" target="_blank" rel="noreferrer">
          项目许可
          <ExternalLink :size="14" aria-hidden="true" />
        </a>
      </div>
    </div>

    <div class="container footer-legal">
      <p>Amber 是独立开源项目，与 OpenAI、sub2api、CCSwitch 不存在官方隶属或背书关系。</p>
      <p>请遵守上游服务条款，不要用于未经授权的共享或公开转售。</p>
    </div>
  </footer>
</template>
