import { stableRelease } from "../config/releases";

export type FaqCategory = "本地服务" | "代理与账号" | "Codex 与 SSH" | "云同步与共享";
export type FaqVersion = "current" | "mixed";

export interface FaqItem {
  id: string;
  category: FaqCategory;
  question: string;
  paragraphs: readonly string[];
  steps?: readonly string[];
  note?: string;
  keywords: readonly string[];
  version?: FaqVersion;
}

export const faqCategories = ["本地服务", "代理与账号", "Codex 与 SSH", "云同步与共享"] as const satisfies readonly FaqCategory[];

const currentVersion = `v${stableRelease.version}`;

const faqItems = [
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
      `${currentVersion} 的“启动服务并注入”会在写入配置前启动本地服务，并验证 Amber 实例、本地 API Key 与 /v1/models。若之后仍出现 502，先确认 Amber 没有退出、端口未变化，再查看统计页最近错误，区分本地连接问题与上游账号问题。`,
    ],
    steps: ["打开 Amber。", "在仪表盘启动服务并确认运行中。", "重新加载 Codex 后再次请求。"],
    keywords: ["502", "bad gateway", "responses", "服务未启动", "注入"],
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
      `${currentVersion} 会在注入前验证服务是否成功启动；端口冲突或健康检查失败时不会继续写入 Codex 配置。修复占用或改用新端口后，再执行“启动服务并注入”。`,
    ],
    note: "不要关闭来源不明的系统进程。无法确认占用者时，优先改用其他端口。",
    keywords: ["8080", "端口冲突", "address already in use", "监听", "占用"],
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
      `在 ${currentVersion} 中，使用“启动服务并注入”完成本地验证和配置写入，再重新加载 Codex。若 Codex 运行在 WSL、容器或远程主机，必须使用对应的远程接入方式，不能直接引用 Windows 的 127.0.0.1。`,
    ],
    keywords: ["账号测试", "codex", "模型权限", "配置", "api key", "wsl", "容器"],
  },
  {
    id: "remote-codex-access",
    category: "Codex 与 SSH",
    question: "远程服务器上的 Codex 如何接入 Amber？",
    paragraphs: [
      "先在 Amber 中添加 SSH 目标并测试连接，通过服务器控制台、可信终端或管理员提供的记录核对 SHA-256 主机指纹。确认无误后，Amber 才能为该 SSH 用户写入并回读远程 Codex 配置。",
      "远程服务器能够访问目标 Base URL 时可使用直连模式；需要借用 Windows 电脑上的账号、代理或网络出口时，使用 SSH 反向隧道让请求回流本机 Amber。反向隧道依赖本机 Amber、本地服务和 SSH 连接持续在线。",
    ],
    steps: ["添加远程 SSH 目标并测试连接。", "从可信渠道核对并确认主机指纹。", "选择直连或反向隧道，执行远程注入后重新加载 Codex。"],
    keywords: ["远程服务器", "远程 codex", "ssh", "直连", "反向隧道", "注入", "host key"],
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
    id: "cloud-account-boundary",
    category: "云同步与共享",
    question: "不登录云账号，可以使用 Amber 的本地功能吗？",
    paragraphs: [
      "可以。导入账号、配置代理、启动本地网关、查看统计与日志，以及本机或 SSH Codex 接入，都不要求注册或登录 Amber 云账号。OAuth 登录、SSH 连接和上游请求仍需各自的网络与凭据可用。",
      "Amber 云账号只用于加密同步、备份、多设备和共享授权。使用这些云功能时需要登录；连接码共享的双方都需要登录各自的云账号。",
    ],
    keywords: ["云账号", "本地功能", "无需登录", "无需注册", "本地网关", "云同步", "共享"],
  },
  {
    id: "cloud-network-diagnostics",
    category: "云同步与共享",
    question: "云同步在 DNS、TCP、TLS 或 HTTP 阶段失败分别意味着什么？",
    paragraphs: [
      "DNS 失败表示域名未解析；TCP 失败表示无法建立到目标端口的连接；TLS 失败通常与证书链、SNI、系统时间或网络拦截有关；HTTP 失败表示连接已建立，但服务返回了错误状态或请求被拒绝。超时则要结合最后完成的阶段判断。",
      `在云账户的“连接设置”中选择系统代理、Amber 已保存代理或直连，运行网络探测，探测成功后应用并重试同步。${currentVersion} 首选 api.amberapp.asia；幂等请求在首选入口不可用时可回退到 Workers 域名。`,
    ],
    keywords: ["云同步", "dns", "tcp", "tls", "http", "workers.dev", "连接设置", "超时"],
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
      `${currentVersion} 的新共享默认绑定创建共享的具体电脑。共享者可以显式配置最多两台具备目标账号且健康的备用设备；未配置的其他在线设备不会自动接管。`,
      "故障转移只发生在上游请求开始之前。上游已经开始后不会跨设备重放，以避免重复扣费或重复执行。",
    ],
    keywords: ["同账号", "多设备", "主设备", "备用设备", "设备定向", "路由"],
  },
  {
    id: "cloud-account-workspaces",
    category: "云同步与共享",
    question: "切换云账号后，本地数据和工作区放在哪里？",
    paragraphs: [
      `${currentVersion} 为每个云账号建立独立本地工作区，各自保存账号、代理、同步队列、Guest Key、日志和 SSH 目标。退出登录不会删除数据或解除工作区归属；登录另一个账号时会切换或创建对应工作区。`,
      "升级时若旧数据库包含多个历史用户或归属不明确的同步数据，Amber 会进入只读恢复工作区，不会猜测归属或自动上传。",
    ],
    keywords: ["切换云账号", "工作区", "数据目录", "隔离", "退出登录", "同步队列"],
  },
] as const satisfies readonly FaqItem[];

export type FaqId = (typeof faqItems)[number]["id"];

export const faqs: readonly FaqItem[] = faqItems;

export const homeFaqIds = [
  "bad-gateway",
  "proxy-tun",
  "remote-codex-access",
  "owner-device-offline",
  "cloud-account-boundary",
] as const satisfies readonly FaqId[];

const faqById = new Map(faqs.map((faq) => [faq.id, faq]));

export const homeFaqs: readonly FaqItem[] = homeFaqIds.map((id) => {
  const faq = faqById.get(id);
  if (!faq) throw new Error(`Missing home FAQ: ${id}`);
  return faq;
});
