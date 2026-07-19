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

const workflow = [
  {
    icon: KeyRound,
    title: "导入并测试账号",
    text: "添加 ChatGPT OAuth 或 OpenAI 兼容账号，先确认凭据与额度可用。",
  },
  {
    icon: SlidersHorizontal,
    title: "按需配置代理",
    text: "为指定账号绑定 HTTP、HTTPS 或 SOCKS5 代理，再启动本地网关。",
  },
  {
    icon: Network,
    title: "接入兼容客户端",
    text: "将 Codex 或其他 OpenAI 兼容客户端指向 Amber，开始发送请求。",
  },
];
</script>

<template>
  <section class="home-hero" aria-labelledby="home-title">
    <div class="hero-product-stage">
      <img
        class="hero-product-image"
        src="/screenshots/accounts.png"
        alt="Amber 账号管理界面，展示三个演示账号的健康、额度与并发状态"
        width="1440"
        height="900"
        fetchpriority="high"
        decoding="async"
      />
    </div>

    <div class="container hero-content">
      <div class="hero-copy">
        <div class="hero-status">
          <span class="status-pill stable"><span class="status-dot"></span>稳定版 v{{ stableRelease.version }}</span>
          <span>Windows 10 / 11 · x64</span>
        </div>
        <h1 id="home-title">Amber</h1>
        <p class="hero-lede">把自己的账号与网络出口，变成 Codex 可用的本地接口。</p>
        <p class="hero-description">统一管理 ChatGPT OAuth 与 OpenAI 兼容账号，按需绑定代理，并为本机或远程 Codex 配置连接。</p>
        <div class="action-row">
          <a class="button button-primary" :href="stableRelease.downloadUrl">
            <Download :size="18" aria-hidden="true" />
            下载 v{{ stableRelease.version }}
          </a>
          <RouterLink class="button button-secondary" to="/docs">
            <BookOpen :size="18" aria-hidden="true" />
            查看使用文档
          </RouterLink>
        </div>
        <p class="hero-note">v{{ upcomingRelease.version }} 正在开发，暂无安装包。</p>
      </div>
    </div>
  </section>

  <section class="boundary-strip" aria-label="产品边界">
    <div class="container boundary-grid">
      <div>
        <Laptop :size="20" aria-hidden="true" />
        <span><strong>本地使用无需注册</strong>账号、代理和统计保存在你的电脑上。</span>
      </div>
      <div>
        <ShieldCheck :size="20" aria-hidden="true" />
        <span><strong>默认只监听本机</strong>本地 API 默认使用 127.0.0.1。</span>
      </div>
      <div>
        <Cloud :size="20" aria-hidden="true" />
        <span><strong>云能力按需开启</strong>只有同步与共享需要 Amber 云账号。</span>
      </div>
    </div>
  </section>

  <section class="section workflow-section" aria-labelledby="workflow-title">
    <div class="container">
      <div class="workflow-heading">
        <div>
          <p class="eyebrow">从账号到接口</p>
          <h2 id="workflow-title">三步跑通第一条请求</h2>
          <p>代理是可选项。每一步都能独立测试，问题不会被带到下一环节。</p>
        </div>
        <RouterLink class="text-link" to="/docs#install">
          打开快速开始
          <ArrowRight :size="17" aria-hidden="true" />
        </RouterLink>
      </div>

      <ol class="workflow-rail">
        <li v-for="(item, index) in workflow" :key="item.title">
          <div class="step-marker">
            <component :is="item.icon" :size="21" aria-hidden="true" />
            <span>0{{ index + 1 }}</span>
          </div>
          <h3>{{ item.title }}</h3>
          <p>{{ item.text }}</p>
        </li>
      </ol>
    </div>
  </section>

  <section class="section operations-section" aria-labelledby="accounts-title">
    <div class="container proof-layout">
      <ImageViewer
        src="/screenshots/account-details.png"
        alt="Amber 账号详情界面，展示演示账号的健康、用量、额度与调度设置"
        caption="Amber v0.4.2 界面示意 · 演示数据"
      />
      <div class="proof-copy">
        <p class="eyebrow">账号与调度</p>
        <h2 id="accounts-title">一处看清账号、调度与调用状态</h2>
        <p>健康、额度、并发与队列集中展示；代理可按账号绑定，请求、Token、延迟和预估费用也能持续追踪。</p>
        <ul class="check-list">
          <li><Gauge :size="18" aria-hidden="true" />额度窗口与并发队列</li>
          <li><Route :size="18" aria-hidden="true" />账号级代理与出口绑定</li>
          <li><Activity :size="18" aria-hidden="true" />失败冷却与自动恢复</li>
        </ul>
        <RouterLink class="text-link" to="/docs#accounts">
          了解账号管理
          <ArrowRight :size="17" aria-hidden="true" />
        </RouterLink>
      </div>
    </div>
  </section>

  <section class="section section-dark codex-section" aria-labelledby="codex-title">
    <div class="container codex-layout">
      <ImageViewer
        src="/screenshots/codex.png"
        alt="Amber Codex 远程接入界面，展示 SSH 反向隧道配置与运行状态"
        caption="Amber v0.4.2 远程接入界面 · 演示数据"
      />
      <div class="codex-copy">
        <p class="eyebrow">Codex 接入</p>
        <h2 id="codex-title">让本机与远程 Codex 使用这条出口</h2>
        <p>本机 Codex 可以直接连接 Amber。远程服务器则可通过 SSH 直连或反向隧道，继续使用安装 Amber 的电脑和网络出口。</p>
        <div class="route-flow" aria-label="远程反向隧道请求路径">
          <span><Server :size="15" aria-hidden="true" />远程 Codex</span>
          <ArrowRight :size="16" aria-hidden="true" />
          <span>SSH 隧道</span>
          <ArrowRight :size="16" aria-hidden="true" />
          <span><Laptop :size="15" aria-hidden="true" />本机 Amber</span>
        </div>
        <p class="version-note"><strong>v{{ stableRelease.version }} 提示：</strong>请先在仪表盘启动服务，再进行 Codex 注入。</p>
        <RouterLink class="button button-dark" to="/docs#codex-local">查看 Codex 教程</RouterLink>
      </div>
    </div>
  </section>

  <section class="section sharing-section" aria-labelledby="sharing-title">
    <div class="container sharing-layout">
      <div class="sharing-copy">
        <p class="eyebrow">可控共享</p>
        <h2 id="sharing-title">共享的是权限，不是账号凭据</h2>
        <p>连接码与临时密码只用于领取权限。每位接收者获得独立 Guest Key，可以分别限流、暂停和撤销。</p>
        <div class="sharing-points">
          <p><KeyRound :size="19" aria-hidden="true" /><span><strong>独立密钥</strong>接收者之间互不影响。</span></p>
          <p><UsersRound :size="19" aria-hidden="true" /><span><strong>独立限制</strong>分别设置 RPM、并发与额度。</span></p>
          <p><ShieldCheck :size="19" aria-hidden="true" /><span><strong>随时撤销</strong>停止授权不需要更换主账号。</span></p>
        </div>
        <p class="roadmap-note">v{{ upcomingRelease.version }} 计划加入指定承载设备与独立工作区，目前仍在开发。</p>
        <RouterLink class="text-link" to="/changelog">
          查看版本进展
          <ArrowRight :size="17" aria-hidden="true" />
        </RouterLink>
      </div>

      <div class="data-boundary" aria-labelledby="boundary-title">
        <p class="eyebrow">数据边界</p>
        <h3 id="boundary-title">清楚知道数据留在哪里</h3>
        <div class="boundary-lines">
          <p><Laptop :size="19" aria-hidden="true" /><span><strong>本地</strong>账号、代理、设置、日志与本地 API Key 保存在 Amber 数据目录。</span></p>
          <p><Cloud :size="19" aria-hidden="true" /><span><strong>云同步</strong>登录后才会处理身份、加密保险库密文与必要的同步元数据。</span></p>
          <p><Route :size="19" aria-hidden="true" /><span><strong>共享回流</strong>OAuth 请求由共享者设备处理；设备离线时，请求无法继续。</span></p>
        </div>
        <RouterLink class="button button-secondary" to="/security">阅读安全与隐私说明</RouterLink>
      </div>
    </div>
  </section>

  <section class="home-cta" aria-labelledby="cta-title">
    <div class="container cta-inner">
      <div>
        <p class="eyebrow">稳定版 v{{ stableRelease.version }}</p>
        <h2 id="cta-title">在 Windows 上开始使用 Amber</h2>
        <p>下载 x64 安装包，按文档完成账号导入、服务启动与接口验证。</p>
      </div>
      <div class="action-row">
        <a class="button button-primary" :href="stableRelease.downloadUrl"><Download :size="18" aria-hidden="true" />下载安装包</a>
        <RouterLink class="button button-secondary" to="/docs"><BookOpen :size="18" aria-hidden="true" />阅读文档</RouterLink>
      </div>
    </div>
  </section>
