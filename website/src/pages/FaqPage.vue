<script setup lang="ts">
import { ArrowRight, ChevronDown, Search, SearchX, X } from "lucide-vue-next";
import { computed, nextTick, ref } from "vue";
import { RouterLink } from "vue-router";
import PageIntro from "../components/PageIntro.vue";
import { stableRelease, upcomingRelease } from "../config/releases";

type FaqCategory = "本地服务" | "代理与账号" | "Codex 与 SSH" | "云同步与共享";
type FaqVersion = "current" | "mixed";

interface FaqItem {
  id: string;
  category: FaqCategory;
  question: string;
  paragraphs: string[];
  steps?: string[];
  note?: string;
  keywords: string[];
  version?: FaqVersion;
}

const currentVersion = `v${stableRelease.version}`;
const nextVersion = `v${upcomingRelease.version}`;
const categories: Array<"全部" | FaqCategory> = ["全部", "本地服务", "代理与账号", "Codex 与 SSH", "云同步与共享"];

const faqs: FaqItem[] = [
  {
    id: "localhost-connection",
    category: "本地服务",
    question: "127.0.0.1:8080 连接失败，先检查什么？",
    paragraphs: [
      "127.0.0.1 只代表正在运行客户端的那台电脑。先确认 Amber 左下角后台状态正常，仪表盘中的本地服务显示“运行中”，再核对客户端 Base URL、端口和本地 API Key。默认地址是 http://127.0.0.1:8080/v1。",
      "如果客户端已经自动追加 /v1，不要在地址中重复填写。WSL、Dev Container 或 SSH Remote 中的 127.0.0.1 也不是 Windows 主机，应改用对应的远程注入或 SSH 隧道。",
    ],
    steps: ["打开仪表盘并启动服务。", "确认客户端端口与 Amber 设置一致。", "重新复制本地 API Key，并重新加载客户端。"],
    keywords: ["localhost", "127.0.0.1", "8080", "连接拒绝", "base url", "api key", "wsl"],
  },
  {
    id: "bad-gateway",
    category: "本地服务",
    question: "Codex 返回 502 Bad Gateway 怎么处理？",
    paragraphs: [
      `${currentVersion} 的常见原因是已经写入 Codex 配置，但 Amber 本地服务尚未启动。打开 Amber，在仪表盘启动服务，确认状态变为“运行中”，然后重新发起 Codex 请求。若仍失败，再查看统计页中的最近错误，区分本地连接问题和上游账号问题。`,
      `${nextVersion} 计划提供“启动服务并注入”：只有本地健康检查和 /v1/models 验证通过后才写入 Codex 配置。该行为仍是即将发布内容，不能按当前版本使用。`,
    ],
    steps: ["打开 Amber。", "在仪表盘启动服务并确认运行中。", "重新加载 Codex 后再次请求。"],
    keywords: ["502", "bad gateway", "responses", "服务未启动", "注入"],
    version: "mixed",
  },
  {
    id: "service-stopped",
    category: "本地服务",
    question: "本地服务显示已停止或启动失败怎么办？",
    paragraphs: [
      "先在仪表盘手动启动服务。若状态很快回到“已停止”或显示“启动失败”，进入设置中的运行诊断，检查 Sidecar、数据目录、监听端口和健康检查结果。不要用反复卸载代替诊断。",
      "如果已启用“启动应用时自动开启服务”，仍应先解决诊断中报告的具体阻断项；自动启动不会绕过端口占用或数据目录错误。",
    ],
    keywords: ["已停止", "启动失败", "sidecar", "健康检查", "诊断", "自动启动"],
  },
  {
    id: "port-conflict",
    category: "本地服务",
    question: "8080 端口被占用时该怎么做？",
    paragraphs: [
      "停止占用该端口且不再需要的程序，或在 Amber 设置中改用一个未占用端口。端口改变后，所有客户端和 Codex 配置中的 Base URL 都必须同步更新。",
      `${nextVersion} 计划在注入前阻止继续操作，并显示冲突端口和占用信息；当前 ${currentVersion} 用户应先确认服务实际启动成功，再重新执行注入。`,
    ],
    note: "不要关闭来源不明的系统进程。无法确认占用者时，优先改用其他端口。",
    keywords: ["8080", "端口冲突", "address already in use", "监听", "占用"],
    version: "mixed",
  },
  {
    id: "proxy-tun",
    category: "代理与账号",
    question: "代理应该怎么配置，普通用户需要开启 TUN 吗？",
    paragraphs: [
      "Amber 支持 HTTP、HTTPS 和 SOCKS5 代理。先在代理页添加并测试，再绑定到账号；云连接可在系统代理、Amber 已保存代理和直连之间单独选择。代理测试通过后，还应执行账号测试，因为真实请求还会涉及凭据、模型权限和目标站点。",
      "普通用户不需要为了 Amber Cloud 专门开启 TUN。TUN 会改变系统 DNS 和路由，若与系统代理或账号代理叠加，反而可能让诊断更困难。必须使用 TUN 时，先避免重复代理，再逐项测试 Amber 的代理链路和云网络探测。",
    ],
    keywords: ["代理", "tun", "http", "https", "socks5", "系统代理", "网络出口"],
  },
  {
    id: "account-test-codex-fails",
    category: "代理与账号",
    question: "账号测试成功，为什么 Codex 仍然不可用？",
    paragraphs: [
      "账号测试说明该账号当时能够通过所选网络访问上游，不代表本地 API 服务、Codex 配置和运行环境都正确。继续检查 Amber 服务是否运行、Base URL 与端口是否匹配、本地 API Key 是否最新，以及所选模型是否对该账号开放。",
      `在 ${currentVersion} 中，应先启动服务，再执行一键注入，并重新加载 Codex。若 Codex 运行在 WSL、容器或远程主机，必须使用对应的远程接入方式，不能直接引用 Windows 的 127.0.0.1。${nextVersion} 将在写入配置前完成这些本地验证。`,
    ],
    keywords: ["账号测试", "codex", "模型权限", "配置", "api key", "wsl", "容器"],
    version: "mixed",
  },
  {
    id: "reverse-tunnel-online",
    category: "Codex 与 SSH",
    question: "SSH 反向隧道为什么要求本机 Amber 保持在线？",
    paragraphs: [
      "反向隧道把远程服务器上的 Codex 请求送回安装 Amber 的电脑，再使用这台电脑上的账号、代理和网络出口访问上游。本机 Amber、SSH 连接或目标卡片中的路由开关任一离线，链路都会中断。",
      "若远程服务器本身可以直接访问目标 Base URL，可改用直连模式；直连不会建立回流本机的隧道。",
    ],
    keywords: ["ssh", "反向隧道", "本机在线", "远程 codex", "回流", "路由开关"],
  },
  {
    id: "host-key",
    category: "Codex 与 SSH",
    question: "第一次连接 SSH 时，主机密钥该如何确认？",
    paragraphs: [
      "点击“测试连接”取得远程服务器的 SHA256 主机指纹，然后通过服务器控制台查看指纹，或向服务器管理员从可信渠道索取。只有两边完全一致时，才在本机 Amber 点击“信任并继续”。",
      "这里确认的是表单中填写的 SSH 服务器，不是 Amber 电脑、代理服务器或上游服务。已经信任的主机指纹若意外变化，应取消连接并先查明服务器重装、密钥轮换或中间人风险。",
    ],
    keywords: ["ssh", "主机密钥", "host key", "sha256", "指纹", "信任"],
  },
  {
    id: "cloud-network-diagnostics",
    category: "云同步与共享",
    question: "云同步在 DNS、TCP、TLS 或 HTTP 阶段失败分别意味着什么？",
    paragraphs: [
      "DNS 失败表示域名未解析；TCP 失败表示无法建立到目标端口的连接；TLS 失败通常与证书链、SNI、系统时间或网络拦截有关；HTTP 失败表示连接已建立，但服务返回了错误状态或请求被拒绝。超时则要结合最后完成的阶段判断。",
      `在云账户的“连接设置”中选择系统代理、Amber 已保存代理或直连，运行网络探测，探测成功后应用并重试同步。部分网络无法连接 workers.dev；api.amberapp.asia 是 ${nextVersion} 的计划首选入口，目前尚不能假定已经可用。`,
    ],
    keywords: ["云同步", "dns", "tcp", "tls", "http", "workers.dev", "连接设置", "超时"],
    version: "mixed",
  },
  {
    id: "owner-device-offline",
    category: "云同步与共享",
    question: "共享者设备离线后，接收者为什么无法调用？",
    paragraphs: [
      "OAuth 共享默认由共享者设备回流：Owner Relay 负责把请求送到共享者的 Amber，最终上游请求由该设备使用本地账号、代理和网络出口发出。因此共享者设备、Amber 或 Relay 离线时，请求会返回设备离线类错误。",
      "等待共享者设备恢复在线，或请共享者检查 Amber、云登录和共享路由。只有明确配置为 Worker 直连的兼容 API Key 共享不依赖拥有者设备；不要把两种路径混为一谈。",
    ],
    keywords: ["共享者", "设备离线", "owner relay", "oauth", "回流", "worker direct"],
  },
  {
    id: "multi-device-routing",
    category: "云同步与共享",
    question: "同一云账号有多台设备在线时，共享请求走哪一台？",
    paragraphs: [
      `${currentVersion} 当前按云用户的在线主设备优先选择，主设备离线时可能选择其他在线设备；它不能保证被选设备持有目标账号，也不是可靠的主备切换方案。`,
      `${nextVersion} 计划让新共享默认绑定创建共享的具体电脑，并允许用户显式配置具备同一目标账号且网络健康的备用设备。上游请求开始后不会跨设备重放。该设备定向行为尚未作为稳定版发布。`,
    ],
    keywords: ["同账号", "多设备", "主设备", "备用设备", "设备定向", "路由"],
    version: "mixed",
  },
  {
    id: "cloud-account-workspaces",
    category: "云同步与共享",
    question: "切换云账号后，本地数据和工作区放在哪里？",
    paragraphs: [
      `${currentVersion} 的普通账号、代理和同步队列尚未完整按云用户隔离。当前不要在同一数据目录中反复登录不同云账号，以免数据归属和待同步队列混在一起。`,
      `${nextVersion} 计划为每个云账号建立独立工作区，各自保存账号、代理、同步队列、Guest Key、日志和 SSH 目标。退出登录不会删除数据或解除工作区归属；切换账号将改为切换到对应工作区。该机制仍是即将发布内容。`,
    ],
    keywords: ["切换云账号", "工作区", "数据目录", "隔离", "退出登录", "同步队列"],
    version: "mixed",
  },
];

