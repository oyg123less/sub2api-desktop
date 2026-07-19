<script setup lang="ts">
import {
  Activity,
  CircleDashed,
  Cloud,
  ExternalLink,
  Info,
  LogIn,
  Mail,
  RadioTower,
} from "lucide-vue-next";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";

const services = [
  {
    name: "Amber Cloud API",
    description: "云账号、同步与共享所使用的公开服务入口。",
    icon: Cloud,
  },
  {
    name: "登录与注册",
    description: "邮箱注册、登录、会话刷新与退出流程。",
    icon: LogIn,
  },
  {
    name: "验证码邮件",
    description: "国内邮箱与其他邮箱的验证码投递链路。",
    icon: Mail,
  },
  {
    name: "Owner Relay",
    description: "共享账号请求返回提供方设备的 WebSocket 通道。",
    icon: RadioTower,
  },
];
</script>

<template>
  <PageIntro
    eyebrow="服务状态"
    title="Amber 服务状态"
    description="公开监控尚未接入。此页面目前不提供实时可用性结论，也不会把未知状态显示为正常运行。"
  />

  <section class="section" aria-labelledby="overview-heading">
    <div class="container status-layout">
      <div class="status-summary">
        <div class="summary-icon" aria-hidden="true">
          <Activity :size="24" />
        </div>
        <div>
          <span class="status-pill neutral">
            <CircleDashed :size="14" aria-hidden="true" />
            公开监控尚未接入
          </span>
          <h2 id="overview-heading">实时状态未知</h2>
          <p>
            下列项目是计划公开监控的服务范围。当前页面没有连接生产监控或状态 API，因此不能据此判断服务在线或离线。
          </p>
        </div>
      </div>

      <div class="service-list" aria-label="计划监控的服务">
        <article v-for="service in services" :key="service.name" class="service-row">
          <div class="service-icon" aria-hidden="true">
            <component :is="service.icon" :size="21" />
          </div>
          <div class="service-copy">
            <h3>{{ service.name }}</h3>
            <p>{{ service.description }}</p>
          </div>
          <span class="service-state">
            <CircleDashed :size="15" aria-hidden="true" />
            未连接公开监控
          </span>
        </article>
      </div>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="events-heading">
    <div class="container events-layout">
      <div>
        <p class="eyebrow">事件与维护</p>
        <h2 id="events-heading">暂无公开状态数据源</h2>
        <p class="events-copy">
          在只读状态接口上线前，本页无法确认当前事件或维护安排。版本发布信息以 GitHub Release 为准，使用问题可先查看常见问题。
        </p>
      </div>

      <div class="status-boundary">
        <Info :size="21" aria-hidden="true" />
        <div>
          <h3>数据边界</h3>
          <dl>
            <div>
              <dt>数据来源</dt>
              <dd>尚无公开只读状态源</dd>
            </div>
            <div>
              <dt>自动刷新</dt>
              <dd>未启用</dd>
            </div>
            <div>
              <dt>生产接口请求</dt>
              <dd>无</dd>
            </div>
          </dl>
        </div>
      </div>

      <div class="action-row">
        <RouterLink class="button button-primary" to="/faq">查看常见问题</RouterLink>
        <a
          class="button button-secondary"
          href="https://github.com/oyg123less/sub2api-desktop/releases"
        >
          GitHub Releases
          <ExternalLink :size="16" aria-hidden="true" />
        </a>
      </div>
    </div>
  </section>
</template>

<style scoped>
.status-layout {
  display: grid;
  gap: 42px;
}

.status-summary {
  display: grid;
  grid-template-columns: 52px minmax(0, 1fr);
  gap: 20px;
  max-width: 900px;
}

.summary-icon,
.service-icon {
  display: grid;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface-muted);
  color: var(--ink-soft);
}

.summary-icon {
  width: 52px;
  height: 52px;
}

.status-pill.neutral {
  background: var(--surface-muted);
  color: var(--ink-soft);
}

.status-summary h2 {
  margin: 14px 0 10px;
}

.status-summary p {
  margin: 0;
  color: var(--ink-soft);
  font-size: 17px;
}

.service-list {
  border-top: 1px solid var(--border);
}

.service-row {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) minmax(180px, auto);
  gap: 18px;
  align-items: center;
  padding-block: 22px;
  border-bottom: 1px solid var(--border);
}

.service-icon {
  width: 44px;
  height: 44px;
}

.service-copy h3 {
  margin-bottom: 4px;
}

.service-copy p {
  margin: 0;
  color: var(--ink-soft);
}

.service-state {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
}

.events-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(320px, 0.9fr);
  gap: 30px 64px;
  align-items: start;
}

.events-copy {
  max-width: 670px;
  margin-bottom: 0;
  color: var(--ink-soft);
  font-size: 17px;
}

.status-boundary {
  display: grid;
  grid-template-columns: 25px minmax(0, 1fr);
  gap: 14px;
  padding: 23px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
}

.status-boundary > svg {
  margin-top: 2px;
  color: var(--teal);
}

.status-boundary h3 {
  margin-bottom: 13px;
}

.status-boundary dl {
  margin: 0;
}

.status-boundary dl > div {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 16px;
  padding-block: 9px;
  border-top: 1px solid var(--border);
}

.status-boundary dt {
  color: var(--ink-soft);
  font-size: 13px;
}

.status-boundary dd {
  margin: 0;
  font-size: 14px;
  font-weight: 680;
}

.events-layout > .action-row {
  grid-column: 1 / -1;
}

@media (max-width: 780px) {
  .events-layout {
    grid-template-columns: 1fr;
  }

  .service-row {
    grid-template-columns: 44px minmax(0, 1fr);
  }

  .service-state {
    grid-column: 2;
    justify-content: flex-start;
  }
}

@media (max-width: 480px) {
  .status-summary {
    grid-template-columns: 1fr;
  }

  .service-row {
    gap: 14px;
  }

  .service-state {
    white-space: normal;
  }

  .status-boundary {
    padding: 19px;
  }

  .status-boundary dl > div {
    grid-template-columns: 1fr;
    gap: 2px;
  }

  .events-layout .button {
    width: 100%;
  }
}
</style>