</template>

<style scoped>
.home-hero {
  position: relative;
  height: clamp(560px, calc(100svh - 150px), 640px);
  overflow: hidden;
  border-bottom: 1px solid var(--border);
  background: #f1f0ea;
}

.home-hero::after {
  position: absolute;
  z-index: 1;
  inset: 0 auto 0 0;
  width: calc(50% + 10px);
  background: rgba(247, 247, 244, 0.97);
  box-shadow: 28px 0 48px rgba(31, 33, 28, 0.04);
  content: "";
}

.hero-product-stage {
  position: absolute;
  z-index: 0;
  inset: 0;
  overflow: hidden;
}

.hero-product-image {
  position: absolute;
  top: 0;
  right: 0;
  width: min(1440px, 100vw);
  max-width: none;
  height: auto;
}

.hero-content {
  position: relative;
  z-index: 2;
  display: flex;
  height: 100%;
  align-items: center;
}

.hero-copy {
  width: min(510px, 46vw);
  padding-block: 42px;
}

.hero-status {
  display: flex;
  align-items: center;
  gap: 11px;
  color: var(--ink-soft);
  font-size: 13px;
}

.hero-copy h1 {
  margin: 16px 0 11px;
  font-size: 60px;
}

.hero-lede {
  max-width: 500px;
  margin-bottom: 13px;
  color: var(--ink);
  font-size: 26px;
  font-weight: 700;
  line-height: 1.45;
}