const query = ref("");
const activeCategory = ref<"全部" | FaqCategory>("全部");
const searchInput = ref<HTMLInputElement | null>(null);

async function clearSearch() {
  query.value = "";
  await nextTick();
  searchInput.value?.focus();
}

const filteredFaqs = computed(() => {
  const term = query.value.trim().toLocaleLowerCase();

  return faqs.filter((faq) => {
    if (activeCategory.value !== "全部" && faq.category !== activeCategory.value) return false;
    if (!term) return true;

    const searchable = [faq.question, faq.category, ...faq.paragraphs, ...(faq.steps ?? []), faq.note ?? "", ...faq.keywords]
      .join(" ")
      .toLocaleLowerCase();
    return searchable.includes(term);
  });
});
</script>

<template>
  <div class="faq-page">
    <PageIntro
      eyebrow="FAQ · 排障"
      title="常见问题与排障"
      description="先确认问题发生在哪一段链路，再修改配置。页面同时标明当前稳定版与即将发布行为，避免把计划中的修复当成现有功能。"
    >
      <div class="version-key" aria-label="版本说明">
        <span class="status-pill stable"><span class="status-dot" />{{ currentVersion }} 当前稳定版</span>
        <span class="status-pill upcoming"><span class="status-dot" />{{ nextVersion }} 即将发布</span>
      </div>
    </PageIntro>

    <section class="section-compact" aria-labelledby="faq-results-heading">
      <div class="container faq-layout">
        <div class="faq-controls" role="search" aria-label="筛选常见问题">
          <label class="search-field">
            <span class="sr-only">搜索问题、错误码或关键词</span>
            <Search :size="19" aria-hidden="true" />
            <input ref="searchInput" v-model="query" type="search" placeholder="搜索 502、端口、SSH、同步..." autocomplete="off" />
            <button v-if="query" type="button" aria-label="清除搜索" title="清除搜索" @click="clearSearch">
              <X :size="18" aria-hidden="true" />
            </button>
          </label>

          <label class="category-field">
            <span>问题分类</span>
            <select v-model="activeCategory">
              <option v-for="category in categories" :key="category" :value="category">{{ category }}</option>
            </select>
          </label>
        </div>

        <div class="faq-results-heading">
          <h2 id="faq-results-heading">排障答案</h2>
          <p aria-live="polite">找到 {{ filteredFaqs.length }} 个问题</p>
        </div>

        <div v-if="filteredFaqs.length" class="faq-list">
          <details v-for="faq in filteredFaqs" :key="faq.id" :id="faq.id" :open="faq.id === 'bad-gateway'">
            <summary>
              <span class="faq-summary-copy">
                <span class="faq-category">{{ faq.category }}</span>
                <span class="faq-question">{{ faq.question }}</span>
              </span>
              <span class="faq-summary-meta">
                <span v-if="faq.version === 'mixed'" class="status-pill warning">版本差异</span>
                <ChevronDown class="faq-chevron" :size="20" aria-hidden="true" />
              </span>
            </summary>
            <div class="faq-answer">
              <p v-for="paragraph in faq.paragraphs" :key="paragraph">{{ paragraph }}</p>
              <ol v-if="faq.steps" class="answer-steps">
                <li v-for="step in faq.steps" :key="step">{{ step }}</li>
              </ol>
              <p v-if="faq.note" class="answer-note"><strong>注意：</strong>{{ faq.note }}</p>
            </div>
          </details>
        </div>

        <div v-else class="empty-state">
          <SearchX :size="30" aria-hidden="true" />
          <h3>没有匹配的问题</h3>
          <p>换一个关键词，或将分类切回“全部”。</p>
        </div>
      </div>
    </section>

    <section class="section-compact section-muted" aria-labelledby="faq-next-heading">
      <div class="container faq-next">
        <div>
          <h2 id="faq-next-heading">还需要上下文？</h2>
          <p>使用文档提供完整操作路径；安全页说明本地、云端与共享链路分别处理哪些数据。</p>
        </div>
        <div class="action-row">
          <RouterLink class="button button-secondary" to="/docs">查看使用文档 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
          <RouterLink class="button button-secondary" to="/security">查看安全边界 <ArrowRight :size="17" aria-hidden="true" /></RouterLink>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.version-key {
  display: flex;
  flex-wrap: wrap;
  gap: 9px;
  margin-top: 24px;
}

