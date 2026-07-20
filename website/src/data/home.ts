import { stableRelease } from "../config/releases";

export const trustItems = ["Windows x64", "本地优先", "OpenAI 兼容 API", "开源", `v${stableRelease.version}`];

export const capabilityItems = [
  {
    id: "routing",
    title: "多账号调度",
    description: "额度感知、并发队列、批量测试与启停控制，让账号池保持清晰可用。",
    to: "/docs#accounts",
    tone: "amber" as const,
  },
  {
    id: "sharing",
    title: "连接码受控共享",
    description: "多账号共享池、独立 Guest Key、额度、限流、暂停与随时撤销。",
    to: "/docs#sharing",
    tone: "green" as const,
  },
  {
    id: "codex",
    title: "Codex 一键接入",
    description: "本机启动服务并注入，或验证 SSH 主机后建立远程反向隧道。",
    to: "/docs#codex-local",
    tone: "teal" as const,
  },
];

export type ShowcaseId = "accounts" | "network" | "sharing" | "codex";

export interface ShowcaseItem {
  id: ShowcaseId;
  label: string;
  title: string;
  description: string;
  points: string[];
  image: string;
  mobileImage: string;
  alt: string;
  caption: string;
  docsLink: string;
}

export const showcaseItems: ShowcaseItem[] = [
  {
    id: "accounts",
    label: "账号调度",
    title: "从零散账号到可观察的额度池",
    description: "统一查看健康状态、窗口额度和并发占用，在真正请求前完成批量测试。",
    points: ["额度感知与优先级调度", "批量测试、启停和队列状态", "统计与费用保持可追踪"],
    image: "/screenshots/v044/accounts-v044.png",
    mobileImage: "/screenshots/v044/accounts-v044-compact.png",
    alt: "Amber v0.4.4 账号调度真实界面",
    caption: "真实 v0.4.4 界面；账号身份、用量与费用已替换为演示值。",
    docsLink: "/docs#accounts",
  },
  {
    id: "network",
    label: "代理与网络",
    title: "每个账号都明确知道请求从哪里出去",
    description: "直连、系统代理和指定代理是三种独立模式，测试结果与账号绑定关系一目了然。",
    points: ["direct / system / proxy 明确分离", "代理测试与账号测试分步执行", "云连接可以选择独立网络出口"],
    image: "/screenshots/v044/network-v044.png",
    mobileImage: "/screenshots/v044/network-v044-compact.png",
    alt: "Amber v0.4.4 代理与网络模式真实界面",
    caption: "真实 v0.4.4 界面；代理名称、地址与绑定数量已替换为演示值。",
    docsLink: "/docs#proxies",
  },
  {
    id: "sharing",
    label: "云账户与共享",
    title: "用连接码共享，用独立授权控制边界",
    description: "选择账号池并生成连接信息，接收者获得独立 Guest Key，可单独限额、暂停或撤销。",
    points: ["无需先建立好友关系", "主设备与备用设备路由明确", "每位接收者独立额度与授权"],
    image: "/screenshots/v044/cloud-sharing-v044.png",
    mobileImage: "/screenshots/v044/cloud-sharing-v044-compact.png",
    alt: "Amber v0.4.4 云账户与连接码共享真实界面",
    caption: "真实 v0.4.4 界面；账户、连接码与用户标识已替换为演示值。",
    docsLink: "/docs#sharing",
  },
  {
    id: "codex",
    label: "Codex 接入",
    title: "本机与远程 Codex 都走同一套检查闭环",
    description: "Amber 先检查服务、密钥、模型与目标主机，再写入配置并回读确认。",
    points: ["本机启动服务并注入", "SSH 指纹确认后建立反向隧道", "失败时保留明确诊断阶段"],
    image: "/screenshots/v044/codex-injection-v044.png",
    mobileImage: "/screenshots/v044/codex-injection-v044-compact.png",
    alt: "Amber v0.4.4 本机 Codex 接入真实界面",
    caption: "真实 v0.4.4 本机接入界面；本机路径与备份时间已替换为演示值。",
    docsLink: "/docs#codex-local",
  },
];

export const workflowItems = [
  {
    number: "01",
    title: "导入账号",
    description: "批量导入后测试状态，按需绑定网络模式。",
    image: "/screenshots/v044/accounts-v044-compact.png",
  },
  {
    number: "02",
    title: "启动服务并注入",
    description: "验证本地网关和 Codex 配置，再开始请求。",
    image: "/screenshots/v044/codex-injection-v044-compact.png",
  },
  {
    number: "03",
    title: "使用或受控共享",
    description: "自己调用，或向固定接收者发放独立授权。",
    image: "/screenshots/v044/cloud-sharing-v044-compact.png",
  },
];

export const useCases = [
  {
    title: "多账号个人用户",
    description: "希望统一观察额度、健康状态和并发，而不是在多个客户端间反复切换。",
  },
  {
    title: "远程服务器 Codex",
    description: "需要把 Windows 电脑上的账号和网络出口安全送到自己的开发服务器。",
  },
  {
    title: "固定范围共享",
    description: "只向少量接收者开放选定账号，并保留限额、暂停和撤销能力。",
  },
];