.hero-description {
  max-width: 490px;
  margin-bottom: 24px;
  color: var(--ink-soft);
  font-size: 16px;
}

.hero-note {
  margin: 14px 0 0;
  color: var(--ink-soft);
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
  grid-template-columns: 23px minmax(0, 1fr);
  gap: 10px;
  padding: 19px 22px;
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
  color: var(--ink);
}

.boundary-grid span {
  color: var(--ink-soft);
  font-size: 13px;
}

.workflow-section {
  background: var(--surface);
}

.workflow-section,
.operations-section,
.codex-section,
.sharing-section {
  padding-block: 76px;
}

.workflow-heading {
  display: flex;
  margin-bottom: 35px;
  align-items: end;
  justify-content: space-between;
  gap: 34px;
}

.workflow-heading > div {
  max-width: 700px;
}

.workflow-heading h2,
.proof-copy h2,
.codex-copy h2,
.sharing-copy h2 {
  margin-bottom: 12px;
}

.workflow-heading p:last-child,
.proof-copy > p,
.sharing-copy > p:not(.eyebrow, .roadmap-note) {
  margin-bottom: 0;
  color: var(--ink-soft);
  font-size: 17px;
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

.workflow-rail {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 0;
  padding: 0;
  border-block: 1px solid var(--border);
  list-style: none;
}

.workflow-rail li {
  min-width: 0;
  padding: 27px 30px 29px;
  border-left: 1px solid var(--border);
}

.workflow-rail li:last-child {
  border-right: 1px solid var(--border);
}

.step-marker {
  display: flex;
  margin-bottom: 18px;
  align-items: center;
  justify-content: space-between;
  color: var(--amber);
}

.step-marker span {
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 760;
}

.workflow-rail h3 {
  margin-bottom: 7px;
  font-size: 18px;
}

.workflow-rail p {
  margin-bottom: 0;
  color: var(--ink-soft);
  font-size: 14px;
}

.operations-section {
  border-block: 1px solid var(--border);
  background: var(--surface-muted);
}

.proof-layout,
.codex-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.32fr) minmax(310px, 0.68fr);
  align-items: center;
  gap: 62px;
}

