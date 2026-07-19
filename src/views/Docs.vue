<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { useI18n } from "vue-i18n";
import Icon from "../components/Icon.vue";
import { useAppStore } from "../store";
import imgAccountDetails from "../assets/docs/account-details.png";
import imgAccounts from "../assets/docs/accounts.png";
import imgCloudRegister from "../assets/docs/cloud-register.png";
import imgCloudWorkspace from "../assets/docs/cloud-workspace.png";
import imgCodex from "../assets/docs/codex.png";
import imgCodexHostKey from "../assets/docs/codex-host-key.png";
import imgCodexLocal from "../assets/docs/codex-local.png";
import imgDashboard from "../assets/docs/dashboard.png";
import imgImport from "../assets/docs/import.png";
import imgModels from "../assets/docs/models.png";
import imgProxies from "../assets/docs/proxies.png";
import imgSettings from "../assets/docs/settings.png";
import imgStatistics from "../assets/docs/statistics.png";

const { locale } = useI18n();
const app = useAppStore();
const docRoot = ref<HTMLElement | null>(null);
const lightbox = ref<HTMLElement | null>(null);
const activeShot = ref<Shot | null>(null);
const zh = computed(() => String(locale.value).startsWith("zh"));

interface Shot {
  src: string;
  caption: string;
}

interface CodeBlock {
  label: string;
  cmd: string;
}

interface Section {
  n: number;
  title: string;
  lead: string;
  body: string[];
  bullets?: string[];
  note?: string;
  steps?: Array<{ title: string; text: string }>;
  codes?: CodeBlock[];
  shots?: Shot[];
}

async function openShot(shot: Shot) {
  activeShot.value = shot;
  await nextTick();
  lightbox.value?.focus();
}

function closeShot() {
  activeShot.value = null;
}

function bulletParts(value: string): { title: string; text: string } {
  const index = value.search(/[：:]/);
  if (index < 0) return { title: "", text: value };
  return { title: value.slice(0, index + 1), text: value.slice(index + 1).trim() };
}

async function copyCmd(cmd: string) {
  try {
    await navigator.clipboard.writeText(cmd);
    app.toast(zh.value ? "已复制" : "Copied", "success");
  } catch {
    app.toast(zh.value ? "复制失败" : "Copy failed", "error");
  }
}

function setAllSections(open: boolean) {
  docRoot.value?.querySelectorAll<HTMLDetailsElement>("details.doc-section").forEach((item) => {
    item.open = open;
  });
}

const quickPath = computed(() => zh.value
  ? ["导入账号", "按需配置代理", "启动本地服务", "一键注入或复制配置", "在客户端验证"]
  : ["Import accounts", "Configure a proxy if needed", "Start the local service", "Inject or copy config", "Verify from the client"]);

