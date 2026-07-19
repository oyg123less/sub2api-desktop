<script setup lang="ts">
import {
  Activity,
  ArrowRight,
  BookOpen,
  Cloud,
  Download,
  Gauge,
  KeyRound,
  Laptop,
  Network,
  Route,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  UsersRound,
} from "lucide-vue-next";
import { RouterLink } from "vue-router";
import ImageViewer from "../components/ImageViewer.vue";
import { stableRelease, upcomingRelease } from "../config/releases";

const features = [
  {
    icon: KeyRound,
    title: "自己的账号，统一接入",
    text: "导入 ChatGPT OAuth 或 OpenAI 兼容 API 账号，在一个本地接口中统一管理。",
  },
  {
    icon: SlidersHorizontal,
    title: "代理与调度可见",
    text: "为账号绑定 HTTP、HTTPS 或 SOCKS5 代理，并控制并发、队列、限流与恢复。",
  },
  {
    icon: Activity,
    title: "请求结果可追踪",
    text: "查看请求、Token、延迟、错误与预估费用，定位问题不再依赖猜测。",
  },
  {
    icon: Network,
    title: "Codex 本地与远程接入",
    text: "支持本机配置、SSH 远程注入与反向隧道，让远程 Codex 使用本机网络出口。",
  },
  {
    icon: UsersRound,
    title: "共享可以单独撤销",
    text: "每位接收者使用独立 Guest Key，并拥有独立的 RPM、并发、额度与暂停状态。",
  },
  {
    icon: ShieldCheck,
    title: "默认留在本机",
    text: "本地功能无需注册。默认只监听 127.0.0.1，不把本地网关直接暴露到公网。",
  },
];
</script>