.faq-layout {
  max-width: 980px;
}

.faq-controls {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 230px;
  gap: 14px;
  margin-bottom: 42px;
}

.search-field {
  position: relative;
  display: flex;
  min-width: 0;
  height: 48px;
  align-items: center;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  background: var(--surface);
  color: var(--ink-soft);
}

.search-field:focus-within,
.category-field select:focus-visible {
  border-color: var(--teal);
  box-shadow: 0 0 0 3px rgba(53, 106, 112, 0.14);
}

.search-field > svg {
  flex: 0 0 auto;
  margin-left: 15px;
}

.search-field input {
  width: 100%;
  min-width: 0;
  height: 100%;
  padding: 0 12px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ink);
}

.search-field input::-webkit-search-cancel-button {
  display: none;
}

.search-field button {
  display: grid;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  margin-right: 4px;
  place-items: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--ink-soft);
  cursor: pointer;
}

.search-field button:hover {
  background: var(--surface-muted);
  color: var(--ink);
}

.category-field {
  display: grid;
  gap: 5px;
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.category-field select {
  width: 100%;
  height: 48px;
  padding: 0 38px 0 12px;
  border: 1px solid var(--border-strong);
  border-radius: 6px;
  outline: 0;
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
}

.faq-results-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.faq-results-heading h2,
.faq-results-heading p {
  margin-bottom: 0;
}

.faq-results-heading h2 {
  font-size: 28px;
}

.faq-results-heading p {
  color: var(--ink-soft);
  font-size: 14px;
}

.faq-list {
  border-block: 1px solid var(--border);
}

.faq-list details + details {
  border-top: 1px solid var(--border);
}

.faq-list details[open] {
  background: var(--surface);
}

.faq-list summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 24px;
  padding: 23px 18px;
  align-items: center;
  cursor: pointer;
  list-style: none;
}

