<script setup lang="ts">
import {
  CalendarClock,
  CheckCircle2,
  CircleDashed,
  Laptop,
  Plus,
  TriangleAlert,
  Wrench,
} from "lucide-vue-next";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";
import {
  formatPublishedDate,
  stableRelease,
  upcomingRelease,
} from "../config/releases";

const history = [
  {
    version: "v0.4.2",
    summary: "加入共享码与临时密码快速共享，并修复 Owner Relay 连接恢复。",
  },
  {
    version: "v0.4.1",
    summary: "补充账号操作、用量与费用展示，改进云网络设置、文档和共享稳定性。",
  },
  {
    version: "v0.4.0",
    summary: "引入云账号、好友、多账号共享和 Owner Relay 初版。",
  },
  {
    version: "v0.3.3",
    summary: "让同步操作保持幂等，并收紧超时处理与推送可靠性。",
  },
  {
    version: "v0.3.2",
    summary: "改进账号面板、批量操作、并发队列和界面反馈。",
  },
  {
    version: "v0.3.1",
    summary: "建立云同步与共享的可靠性基础。",
  },
];
</script>

<template>
  <PageIntro
    eyebrow="版本记录"
    title="Amber 更新日志"
    description="查看当前稳定版、后续版本计划和近期版本演进。只有已经发布的版本才提供下载入口。"
  />

  <section class="section" aria-labelledby="upcoming-heading">
    <div class="container release-layout">
      <div class="release-heading">
        <div>
          <span class="status-pill upcoming">
            <CircleDashed :size="14" aria-hidden="true" />
            即将发布
          </span>
          <h2 id="upcoming-heading">v{{ upcomingRelease.version }}</h2>
        </div>
        <div class="release-meta">
          <CalendarClock :size="18" aria-hidden="true" />
          <span>发布日期待定</span>
        </div>
      </div>

      <div class="callout warning upcoming-note">
        <TriangleAlert :size="21" aria-hidden="true" />
        <div>
          <strong>此版本仍在规划与开发中</strong>
          <p>目前没有安装包、校验值或 Release 页面，以下内容可能随实现与验证结果调整。</p>
        </div>
      </div>

      <div class="planned-list" :aria-label="`v${upcomingRelease.version} 计划内容`">
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>工作区隔离</h3>
            <p>计划按云用户隔离本地账号、同步状态和运行数据，降低切换账号时的数据串用风险。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>设备定向共享</h3>
            <p>计划按目标账号选择实际持有该账号的在线设备，不再只依赖主设备优先。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>共享流程收敛</h3>
            <p>计划移除好友入口依赖，继续围绕共享码、临时密码和接收授权组织共享。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>可靠启动与注入</h3>
            <p>计划将启动本地服务与 Codex 注入合并为可验证的连续操作，并提供更明确的失败反馈。</p>
          </div>
        </article>
      </div>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="stable-heading">
    <div class="container release-layout">
      <div class="release-heading">
        <div>
          <span class="status-pill stable">
            <span class="status-dot" aria-hidden="true"></span>
            当前稳定版
          </span>
          <h2 id="stable-heading">v{{ stableRelease.version }}</h2>
        </div>
        <div class="release-meta">
          <CalendarClock :size="18" aria-hidden="true" />
          <time :datetime="stableRelease.publishedAt">
            {{ formatPublishedDate(stableRelease.publishedAt) }}
          </time>
        </div>
      </div>

      <p class="stable-summary">
        v{{ stableRelease.version }} 统一了受管理云共享账号在云账户、账号页面、连接测试和 Codex 调用中的路由与数据表现。
      </p>

      <div class="change-grid">
        <article class="change-group">
          <Plus :size="21" aria-hidden="true" />
          <div>
            <h3>新增</h3>
            <ul>
              <li>在云账户和账号工作流中统一呈现可用的受管理共享账号。</li>
              <li>补齐共享账号测试与 Codex 使用路径所需的上下文。</li>
            </ul>
          </div>
        </article>

        <article class="change-group">
          <Wrench :size="21" aria-hidden="true" />
          <div>
            <h3>修复</h3>
            <ul>
              <li>统一云账户、账号页、连接测试和 Codex 调用的共享路由。</li>
              <li>规范接收共享时的账号标识与模型输入，减少不同入口的结果差异。</li>
            </ul>
          </div>
        </article>

        <article class="change-group">
          <Laptop :size="21" aria-hidden="true" />
          <div>
            <h3>兼容性</h3>
            <ul>
              <li>当前安装包面向 Windows x64。</li>
              <li>继续支持 v0.4.2 引入的共享码与临时密码流程。</li>
              <li>当前界面仍保留好友功能；移除工作属于 v{{ upcomingRelease.version }} 计划。</li>
            </ul>
          </div>
        </article>

        <article class="change-group limitations">
          <TriangleAlert :size="21" aria-hidden="true" />
          <div>
            <h3>已知限制</h3>
            <ul>
              <li>Owner Relay 优先选择在线主设备，不能保证该设备持有目标账号。</li>
              <li>本地普通账号和同步队列尚未按云用户完整隔离。</li>
              <li>需要设备回流的共享要求提供方设备保持 Amber 在线。</li>
            </ul>
          </div>
        </article>
      </div>

      <div class="release-actions action-row">
        <RouterLink class="button button-primary" to="/download">
          下载 v{{ stableRelease.version }}
        </RouterLink>
        <a class="button button-secondary" :href="stableRelease.releaseUrl">
          查看 GitHub Release
        </a>
      </div>
    </div>
  </section>

  <section class="section" aria-labelledby="history-heading">
    <div class="container history-layout">
      <div class="history-intro">
        <p class="eyebrow">近期版本</p>
        <h2 id="history-heading">版本演进</h2>
        <p>这里保留近期主要变化的简要索引，完整提交记录可在 GitHub 仓库查看。</p>
      </div>

      <ol class="history-list">
        <li v-for="release in history" :key="release.version">
          <span class="history-version">{{ release.version }}</span>
          <p>{{ release.summary }}</p>
        </li>
      </ol>
    </div>
  </section>