.proof-layout :deep(.image-viewer),
.codex-layout :deep(.image-viewer) {
  min-width: 0;
  margin: 0;
}

.proof-copy > p {
  margin-bottom: 0;
}

.check-list {
  display: grid;
  margin: 25px 0 24px;
  padding: 0;
  gap: 11px;
  list-style: none;
}

.check-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 650;
}

.check-list svg {
  flex: 0 0 auto;
  color: var(--amber);
}

.codex-section {
  border-bottom: 1px solid #393c36;
}

.codex-layout {
  grid-template-columns: minmax(0, 1.18fr) minmax(330px, 0.82fr);
}

.codex-layout :deep(.image-button) {
  border-color: #4a4d47;
  box-shadow: 0 18px 42px rgba(0, 0, 0, 0.2);
}

.codex-layout :deep(figcaption) {
  color: #cdd0c8;
}

.codex-copy .eyebrow {
  color: #ef9d70;
}

.codex-copy > p:not(.eyebrow, .version-note) {
  color: #cdd0c8;
  font-size: 16px;
}

.route-flow {
  display: flex;
  margin: 24px 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.route-flow span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 9px;
  border: 1px solid #565a53;
  border-radius: 5px;
  color: #fff;
  font-size: 13px;
  font-weight: 680;
}

.version-note {
  margin-bottom: 24px;
  padding-left: 14px;
  border-left: 3px solid #e58a57;
  color: #daddd5;
  font-size: 14px;
}

.sharing-section {
  background: var(--surface);
}

.sharing-layout {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
  gap: 76px;
}

.sharing-points {
  margin: 25px 0 20px;
  border-top: 1px solid var(--border);
}

.sharing-points p,
.boundary-lines p {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 11px;
  margin: 0;
  padding-block: 14px;
  border-bottom: 1px solid var(--border);
}

.sharing-points svg,
.boundary-lines svg {
  margin-top: 3px;
  color: var(--teal);
}

.sharing-points strong,
.boundary-lines strong {
  display: block;
  color: var(--ink);
}

.sharing-points span,
.boundary-lines span {
  color: var(--ink-soft);
  font-size: 14px;
}

.roadmap-note {
  margin-bottom: 18px;
  color: var(--teal);
  font-size: 13px;
}

.data-boundary {
  padding-left: 58px;
  border-left: 1px solid var(--border);
}

.data-boundary h3 {
  margin-bottom: 19px;
  font-size: 25px;
}

.boundary-lines {
  margin-bottom: 24px;
  border-top: 1px solid var(--border);
}

.home-cta {
  padding-block: 46px;
  border-block: 1px solid #e2c8b8;
  background: var(--amber-soft);
}

.cta-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 38px;
}

.cta-inner .eyebrow {
  margin-bottom: 8px;
}

.cta-inner h2 {
  margin-bottom: 7px;
  font-size: 28px;
}

