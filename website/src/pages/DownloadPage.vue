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
import PageIntro from "../components/PageIntro.vue";
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
  <PageIntro
    eyebrow="Windows 下载"
    :title="`下载 Amber v${stableRelease.version}`"
    description="当前正式版本面向 Windows 10 / 11 x64。安装包由 GitHub Releases 直接托管，官网不会通过 Worker 转发可执行文件。"
  >
    <div class="intro-actions">
      <a class="button button-primary" :href="stableRelease.downloadUrl">
        <Download :size="18" aria-hidden="true" />
        下载 Windows x64
      </a>
      <a class="button button-secondary" :href="stableRelease.releaseUrl" target="_blank" rel="noreferrer">
        <ExternalLink :size="17" aria-hidden="true" />
        查看 GitHub Release
      </a>
    </div>
  </PageIntro>

  <section class="section" aria-labelledby="release-title">
    <div class="container release-layout">
      <div>
        <div class="release-heading">
          <span class="status-pill stable"><span class="status-dot"></span>当前稳定版</span>
          <h2 id="release-title">Amber v{{ stableRelease.version }}</h2>
          <p>发布日期按中国标准时间显示。当前 Release 仅提供 NSIS EXE 安装包。</p>
        </div>

        <dl class="data-list release-facts">
          <div><dt>平台</dt><dd>Windows 10 / 11 · x64</dd></div>
          <div><dt>发布日期</dt><dd>{{ formatPublishedDate(stableRelease.publishedAt) }}</dd></div>
          <div><dt>安装包</dt><dd class="mono">{{ stableRelease.installerName }}</dd></div>
          <div><dt>文件大小</dt><dd>{{ formatFileSize(stableRelease.installerSizeBytes) }}（{{ stableRelease.installerSizeBytes.toLocaleString("zh-CN") }} 字节）</dd></div>
          <div><dt>托管位置</dt><dd>GitHub Releases</dd></div>
        </dl>
      </div>

      <aside class="download-panel" aria-label="安装包下载">
        <Monitor :size="30" aria-hidden="true" />
        <h3>Windows x64 安装包</h3>
        <p>{{ stableRelease.installerName }}</p>
        <a class="button button-primary" :href="stableRelease.downloadUrl">
          <Download :size="18" aria-hidden="true" />
          开始下载
        </a>
        <span>从 github.com 下载 · {{ formatFileSize(stableRelease.installerSizeBytes) }}</span>
      </aside>
    </div>
  </section>

  <section class="section section-muted" aria-labelledby="checksum-title">
    <div class="container checksum-layout">
      <div>
        <p class="eyebrow">完整性校验</p>
        <h2 id="checksum-title">安装前核对 SHA-256</h2>
        <p>下载完成后计算文件哈希。只有文件名、来源与下列 SHA-256 全部一致时再运行安装包。</p>
      </div>
      <div class="checksum-box">
        <code>{{ stableRelease.sha256 }}</code>
        <button class="icon-button" type="button" :aria-label="copied ? '校验值已复制' : '复制 SHA-256'" @click="copyChecksum">
          <Check v-if="copied" :size="19" aria-hidden="true" />
          <Copy v-else :size="19" aria-hidden="true" />
        </button>
        <span class="sr-only" aria-live="polite">{{ copied ? "SHA-256 已复制" : "" }}</span>
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
.intro-actions {
  display: flex;
  margin-top: 28px;
  flex-wrap: wrap;
  gap: 12px;
}

.release-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(300px, 0.75fr);
  gap: 72px;
  align-items: start;
}

.release-heading {
  margin-bottom: 28px;
}

.release-heading h2 {
  margin: 14px 0 8px;
}

.release-heading p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.download-panel {
  padding: 28px;
  border: 1px solid var(--border);
  border-radius: 7px;
  background: var(--surface);
  box-shadow: 0 12px 34px rgba(31, 33, 28, 0.08);
}

.download-panel > svg {
  margin-bottom: 19px;
  color: var(--amber);
}

.download-panel h3 {
  margin-bottom: 5px;
}

.download-panel p {
  overflow-wrap: anywhere;
  color: var(--ink-soft);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 13px;
}

.download-panel .button {
  width: 100%;
  margin-block: 11px;
}

.download-panel span {
  display: block;
  color: var(--ink-soft);
  font-size: 12px;
  text-align: center;
}

.checksum-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.7fr) minmax(0, 1.3fr);
  gap: 28px 72px;
  align-items: end;
}

.checksum-layout > div:first-child p:last-child {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.checksum-box {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 40px;
  gap: 11px;
  align-items: center;
}

.checksum-box code {
  display: block;
  padding: 15px;
  overflow-wrap: anywhere;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  font-size: 13px;
  line-height: 1.55;
}

.checksum-layout .code-block {
  grid-column: 2;
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

@media (max-width: 900px) {
  .release-layout,
  .checksum-layout,
  .install-grid,
  .smartscreen-layout {
    grid-template-columns: 1fr;
    gap: 40px;
  }

  .checksum-layout .code-block {
    grid-column: 1;
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

@media (max-width: 640px) {
  .intro-actions .button,
  .future-release .button {
    width: 100%;
  }

  .checksum-box {
    grid-template-columns: minmax(0, 1fr) 40px;
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
</style>
