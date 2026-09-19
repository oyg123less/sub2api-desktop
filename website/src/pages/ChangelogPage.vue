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
    version: "v0.4.5",
    summary: "修复 Windows 覆盖安装竞态，统一本地与云端 Codex 真实客户端标识，并优化多账号故障切换兼容性。",
  },
  {
    version: "v0.4.4",
    summary: "完成同机多用户工作区隔离、设备定向共享、无好友前置连接码，以及可靠的 Codex 启动与注入闭环。",
  },
  {
    version: "v0.4.3",
    summary: "统一受管理共享账号在云账户、账号测试和 Codex 调用中的路由。",
  },
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
    description="先查看当前稳定版的已发布能力，再沿时间线回顾近期演进。尚未发布的计划单独列在页面末尾。"
  />

  <section class="section section-muted stable-release-section" aria-labelledby="stable-heading">
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
        v{{ stableRelease.version }} 深度打通 Codex 桌面版多模型生态与安全配置合并：实现动态模型探测自动写入 amber_models.json、多模型原生下拉切换、非破坏性 Safe Merge 保护用户所有自定义配置，并在账号切换与启动时全自动同步。
      </p>

      <div class="change-grid">
        <article class="change-group">
          <Plus :size="21" aria-hidden="true" />
          <div>
            <h3>新增</h3>
            <ul>
              <li>Codex 桌面版多模型自动感知：注入时自动探测已启用账号的所有模型并生成 <code>~/.codex/amber_models.json</code>，Codex 桌面端原生下拉框即可直接选择 Claude、OpenAI、Gemini、DeepSeek 等不同供应商模型。</li>
              <li>非破坏性 Safe Merge：深度改造 <code>config.toml</code> 生成机制，智能保留用户手写的 <code>[projects]</code>、<code>[windows]</code>、<code>[mcp_servers]</code>、<code>notify</code> 等全部自定义段落。</li>
              <li>全生命周期自动维护：账号切换、导入、启用/禁用及 Amber 启动时自动同步模型清单与配置，无感协同。</li>
            </ul>
          </div>
        </article>

        <article class="change-group">
          <Wrench :size="21" aria-hidden="true" />
          <div>
            <h3>修复</h3>
            <ul>
              <li>修复 Codex 桌面版仅能识别单个硬编码模型的问题，支持任意供应商与模型自由切换。</li>
              <li>修复每次覆盖写入 <code>config.toml</code> 导致用户自定义配置段被清除的问题。</li>
              <li>优化配置注入状态回显与多模型数量实时提示。</li>
            </ul>
          </div>
        </article>

        <article class="change-group">
          <Laptop :size="21" aria-hidden="true" />
          <div>
            <h3>兼容性</h3>
            <ul>
              <li>完全兼容 Codex CLI 与 Codex 桌面版最新架构，继续向后兼容 Amber 既有数据库。</li>
              <li>安装包提供推荐的 Windows x64 NSIS EXE 与 MSI 双格式，v0.4.4 / v0.4.5 均可直接覆盖升级。</li>
              <li>非 Codex 客户端继续使用 Amber 统一的高兼容性代理头与 OpenAI 兼容格式。</li>
            </ul>
          </div>
        </article>

        <article class="change-group limitations">
          <TriangleAlert :size="21" aria-hidden="true" />
          <div>
            <h3>已知限制</h3>
            <ul>
              <li>Codex 桌面版如已启动，切换模型或注入新配置后建议通过顶部菜单重新载入或重启 Codex 客户端以确保立即生效。</li>
              <li>需要设备回流的共享仍要求主设备或已配置的合格备用设备保持 Amber 在线。</li>
              <li>上游请求开始后不会跨设备重放；连接中断时可能返回结果未知。</li>
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

  <section class="section-compact section-muted future-release-section" aria-labelledby="upcoming-heading">
    <div class="container release-layout">
      <div class="release-heading">
        <div>
          <span class="status-pill upcoming">
            <CircleDashed :size="14" aria-hidden="true" />
            规划中
          </span>
          <h2 id="upcoming-heading">后续 v{{ upcomingRelease.version }}</h2>
        </div>
        <div class="release-meta">
          <CalendarClock :size="18" aria-hidden="true" />
          <span>发布日期待定</span>
        </div>
      </div>

      <div class="callout warning upcoming-note">
        <TriangleAlert :size="21" aria-hidden="true" />
        <div>
          <strong>以下内容尚未发布</strong>
          <p>目前没有安装包、校验值或 Release 页面，计划可能随实现与验证结果调整。</p>
        </div>
      </div>

      <div class="planned-list" :aria-label="`v${upcomingRelease.version} 计划内容`">
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>可靠性维护</h3>
            <p>继续改进同步、共享、网络诊断与本地网关的稳定性，并根据实际反馈修复问题。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>使用体验</h3>
            <p>继续减少高频操作步骤，改善状态反馈、错误说明与窄窗口下的可用性。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>兼容性验证</h3>
            <p>在保持数据和接口兼容的前提下推进后续改进，具体内容以完成验证后的发布说明为准。</p>
          </div>
        </article>
        <article>
          <CheckCircle2 :size="20" aria-hidden="true" />
          <div>
            <h3>文档与诊断</h3>
            <p>根据用户实际问题持续补全文档、诊断路径和可操作的恢复建议。</p>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.release-layout {
  display: grid;
  gap: 30px;
}

.stable-release-section .release-heading {
  padding-bottom: 24px;
  border-bottom: 1px solid var(--border-strong);
}

.future-release-section .release-layout {
  gap: 24px;
}

.future-release-section .release-heading h2 {
  font-size: 28px;
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
  position: relative;
  margin: 0;
  padding: 0 0 0 28px;
  list-style: none;
}

.history-list::before {
  position: absolute;
  top: 8px;
  bottom: 8px;
  left: 5px;
  width: 1px;
  background: var(--border-strong);
  content: "";
}

.history-list li {
  position: relative;
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 24px;
  padding-block: 17px;
  border-bottom: 1px solid var(--border);
}

.history-list li::before {
  position: absolute;
  top: 24px;
  left: -27px;
  width: 10px;
  height: 10px;
  border: 2px solid var(--surface);
  border-radius: 50%;
  background: var(--amber);
  box-shadow: 0 0 0 1px var(--border-strong);
  content: "";
}

.history-list li:last-child {
  border-bottom: 0;
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
