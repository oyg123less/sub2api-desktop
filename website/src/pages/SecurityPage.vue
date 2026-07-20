<script setup lang="ts">
import {
  ArrowRight,
  Ban,
  Cloud,
  Database,
  ExternalLink,
  EyeOff,
  HardDrive,
  KeyRound,
  Laptop,
  Route,
  ShieldCheck,
  Trash2,
  UserRoundX,
} from "lucide-vue-next";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";
import { stableRelease } from "../config/releases";

const currentVersion = `v${stableRelease.version}`;
</script>

<template>
  <div class="security-page">
    <PageIntro
      eyebrow="安全与隐私"
      title="知道数据在哪里，才能做出正确选择"
      description="Amber 的本地网关、可选云同步和共享回流采用不同的数据路径。这里说明每一段实际处理什么，以及用户仍需承担哪些保护责任。"
    >
      <div class="intro-points">
        <span class="status-pill stable"><span class="status-dot" />本地功能无需云账号</span>
        <span class="status-pill"><span class="status-dot" />官网不接收账号凭据</span>
      </div>
    </PageIntro>

    <section class="section" aria-labelledby="boundaries-heading">
      <div class="container">
        <div class="section-header">
          <div>
            <p class="eyebrow">Data boundaries</p>
            <h2 id="boundaries-heading">本地、云端与回流各有边界</h2>
            <p>云账号只在同步、多设备与共享功能中需要。单纯使用本地网关、代理、统计或 Codex 本机接入，不要求注册。</p>
          </div>
        </div>

        <div class="feature-grid boundary-grid">
          <article class="feature-item">
            <HardDrive :size="25" aria-hidden="true" />
            <h3>本地数据目录</h3>
            <p>账号、代理、设置、请求日志、Codex 与 SSH 目标默认保存在本机。敏感字段经过应用加密层保存，但本机数据目录、Windows 账号和主密码仍需由你保护。</p>
          </article>
          <article class="feature-item">
            <Cloud :size="25" aria-hidden="true" />
            <h3>Amber Cloud</h3>
            <p>云端处理登录会话、同步版本、共享授权和必要审计元数据。保险库内容以客户端生成的密文同步，服务端不持有保险库密钥，但仍会看到账号与服务运行所需的元数据。</p>
          </article>
          <article class="feature-item">
            <Route :size="25" aria-hidden="true" />
            <h3>共享与 Owner Relay</h3>
            <p>OAuth 共享通常由拥有者设备回流，上游 Token 留在拥有者本机。Worker 仍参与鉴权、额度、路由与请求转发；这条链路不能笼统描述为云端完全不可见。</p>
          </article>
        </div>

        <div class="callout boundary-callout">
          <ShieldCheck :size="21" aria-hidden="true" />
          <div>
            <strong>默认只监听本机</strong>
            <p>本地 API 默认使用 <code>127.0.0.1</code>。不要把本地 HTTP 服务端口直接暴露到公网；远程使用优先选择 SSH 隧道或可信的反向代理。</p>
          </div>
        </div>
      </div>
    </section>

    <section class="section section-muted" aria-labelledby="relay-heading">
      <div class="container">
        <div class="section-header">
          <div>
            <p class="eyebrow">Request path</p>
            <h2 id="relay-heading">共享请求经过哪些组件</h2>
            <p>具体路径取决于账号类型和共享模式。两种模式的凭据位置与在线要求不同。</p>
          </div>
        </div>

        <div class="relay-flow" aria-label="OAuth 设备回流路径">
          <span>接收者客户端</span>
          <ArrowRight :size="18" aria-hidden="true" />
          <span>Cloud Worker</span>
          <ArrowRight :size="18" aria-hidden="true" />
          <span>Owner Relay</span>
          <ArrowRight :size="18" aria-hidden="true" />
          <span>拥有者 Amber</span>
          <ArrowRight :size="18" aria-hidden="true" />
          <span>上游服务</span>
        </div>

        <div class="route-modes">
          <article>
            <Laptop :size="23" aria-hidden="true" />
            <div>
              <h3>OAuth 设备回流</h3>
              <p>OAuth Token 留在拥有者本机，最终上游请求从该设备发出。因此拥有者 Amber、Relay 和相关路由必须在线，且请求会使用拥有者设备的代理与网络出口。</p>
            </div>
          </article>
          <article>
            <KeyRound :size="23" aria-hidden="true" />
            <div>
              <h3>兼容 API Key 的 Worker 直连</h3>
              <p>只有共享者明确选择 Worker 直连时才使用。上游凭据以服务端密钥加密保存，但 Worker 转发请求时需要解密使用；因此它不属于“只有客户端可解密”的保险库同步边界。</p>
            </div>
          </article>
        </div>

        <p class="metadata-note">
          共享服务的用量记录设计为保存时间、模型、HTTP 状态、延迟、配额和审计等必要元数据，不保存请求与响应正文。网络服务仍会在转发过程中处理请求，不应把“不落日志”等同于“服务无法接触”。
        </p>
      </div>
    </section>

    <section class="section" aria-labelledby="redaction-heading">
      <div class="container">
        <div class="section-header">
          <div>
            <p class="eyebrow">Operational hygiene</p>
            <h2 id="redaction-heading">凭据、日志与截图必须先脱敏</h2>
            <p>诊断信息的价值不能以泄露账号和基础设施为代价。上传 Issue 或转发截图前，检查原文件本身。</p>
          </div>
        </div>

        <div class="practice-grid">
          <article>
            <EyeOff :size="24" aria-hidden="true" />
            <h3>不得公开的字段</h3>
            <ul>
              <li>完整邮箱、ChatGPT account ID、设备名称和真实服务器 IP。</li>
              <li>Access / Refresh Token、API Key、本地 API Key、Guest Key 和云主密码。</li>
              <li>代理用户名与密码、SSH 密码、私钥、管理员密钥和部署 Token。</li>
              <li>包含凭据、请求正文或完整上游错误内容的日志。</li>
            </ul>
          </article>
          <article>
            <Database :size="24" aria-hidden="true" />
            <h3>正确的脱敏方式</h3>
            <ul>
              <li>优先使用 Amber 导出的脱敏诊断摘要，只保留错误阶段、状态码、请求 ID 和时间。</li>
              <li>在生成图片前替换或裁掉敏感像素，并再次打开最终图片检查。</li>
              <li>不要只用网页 CSS 盖住字段；原图仍可被下载并查看。</li>
              <li>分享配置片段时同时检查 shell 历史、路径、环境变量和文件附件。</li>
            </ul>
          </article>
        </div>

        <div class="callout warning redaction-warning">
          <EyeOff :size="21" aria-hidden="true" />
          <div>
            <strong>连接码和临时密码也是凭据</strong>
            <p>只通过预期渠道发给接收者。若意外公开，应立即刷新连接信息或撤销已领取授权，而不是等待它自行失效。</p>
          </div>
        </div>
      </div>
    </section>

    <section class="section section-muted" aria-labelledby="control-heading">
      <div class="container control-grid">
        <article aria-labelledby="revoke-heading">
          <UserRoundX :size="26" aria-hidden="true" />
          <h2 id="revoke-heading">撤销共享与设备访问</h2>
          <ol class="control-steps">
            <li>在云账户的共享管理中定位对应共享或接收者。</li>
            <li>需要临时阻断时先暂停；确定不再授权时删除接收者或整个共享。</li>
            <li>怀疑 Guest Key 泄露时轮换密钥，并更新合法接收者的配置。</li>
            <li>设备遗失或会话异常时，注销相关会话，并在设备管理中撤销不再可信的设备。</li>
          </ol>
          <p>{{ currentVersion }} 的连接码共享不要求好友关系，并继续按接收者管理独立 Guest Key、额度、暂停与撤销。</p>
        </article>

        <article aria-labelledby="delete-heading">
          <Trash2 :size="26" aria-hidden="true" />
          <h2 id="delete-heading">删除本地工作区数据</h2>
          <p><strong>退出云账号不会删除本地数据。</strong>{{ currentVersion }} 按云用户使用独立工作区保存账号、代理、日志、设置和加密材料。</p>
          <ol class="control-steps">
            <li>先在设置中确认当前数据目录；如需保留内容，创建完整备份。</li>
            <li>停止客户端请求和 Amber 本地服务，退出 Amber，避免复制或删除正在使用的数据库。</li>
            <li>在工作区管理中确认归属和目标后再删除对应工作区。该操作会清除其中的全部本地数据，并且不可撤销。</li>
            <li>本地删除不会自动清除云端保险库、登录会话或共享授权；应先注销会话并撤销共享，再按云服务提供的能力处理云端数据。</li>
          </ol>
          <p>归属不明确的旧数据库会以只读恢复工作区打开；确认并导出需要的数据前，不要直接删除恢复工作区或迁移前备份。</p>
        </article>
      </div>
    </section>

    <section class="section section-dark" aria-labelledby="limits-heading">
      <div class="container limits-layout">
        <div>
          <p class="eyebrow">Limits</p>
          <h2 id="limits-heading">这些承诺不会出现在 Amber 文案中</h2>
          <p>安全说明应描述可验证的机制，而不是给出无法证明的绝对结论。</p>
        </div>
        <ul class="limit-list">
          <li><Ban :size="20" aria-hidden="true" /><span><strong>不承诺绝对匿名。</strong>上游、网络服务和共享参与方仍可能看到账号、IP、流量或请求相关信息。</span></li>
          <li><Ban :size="20" aria-hidden="true" /><span><strong>不把所有链路都称为端到端加密。</strong>保险库同步是客户端密文；登录、授权、路由和 Worker 直连有不同的服务端处理边界。</span></li>
          <li><Ban :size="20" aria-hidden="true" /><span><strong>不保证账号绝对安全。</strong>非官方转发、代理或兼容配置可能触发上游限制，用户仍需遵守相关服务条款。</span></li>
          <li><Ban :size="20" aria-hidden="true" /><span><strong>不声称“官方无法看到任何数据”。</strong>应以代码、部署配置和具体数据路径的审计结果为准。</span></li>
        </ul>
      </div>
    </section>

    <section class="section-compact" aria-labelledby="website-boundary-heading">
      <div class="container website-boundary">
        <ShieldCheck :size="28" aria-hidden="true" />
        <div>
          <h2 id="website-boundary-heading">官网不收集 Amber 凭据</h2>
          <p>当前官网不提供账号登录、日志上传、反馈收集或 Guest Key 输入表单，也不会要求把账号配置粘贴到浏览器。未来若增加收集功能，需要单独完成隐私、存储和反滥用评审。</p>
          <p class="independence">Amber 是独立开源项目，与 OpenAI、sub2api、CCSwitch 不存在官方隶属、合作或背书关系。</p>
        </div>
        <div class="boundary-actions">
          <RouterLink class="button button-secondary" to="/docs">查看使用文档 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
          <a class="button button-secondary" href="https://github.com/oyg123less/sub2api-desktop" target="_blank" rel="noopener noreferrer">
            查看源代码 <ExternalLink :size="16" aria-hidden="true" />
          </a>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.intro-points {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
  margin-top: 24px;
}

