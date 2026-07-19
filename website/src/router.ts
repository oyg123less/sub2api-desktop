import type { RouterScrollBehavior, RouteRecordRaw } from "vue-router";
import { stableRelease, upcomingRelease } from "./config/releases";

declare module "vue-router" {
  interface RouteMeta {
    title: string;
    description: string;
  }
}

export const routes: RouteRecordRaw[] = [
  {
    path: "/",
    name: "home",
    component: () => import("./pages/HomePage.vue"),
    meta: {
      title: "Amber - Windows Codex 桌面网关与多账号调度工具",
      description: "Amber 是独立开源的 Windows Codex 桌面网关，基于 sub2api 网关能力并借鉴 CCSwitch 使用体验，提供 OpenAI 兼容 API，支持多账号调度、代理配置、加密云同步、连接码受控共享及本地/远程一键注入。",
    },
  },
  {
    path: "/download",
    name: "download",
    component: () => import("./pages/DownloadPage.vue"),
    meta: {
      title: `下载 Amber v${stableRelease.version} - Windows x64`,
      description: "下载 Amber Windows x64 安装包，查看 SHA-256、覆盖安装与 SmartScreen 说明。",
    },
  },
  {
    path: "/docs",
    name: "docs",
    component: () => import("./pages/DocsPage.vue"),
    meta: {
      title: "Amber 使用文档",
      description: "从账号导入、代理配置和本地服务，到 Codex、SSH、云同步与共享的完整 Amber 使用文档。",
    },
  },
  {
    path: "/changelog",
    name: "changelog",
    component: () => import("./pages/ChangelogPage.vue"),
    meta: {
      title: "Amber 更新日志",
      description: `查看 Amber 当前稳定版、历史版本和 v${upcomingRelease.version} 即将发布内容。`,
    },
  },
  {
    path: "/faq",
    name: "faq",
    component: () => import("./pages/FaqPage.vue"),
    meta: {
      title: "Amber 常见问题",
      description: "排查 502、端口冲突、代理、SSH、云同步和多设备共享等 Amber 常见问题。",
    },
  },
  {
    path: "/security",
    name: "security",
    component: () => import("./pages/SecurityPage.vue"),
    meta: {
      title: "Amber 安全与隐私",
      description: "了解 Amber 本地数据、加密云同步、共享回流、日志脱敏与授权撤销边界。",
    },
  },
  {
    path: "/status",
    name: "status",
    component: () => import("./pages/StatusPage.vue"),
    meta: {
      title: "Amber 服务状态",
      description: "Amber Cloud API、登录注册、验证码邮件与 Owner Relay 的状态页外壳。",
    },
  },
  {
    path: "/404",
    name: "not-found-static",
    component: () => import("./pages/NotFoundPage.vue"),
    meta: {
      title: "页面未找到 - Amber",
      description: "请求的 Amber 页面不存在。",
    },
  },
  {
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: () => import("./pages/NotFoundPage.vue"),
    meta: {
      title: "页面未找到 - Amber",
      description: "请求的 Amber 页面不存在。",
    },
  },
];

export const scrollBehavior: RouterScrollBehavior = (to, _from, savedPosition) => {
  if (savedPosition) return savedPosition;
  if (to.hash) {
    const reduceMotion = typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    return { el: to.hash, top: 88, behavior: reduceMotion ? "auto" : "smooth" };
  }
  return { top: 0 };
};
