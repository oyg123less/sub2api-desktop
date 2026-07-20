<script setup lang="ts">
import {
  AlertTriangle,
  Check,
  Copy,
  Download,
  ExternalLink,
  HardDrive,
  History,
  Info,
  Monitor,
  ShieldCheck,
} from "lucide-vue-next";
import { ref } from "vue";
import { RouterLink } from "vue-router";
import { formatFileSize, formatPublishedDate, stableRelease, upcomingRelease } from "../config/releases";

const copied = ref(false);

async function copyChecksum() {
  await navigator.clipboard.writeText(stableRelease.sha256);
  copied.value = true;
  window.setTimeout(() => {
    copied.value = false;
  }, 1800);
}
</script>

<template>
  <header class="download-hero" aria-labelledby="download-title">
    <div class="container download-hero-layout">
      <div class="download-hero-copy">
        <p class="eyebrow">Windows 下载</p>
        <h1 id="download-title">下载 Amber v{{ stableRelease.version }}</h1>
        <p class="download-hero-lede">
          当前稳定版面向 Windows 10 / 11 x64。安装包由 GitHub Releases 直接托管，官网不会通过 Worker 转发可执行文件。
        </p>
        <div class="download-signals" aria-label="安装说明">
          <span class="status-pill stable"><span class="status-dot" aria-hidden="true"></span>当前稳定版</span>
          <span><ShieldCheck :size="17" aria-hidden="true" />可直接覆盖安装，无需先卸载</span>
        </div>
        <div class="action-row download-secondary-actions">
          <a class="button button-secondary" :href="stableRelease.releaseUrl" target="_blank" rel="noreferrer">
            <ExternalLink :size="17" aria-hidden="true" />
            GitHub Release
          </a>
          <RouterLink class="button button-secondary" to="/docs">使用文档</RouterLink>
        </div>
      </div>

      <aside class="download-asset" aria-label="当前稳定版安装包">
        <div class="asset-heading">
          <span class="asset-icon" aria-hidden="true"><Monitor :size="24" /></span>
          <div>
            <p>Amber v{{ stableRelease.version }}</p>
            <h2>Windows x64 安装包</h2>
          </div>
        </div>
        <p class="asset-filename mono">{{ stableRelease.installerName }}</p>

        <a class="button button-primary asset-download" :href="stableRelease.downloadUrl">
          <Download :size="19" aria-hidden="true" />
          下载 Amber v{{ stableRelease.version }}
        </a>
        <p class="asset-source">从 github.com 下载 · {{ stableRelease.installerSizeBytes.toLocaleString("zh-CN") }} 字节</p>

        <dl class="asset-facts">
          <div><dt>平台</dt><dd>Windows 10 / 11 · x64</dd></div>
          <div><dt>文件大小</dt><dd>{{ formatFileSize(stableRelease.installerSizeBytes) }}</dd></div>
          <div><dt>发布日期</dt><dd>{{ formatPublishedDate(stableRelease.publishedAt) }}</dd></div>
          <div class="asset-checksum">
            <dt>SHA-256</dt>
            <dd>
              <code>{{ stableRelease.sha256 }}</code>
              <button
                class="icon-button"
                type="button"
                :aria-label="copied ? '校验值已复制' : '复制 SHA-256'"
                :title="copied ? '校验值已复制' : '复制 SHA-256'"
                @click="copyChecksum"
              >
                <Check v-if="copied" :size="19" aria-hidden="true" />
                <Copy v-else :size="19" aria-hidden="true" />
              </button>
              <span class="sr-only" aria-live="polite">{{ copied ? "SHA-256 已复制" : "" }}</span>
            </dd>
          </div>
        </dl>
      </aside>
    </div>
  </header>

  <section class="section-compact section-muted" aria-labelledby="checksum-title">
    <div class="container verification-layout">
      <div>
        <p class="eyebrow">完整性校验</p>
        <h2 id="checksum-title">下载后再核对一次</h2>
        <p>在 PowerShell 中计算文件哈希。文件名、下载来源与首屏 SHA-256 全部一致后，再运行安装包。</p>
      </div>
      <div class="code-block" tabindex="0" aria-label="PowerShell 计算 SHA-256 命令">Get-FileHash .\{{ stableRelease.installerName }} -Algorithm SHA256</div>
    </div>
  </section>

  <section class="section" aria-labelledby="install-title">
    <div class="container install-grid">
      <div>
        <p class="eyebrow">覆盖安装</p>
        <h2 id="install-title">从旧版本升级</h2>
        <ol class="step-list">
          <li><div><h3>备份数据</h3><p>先在 Amber 设置中确认数据目录并保留完整备份。</p></div></li>
          <li><div><h3>退出正在运行的 Amber</h3><p>保存工作，停止高频客户端请求，再正常退出应用。</p></div></li>
          <li><div><h3>直接运行新安装包</h3><p>不需要先卸载。覆盖安装会保留本地数据库、账号、代理和设置。</p></div></li>
          <li><div><h3>启动并运行诊断</h3><p>确认后台、端口、账号与本地 API 均正常后，再恢复客户端调用。</p></div></li>
        </ol>
      </div>

      <div class="install-notes">
        <div class="callout warning">
          <AlertTriangle :size="21" aria-hidden="true" />
          <div>
            <strong>不要先卸载</strong>
            <p>普通版本升级可直接覆盖安装。卸载不是解决端口、Sidecar 或 WebView2 问题的诊断步骤。</p>
          </div>
        </div>
        <div class="callout">
          <HardDrive :size="21" aria-hidden="true" />
          <div>
            <strong>保护数据目录</strong>
            <p>不要手工复制正在使用的 SQLite WAL 文件。需要迁移时优先使用 Amber 内置流程。</p>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="smartscreen-title">
    <div class="container smartscreen-layout">
      <div>
        <p class="eyebrow">Windows 安全提示</p>
        <h2 id="smartscreen-title">SmartScreen 显示提醒时</h2>
        <p>新版本或下载量较少的安装包可能触发 Microsoft Defender SmartScreen。提醒本身不等于文件已感染，也不能替代哈希校验。</p>
      </div>
      <div class="smartscreen-steps">
        <p><ShieldCheck :size="20" aria-hidden="true" /><span>确认下载地址属于 <code>github.com/oyg123less/sub2api-desktop</code>。</span></p>
        <p><Check :size="20" aria-hidden="true" /><span>用 PowerShell 计算 SHA-256，并逐字核对本页校验值。</span></p>
        <p><Info :size="20" aria-hidden="true" /><span>来源或哈希不一致时立即删除文件，不要选择继续运行。</span></p>
      </div>
    </div>
  </section>

  <section class="section" aria-labelledby="next-version-title">
    <div class="container future-release">
      <span class="status-pill upcoming">即将发布</span>
      <div>
        <h2 id="next-version-title">v{{ upcomingRelease.version }} 尚未发布</h2>
        <p>后续 0.4.x 版本将根据稳定性验证与用户反馈继续迭代。未完成发布门禁前，本页不会提前提供安装包或校验值。</p>
      </div>
      <RouterLink class="button button-secondary" to="/changelog">查看开发计划</RouterLink>
    </div>
  </section>

  <section class="section-compact section-dark">
    <div class="container history-row">
      <div>
        <History :size="25" aria-hidden="true" />
        <div><h2>查找历史版本</h2><p>历史安装包与 Release 说明统一保存在 GitHub。</p></div>
      </div>
      <a class="button button-dark" href="https://github.com/oyg123less/sub2api-desktop/releases" target="_blank" rel="noreferrer">
        <ExternalLink :size="17" aria-hidden="true" />
        打开全部 Releases
      </a>
    </div>
  </section>