const sections = computed<Section[]>(() => zh.value ? [
  {
    n: 1,
    title: "第一次启动与最短使用路径",
    lead: "先确认后台已就绪，再完成账号、服务和客户端三步连接。",
    body: [
      "安装并打开 Amber 后，左下角状态应显示“已就绪”。如果显示“启动失败”，先进入“设置 → 运行诊断”，不要反复重装。",
      "进入“账号”导入至少一个可用账号；回到“仪表盘”确认服务已启动，然后复制 Base URL 和本地 API Key。",
      "普通客户端可以复制 Base URL 与 API Key；Codex 用户优先进入“Codex 接入”，使用本机或远程“一键注入”，避免手工编辑配置文件。",
      "客户端必须选择 OpenAI 兼容服务。Base URL 默认是 http://127.0.0.1:8080/v1，API Key 使用仪表盘显示的本地密钥。",
    ],
    bullets: [
      "仅本机使用时保持“允许局域网访问”关闭。",
      "修改监听端口后，客户端地址也要同步修改。",
      "API Key 属于本地访问凭据，不是 OpenAI 官方 Key。",
    ],
    shots: [{ src: imgDashboard, caption: "仪表盘：服务状态、今日统计、Base URL 与本地 API Key" }],
  },
  {
    n: 2,
    title: "导入账号：Base URL、OAuth 与 JSON",
    lead: "所有导入方式集中在一个入口，并且可以在导入前选择代理。",
    body: [
      "进入“账号”，点击右上角“导入账号”。弹窗提供三种方式：Base URL + API Key、ChatGPT 授权登录、JSON 文件。",
      "Base URL 方式适合 OpenAI 兼容上游；填写完整 /v1 地址、API Key 和便于识别的名称。OAuth 会打开浏览器完成 ChatGPT 授权。",
      "JSON 支持单个文件、多个文件，以及一个文件内包含多个账号。提交前先预览，确认新增、更新、跳过和冲突数量，再执行导入。",
    ],
    bullets: [
      "JSON 可使用账号数组，或 { \"accounts\": [ ... ] } 结构。",
      "已存在的同一账号会更新凭据，不会创建重复行。",
      "代理选项会随本次导入提交；“直连”表示明确清除代理。",
    ],
    note: "不要把包含真实 Token 的 JSON 发到聊天、工单或公开仓库。",
    shots: [
      { src: imgAccounts, caption: "账号列表与统一“导入账号”入口" },
      { src: imgImport, caption: "统一导入弹窗：三种方式在同一处选择" },
    ],
  },
  {
    n: 3,
    title: "账号管理、并发队列与批处理",
    lead: "一行一个账号；常用操作直接完成，详细设置在右侧详情弹窗中调整。",
    body: [
      "账号行会显示状态、并发/排队、额度窗口、Token 和预估费用。右侧开关决定账号是否参与后续调度；关闭后不会再被调用。",
      "闪电按钮测试单个账号，信息按钮打开详情，垃圾桶删除账号。详情中可设置最大并发和排队容量，并查看请求量、Token、费用与额度窗口。",
      "勾选账号后可批量测试或批量删除。全选只选择当前页；每页最多 20 个账号，翻页后已选状态会保留。",
    ],
    bullets: [
      "最大并发：同一账号同时处理的请求上限。",
      "排队容量：并发已满时允许等待的请求数；超过后立即拒绝。",
      "严重认证异常可能自动停用账号；修复凭据并测试成功后再重新开启。",
      "批量删除不可恢复，确认前检查已选数量。",
    ],
    shots: [{ src: imgAccountDetails, caption: "账号详情：调度限制、健康状态、用量和额度窗口" }],
  },
  {
    n: 4,
    title: "代理配置与一键应用",
    lead: "支持 HTTP、HTTPS 和 SOCKS5，可单独绑定，也可一次应用到全部现有账号。",
    body: [
      "进入“代理”添加代理并填写类型、主机、端口及可选凭据。先测试代理，确认 DNS、连接、TLS 和 HTTP 阶段正常。",
      "页面顶部工具栏可把选定代理应用到全部现有账号、清除全部绑定，或启动全部账号连通性测试。单个代理行也可直接应用到全部账号。",
      "批量应用是一次性写入，不是永久全局规则。以后新导入的账号仍需在导入流程中选择代理。",
    ],
    note: "代理测试成功只说明代理链路可用；账号测试还会验证凭据和上游模型权限。",
    shots: [{ src: imgProxies, caption: "代理页面：紧凑批量工具栏、绑定摘要和代理列表" }],
  },
  {
    n: 5,
    title: "启动服务并接入客户端",
    lead: "客户端只需要 Base URL 和本地 API Key。先用模型列表接口排除配置错误。",
    body: [
      "在仪表盘启动服务，复制 Base URL 与 API Key。在 Cherry Studio、Cursor、ChatBox 等客户端中选择“OpenAI 兼容”，填入这两个值。",
      "如果客户端自动补 /v1，请避免最终地址出现 /v1/v1。测试时先请求 /models，再发起一个最小对话。",
      "只有确实需要其他设备访问时才开启局域网访问，并使用 Windows 防火墙限制可信网段。不要直接把本地端口暴露到公网。",
    ],
    codes: [
      { label: "检查模型列表", cmd: 'curl http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-local-替换为你的密钥"' },
      { label: "最小 Chat Completions 请求", cmd: 'curl http://127.0.0.1:8080/v1/chat/completions -H "Authorization: Bearer sk-local-替换为你的密钥" -H "Content-Type: application/json" -d "{\\\"model\\\":\\\"gpt-5.6-sol\\\",\\\"messages\\\":[{\\\"role\\\":\\\"user\\\",\\\"content\\\":\\\"ping\\\"}]}"' },
    ],
  },
  {
    n: 6,
    title: "模型广场与价格说明",
    lead: "模型名和价格来自本地控制 API 的统一目录，不在前端写死。",
    body: [
      "“模型广场”按卡片展示输入、输出、缓存输入和长上下文价格。点击卡片右上角复制按钮即可复制准确模型名。",
      "价格单位是美元 / 1M Tokens，Standard 表示标准档官方价。272K+ 标签表示该模型存在长上下文价格档。",
      "页面底部显示官方来源和价格版本日期；实际可用模型仍取决于账号权限。",
    ],
    shots: [{ src: imgModels, caption: "模型广场：模型名、价格、长上下文档与价格版本" }],
  },
  {
    n: 7,
    title: "统计、预估费用与请求排障",
    lead: "先看整体趋势，再用请求日志定位账号、模型或上游错误。",
    body: [
      "“统计”展示请求数、成功率、Token、平均延迟和预估费用。超过 $1,000 后使用 K/M/B 缩写并保留四位小数，悬停可查看精确金额。",
      "费用按页面标注的 Standard 官方价格估算，不是账单。缓存 Token、长上下文和未知模型回退会影响估算结果。",
      "请求日志包含状态码、模型、Token、延迟、尝试次数和错误类型；排障时优先记录 request_id 和错误类型，不要公开凭据。",
    ],
    shots: [{ src: imgStatistics, caption: "统计页面：趋势、模型分布、价格版本与请求日志" }],
  },
  {
    n: 8,
    title: "Codex 一键注入：本机与远程服务器",
    lead: "优先使用一键注入；首次 SSH 连接必须由当前用户在本机核对并信任服务器指纹。",
    body: [
      "本机接入会先备份当前 ~/.codex/config.toml 与 auth.json，再写入 Amber 的 Base URL、API Key 和所选模型。成功后 Codex 直接使用本机 Amber；需要撤销时点击“恢复”即可还原注入前文件。",
      "远程服务器接入提供“反向隧道”和“直连 Base URL”两种模式。服务器无法访问 ChatGPT 时，应选择反向隧道，让服务器 Codex 通过 SSH 回到本机 Amber，再使用本机代理和账号访问上游。",
      "首次测试连接时，Amber 获取的是远程 SSH 服务器的主机公钥指纹。确认动作发生在当前电脑上的 Amber，不是让服务器管理员在服务器上点击，也不是等待服务器自动确认。",
    ],
    bullets: [
      "本机一键注入：选择模型，核对配置预览，点击“一键注入”；出现“已应用”后重启或重新加载 Codex。",
      "反向隧道：要求本机 Amber、SSH 连接和目标卡片的路由开关持续在线。",
      "直连模式：适合服务器能够直接访问目标 Base URL；已保存目标再次注入时，密码和 API Key 可以留空复用。",
      "服务器要求：sshd 需要 AllowTcpForwarding yes；远程端口必须空闲，个人使用不需要 GatewayPorts。",
      "恢复操作：还原注入前的 config.toml 与 auth.json，并关闭对应反向隧道。",
    ],
    steps: [
      { title: "点击“测试连接”", text: "Amber 使用填写的主机、SSH 端口、用户名和密码连接远程服务器。" },
      { title: "查看确认弹窗", text: "首次连接会弹出“确认服务器主机密钥”，并显示 SHA256:xxxx 指纹。" },
      { title: "在可信终端核对", text: "通过服务器控制台或向管理员取得指纹，不能只凭同一条未知网络连接盲目信任。" },
      { title: "确认是哪台主机", text: "这里确认的是表单中填写的远程 SSH 服务器，不是本机、ChatGPT 或代理服务器。" },
      { title: "在本机 Amber 点击“信任并继续”", text: "这个按钮表示信任主机密钥。只有指纹完全一致时才继续；不一致应取消并检查地址或联系管理员。" },
      { title: "再点击“一键注入”", text: "Amber 备份远端 ~/.codex 文件、写入新配置并建立反向隧道。" },
    ],
    note: "重点注意：“等待确认主机密钥”表示 Amber 正在等待当前用户在本机确认，不是让服务器自动确认。若没有看到确认弹窗，请再次点击“测试连接”或“一键注入”，不要一直等待。",
    codes: [
      { label: "在服务器可信终端查看所有 SSH 主机指纹", cmd: "for key in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf \"$key\" -E sha256; done" },
      { label: "手动备用 ssh -R", cmd: "ssh -R 8080:127.0.0.1:8080 user@远程服务器" },
    ],
    shots: [
      { src: imgCodexLocal, caption: "本机接入：选择模型、预览配置并一键注入" },
      { src: imgCodex, caption: "远程接入：选择反向隧道或直连 Base URL" },
      { src: imgCodexHostKey, caption: "首次连接：核对 SHA256 指纹后在本机 Amber 信任主机密钥" },
    ],
  },
  {
    n: 9,
    title: "云账户、连接码快速共享与好友管理",
    lead: "日常共享只需连接码和临时密码；好友与共享组保留为可选的高级管理。",
    body: [
      "注册时在同一页填写邮箱、主密码和确认密码，勾选恢复风险确认，通过 Turnstile 后创建账号，再输入邮件中的 6 位验证码。主密码无法找回，请离线保存。",
      "登录后可立即同步账号、代理、设置和 Codex 目标。同步失败时先打开“连接设置”，测试系统代理、Amber 代理或直连，然后重试。",
      "最快路径：共享者在“我的共享”选择账号并开始共享，复制 9 位连接码与 6 位临时密码；接收者粘贴整段信息，点击“连接并使用”，共享会自动加入本地调度。",
      "同一临时密码可允许多人领取，但每人得到独立 Guest Key，可单独暂停或移除。刷新临时密码不会影响已连接用户；暂停共享会暂时阻止所有调用。",
      "OAuth 账号通过拥有者设备回源，拥有者的 Amber Agent 必须在线；API Key 账号可选择本机回源或 Worker 直连。Friend Code 和旧共享组仍可用于固定联系人和详细规则。",
    ],
    bullets: [
      "连接码和临时密码只用于领取权限，不是模型 API Key。",
      "每个接收好友使用独立密钥，发起者可以暂停、限流、修改额度或撤销。",
      "云端保存的是加密同步数据；仍需保护主密码和本机数据目录。",
    ],
    shots: [
      { src: imgCloudRegister, caption: "云账户注册：邮箱、主密码、确认和人机验证位于同一流程" },
      { src: imgCloudWorkspace, caption: "云工作台：复制连接信息，或粘贴后一次点击连接并使用" },
    ],
  },
  {
    n: 10,
    title: "设置、数据目录与常见故障",
    lead: "设置页负责模型、调度、日志、语言、数据目录和内置诊断。",
    body: [
      "默认模型用于客户端请求无法直接匹配时的回退；账号策略建议保持“额度感知”。日志保留天数和最大行数会影响磁盘占用。",
      "移动数据目录前先停止高频请求并保留备份。不要手工复制正在使用的数据库文件。",
      "遇到后台失联、代理失败、云同步失败或端口占用时，展开“运行诊断”，按失败阶段处理，再导出脱敏报告。",
    ],
    bullets: [
      "The session has expired：重新登录对应云账户或 OAuth 账号。",
      "客户端 401：检查是否使用了最新本地 API Key。",
      "客户端连接失败：确认服务已启动、端口一致，且没有重复 /v1。",
      "账号自动停用：查看详情中的状态原因，修复后先测试再开启。",
      "云同步被拒绝：检查登录状态、Worker 地址、网络诊断和待同步数据大小。",
    ],
    shots: [{ src: imgSettings, caption: "设置页面：默认模型、调度、日志、数据目录和运行诊断" }],
  },
] : [
  {
    n: 1,
    title: "First launch and shortest path",
    lead: "Make sure the backend is ready, then connect an account, the service, and your client.",
    body: [
      "After opening Amber, the bottom-left backend state should read Ready. If startup fails, use Settings → Diagnostics before reinstalling.",
      "Import at least one usable account, return to Dashboard, start the service, then copy the Base URL and local API key.",
      "Choose an OpenAI-compatible provider in your client. The default Base URL is http://127.0.0.1:8080/v1.",
    ],
    bullets: ["Keep LAN access off for local-only use.", "Update the client URL after changing the port.", "The local key is not an official OpenAI API key."],
    shots: [{ src: imgDashboard, caption: "Dashboard: service state, daily metrics, Base URL, and local API key" }],
  },
  {
    n: 2,
    title: "Import accounts: Base URL, OAuth, and JSON",
    lead: "All import methods share one entry and can select a proxy before import.",
    body: [
      "Open Accounts and click Import account. Choose Base URL + API key, ChatGPT OAuth, or JSON files.",
      "JSON supports one file, multiple files, or one file containing many accounts. Preview the create, update, skip, and conflict counts before committing.",
      "An existing identity is updated instead of duplicated. Selecting Direct explicitly clears its proxy binding.",
    ],
    note: "Never post JSON containing real tokens in chat, tickets, or a public repository.",
    shots: [{ src: imgAccounts, caption: "Accounts and the unified import entry" }, { src: imgImport, caption: "Unified import method chooser" }],
  },
  {
    n: 3,
    title: "Account management, concurrency, and batching",
    lead: "Common actions stay on each row; scheduling and usage details live in the details dialog.",
    body: [
      "Rows show status, in-flight/queued requests, quota windows, tokens, and estimated cost. The switch controls whether the account participates in routing.",
      "Use the lightning button to test, the info button for details, and the trash button to delete. Details include maximum concurrency and queue capacity.",
      "Select accounts for batch testing or deletion. Select all affects the current 20-item page; selection is retained across pages.",
    ],
    bullets: ["Severe authentication failures may disable an account automatically.", "Test successfully before re-enabling a repaired account.", "Batch deletion cannot be undone."],
    shots: [{ src: imgAccountDetails, caption: "Account details: scheduling limits, health, usage, and quota windows" }],
  },
  {
    n: 4,
    title: "Proxy configuration and apply-to-all",
    lead: "HTTP, HTTPS, and SOCKS5 proxies can be bound per account or applied to all current accounts.",
    body: [
      "Add a proxy, then test its DNS, connect, TLS, and HTTP stages.",
      "The compact toolbar applies or clears a proxy for all current accounts and can start a full account connectivity test.",
      "Apply-to-all is a one-time batch update, not a permanent global policy. Choose a proxy again for accounts imported later.",
    ],
    shots: [{ src: imgProxies, caption: "Proxy batch toolbar, binding summary, and proxy list" }],
  },
  {
    n: 5,
    title: "Start the service and connect a client",
    lead: "Clients only need the Base URL and local API key.",
    body: [
      "Start the service on Dashboard and copy both values into an OpenAI-compatible provider in your client.",
      "Avoid a final /v1/v1 when the client appends /v1 automatically. Test /models before sending a chat request.",
      "Only enable LAN access when needed, restrict it with Windows Firewall, and never expose the local port directly to the public internet.",
    ],
    codes: [{ label: "Check models", cmd: 'curl http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-local-REPLACE-ME"' }],
  },
  {
    n: 6,
    title: "Model plaza and pricing",
    lead: "Names and prices come from the shared control API catalog rather than frontend constants.",
    body: ["Cards show input, output, cached-input, and long-context pricing per 1M tokens.", "Use the top-right copy icon for the exact model name. Availability still depends on the account."],
    shots: [{ src: imgModels, caption: "Model names, prices, long-context tier, and price version" }],
  },
  {
    n: 7,
    title: "Statistics, estimated cost, and troubleshooting",
    lead: "Start with trends, then use request logs to isolate account, model, and upstream errors.",
    body: ["Statistics include requests, success rate, tokens, latency, and estimated cost. Totals above $1,000 use K/M/B with four decimals.", "Estimates use the displayed Standard price version and are not invoices.", "Keep request IDs and error kinds for troubleshooting, but never share credentials."],
    shots: [{ src: imgStatistics, caption: "Trends, model distribution, price version, and request logs" }],
  },
  {
    n: 8,
    title: "One-click Codex injection: local and remote",
    lead: "Prefer one-click injection. On the first SSH connection, verify and trust the remote server fingerprint in Amber on this computer.",
    body: [
      "Local injection backs up ~/.codex/config.toml and auth.json before writing the Amber URL, API key, and model. Restore returns both files to their previous state.",
      "Choose a reverse tunnel when the remote server cannot reach ChatGPT. Remote Codex then returns over SSH to local Amber and uses the local proxy and accounts.",
      "The host key belongs to the SSH server entered in the form. The user confirms it in local Amber; the server does not display a button or confirm itself.",
    ],
    bullets: ["Reverse tunnel: local Amber, SSH, and routing must remain online.", "Direct mode: saved passwords and API keys can be reused when left blank.", "Server: AllowTcpForwarding must be enabled and the remote port must be free."],
    steps: [
      { title: "Click Test connection", text: "Amber connects using the entered host, port, user, and password." },
      { title: "Read the confirmation dialog", text: "A first-time server displays a SHA256 fingerprint." },
      { title: "Verify through a trusted channel", text: "Compare it with the server console or a value supplied by its administrator." },
      { title: "Identify the host", text: "It is the remote SSH server, not the Amber PC, ChatGPT, or proxy." },
      { title: "Click Trust and continue in local Amber", text: "This trusts the host key. Continue only when the fingerprints match exactly." },
      { title: "Click Inject now", text: "Amber backs up remote Codex files, writes configuration, and starts the reverse tunnel." },
    ],
    note: "Important: Host key confirmation pending means Amber is waiting for the current user to confirm locally. It is not waiting for the server to confirm itself.",
    codes: [{ label: "Show SSH host fingerprints on the trusted server console", cmd: "for key in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf \"$key\" -E sha256; done" }, { label: "Manual ssh -R fallback", cmd: "ssh -R 8080:127.0.0.1:8080 user@remote-server" }],
    shots: [{ src: imgCodexLocal, caption: "Local model, config preview, and one-click injection" }, { src: imgCodex, caption: "Remote reverse-tunnel and direct modes" }, { src: imgCodexHostKey, caption: "Verify SHA256, then trust the SSH server in local Amber" }],
  },
  {
    n: 9,
    title: "Cloud account and connection-code sharing",
    lead: "Everyday sharing needs only a connection code and temporary password; friends and groups remain optional advanced tools.",
    body: ["Register with email, master password, confirmation, recovery acknowledgment, Turnstile, and the six-digit email code. The master password cannot be recovered.", "The owner selects an account pool, starts sharing, and copies the nine-digit code plus six-character password. The recipient pastes the block and clicks Connect and use.", "One password may allow several claims, while every recipient receives an isolated Guest Key that the owner can pause or revoke independently.", "OAuth routes require the owner's Amber Agent online. API-key accounts may use owner relay or Worker direct. Received access joins local routing automatically."],
    bullets: ["Connection details grant access; they are not model API keys.", "Refreshing a password blocks new claims without disconnecting current users.", "Friends are optional saved contacts and Amber does not provide chat."],
    shots: [{ src: imgCloudRegister, caption: "Cloud registration in one continuous form" }, { src: imgCloudWorkspace, caption: "Copy connection details or paste them and connect in one click" }],
  },
  {
    n: 10,
    title: "Settings, data directory, and common failures",
    lead: "Settings covers fallback model, routing strategy, logs, language, data storage, and diagnostics.",
    body: ["Quota-aware is the recommended account strategy. Retention limits affect disk usage.", "Back up before moving the data directory and never copy a live database manually.", "Use embedded Diagnostics for backend, proxy, cloud, or port failures and export only redacted reports."],
    bullets: ["Session expired: sign in again.", "Client 401: use the latest local API key.", "Connection refused: check service state, port, and duplicate /v1.", "Auto-disabled account: repair, test, then enable."],
    shots: [{ src: imgSettings, caption: "Models, routing, logs, data directory, and diagnostics" }],
  },
]);
</script>