.cta-inner p:last-child {
  margin-bottom: 0;
  color: var(--ink-soft);
}

@media (max-width: 1000px) {
  .home-hero::after {
    width: 56%;
  }

  .hero-product-image {
    width: 1160px;
  }

  .hero-copy {
    width: 49vw;
  }

  .proof-layout,
  .codex-layout {
    gap: 42px;
  }

  .sharing-layout {
    gap: 50px;
  }

  .data-boundary {
    padding-left: 42px;
  }
}

@media (max-width: 900px) {
  .workflow-heading {
    align-items: start;
    flex-direction: column;
    gap: 16px;
  }

  .workflow-rail {
    grid-template-columns: 1fr;
  }

  .workflow-rail li,
  .workflow-rail li:last-child {
    border-right: 1px solid var(--border);
  }

  .workflow-rail li + li {
    border-top: 1px solid var(--border);
  }

  .proof-layout,
  .codex-layout,
  .sharing-layout {
    grid-template-columns: 1fr;
    gap: 40px;
  }

  .proof-copy,
  .codex-copy,
  .sharing-copy {
    max-width: 720px;
  }

  .codex-copy {
    grid-row: 1;
  }

  .data-boundary {
    padding: 40px 0 0;
    border-top: 1px solid var(--border);
    border-left: 0;
  }
}

@media (max-width: 700px) {
  .home-hero {
    display: flex;
    height: auto;
    min-height: 0;
    flex-direction: column;
  }

  .home-hero::after {
    display: none;
  }

  .hero-product-stage {
    position: relative;
    order: 1;
    width: 100%;
    height: clamp(200px, 30svh, 250px);
    flex: 0 0 auto;
  }

  .hero-product-image {
    top: 0;
    right: auto;
    left: -130px;
    width: 900px;
  }

  .hero-content {
    display: block;
    order: 0;
    height: auto;
  }

  .hero-copy {
    width: 100%;
    padding-block: 27px 18px;
  }

  .hero-copy h1 {
    margin-block: 10px 7px;
    font-size: 43px;
  }

  .hero-lede {
    margin-bottom: 8px;
    font-size: 20px;
    line-height: 1.4;
  }

  .hero-description {
    margin-bottom: 16px;
    font-size: 14px;
    line-height: 1.55;
  }

  .hero-status {
    gap: 8px;
    font-size: 12px;
  }

  .hero-copy .action-row {
    gap: 8px;
  }

  .hero-copy .button {
    min-height: 42px;
    padding: 9px 12px;
    font-size: 14px;
  }

  .hero-note {
    margin-top: 9px;
    font-size: 12px;
  }

  .boundary-grid {
    grid-template-columns: 1fr;
  }

  .boundary-grid > div {
    padding: 15px 17px;
    border-right: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  .boundary-grid > div:last-child {
    border-bottom: 0;
  }

  .proof-layout,
  .codex-layout,
  .sharing-layout {
    gap: 32px;
  }

  .workflow-section,
  .operations-section,
  .codex-section,
  .sharing-section {
    padding-block: 58px;
  }

  .proof-layout :deep(.image-button),
  .codex-layout :deep(.image-button) {
    aspect-ratio: 1.36;
  }

  .proof-layout :deep(.image-button img),
  .codex-layout :deep(.image-button img) {
    width: 118%;
    max-width: none;
    transform: translateX(-15%);
  }

  .route-flow > svg {
    display: none;
  }

  .data-boundary {
    padding-top: 32px;
  }

  .cta-inner {
    align-items: stretch;
    flex-direction: column;
    gap: 22px;
  }

  .cta-inner .button {
    flex: 1 1 100%;
  }
}

@media (max-width: 400px) {
  .hero-status {
    align-items: flex-start;
    flex-direction: column;
    gap: 5px;
  }

  .hero-copy h1 {
    margin-top: 7px;
  }
}
</style>