<template>
  <section class="home-hero" aria-labelledby="home-title">
    <div class="hero-image" role="img" aria-label="Amber 仪表盘真实产品界面"></div>
    <div class="hero-band">
      <div class="container hero-copy">
        <div class="hero-status">
          <span class="status-pill stable"><span class="status-dot"></span>稳定版 v{{ stableRelease.version }}</span>
          <span>Windows 10 / 11 · x64</span>
        </div>
        <h1 id="home-title">Amber</h1>
        <p>Windows 本地 OpenAI 兼容网关。管理自己的账号与代理，为 Codex 提供本地、SSH 远程接入和可控共享。</p>
        <div class="action-row">
          <a class="button button-primary" :href="stableRelease.downloadUrl">
            <Download :size="18" aria-hidden="true" />
            下载 v{{ stableRelease.version }}
          </a>
          <RouterLink class="button hero-secondary" to="/docs">
            <BookOpen :size="18" aria-hidden="true" />
            查看使用文档
          </RouterLink>
        </div>
        <p class="hero-note">当前正式版本为 v{{ stableRelease.version }}。v{{ upcomingRelease.version }} 仍在开发中，尚未提供下载。</p>
      </div>
    </div>
  </section>

  <section class="boundary-strip" aria-label="产品边界">
    <div class="container boundary-grid">
      <div>
        <Laptop :size="20" aria-hidden="true" />
        <span><strong>本地使用无需注册</strong>账号、代理、网关和统计保存在你的电脑上。</span>
      </div>
      <div>
        <Cloud :size="20" aria-hidden="true" />
        <span><strong>云能力按需开启</strong>仅同步、跨设备和共享需要 Amber 云账号。</span>
      </div>
      <div>
        <ShieldCheck :size="20" aria-hidden="true" />
        <span><strong>默认不暴露公网</strong>本地 API 默认监听 127.0.0.1。</span>
      </div>
    </div>
  </section>

  <section class="section" aria-labelledby="workflow-title">
    <div class="container workflow-layout">
      <div class="workflow-intro">
        <p class="eyebrow">最短使用路径</p>
        <h2 id="workflow-title">从账号到 Codex，四步完成</h2>
        <p>Amber 把导入、代理、网关和 Codex 配置放在同一个 Windows 工具中。先验证每一步，再进入下一步。</p>
        <RouterLink class="text-link" to="/docs#install">
          打开快速开始
          <ArrowRight :size="17" aria-hidden="true" />
        </RouterLink>
      </div>
      <ol class="step-list">
        <li>
          <div><h3>导入账号</h3><p>添加 ChatGPT OAuth、Base URL + API Key 或 JSON 账号。</p></div>
        </li>
        <li>
          <div><h3>检查代理</h3><p>按需绑定代理，并先完成 DNS、连接、TLS 与 HTTP 测试。</p></div>
        </li>
        <li>
          <div><h3>启动本地服务</h3><p>确认网关运行，并用 <code>/v1/models</code> 验证接口。</p></div>
        </li>
        <li>
          <div><h3>接入 Codex</h3><p>使用本机注入，或通过 SSH 直连与反向隧道接入远程环境。</p></div>
        </li>
      </ol>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="capabilities-title">
    <div class="container">
      <div class="section-header">
        <div>
          <p class="eyebrow">本地网关能力</p>
          <h2 id="capabilities-title">安静的界面，完整的运行控制</h2>
          <p>账号健康、代理出口、并发队列和请求结果都保持可见，适合重复使用与持续排障。</p>
        </div>
      </div>
      <div class="feature-grid">
        <article v-for="feature in features" :key="feature.title" class="feature-item">
          <component :is="feature.icon" :size="25" aria-hidden="true" />
          <h3>{{ feature.title }}</h3>
          <p>{{ feature.text }}</p>
        </article>
      </div>
    </div>
  </section>

  <section class="section product-section" aria-labelledby="accounts-title">
    <div class="container product-layout">
      <div class="product-copy">
        <p class="eyebrow">账号与调度</p>
        <h2 id="accounts-title">一眼看清账号是否真的可用</h2>
        <p>Amber 将健康状态、额度窗口、并发、队列、Token 和预估费用放在同一行。账号测试和批量操作不会遮住关键状态。</p>
        <ul class="check-list">
          <li><Gauge :size="18" aria-hidden="true" />额度感知与并发队列</li>
          <li><Route :size="18" aria-hidden="true" />账号级代理绑定</li>
          <li><Activity :size="18" aria-hidden="true" />临时故障冷却与自动恢复</li>
        </ul>
      </div>
      <ImageViewer
        src="/screenshots/accounts.png"
        alt="Amber 账号管理界面，展示演示账号的健康、额度和并发信息"
        :caption="`现有 v0.4.x 界面，内容为模拟数据；v${upcomingRelease.version} 正式发布前将替换最终截图。`"
      />
    </div>
  </section>

  <section class="section section-dark" aria-labelledby="codex-title">
    <div class="container codex-layout">
      <div>
        <p class="eyebrow">Codex 接入</p>
        <h2 id="codex-title">本机配置，也能把本机出口带到远程服务器</h2>
        <p>本机 Codex 直接连接 Amber。远程服务器可使用 SSH 直连或反向隧道，通过安装 Amber 的电脑访问上游。</p>
        <div class="route-flow" aria-label="远程反向隧道请求路径">
          <span>远程 Codex</span><ArrowRight :size="16" aria-hidden="true" />
          <span>SSH 隧道</span><ArrowRight :size="16" aria-hidden="true" />
          <span>本机 Amber</span><ArrowRight :size="16" aria-hidden="true" />
          <span>账号与代理</span>
        </div>
        <p class="version-note"><strong>v{{ stableRelease.version }} 提示：</strong>注入前请先在仪表盘启动服务。v{{ upcomingRelease.version }} 计划改为健康检查通过后再写入配置。</p>
        <RouterLink class="button button-dark" to="/docs#codex-local">查看 Codex 教程</RouterLink>
      </div>
      <ImageViewer
        src="/screenshots/codex.png"
        alt="Amber Codex 远程接入界面"
        caption="现有 v0.4.x 远程接入界面，服务器信息为演示数据。"
      />
    </div>
  </section>

  <section class="section" aria-labelledby="sharing-title">
    <div class="container sharing-layout">
      <div class="sharing-heading">
        <p class="eyebrow">可控共享</p>
        <h2 id="sharing-title">授权给具体的人，随时暂停或撤销</h2>
        <p>连接码与临时密码只用于领取权限，不是模型 API Key。每位接收者得到独立 Guest Key，限制和使用记录互不影响。</p>
      </div>
      <div class="sharing-paths">
        <article>
          <span class="status-pill stable">v{{ stableRelease.version }} 当前</span>
          <h3>连接码快速共享</h3>
          <p>选择账号池，生成连接码和临时密码。好友入口仍存在，但不是快速共享的前置条件。</p>
        </article>
        <article>
          <span class="status-pill upcoming">v{{ upcomingRelease.version }} 即将发布</span>
          <h3>设备定向与独立工作区</h3>
          <p>计划移除好友主流程，让共享固定由指定电脑承载，并为不同云账号建立独立工作区。</p>
        </article>
      </div>
      <RouterLink class="text-link" to="/changelog">
        查看版本差异
        <ArrowRight :size="17" aria-hidden="true" />
      </RouterLink>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="boundary-title">
    <div class="container security-band">
      <div>
        <p class="eyebrow">数据边界</p>
        <h2 id="boundary-title">知道数据在哪里，比一句“安全”更重要</h2>
      </div>
      <div class="boundary-lines">
        <p><Server :size="19" aria-hidden="true" /><span><strong>本地：</strong>账号、代理、设置、日志与本地 API Key 保存在 Amber 数据目录。</span></p>
        <p><Cloud :size="19" aria-hidden="true" /><span><strong>云同步：</strong>需要登录，云端处理身份、加密保险库密文和必要的同步元数据。</span></p>
        <p><Route :size="19" aria-hidden="true" /><span><strong>共享回流：</strong>OAuth 请求由共享者设备处理；设备或 Amber 离线时，请求无法继续。</span></p>
      </div>
      <RouterLink class="button button-secondary" to="/security">阅读安全与隐私说明</RouterLink>
    </div>
  </section>

  <section class="section section-dark home-cta" aria-labelledby="cta-title">
    <div class="container cta-inner">
      <div>
        <p class="eyebrow">当前稳定版 v{{ stableRelease.version }}</p>
        <h2 id="cta-title">先在本机把第一条请求跑通</h2>
        <p>下载 Windows x64 安装包，按文档完成导入、启动和模型列表验证。</p>
      </div>
      <div class="action-row">
        <a class="button button-primary" :href="stableRelease.downloadUrl"><Download :size="18" aria-hidden="true" />下载</a>
        <RouterLink class="button button-dark" to="/docs"><BookOpen :size="18" aria-hidden="true" />阅读文档</RouterLink>
      </div>
    </div>
  </section>