</template>

<style scoped>
.download-hero {
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.download-hero-layout {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(420px, 1.1fr);
  gap: 72px;
  align-items: center;
  padding-block: 64px 70px;
}

.download-hero-copy {
  max-width: 590px;
}

.download-hero-copy h1 {
  margin-bottom: 18px;
  font-size: 48px;
}

.download-hero-lede {
  margin-bottom: 22px;
  color: var(--ink-soft);
  font-size: 18px;
  line-height: 1.7;
}

.download-signals {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
}

.download-signals > span:last-child {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--ink-soft);
  font-size: 13px;
  font-weight: 680;
}

.download-signals > span:last-child svg {
  color: var(--green);
}

.download-secondary-actions {
  margin-top: 28px;
}

.download-asset {
  min-width: 0;
  padding: 28px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  box-shadow: var(--shadow-md, 0 14px 34px rgba(28, 34, 30, 0.11));
  transition:
    transform var(--motion-normal) var(--ease-out),
    box-shadow var(--motion-normal) var(--ease-out),
    border-color var(--motion-normal) ease;
}

@media (hover: hover) and (pointer: fine) {
  .download-asset:hover {
    border-color: rgba(189, 81, 39, 0.28);
    box-shadow: var(--shadow-hero);
    transform: translateY(-3px);
  }
}

.asset-heading {
  display: flex;
  align-items: center;
  gap: 14px;
}

.asset-icon {
  display: grid;
  width: 46px;
  height: 46px;
  flex: 0 0 46px;
  place-items: center;
  border-radius: 6px;
  background: var(--amber-soft);
  color: var(--amber-dark);
}

.asset-heading p {
  margin-bottom: 2px;
  color: var(--amber-dark);
  font-size: 12px;
  font-weight: 760;
}

.asset-heading h2 {
  margin-bottom: 0;
  font-size: 23px;
}

.asset-filename {
  margin: 22px 0 0;
  padding: 11px 12px;
  overflow-wrap: anywhere;
  border-radius: 6px;
  background: var(--surface-muted);
  color: var(--ink-soft);
  font-size: 13px;
}