<template>
  <div ref="docRoot" class="docs-page">
    <header class="page-header docs-header">
      <div>
        <h1 class="page-title">{{ zh ? "Amber 使用手册" : "Amber user guide" }}</h1>
        <p class="page-desc">{{ zh ? "v0.4.3 · 从账号导入到可靠云共享的完整操作说明" : "v0.4.3 · From account import to reliable cloud sharing" }}</p>
      </div>
      <div class="docs-header-actions">
        <button class="btn btn-ghost btn-sm" type="button" @click="setAllSections(true)"><Icon name="plus" :size="13" />{{ zh ? "全部展开" : "Expand all" }}</button>
        <button class="btn btn-ghost btn-sm" type="button" @click="setAllSections(false)"><Icon name="stop" :size="13" />{{ zh ? "全部收起" : "Collapse all" }}</button>
      </div>
    </header>

    <section class="doc-overview">
      <div class="overview-copy">
        <Icon name="docs" :size="20" />
        <div>
          <strong>{{ zh ? "第一次使用只需完成这条路径" : "The first-use path" }}</strong>
          <p>{{ zh ? "每个章节都可以点击标题展开；先按顺序完成前五步，再按需查看其他功能。" : "Click any chapter title to expand it. Finish these five steps first, then explore the remaining features as needed." }}</p>
        </div>
      </div>
      <ol class="quick-path">
        <li v-for="(item, index) in quickPath" :key="item"><span>{{ index + 1 }}</span>{{ item }}</li>
      </ol>
    </section>

    <section class="doc-sections" :aria-label="zh ? '使用章节' : 'Guide chapters'">
      <details v-for="section in sections" :key="section.n" class="doc-section" :open="section.n === 1">
        <summary>
          <span class="section-number">{{ String(section.n).padStart(2, "0") }}</span>
          <span class="section-heading"><strong>{{ section.title }}</strong><small>{{ section.lead }}</small></span>
          <Icon name="chevron-down" :size="17" class="section-chevron" />
        </summary>

        <div class="section-content">
          <div class="section-copy">
            <p v-for="line in section.body" :key="line">{{ line }}</p>
            <ul v-if="section.bullets?.length">
              <li v-for="item in section.bullets" :key="item">
                <strong v-if="bulletParts(item).title">{{ bulletParts(item).title }}</strong>
                <span>{{ bulletParts(item).text }}</span>
              </li>
            </ul>

            <ol v-if="section.steps?.length" class="doc-procedure">
              <li v-for="(step, index) in section.steps" :key="step.title">
                <span>{{ index + 1 }}</span>
                <div><strong>{{ step.title }}</strong><p>{{ step.text }}</p></div>
              </li>
            </ol>

            <aside v-if="section.note" class="section-note"><Icon name="warn" :size="17" /><div><strong>{{ zh ? "重点注意" : "Important" }}</strong><span>{{ section.note }}</span></div></aside>

            <div v-for="code in section.codes" :key="code.label" class="doc-code">
              <div class="doc-code-head"><span>{{ code.label }}</span><button class="icon-button" type="button" :title="zh ? '复制' : 'Copy'" @click="copyCmd(code.cmd)"><Icon name="copy" :size="14" /></button></div>
              <pre>{{ code.cmd }}</pre>
            </div>
          </div>

          <div v-if="section.shots?.length" class="section-media">
            <figure v-for="shot in section.shots" :key="shot.src">
              <button class="doc-image-button" type="button" :title="zh ? '点击放大图片' : 'Click to enlarge image'" @click="openShot(shot)">
                <img :src="shot.src" :alt="shot.caption" loading="lazy" />
                <span class="image-zoom"><Icon name="search" :size="16" /></span>
              </button>
              <figcaption>{{ shot.caption }}</figcaption>
            </figure>
          </div>
        </div>
      </details>
    </section>

    <footer class="docs-footer-note">
      <Icon name="warn" :size="17" />
      <p>{{ zh
        ? "截图全部使用演示数据。使用非官方转发仍可能触发上游账号限制；兼容配置、代理或 TLS profile 都不能保证账号安全，请遵守相关服务条款。"
        : "All screenshots use demonstration data. Unofficial forwarding can still trigger upstream account restrictions; compatibility settings, proxies, and TLS profiles cannot guarantee account safety. Follow the applicable terms of service." }}</p>
    </footer>

    <Teleport to="body">
      <div v-if="activeShot" ref="lightbox" class="doc-lightbox" role="dialog" aria-modal="true" :aria-label="activeShot.caption" tabindex="-1" @click.self="closeShot" @keydown.esc="closeShot">
        <div class="doc-lightbox-content">
          <header><strong>{{ activeShot.caption }}</strong><button class="icon-button" type="button" :title="zh ? '关闭大图' : 'Close image'" :aria-label="zh ? '关闭大图' : 'Close image'" @click="closeShot"><Icon name="close" :size="16" /></button></header>
          <img :src="activeShot.src" :alt="activeShot.caption" />
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.docs-page { width: 100%; max-width: 1280px; margin: 0 auto; }
.docs-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
.docs-header-actions { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
.doc-overview { margin-bottom: 18px; padding: 16px 0 18px; border-block: 1px solid var(--border-soft); }
.overview-copy { display: flex; align-items: flex-start; gap: 11px; color: var(--primary); }
.overview-copy strong { display: block; color: var(--text); font-size: 14px; }
.overview-copy p { margin: 4px 0 0; color: var(--text); font-size: 14px; line-height: 1.65; }
.quick-path { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 1px; margin: 14px 0 0; padding: 0; border: 1px solid var(--border-soft); border-radius: 7px; background: var(--border-soft); overflow: hidden; list-style: none; }
.quick-path li { min-width: 0; display: flex; align-items: center; gap: 8px; padding: 12px; background: var(--bg-card); color: var(--text); font-size: 13.5px; font-weight: 550; }
.quick-path span { display: grid; place-items: center; width: 20px; height: 20px; flex: 0 0 auto; border-radius: 5px; background: var(--primary-soft); color: var(--primary); font-family: var(--mono); font-size: 10px; font-weight: 700; }
.doc-sections { border-top: 1px solid var(--border); }
.doc-section { border-bottom: 1px solid var(--border); }
.doc-section summary { min-height: 82px; display: grid; grid-template-columns: 42px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 12px 4px; cursor: pointer; list-style: none; }
.doc-section summary::-webkit-details-marker { display: none; }
.doc-section summary:hover .section-heading strong { color: var(--primary); }
.doc-section summary:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
.section-number { color: var(--text-faint); font-family: var(--mono); font-size: 12px; font-weight: 650; }
.section-heading { min-width: 0; display: grid; gap: 4px; }
.section-heading strong { color: var(--text); font-size: 17px; transition: color var(--motion-fast) var(--motion-ease); }
.section-heading small { color: var(--text-dim); font-size: 13.5px; line-height: 1.55; }
.section-chevron { color: var(--text-faint); transition: transform var(--motion-fast) var(--motion-ease); }
.doc-section[open] .section-chevron { transform: rotate(180deg); }
.section-content { display: grid; grid-template-columns: minmax(0, 1fr); gap: 24px; padding: 4px 4px 36px 58px; }
.section-copy { min-width: 0; max-width: 1080px; }
.section-copy > p { margin: 0 0 12px; color: var(--text); font-size: 15px; line-height: 1.85; }
.section-copy ul { display: grid; gap: 9px; margin: 16px 0; padding-left: 20px; color: var(--text); font-size: 14.5px; line-height: 1.72; }
.section-copy li strong { margin-right: 4px; color: var(--text); font-weight: 700; }
.section-copy li::marker { color: var(--primary); }
.section-note { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 10px; margin-top: 18px; padding: 13px 15px; border-left: 4px solid var(--warn); background: var(--warn-soft); color: var(--text); font-size: 14px; line-height: 1.7; }
.section-note :deep(svg) { color: var(--warn); }
.section-note > div { display: grid; gap: 3px; }.section-note strong { color: var(--warn); font-size: 13px; }
.doc-procedure { max-width: 1040px; display: grid; gap: 0; margin: 20px 0; padding: 0; border-block: 1px solid var(--border); list-style: none; }
.doc-procedure li { display: grid; grid-template-columns: 32px minmax(0, 1fr); gap: 12px; padding: 13px 4px; border-bottom: 1px solid var(--border-soft); }
.doc-procedure li:last-child { border-bottom: 0; }.doc-procedure > li > span { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 6px; background: var(--primary-soft); color: var(--primary); font-family: var(--mono); font-size: 12px; font-weight: 700; }
.doc-procedure div { display: grid; gap: 4px; }.doc-procedure strong { color: var(--text); font-size: 14.5px; }.doc-procedure p { margin: 0; color: var(--text-dim); font-size: 14px; line-height: 1.65; }
.section-media { min-width: 0; width: 100%; display: grid; gap: 24px; align-content: start; }
.section-media figure { margin: 0; }
.doc-image-button { position: relative; width: 100%; display: block; padding: 0; border: 1px solid var(--border); border-radius: 7px; background: var(--bg-card); box-shadow: var(--shadow-xs); overflow: hidden; cursor: zoom-in; }
.doc-image-button:hover { border-color: var(--primary); box-shadow: var(--shadow-hover); }.doc-image-button:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; }
.section-media img { width: 100%; display: block; background: var(--bg-card); }
.image-zoom { position: absolute; right: 12px; bottom: 12px; display: grid; place-items: center; width: 34px; height: 34px; border: 1px solid rgba(255,255,255,.72); border-radius: 6px; background: rgba(20,22,25,.72); color: #fff; }
.section-media figcaption { margin-top: 8px; color: var(--text-dim); font-size: 13px; line-height: 1.55; text-align: center; }
.doc-code { margin-top: 12px; border: 1px solid var(--border); border-radius: 7px; background: var(--bg-elev); overflow: hidden; }
.doc-code-head { min-height: 34px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 5px 4px 11px; border-bottom: 1px solid var(--border-soft); color: var(--text-dim); font-size: 11.5px; font-weight: 600; }
.doc-code pre { margin: 0; padding: 13px 14px; color: var(--text); font-family: var(--mono); font-size: 13px; line-height: 1.7; overflow-x: auto; white-space: pre-wrap; overflow-wrap: anywhere; }
.doc-lightbox { position: fixed; z-index: 3000; inset: 0; display: grid; place-items: center; padding: 24px; background: rgba(12,14,18,.82); backdrop-filter: blur(3px); }
.doc-lightbox-content { width: min(1520px, 96vw); max-height: 94vh; display: grid; grid-template-rows: auto minmax(0, 1fr); border: 1px solid rgba(255,255,255,.2); border-radius: 8px; background: var(--bg-card); box-shadow: 0 24px 80px rgba(0,0,0,.4); overflow: hidden; }
.doc-lightbox-content header { min-height: 48px; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 8px 10px 8px 16px; border-bottom: 1px solid var(--border); }.doc-lightbox-content header strong { font-size: 14px; }
.doc-lightbox-content > img { width: 100%; height: 100%; max-height: calc(94vh - 49px); object-fit: contain; background: #111; }
.docs-footer-note { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 9px; margin-top: 18px; padding: 14px 2px; color: var(--text-faint); }
.docs-footer-note :deep(svg) { color: var(--warn); }
.docs-footer-note p { margin: 0; font-size: 11.5px; line-height: 1.65; }
@media (max-width: 1080px) { .quick-path { grid-template-columns: repeat(2, minmax(0, 1fr)); }.quick-path li:last-child { grid-column: 1 / -1; } }
@media (max-width: 720px) { .docs-header { flex-direction: column; }.docs-header-actions { justify-content: flex-start; }.doc-section summary { grid-template-columns: 32px minmax(0, 1fr) auto; }.section-content { padding-left: 36px; }.quick-path { grid-template-columns: minmax(0, 1fr); }.quick-path li:last-child { grid-column: auto; }.doc-lightbox { padding: 8px; }.doc-lightbox-content { width: 100%; max-height: 98vh; }.section-heading strong { font-size: 15.5px; }.section-copy > p { font-size: 14.5px; } }
@media (prefers-reduced-motion: reduce) { .section-chevron, .section-heading strong { transition: none; } }
</style>
