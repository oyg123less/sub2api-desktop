<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import { useAppStore } from "../store";
import imgAccounts from "../assets/docs/accounts.png";
import imgImport from "../assets/docs/import.png";
import imgProxies from "../assets/docs/proxies.png";
import imgDashboard from "../assets/docs/dashboard.png";
import imgSettings from "../assets/docs/settings.png";
import imgStatistics from "../assets/docs/statistics.png";

const { locale } = useI18n();
const app = useAppStore();
const zh = computed(() => String(locale.value).startsWith("zh"));

interface Shot {
  src: string;
  caption: string;
}
interface CodeBlock {
  label?: string;
  cmd: string;
}
interface Step {
  n: number;
  title: string;
  body: string[];
  codes?: CodeBlock[];
  shots?: Shot[];
}

async function copyCmd(cmd: string) {
  try {
    await navigator.clipboard.writeText(cmd);
    app.toast(zh.value ? "已复制" : "Copied", "success");
  } catch {
    app.toast(zh.value ? "复制失败" : "Copy failed", "error");
  }
}

const intro = computed(() =>
  zh.value
    ? "Sub2API Desktop 把你自己的 ChatGPT 订阅，在本地转成一个 OpenAI 兼容接口（http://127.0.0.1:8080/v1），让 Cherry Studio、Cursor、ChatBox 等工具像用 OpenAI API 一样直接调用。所有数据只保存在你本机，令牌加密存储。"
    : "Sub2API Desktop turns your own ChatGPT subscription into a local OpenAI-compatible endpoint (http://127.0.0.1:8080/v1), so tools like Cherry Studio, Cursor and ChatBox can call it just like the OpenAI API. Everything stays on your machine and tokens are stored encrypted.",
);

const quickPath = computed(() =>
  zh.value
    ? ["添加账号", "（可选）配置代理", "启动服务", "在客户端填入地址与密钥"]
    : ["Add an account", "(Optional) Configure a proxy", "Start the service", "Fill the URL & key in your client"],
);