.asset-facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 20px 0;
  border-block: 1px solid var(--border);
}

.asset-facts > div {
  display: grid;
  min-width: 0;
  gap: 3px;
  padding: 13px 12px;
}

.asset-facts > div:nth-child(2),
.asset-facts > div:nth-child(3) {
  border-left: 1px solid var(--border);
}

.asset-facts dt {
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 680;
}

.asset-facts dd {
  min-width: 0;
  margin: 0;
  color: var(--ink);
  font-size: 13px;
  font-weight: 700;
}

.asset-checksum {
  grid-column: 1 / -1;
  grid-template-columns: minmax(0, 1fr);
  border-top: 1px solid var(--border);
}

.asset-checksum dd {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 40px;
  gap: 8px;
  align-items: center;
}

.asset-checksum code {
  display: block;
  overflow-wrap: anywhere;
  color: var(--ink-soft);
  font-size: 11px;
  font-weight: 600;
  line-height: 1.45;
}

.asset-download {
  width: 100%;
  margin-top: 16px;
}

.asset-source {
  margin: 10px 0 0;
  color: var(--ink-soft);
  font-size: 12px;
  text-align: center;
}

.verification-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.75fr) minmax(0, 1.25fr);
  gap: 72px;
  align-items: center;
}

.verification-layout h2 {
  margin-bottom: 10px;
}

.verification-layout > div:first-child p:last-child {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.verification-layout .code-block {
  margin: 0;
}

.install-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(280px, 0.75fr);
  gap: 72px;
}

.install-notes {
  display: grid;
  align-content: start;
  gap: 15px;
  padding-top: 82px;
}

.smartscreen-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.78fr) minmax(0, 1.22fr);
  gap: 72px;
}

.smartscreen-layout > div:first-child p:last-child {
  color: var(--ink-soft);
}

.smartscreen-steps {
  border-top: 1px solid var(--border);
}

.smartscreen-steps p {
  display: grid;
  grid-template-columns: 26px minmax(0, 1fr);
  gap: 13px;
  margin: 0;
  padding-block: 18px;
  border-bottom: 1px solid var(--border);
}

.smartscreen-steps svg {
  margin-top: 3px;
  color: var(--teal);
}

.future-release {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 24px;
  align-items: center;
}

.future-release h2 {
  margin-bottom: 6px;
}

.future-release p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.history-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 30px;
}

.history-row > div {
  display: flex;
  align-items: center;
  gap: 18px;
}

.history-row > div > svg {
  flex: 0 0 auto;
  color: #ef9d70;
}

.history-row h2 {
  margin-bottom: 5px;
  font-size: 24px;
}

.history-row p {
  margin-bottom: 0;
}

@media (max-width: 1000px) {
  .download-hero-layout {
    gap: 40px;
  }
}

@media (max-width: 900px) {
  .verification-layout,
  .install-grid,
  .smartscreen-layout {
    grid-template-columns: 1fr;
    gap: 40px;
  }

  .install-notes {
    padding-top: 0;
  }

  .future-release {
    grid-template-columns: auto 1fr;
  }

  .future-release .button {
    grid-column: 2;
    justify-self: start;
  }
}

@media (max-width: 820px) {
  .download-hero-layout {
    grid-template-columns: 1fr;
    gap: 38px;
    padding-block: 52px 58px;
  }

  .download-hero-copy {
    max-width: 680px;
  }
}

@media (max-width: 640px) {
  .download-hero-copy h1 {
    font-size: 38px;
  }

  .download-hero-lede {
    font-size: 16px;
  }

  .download-secondary-actions,
  .future-release .button {
    width: 100%;
  }

  .download-secondary-actions {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .download-secondary-actions .button {
    min-width: 0;
  }

  .download-asset {
    padding: 21px;
  }

  .asset-facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .asset-facts > div:nth-child(3) {
    grid-column: 1 / -1;
    border-top: 1px solid var(--border);
    border-left: 0;
  }

  .future-release {
    grid-template-columns: 1fr;
  }

  .future-release .button {
    grid-column: 1;
  }

  .history-row,
  .history-row > div {
    align-items: flex-start;
  }

  .history-row {
    flex-direction: column;
  }

  .history-row .button {
    width: 100%;
  }
}

@media (max-width: 360px) {
  .download-hero-layout {
    padding-block: 44px 48px;
  }

  .download-secondary-actions {
    grid-template-columns: 1fr;
  }

  .asset-heading {
    align-items: flex-start;
  }

  .asset-facts {
    grid-template-columns: 1fr;
  }

  .asset-facts > div:nth-child(2),
  .asset-facts > div:nth-child(3) {
    grid-column: 1;
    border-top: 1px solid var(--border);
    border-left: 0;
  }

  .asset-checksum {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .download-asset:hover {
    transform: none;
  }
}
</style>