.boundary-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.boundary-callout {
  margin-top: 28px;
}

.relay-flow {
  display: grid;
  grid-template-columns: repeat(9, auto);
  gap: 12px;
  align-items: center;
  justify-content: start;
  padding-block: 24px;
  border-block: 1px solid var(--border-strong);
  overflow-x: auto;
}

.relay-flow span {
  padding: 9px 12px;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  background: var(--surface);
  color: var(--ink);
  font-size: 14px;
  font-weight: 700;
  white-space: nowrap;
}

.relay-flow > svg {
  color: var(--teal);
}

.route-modes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-top: 38px;
  border-block: 1px solid var(--border);
}

.route-modes article {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr);
  gap: 14px;
  padding: 26px 28px 26px 0;
}

.route-modes article + article {
  padding-right: 0;
  padding-left: 28px;
  border-left: 1px solid var(--border);
}

.route-modes svg {
  margin-top: 2px;
  color: var(--amber);
}

.route-modes h3 {
  margin-bottom: 8px;
}

.route-modes p,
.metadata-note {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.metadata-note {
  max-width: 880px;
  margin-top: 24px;
  font-size: 14px;
}

.practice-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  border-block: 1px solid var(--border);
}

.practice-grid article {
  padding: 28px 32px 28px 0;
}