const steps = computed<Step[]>(() =>
  zh.value
    ? [
        {
          n: 1,
          title: "添加 ChatGPT 账号",
          body: [
            "进入「账号」页面，有两种方式添加账号：",
            "① 点击右上角「登录 ChatGPT 账号」，会打开浏览器走标准 OAuth 登录，在官网授权后自动回填账号。",
            "② 如果你已从别的工具导出了令牌，点击「批量导入」，粘贴 JSON 即可一次导入多个账号。",
          ],
          shots: [{ src: imgAccounts, caption: "账号页右上角：登录 / 批量导入" }],
        },
        {
          n: 2,
          title: "批量导入（JSON 格式）",
          body: [
            "在弹窗中粘贴 JSON，支持两种结构：账号数组 [ {…}, {…} ]，或对象 { \"accounts\": [ … ] }。",
            "每条至少要有 access_token 或 refresh_token；若提供 id_token，会自动解析出邮箱、账号 ID 和套餐。",
            "按 chatgpt_account_id 去重：已存在的账号会更新令牌，而不会重复创建。导入完成后会提示「新增 / 更新 / 跳过」的数量。",
          ],
          shots: [{ src: imgImport, caption: "粘贴 JSON 后点击「批量导入」" }],
        },
        {
          n: 3,
          title: "（可选）配置代理",
          body: [
            "如果需要通过代理访问，进入「代理」页面点击「添加代理」，支持 HTTP / HTTPS / SOCKS5，凭据加密保存。",
            "添加后可点「测试」检测连通性与延迟；回到「账号」页面即可为每个账号单独绑定代理。",
          ],
          shots: [{ src: imgProxies, caption: "代理页右上角：添加代理" }],
        },
        {
          n: 4,
          title: "启动服务",
          body: [
            "回到「仪表盘」，确认至少已添加一个账号后，点击右上角「启动服务」。",
            "启动后即可复制页面上的「接口地址 (Base URL)」和「API 密钥」，这两项就是客户端要填的内容。",
          ],
          shots: [{ src: imgDashboard, caption: "仪表盘：启动服务 + 复制 Base URL / API 密钥" }],
        },
        {
          n: 5,
          title: "在客户端中使用",
          body: [
            "在 Cherry Studio / Cursor / ChatBox 等工具中，模型服务商选择「OpenAI 兼容 (OpenAI-Compatible)」。",
            "Base URL 填 http://127.0.0.1:8080/v1，API Key 填仪表盘上的本地密钥（形如 sk-local-…）。",
            "保存后即可像调用 OpenAI 一样发起对话，请求会自动转发到你的 ChatGPT 账号。",
          ],
        },
        {
          n: 6,
          title: "远程开发接入（SSH 隧道）",
          body: [
            "场景：网关跑在你本地 Windows，但你用 VS Code Remote-SSH 连到远程服务器、在远程用 Codex 插件。此时插件跑在远程，直接填 127.0.0.1:8080 是连不上的（那是远程的本地）。",
            "解决办法：用 SSH「反向端口转发」把远程的 8080 转发回你 Windows 的 8080。这样远程访问 http://127.0.0.1:8080/v1 就等于访问你本地的网关，App 无需任何改动。",
            "方式一：命令行反向隧道（连接时加 -R，注意是 -R 不是 -L）。",
            "方式二（推荐）：写进 ~/.ssh/config，VS Code Remote-SSH 连上后自动转发。",
            "验证：在远程终端执行下面的 curl，能返回模型列表即成功（把 sk-local-… 换成仪表盘上的密钥）。",
            "提示：远程 sshd 需 AllowTcpForwarding yes（默认开启）；仅自用时无需 GatewayPorts。",
          ],
          codes: [
            { label: "方式一：命令行", cmd: "ssh -R 8080:127.0.0.1:8080 user@远程服务器" },
            {
              label: "方式二：~/.ssh/config",
              cmd: "Host myserver\n    HostName 远程IP\n    User user\n    RemoteForward 8080 127.0.0.1:8080",
            },
            {
              label: "验证",
              cmd: 'curl http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-local-…"',
            },
          ],
        },
        {
          n: 7,
          title: "查看统计",
          body: [
            "「统计」页面展示请求量、成功率、Token 消耗与平均延迟趋势，并可查看每条请求日志，方便排查问题。",
          ],
          shots: [{ src: imgStatistics, caption: "统计页：请求量 / 成功率 / Token / 延迟" }],
        },
        {
          n: 8,
          title: "设置说明",
          body: [
            "监听端口：本地 API 服务的端口，改动后需重启服务。",
            "本地 API 密钥：客户端连接使用的密钥，可随时「重新生成」（旧密钥立即失效）。",
						"客户端兼容模式：Standard 为默认；Codex profile 仅用于实验性协议兼容，不能保证规避平台风控。",
            "默认模型：当客户端请求的模型不是 gpt-5* 系列时使用的回退模型。语言可在此切换中文 / English。",
          ],
					shots: [{ src: imgSettings, caption: "设置页：端口 / 密钥 / 客户端兼容模式" }],
        },
      ]
    : [
        {
          n: 1,
          title: "Add a ChatGPT account",
          body: [
            "Open the Accounts page. There are two ways to add an account:",
            "1) Click \"Log in to ChatGPT\" (top right) to run the standard OAuth login in your browser; the account is filled in automatically after you authorize.",
            "2) If you already exported tokens from another tool, click \"Bulk import\" and paste JSON to import many accounts at once.",
          ],
          shots: [{ src: imgAccounts, caption: "Accounts page top right: Log in / Bulk import" }],
        },
        {
          n: 2,
          title: "Bulk import (JSON)",
          body: [
            "Paste JSON in the dialog. Two shapes are supported: an array [ {…}, {…} ], or an object { \"accounts\": [ … ] }.",
            "Each entry needs at least an access_token or refresh_token; if id_token is provided, the email, account ID and plan are parsed automatically.",
            "Entries are deduplicated by chatgpt_account_id: an existing account is updated instead of duplicated. A summary of imported / updated / skipped is shown.",
          ],
          shots: [{ src: imgImport, caption: "Paste JSON, then click Bulk import" }],
        },
        {
          n: 3,
          title: "(Optional) Configure a proxy",
          body: [
            "If you need a proxy, open the Proxies page and click \"Add proxy\". HTTP / HTTPS / SOCKS5 are supported and credentials are stored encrypted.",
            "Use \"Test\" to check connectivity and latency, then bind a proxy per account on the Accounts page.",
          ],
          shots: [{ src: imgProxies, caption: "Proxies page top right: Add proxy" }],
        },
        {
          n: 4,
          title: "Start the service",
          body: [
            "Back on the Dashboard, once at least one account exists, click \"Start service\" (top right).",
            "Then copy the Base URL and API key shown on the page — these are exactly what your client needs.",
          ],
          shots: [{ src: imgDashboard, caption: "Dashboard: Start service + copy Base URL / API key" }],
        },
        {
          n: 5,
          title: "Use it in your client",
          body: [
            "In Cherry Studio / Cursor / ChatBox, choose the \"OpenAI-Compatible\" provider type.",
            "Set Base URL to http://127.0.0.1:8080/v1 and API Key to the local key from the Dashboard (looks like sk-local-…).",
            "Save, then chat as if calling OpenAI — requests are forwarded to your ChatGPT account.",
          ],
        },
        {
          n: 6,
          title: "Remote development (SSH tunnel)",
          body: [
            "Scenario: the gateway runs on your local Windows, but you use VS Code Remote-SSH to a remote server and run the Codex plugin there. The plugin runs remotely, so filling in 127.0.0.1:8080 fails (that's the remote's localhost).",
            "Fix: use SSH reverse port forwarding to forward the remote's 8080 back to your Windows 8080. Then accessing http://127.0.0.1:8080/v1 on the remote hits your local gateway — no app changes needed.",
            "Option 1: command-line reverse tunnel (use -R when connecting — note it's -R, not -L).",
            "Option 2 (recommended): put it in ~/.ssh/config so VS Code Remote-SSH forwards automatically on connect.",
            "Verify: run the curl below on the remote terminal; a model list means success (replace sk-local-… with your dashboard key).",
            "Note: the remote sshd needs AllowTcpForwarding yes (on by default); GatewayPorts is not needed for personal use.",
          ],
          codes: [
            { label: "Option 1: command line", cmd: "ssh -R 8080:127.0.0.1:8080 user@remote-server" },
            {
              label: "Option 2: ~/.ssh/config",
              cmd: "Host myserver\n    HostName REMOTE_IP\n    User user\n    RemoteForward 8080 127.0.0.1:8080",
            },
            {
              label: "Verify",
              cmd: 'curl http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-local-…"',
            },
          ],
        },
        {
          n: 7,
          title: "Check statistics",
          body: [
            "The Statistics page shows request volume, success rate, token usage and average-latency trends, plus per-request logs for troubleshooting.",
          ],
          shots: [{ src: imgStatistics, caption: "Statistics: volume / success rate / tokens / latency" }],
        },
        {
          n: 8,
          title: "Settings reference",
          body: [
            "Listen port: the port of the local API service; restart the service after changing it.",
            "Local API key: the key clients use to connect; you can Regenerate it anytime (the old key stops working immediately).",
            "Anti-ban: keep \"Inject Codex instructions\" and \"TLS fingerprint\" on to reduce the risk of detection/bans.",
            "Default model: fallback used when a client requests a non gpt-5* model. Language (Chinese / English) is switched here too.",
          ],
					shots: [{ src: imgSettings, caption: "Settings: port / key / client compatibility" }],
        },
      ],
);
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">{{ zh ? "使用文档" : "Documentation" }}</h1>
      <p class="page-desc">{{ zh ? "从添加账号到在客户端调用的完整图文指引" : "A step-by-step guide from adding accounts to calling from your client" }}</p>
    </div>

    <div class="card doc-intro">
      <p style="margin: 0 0 14px">{{ intro }}</p>
      <div class="doc-path">
        <template v-for="(s, i) in quickPath" :key="i">
          <span class="doc-chip">{{ i + 1 }}. {{ s }}</span>
          <Icon v-if="i < quickPath.length - 1" name="external" :size="13" class="doc-arrow" />
        </template>
      </div>
    </div>

    <div v-for="step in steps" :key="step.n" class="card doc-step">
      <div class="doc-step-head">
        <span class="doc-step-num">{{ step.n }}</span>
        <h3 class="doc-step-title">{{ step.title }}</h3>
      </div>
      <p v-for="(line, i) in step.body" :key="i" class="doc-line">{{ line }}</p>
      <div v-for="(c, i) in step.codes" :key="'c' + i" class="doc-code">
        <div class="doc-code-head">
          <span class="doc-code-label">{{ c.label }}</span>
          <button class="btn btn-ghost btn-sm" @click="copyCmd(c.cmd)">
            <Icon name="copy" :size="13" /> {{ zh ? "复制" : "Copy" }}
          </button>
        </div>
        <pre>{{ c.cmd }}</pre>
      </div>
      <figure v-for="(shot, i) in step.shots" :key="i" class="doc-figure">
        <img :src="shot.src" :alt="shot.caption" loading="lazy" />
        <figcaption>{{ shot.caption }}</figcaption>
      </figure>
    </div>

    <div class="card doc-note">
      <div class="flex items-center gap-8" style="margin-bottom: 8px">
        <Icon name="warn" :size="16" style="color: var(--warn)" />
        <strong>{{ zh ? "注意事项" : "Notes" }}</strong>
      </div>
      <p class="doc-line" style="margin: 0">
        {{ zh
			? "使用非官方转发存在账号被 OpenAI 限制的风险，客户端兼容措施不能保证账号安全。AES 密钥与数据库位于同一数据目录，不等同于 Windows DPAPI。"
			: "Unofficial forwarding can lead to account restrictions, and client compatibility measures cannot guarantee account safety. The AES key shares the data directory with the database and is not equivalent to Windows DPAPI." }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.doc-intro {
  border-color: var(--primary-soft);
}
.doc-path {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.doc-chip {
  background: var(--primary-soft);
  color: var(--primary-hover);
  padding: 5px 11px;
  border-radius: 999px;
  font-size: 12.5px;
  font-weight: 550;
}
.doc-arrow {
  color: var(--text-faint);
}
.doc-step {
  margin-top: 16px;
}
.doc-step-head {
  display: flex;
  align-items: center;
  gap: 11px;
  margin-bottom: 12px;
}
.doc-step-num {
  width: 26px;
  height: 26px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 650;
}
.doc-step-title {
  margin: 0;
  font-size: 15.5px;
  font-weight: 620;
}
.doc-line {
  color: var(--text-dim);
  margin: 0 0 8px;
  font-size: 13.5px;
  line-height: 1.7;
}
.doc-code {
  margin: 10px 0;
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  background: var(--bg-elev);
}
.doc-code-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px 6px 12px;
  border-bottom: 1px solid var(--border-soft);
}
.doc-code-label {
  font-size: 12px;
  color: var(--text-dim);
  font-weight: 550;
}
.doc-code pre {
  margin: 0;
  padding: 12px 14px;
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
}
.doc-figure {
  margin: 14px 0 2px;
}
.doc-figure img {
  width: 100%;
  border: 1px solid var(--border);
  border-radius: 10px;
  display: block;
}
.doc-figure figcaption {
  margin-top: 7px;
  font-size: 12px;
  color: var(--text-faint);
  text-align: center;
}
.doc-note {
  margin-top: 16px;
}
</style>