</template>

<style scoped>
.home-hero {
  position: relative;
  display: flex;
  min-height: min(700px, calc(100svh - 120px));
  align-items: end;
  overflow: hidden;
  background: #ecece6;
}

.hero-image {
  position: absolute;
  inset: 0;
  background-image: url("/screenshots/dashboard.png");
  background-position: center top;
  background-size: cover;
}

.hero-band {
  position: relative;
  z-index: 1;
  width: 100%;
  border-top: 1px solid rgba(255, 255, 255, 0.18);
  background: rgba(31, 32, 29, 0.94);
}

.hero-copy {
  padding-block: 34px 30px;
}

.hero-copy h1 {
  margin: 8px 0 7px;
  color: #fff;
  font-size: 58px;
}

.hero-copy > p:not(.hero-note) {
  max-width: 780px;
  margin-bottom: 20px;
  color: #e6e8e2;
  font-size: 19px;
}

.hero-status {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #cfd2ca;
  font-size: 13px;
}

.hero-secondary {
  border-color: #74776f;
  background: transparent;
  color: #fff;
}

.hero-secondary:hover {
  border-color: #fff;
  background: #fff;
  color: var(--ink);
}

.hero-note {
  margin: 14px 0 0;
  color: #bfc3ba;
  font-size: 13px;
}

.boundary-strip {
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.boundary-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.boundary-grid > div {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 11px;
  padding: 22px;
  border-left: 1px solid var(--border);
}

.boundary-grid > div:last-child {
  border-right: 1px solid var(--border);
}

.boundary-grid svg {
  margin-top: 2px;
  color: var(--teal);
}

.boundary-grid strong {
  display: block;
  margin-bottom: 2px;
}

.boundary-grid span {
  color: var(--ink-soft);
  font-size: 13px;
}

.workflow-layout {
  display: grid;
  grid-template-columns: minmax(280px, 0.72fr) minmax(0, 1.28fr);
  gap: 80px;
}

.workflow-intro {
  align-self: start;
}

.workflow-intro h2 {
  margin-bottom: 15px;
}

.workflow-intro p:not(.eyebrow) {
  color: var(--ink-soft);
}

.text-link {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-weight: 720;
  text-decoration: none;
}

.text-link:hover {
  text-decoration: underline;
}

.product-layout,
.codex-layout {
  display: grid;
  grid-template-columns: minmax(300px, 0.75fr) minmax(0, 1.25fr);
  align-items: center;
  gap: 68px;
}

.product-copy > p:not(.eyebrow),
.sharing-heading p {
  color: var(--ink-soft);
  font-size: 17px;
}

.check-list {
  display: grid;
  margin: 26px 0 0;
  padding: 0;
  gap: 12px;
  list-style: none;
}

.check-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 650;
}