.faq-list summary::-webkit-details-marker {
  display: none;
}

.faq-list summary:hover .faq-question {
  color: var(--amber-dark);
}

.faq-summary-copy {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.faq-category {
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.faq-question {
  color: var(--ink);
  font-size: 18px;
  font-weight: 740;
  line-height: 1.45;
}

.faq-summary-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.faq-chevron {
  flex: 0 0 auto;
  color: var(--ink-soft);
  transition: transform 160ms ease;
}

details[open] .faq-chevron {
  transform: rotate(180deg);
}

.faq-answer {
  max-width: 820px;
  padding: 0 76px 28px 18px;
  color: var(--ink-soft);
}

.faq-answer p {
  margin-bottom: 13px;
}

.answer-steps {
  margin: 18px 0 14px;
  padding-left: 22px;
  color: var(--ink);
}

.answer-steps li + li {
  margin-top: 5px;
}

.answer-note {
  padding: 12px 14px;
  border-left: 3px solid var(--warning);
  background: var(--warning-soft);
  color: var(--ink);
}

.empty-state {
  display: grid;
  min-height: 260px;
  place-items: center;
  align-content: center;
  border-block: 1px solid var(--border);
  color: var(--ink-soft);
  text-align: center;
}

.empty-state h3 {
  margin: 12px 0 5px;
}

.empty-state p {
  margin-bottom: 0;
}

.faq-next {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 36px;
}

.faq-next h2 {
  margin-bottom: 8px;
  font-size: 28px;
}

.faq-next p {
  max-width: 680px;
  margin-bottom: 0;
  color: var(--ink-soft);
}

.faq-next .action-row {
  flex: 0 0 auto;
}

@media (max-width: 760px) {
  .faq-controls {
    grid-template-columns: 1fr;
  }

  .faq-list summary {
    gap: 12px;
    padding-inline: 10px;
  }

  .faq-summary-meta .status-pill {
    display: none;
  }

  .faq-answer {
    padding-inline: 10px 42px;
  }

  .faq-next {
    display: grid;
  }

  .faq-next .action-row {
    align-items: stretch;
  }
}

@media (max-width: 480px) {
  .faq-results-heading {
    display: block;
  }

  .faq-results-heading p {
    margin-top: 6px;
  }

  .faq-question {
    font-size: 16px;
  }

  .faq-next .button {
    width: 100%;
  }
}
</style>