.practice-grid article + article {
  padding-right: 0;
  padding-left: 32px;
  border-left: 1px solid var(--border);
}

.practice-grid svg,
.control-grid > article > svg {
  margin-bottom: 14px;
  color: var(--amber);
}

.practice-grid h3 {
  margin-bottom: 14px;
}

.practice-grid ul {
  margin: 0;
  padding-left: 20px;
  color: var(--ink-soft);
}

.practice-grid li + li {
  margin-top: 8px;
}

.redaction-warning {
  margin-top: 28px;
}

.control-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 64px;
}

.control-grid h2 {
  margin-bottom: 18px;
  font-size: 28px;
}

.control-grid p {
  color: var(--ink-soft);
}

.control-steps {
  margin: 22px 0;
  padding: 0;
  list-style: none;
  counter-reset: control-step;
}

.control-steps li {
  position: relative;
  min-height: 35px;
  padding: 5px 0 15px 42px;
  border-bottom: 1px solid var(--border);
  counter-increment: control-step;
}

.control-steps li + li {
  padding-top: 15px;
}

.control-steps li::before {
  position: absolute;
  top: 5px;
  left: 0;
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border: 1px solid var(--border-strong);
  border-radius: 50%;
  color: var(--amber-dark);
  content: counter(control-step);
  font-size: 12px;
  font-weight: 760;
}