</template>

<style scoped>
.release-layout {
  display: grid;
  gap: 30px;
}

.release-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 24px;
}

.release-heading h2 {
  margin: 13px 0 0;
}

.release-meta {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  color: var(--ink-soft);
  font-size: 14px;
  font-weight: 650;
}

.upcoming-note {
  max-width: 850px;
}

.planned-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-block: 1px solid var(--border);
}

.planned-list article {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 14px;
  padding: 25px 28px 25px 0;
  border-bottom: 1px solid var(--border);
}

.planned-list article:nth-child(odd) {
  border-right: 1px solid var(--border);
}

.planned-list article:nth-child(even) {
  padding-left: 28px;
}

.planned-list article:nth-last-child(-n + 2) {
  border-bottom: 0;
}

.planned-list svg {
  margin-top: 2px;
  color: var(--teal);
}

.planned-list h3,
.change-group h3 {
  margin-bottom: 7px;
}

.planned-list p,
.stable-summary {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.stable-summary {
  max-width: 840px;
  font-size: 18px;
}

.change-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  border: 1px solid var(--border);
  background: var(--border);
}

.change-group {
  display: grid;
  grid-template-columns: 25px minmax(0, 1fr);
  gap: 14px;
  padding: 27px;
  background: var(--surface);
}

.change-group > svg {
  margin-top: 2px;
  color: var(--amber);
}

.change-group.limitations > svg {
  color: var(--warning);
}

.change-group ul {
  margin: 0;
  padding-left: 20px;
  color: var(--ink-soft);
}

.change-group li + li {
  margin-top: 7px;
}

.release-actions {
  padding-top: 4px;
}

.history-layout {
  display: grid;
  grid-template-columns: minmax(220px, 0.7fr) minmax(0, 1.3fr);
  gap: 72px;
}

.history-intro {
  align-self: start;
}

.history-intro h2 {
  margin-bottom: 12px;
}

.history-intro > p:last-child {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.history-list {
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: 1px solid var(--border);
}

.history-list li {
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 24px;
  padding-block: 19px;
  border-bottom: 1px solid var(--border);
}

.history-version {
  color: var(--amber-dark);
  font-weight: 760;
}

.history-list p {
  margin: 0;
  color: var(--ink-soft);
}

@media (max-width: 760px) {
  .release-heading {
    align-items: start;
    flex-direction: column;
    gap: 14px;
  }

  .planned-list,
  .change-grid,
  .history-layout {
    grid-template-columns: 1fr;
  }

  .planned-list article,
  .planned-list article:nth-child(even) {
    padding: 22px 0;
    border-right: 0;
    border-bottom: 1px solid var(--border);
  }

  .planned-list article:nth-last-child(2) {
    border-bottom: 1px solid var(--border);
  }

  .planned-list article:last-child {
    border-bottom: 0;
  }

  .history-layout {
    gap: 34px;
  }
}

@media (max-width: 480px) {
  .history-list li {
    grid-template-columns: 1fr;
    gap: 4px;
  }

  .release-actions .button {
    width: 100%;
  }
}
</style>