.check-list svg {
  color: var(--amber);
}

.product-layout :deep(.image-viewer),
.codex-layout :deep(.image-viewer) {
  margin-bottom: 0;
}

.codex-layout :deep(figcaption) {
  color: #cdd0c8;
}

.codex-layout {
  grid-template-columns: minmax(0, 1fr) minmax(360px, 1.15fr);
}

.codex-layout .eyebrow,
.home-cta .eyebrow {
  color: #ef9d70;
}

.route-flow {
  display: flex;
  margin: 25px 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
}

.route-flow span {
  padding: 7px 9px;
  border: 1px solid #52564f;
  border-radius: 5px;
  color: #fff;
  font-size: 13px;
  font-weight: 680;
}

.version-note {
  padding-left: 14px;
  border-left: 3px solid #e58a57;
  font-size: 14px;
}

.sharing-layout {
  display: grid;
  grid-template-columns: minmax(280px, 0.72fr) minmax(0, 1.28fr);
  gap: 42px 72px;
}

.sharing-paths {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  border: 1px solid var(--border);
  background: var(--border);
}

.sharing-paths article {
  padding: 28px;
  background: var(--surface);
}

.sharing-paths h3 {
  margin: 17px 0 9px;
}

.sharing-paths p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.sharing-layout > .text-link {
  grid-column: 2;
  justify-self: start;
}

.security-band {
  display: grid;
  grid-template-columns: minmax(260px, 0.72fr) minmax(0, 1.28fr);
  gap: 34px 72px;
  align-items: start;
}

.boundary-lines {
  border-top: 1px solid var(--border);
}

.boundary-lines p {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 11px;
  margin: 0;
  padding-block: 17px;
  border-bottom: 1px solid var(--border);
}

.boundary-lines svg {
  margin-top: 3px;
  color: var(--teal);
}

.security-band > .button {
  grid-column: 2;
  justify-self: start;
}

.home-cta {
  border-top: 1px solid #393c36;
}

.cta-inner {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 38px;
}

.cta-inner h2 {
  margin-bottom: 10px;
}

.cta-inner p:last-child {
  margin-bottom: 0;
}

@media (max-width: 900px) {
  .home-hero {
    min-height: min(680px, calc(100svh - 92px));
  }

  .hero-image {
    background-position: 28% top;
  }

  .boundary-grid {
    grid-template-columns: 1fr;
  }

  .boundary-grid > div {
    border-right: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  .boundary-grid > div:last-child {
    border-bottom: 0;
  }

  .workflow-layout,
  .product-layout,
  .codex-layout,
  .sharing-layout,
  .security-band {
    grid-template-columns: 1fr;
    gap: 38px;
  }

  .product-copy {
    max-width: 720px;
  }

  .sharing-layout > .text-link,
  .security-band > .button {
    grid-column: 1;
  }
}

@media (max-width: 640px) {
  .home-hero {
    min-height: min(620px, calc(100svh - 82px));
  }

  .hero-image {
    background-position: 26% top;
    background-size: auto 58%;
    background-repeat: no-repeat;
  }

  .hero-copy {
    padding-block: 26px 23px;
  }

  .hero-copy h1 {
    font-size: 43px;
  }

  .hero-copy > p:not(.hero-note) {
    font-size: 16px;
  }

  .hero-status {
    align-items: flex-start;
    flex-direction: column;
    gap: 7px;
  }

  .hero-copy .button {
    flex: 1 1 100%;
  }

  .sharing-paths {
    grid-template-columns: 1fr;
  }

  .route-flow svg {
    display: none;
  }

  .cta-inner {
    display: block;
  }

  .cta-inner .action-row {
    margin-top: 25px;
  }

  .cta-inner .button {
    flex: 1 1 100%;
  }
}
</style>