.control-steps li + li::before {
  top: 15px;
}

.limits-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.8fr) minmax(0, 1.5fr);
  gap: 72px;
  align-items: start;
}

.limits-layout h2 {
  margin-bottom: 14px;
}

.limit-list {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
}

.limit-list li {
  display: grid;
  grid-template-columns: 25px minmax(0, 1fr);
  gap: 12px;
  padding-block: 18px;
  border-top: 1px solid #43463f;
  color: #cdd0c8;
}

.limit-list li:last-child {
  border-bottom: 1px solid #43463f;
}

.limit-list svg {
  margin-top: 3px;
  color: #e2a582;
}

.limit-list strong {
  color: #fff;
}

.website-boundary {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) auto;
  gap: 18px;
  align-items: start;
}

.website-boundary > svg {
  margin-top: 4px;
  color: var(--teal);
}

.website-boundary h2 {
  margin-bottom: 9px;
  font-size: 28px;
}

.website-boundary p {
  max-width: 760px;
  margin-bottom: 10px;
  color: var(--ink-soft);
}

.website-boundary .independence {
  color: var(--ink);
  font-weight: 680;
}

.boundary-actions {
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.boundary-actions .button:hover,
.boundary-actions .button:focus-visible {
  border-color: var(--teal);
  background: var(--teal-soft);
  color: var(--teal);
  box-shadow: var(--shadow-xs);
  transform: none;
}

.boundary-actions .button:active {
  transform: none;
}

@media (max-width: 900px) {
  .boundary-grid,
  .control-grid,
  .limits-layout {
    grid-template-columns: 1fr;
  }

  .control-grid,
  .limits-layout {
    gap: 48px;
  }

  .website-boundary {
    grid-template-columns: 38px minmax(0, 1fr);
  }

  .boundary-actions {
    grid-column: 2;
    flex-direction: row;
    flex-wrap: wrap;
  }
}

@media (max-width: 680px) {
  .relay-flow {
    grid-template-columns: 1fr;
    justify-items: stretch;
  }

  .relay-flow span {
    text-align: center;
  }

  .relay-flow > svg {
    justify-self: center;
    transform: rotate(90deg);
  }

  .route-modes,
  .practice-grid {
    grid-template-columns: 1fr;
  }

  .route-modes article,
  .route-modes article + article,
  .practice-grid article,
  .practice-grid article + article {
    padding: 24px 0;
    border-left: 0;
  }

  .route-modes article + article,
  .practice-grid article + article {
    border-top: 1px solid var(--border);
  }

  .website-boundary {
    grid-template-columns: 1fr;
  }

  .boundary-actions {
    grid-column: auto;
  }
}

@media (max-width: 480px) {
  .boundary-actions,
  .boundary-actions .button {
    width: 100%;
  }
}
</style>
