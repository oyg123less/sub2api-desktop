# Amber 官网 Website R1 视觉与交互改版规格

> 文档用途：交给独立开发窗口直接实施
>
> 文档状态：设计冻结，等待开发
>
> 编写日期：2026-07-20
>
> 参考站点：[CCSwitch 中文官网](https://www.ccswitch.io/zh/)
>
> Amber 正式域名：<https://amberapp.asia>
>
> 本期范围：只改 `website/`，不提升 Amber 客户端版本

---

## 0. 执行结论

Amber 官网当前具备清晰、克制、可访问性良好的基础，但首页只有 Hero 和三项能力条，缺少真实产品演示、功能展开、使用流程和持续交互反馈，因此整体更像“下载入口”，还没有形成成熟产品官网的完成度。

本次改版应借鉴 CCSwitch 的以下优点：

1. 第一屏直接展示产品，而不是依赖抽象插画。
2. 页面通过真实界面、功能卡片、交互演示和 FAQ 形成连续叙事。
3. 卡片、按钮和图标使用统一、克制的悬停动效。
4. 章节留白充足，内容密度有节奏，不靠大量文字填满页面。
5. 桌面、窄窗口和移动端均保持明确的阅读顺序。

不得把 Amber 改成 CCSwitch 的复制品。Amber 必须保留自己的产品定位、琥珀色品牌、青绿色辅助色、6-8px 圆角和 Windows Codex 网关特征。

本期最终目标：

```text
用户打开 amberapp.asia
-> 第一屏理解 Amber 是什么
-> 看到真实产品界面
-> 在 10 秒内找到下载入口
-> 在 30 秒内理解账号调度、共享和远程 Codex
-> 可以自然进入下载、文档或 GitHub
```

---

## 1. 工作边界

### 1.1 允许修改

- `website/src/`
- `website/public/`
- `website/tests/`
- `website/scripts/`
- `website/README.md`
- 官网专用配置和测试文件

### 1.2 禁止修改

- `cloud/`
- `core/`
- `src-tauri/`
- 桌面端 `src/`
- D1、Worker、Resend、QQ SMTP 或 Cloudflare 生产配置
- Amber 客户端版本号
- GitHub Release 安装包、大小和 SHA-256，除非发现公开 Release 数据确实变化
- `api.amberapp.asia`、`mail.amberapp.asia` 的用途

### 1.3 生产约束

- 官网主域名保持 `https://amberapp.asia`。
- `https://www.amberapp.asia/*` 已通过 Cloudflare 301 到根域名。
- canonical、Open Graph、站点地图和 robots 继续使用根域名。
- Cloudflare Pages 项目连接 GitHub `main` 分支。
- Pages 根目录为 `website`，构建命令为 `npm run build`，输出目录为 `dist`。
- 不把任何 Token、API Key、邮箱授权码或生产凭据写入源码、文档和构建日志。

---

## 2. CCSwitch 官网设计拆解

以下结论来自 2026-07-20 的线上 HTML、生产 CSS 和首页组件资源分析。

### 2.1 信息架构

CCSwitch 首页使用以下顺序：

```text
Hero
-> 支持者/可信度
-> 核心功能卡片
-> 可切换的产品演示
-> 技术与工作流
-> 用户反馈
-> FAQ
-> 最终下载 CTA
```

它的优势不是某一个卡片，而是用户每向下滚动一屏都会获得新的产品证据。

Amber 当前首页只有：

```text
Hero
-> 三项能力条
```

因此页面虽然干净，但内容在第二屏就结束，无法承载 Amber 已经具备的账号、代理、共享、云同步和远程 Codex 能力。

### 2.2 尺寸与节奏

CCSwitch 的线上样式具有以下特征：

| 项目 | CCSwitch 观察值 | Amber 当前值 | 本期 Amber 目标 |
| --- | --- | --- | --- |
| 主容器 | 最大约 1400px | 1180px | 1240px |
| Hero 内容 | 最大约 1600px | 1180px | 1320px，仅 Hero 使用 |
| 章节间距 | 64/80/128px 响应式 | 88px 固定为主 | 72/96/112px 分级 |
| 功能网格 | 桌面 3 列、平板 2 列 | 首页只有能力条 | 桌面 3、平板 2、手机 1 |
| 卡片圆角 | 常见 12-16px | 6px | 8px，不照搬大圆角 |
| 动效 | 150-600ms 分级 | 基本无统一动效 | 160-500ms 分级 |

### 2.3 色彩

CCSwitch 使用暖米色背景、橙红主色和深色模式。其生产变量中主色约为 `HSL 15 65% 52%`，背景和卡片均为暖灰褐色。

Amber 不应整套复制该配色，原因：

- Amber 当前已经拥有琥珀色、青绿色、绿色和警示色体系。
- 全站暖米色会削弱账号、代理、云端和状态信息之间的区分。
- Amber 是工作型网关工具，页面应比 CCSwitch 更清爽、技术感更明确。

应借鉴的是“暖色品牌 + 中性背景 + 多状态辅助色”的方法，而不是复制具体颜色。

### 2.4 卡片悬停

CCSwitch 线上代码采用了多级反馈：

- 卡片 `transition-all duration-300`。
- 功能卡片悬停增加 `shadow-xl`。
- 主要按钮悬停 `scale(1.05)` 并从 `shadow-xl` 提升到 `shadow-2xl`。
- 卡片图标在父卡片悬停时 `scale(1.10)`。
- 箭头在父元素悬停时平移约 4px。
- 次级卡片使用 `scale(1.02)`，避免所有元素都剧烈放大。
- 滚动进入通常使用 500-600ms 的透明度与纵向位移动画。

Amber 应借鉴统一反馈，但缩小幅度，避免官网显得浮夸。

### 2.5 产品演示

CCSwitch 最有价值的设计不是 Hero 文案，而是：

- 直接展示接近真实应用的界面。
- 通过标签切换不同功能视图。
- 演示容器具有明确尺寸，不因内容切换跳动。
- 界面中按钮、用量、供应商、代理等细节让用户相信产品真实存在。

Amber 已经有真实账号、代理、云账户、Codex 接入和仪表盘截图，应将其组织为产品演示，而不是只把截图堆进文档页。

### 2.6 进入动画

CCSwitch 使用 Framer Motion 实现：

- Hero 文案 `opacity 0 -> 1`、`y 30 -> 0`，约 600ms。
- Hero 产品画面 `x 50 -> 0`，约 600ms。
- 功能卡片按 100ms 间隔依次进入。
- 滚动进入只执行一次。
- FAQ 展开使用约 200ms 动画。

Amber 是 Vue 项目，不需要为了模仿而引入 React/Framer Motion。建议用一个轻量 `RevealOnScroll` 组件或指令加 `IntersectionObserver` 实现。

### 2.7 不应照搬的内容

- 不复制 CCSwitch 的分栏 Hero。Amber Hero 应继续以品牌与产品本体为中心，真实产品画面在文案下方展开。
- 不复制模糊渐变光球、装饰性发光圆和紫色渐变。
- 不使用 16px 以上卡片圆角。
- 不编造下载量、用户数量、评价或用户证言。
- 不把所有章节都做成浮动卡片。
- 不创建卡片套卡片的结构。
- 不在 Hero 主文案中强调“借鉴 CCSwitch”，这会让 Amber 看起来像附属项目。

CCSwitch 和 sub2api 的关联说明可以保留在 README、页脚声明和适当的生态说明中，但 Hero 应只回答 Amber 给用户带来的价值。

---

## 3. Amber 当前官网问题

### 3.1 首页内容不足

当前 `HomePage.vue` 只有 Hero 和三项能力条。已有的功能没有展开：

- 账号批量导入和调度。
- 代理绑定与网络模式。
- 本地 OpenAI 兼容网关。
- 连接码共享。
- 独立云工作区。
- 本地和远程 Codex 注入。
- 统计、费用、日志与故障诊断。

### 3.2 Hero 产品证据不足

当前 Hero 使用产品封面图，但用户无法直接看清 Amber 的真实界面和操作价值。新 Hero 必须让真实 Amber UI 成为首屏信号。

### 3.3 交互层次不足

当前按钮、功能项和多数卡片只变化背景或边框，没有统一的：

- 上浮。
- 阴影提升。
- 图标缩放。
- 箭头位移。
- 滚动进入。
- `prefers-reduced-motion` 处理。

### 3.4 导航项偏多

桌面导航同时展示首页、下载、文档、更新、FAQ、安全、状态，产品主操作“下载”没有形成明显视觉优先级。

### 3.5 页面之间视觉语言不统一

首页过于简短，下载、文档、FAQ、安全和状态页内容却很长。用户从首页进入内页时，会感到信息密度突然变化。

### 3.6 截图版本问题

现有部分截图来自 v0.4.2 模拟界面。官网改版若要放大展示产品，必须更新为 v0.4.4 的脱敏截图，或使用完全虚构且明确无真实凭据的演示数据。

---

## 4. 新设计原则

### 4.1 产品优先

第一屏必须同时出现：

- `Amber` 品牌名。
- 一句字面可理解的产品定位。
- Windows 下载按钮。
- 文档入口。
- 当前稳定版本。
- 实际 Amber 界面。

### 4.2 简洁不等于内容少

每个章节只解决一个问题，但首页需要完整说明产品。使用短标题、短说明、实际界面和清晰 CTA，避免堆砌大段营销文案。

### 4.3 页面章节使用内容带

普通章节采用全宽背景带和受限内容容器。卡片只用于：

- 重复功能项。
- 可点击演示入口。
- FAQ、版本、下载资产等独立对象。

页面章节本身不得套进大卡片。

### 4.4 动效必须服务反馈

只在以下场景使用动画：

- 指示元素可以点击。
- 表示卡片获得焦点。
- 引导阅读顺序。
- 切换产品演示视图。
- 展开和收起内容。

不添加持续漂浮、自动摇摆或无限呼吸动画。

### 4.5 Amber 自有识别

保留：

- 琥珀色主操作。
- 青绿色网络与云端辅助色。
- 绿色成功状态。
- 深墨色正文。
- 清晰的状态与安全文案。
- Windows、Codex、本地网关、共享回流等真实产品语义。

---

## 5. 视觉 Token 规格

### 5.1 建议变量

在现有 `website/src/styles.css` 变量基础上调整，不更换 CSS 架构：

```css
:root {
  --bg: #f5f6f3;
  --surface: #ffffff;
  --surface-muted: #ecefeb;
  --surface-emphasis: #e4e9e5;

  --ink: #1d211f;
  --ink-soft: #59605b;
  --ink-faint: #7b827d;

  --border: #d8ddd8;
  --border-strong: #b9c1bb;

  --amber: #bd5127;
  --amber-dark: #913b1c;
  --amber-soft: #f7e5dc;

  --teal: #2f696b;
  --teal-soft: #e2f0ef;
  --green: #24744b;

  --shadow-xs: 0 1px 2px rgba(28, 34, 30, 0.05);
  --shadow-sm: 0 6px 18px rgba(28, 34, 30, 0.08);
  --shadow-md: 0 14px 34px rgba(28, 34, 30, 0.11);
  --shadow-hero: 0 24px 54px rgba(28, 34, 30, 0.14);

  --radius-control: 6px;
  --radius-card: 8px;

  --motion-fast: 160ms;
  --motion-normal: 240ms;
  --motion-slow: 480ms;
  --ease-out: cubic-bezier(0.22, 1, 0.36, 1);

  --container: 1240px;
  --hero-container: 1320px;
}
```

颜色允许在实现时做小幅对比度修正，但不得将全站改为单一米色、紫色或深蓝主题。

### 5.2 字体

- 正文继续使用 `Inter, Segoe UI, Microsoft YaHei, system-ui`。
- 代码继续使用 Cascadia Code/Consolas。
- 不使用负字距。
- 不按 viewport 宽度连续缩放字体。
- Hero H1 桌面 60-64px，移动端 44-48px。
- 内页 H1 桌面约 44px，避免所有内页都使用 Hero 尺寸。
- 功能卡标题 18-20px，正文 14-16px。

### 5.3 圆角

- 按钮、输入框：6px。
- 普通卡片：8px。
- 截图框和演示工具：8px。
- 标签可以使用胶囊圆角。
- 不使用 16-24px 大圆角堆满页面。

---

## 6. 动效规格

### 6.1 交互卡片

所有可点击或可聚焦的功能卡片统一使用：

```css
.interactive-card {
  transform: translateY(0) scale(1);
  box-shadow: var(--shadow-xs);
  transition:
    transform var(--motion-normal) var(--ease-out),
    box-shadow var(--motion-normal) var(--ease-out),
    border-color var(--motion-normal) ease,
    background-color var(--motion-normal) ease;
}

@media (hover: hover) and (pointer: fine) {
  .interactive-card:hover {
    transform: translateY(-4px) scale(1.012);
    border-color: rgba(189, 81, 39, 0.32);
    box-shadow: var(--shadow-md);
  }
}

.interactive-card:focus-within {
  transform: translateY(-3px) scale(1.01);
  box-shadow: var(--shadow-md);
}
```

不允许普通内容卡片使用 `scale(1.05)`。大幅缩放只允许下载主按钮或极少量 CTA，且最大 `1.025`。

### 6.2 图标与箭头

- 图标容器悬停缩放 `1.06-1.08`。
- 命令箭头悬停向右移动 `3-4px`。
- 删除、外链等图标不做旋转。
- 图标动画和卡片动画使用相同 easing。

### 6.3 按钮

- 主按钮：上浮 2px、阴影从 `shadow-sm` 到 `shadow-md`。
- 次按钮：边框和背景变化，最多上浮 1px。
- 按下状态：`translateY(0) scale(0.985)`。
- 布局尺寸固定，悬停不得推动周围元素。

### 6.4 滚动进入

新增一个轻量 `RevealOnScroll.vue` 或 `v-reveal`：

- 初始 `opacity: 0; transform: translateY(18px)`。
- 进入 `opacity: 1; transform: translateY(0)`。
- 持续 420-520ms。
- 同组项目间隔 60-90ms。
- 每个元素只执行一次。
- SSR/无 JavaScript 时内容默认可见。

### 6.5 减少动态效果

必须支持：

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    scroll-behavior: auto !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }

  .interactive-card:hover,
  .interactive-card:focus-within {
    transform: none;
  }
}
```

---

## 7. 首页完整布局

### 7.1 顶部导航

桌面导航改为：

```text
Amber | 产品 | 文档 | 更新 | GitHub | [下载 v0.4.4]
```

规则：

- `产品` 链接到首页产品演示章节。
- FAQ、安全、状态移入页脚和文档辅助导航，不占据桌面主导航。
- 下载使用带 Download 图标的主按钮。
- Header 保持 sticky、半透明和模糊，但降低边框存在感。
- 移动端菜单必须继续覆盖全部页面入口。

### 7.2 Hero

结构：

```text
[当前稳定版 v0.4.4]
Amber
把多个账号变成可调度、可共享的 Codex 网关
短说明
[下载 Windows] [查看文档]
Windows 10/11 · x64 · 开源

真实 Amber 产品主界面
```

要求：

- H1 必须为 `Amber`。
- 不把“基于 sub2api、借鉴 CCSwitch”放在主说明第一句。
- 使用真实或完全脱敏的 Amber v0.4.4 界面。
- 产品画面在文案下方，以无装饰框或 8px 工具框呈现。
- 画面保持稳定 aspect ratio，不随加载状态改变布局。
- 首屏底部必须露出下一章节一部分。
- 不添加渐变球、漂浮圆或纯氛围图。

建议 Hero 文案：

```text
Amber
把多个账号变成可调度、可共享的 Codex 网关。

在 Windows 上统一管理账号、代理、共享和本地/远程 Codex 接入。
```

### 7.3 产品可信度条

Hero 下方使用窄内容带，不做大卡片：

```text
Windows x64 | 本地优先 | OpenAI 兼容 API | 开源 | v0.4.4
```

不显示未经核实的用户数、下载数和可用率。

### 7.4 核心能力卡片

三张卡片：

1. 多账号调度
   - 额度感知、并发队列、批量测试、启停控制。
2. 连接码受控共享
   - 多账号共享池、独立 Guest Key、限流、暂停和撤销。
3. Codex 一键接入
   - 本机启动服务并注入、SSH 主机校验、远程反向隧道。

每张卡片：

- 顶部图标。
- 短标题。
- 两行说明。
- `了解更多` 文本链接和右箭头。
- 悬停上浮 4px、缩放 1.012、图标缩放 1.08。
- 卡片高度在同一行保持一致。

### 7.5 产品演示区

这是本次最高优先级组件。

新增 `ProductShowcase.vue`：

```text
[账号调度] [代理] [云账户与共享] [Codex 接入]

标题 + 3 个关键点
对应真实截图
```

要求：

- 使用 segmented tabs，不使用四张图片同时平铺。
- 桌面端截图宽度充足，文字可读。
- 移动端标签可横向滚动，但页面本身不能横向溢出。
- 截图切换使用 180-240ms 淡入，不改变容器高度。
- 截图点击后复用 `ImageViewer` 放大。
- 图片必须包含真实产品状态，不使用纯装饰图。
- 当前模拟截图若与 v0.4.4 不一致，先更新截图再上线。

建议截图：

- `dashboard.png`
- `accounts.png`
- `proxies.png`
- 新增 `cloud-sharing-v044.png`
- 新增 `codex-injection-v044.png`

所有截图必须检查并移除：

- 邮箱。
- 账号 ID。
- Token、API Key、Guest Key。
- 代理地址和密码。
- SSH 主机、用户名和指纹。
- 设备名称。

### 7.6 三步工作流

使用无卡片的横向步骤带：

```text
01 导入账号
02 启动服务并注入
03 使用或受控共享
```

每一步配一个真实小界面或局部截图。桌面横向，移动端纵向。

### 7.7 两个重点功能带

使用交替但克制的图文布局：

#### 账号、代理和网络出口

- 强调 `direct/system/proxy` 三种明确网络模式。
- 展示账号行卡片和代理配置。
- 不在页面中解释过多内部实现。

#### 云共享与远程 Codex

- 展示连接码共享流程。
- 说明需要本地回流的共享由指定设备承载。
- 展示 SSH 反向隧道的请求路径。
- 主 CTA 进入文档对应章节。

这两个章节是内容带，不要套在大卡片内。

### 7.8 适用场景

不使用虚构用户评价，改为三张场景卡：

- 多账号个人用户。
- 需要给远程服务器提供 Codex 的用户。
- 需要把少量账号安全共享给固定使用者的用户。

### 7.9 FAQ

首页只保留 5 个最高频问题：

- 502 Bad Gateway。
- 是否必须开 TUN。
- 远程服务器如何接入。
- 共享者设备为什么需要在线。
- 云账号和本地功能的关系。

使用现有 FAQ 数据或与 FAQ 页面共享数据源，避免两份文案长期分叉。

### 7.10 最终 CTA

使用深炭色平面内容带，不使用渐变：

```text
准备好让 Codex 连接到你的账号池了吗？
[下载 Windows 版] [查看使用文档]
```

CTA 中保留版本、平台和 GitHub 链接。

---

## 8. 内页改版要求

### 8.1 Download

- 第一屏直接展示版本、下载、大小和 SHA-256。
- 主下载区域可使用一个真正的下载资产卡片。
- 安装、覆盖升级、SmartScreen 和历史版本保持无嵌套内容带。
- 下载卡悬停仅轻微上浮，不做明显缩放。
- 保留“无需先卸载”的明确说明。

### 8.2 Docs

- 保持桌面侧边目录和移动目录。
- 增大正文有效宽度与图片显示尺寸。
- 图片悬停显示轻微阴影和放大提示。
- 标题、重点、警告和步骤的层次必须比当前更清楚。
- 不把所有章节改成卡片。
- 截图放大、焦点陷阱和 Escape 关闭行为必须保留。

### 8.3 Changelog

- 当前稳定版使用一张重点版本块。
- 历史版本改成轻量时间线或行列表。
- 未来计划不得与稳定功能混写。
- 版本卡允许轻微上浮，但时间线行不缩放。

### 8.4 FAQ

- 保留搜索和分类筛选。
- FAQ 行悬停只改变背景和边框，不缩放。
- 展开动画 180-220ms。
- 搜索结果数量继续使用 `aria-live`。

### 8.5 Security

- 保持安静、严肃、易扫描。
- 不应用营销型大卡片和强动效。
- 只对可执行操作和外部链接提供悬停反馈。
- 数据路径和安全边界优先于视觉装饰。

### 8.6 Status

- 状态页保持运维工具风格。
- 状态卡允许轻微上浮 2px，但状态颜色不能因悬停变化。
- 不引入自动动画造成“正在变化”的错觉。

### 8.7 404

- 保持简短。
- 提供首页、文档和下载三个清晰入口。
- 不需要大面积卡片或复杂插画。

---

## 9. 组件改造清单

### 9.1 新增

- `website/src/components/InteractiveCard.vue`
- `website/src/components/RevealOnScroll.vue`
- `website/src/components/ProductShowcase.vue`
- `website/src/components/SectionHeading.vue`
- `website/src/components/ProductScreenshot.vue`
- `website/src/data/home.ts`
- `website/src/data/faq.ts`，用于首页与 FAQ 页面共享高频问题

只有真实减少重复时才创建组件。不要为单一页面的两行结构创建抽象。

### 9.2 修改

- `website/src/styles.css`
- `website/src/pages/HomePage.vue`
- `website/src/components/SiteHeader.vue`
- `website/src/components/SiteFooter.vue`
- `website/src/components/PageIntro.vue`
- `website/src/components/ImageViewer.vue`
- 其余页面按第 8 节做局部视觉统一

### 9.3 保留

- 现有 Vue Router 路由。
- SSG 构建方式。
- release 配置单一数据源。
- canonical 和 SEO 生成逻辑。
- 图片非空像素检查。
- 无横向溢出与 Axe 可访问性门禁。

---

## 10. 响应式要求

### 10.1 断点

继续围绕现有断点工作：

- 宽桌面：`>= 1280px`
- 桌面/窄窗口：`901-1279px`
- 平板：`641-900px`
- 手机：`<= 640px`
- 最小验收：`320px`

### 10.2 稳定布局

- 卡片图标、标签、按钮出现时不得改变卡片高度。
- 产品演示切换不得引起页面明显跳动。
- 图片使用 `aspect-ratio` 和明确宽高。
- 导航打开时不得遮挡页面主要操作。
- 320px 下长单词和 URL 必须换行或缩小到可读范围。
- 所有按钮文字必须完整显示。

### 10.3 Hero 首屏

在以下视口必须露出下一章节：

- 1440x900
- 1280x720
- 900x650
- 412x915
- 320x568

真实产品画面不能被裁成只剩氛围背景，用户必须看到可识别的 Amber UI。

---

## 11. 可访问性与交互

- 所有 hover 状态必须有键盘 `focus-visible` 对应状态。
- 不能仅用颜色表示选择、成功或错误。
- 图标按钮需要可读 `aria-label` 和 tooltip。
- 标签切换使用标准 tab 语义，支持方向键或至少正常 Tab/Enter。
- 展开组件使用正确的 `aria-expanded`。
- 不得破坏 ImageViewer 的焦点恢复。
- `prefers-reduced-motion` 下禁用上浮和进入动画。
- 文本对比度至少满足 WCAG AA。
- 页面只有一个 H1。

---

## 12. 性能与资源

- 首屏只预加载 Hero 所需图片。
- 非首屏截图 `loading="lazy"`。
- 为桌面和移动 Hero 提供不同尺寸位图。
- 不用视频替代可以用静态产品截图说明的功能。
- 不添加大型动画依赖；优先 CSS + IntersectionObserver。
- 不使用外部字体作为页面可用的前置条件。
- 构建后检查单个图片体积和总 JS 增量。
- 官网不可依赖 Amber Worker 才能显示基础内容。

---

## 13. 文案要求

### 13.1 Hero

不要继续使用：

```text
基于 sub2api 网关能力，借鉴 CCSwitch 使用体验……
```

作为 Hero 主描述。

推荐：

```text
在 Windows 上统一管理账号、代理、共享和本地/远程 Codex 接入。
```

### 13.2 生态说明

在页脚或 README 保留：

```text
Amber 是独立开源项目，与 OpenAI、sub2api、CCSwitch 不存在官方隶属或背书关系。
```

可以在适当的“项目定位”区域说明 Amber 吸收了 sub2api 网关能力和 CCSwitch 的易用性思路，但不能让用户误认为三者存在官方关系。

### 13.3 禁止文案

- 绝对安全。
- 完全匿名。
- 官方无法看到任何数据。
- 永不封号。
- 无限并发。
- 零延迟。
- 未经验证的用户量、下载量和成功率。

---

## 14. 测试与验收

### 14.1 固定 Node 24

必须使用：

```text
C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe
```

禁止调用系统 Node 18，也不要使用裸 `node`、`npm` 或 `npx` 执行验收。

### 14.2 必须通过

在 `website/` 目录运行：

```powershell
$Node = 'C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'

& $Node '.\node_modules\vue-tsc\bin\vue-tsc.js' --noEmit
& $Node '.\node_modules\vitest\vitest.mjs' run
& $Node '.\node_modules\vite-ssg\bin\vite-ssg.js' build
& $Node '.\scripts\check-dist.mjs'
& $Node '.\scripts\check-links.mjs'
& $Node '.\scripts\check-secrets.mjs'
& $Node '.\node_modules\@playwright\test\cli.js' test --workers=4
```

### 14.3 新增 E2E 断言

必须增加：

- 首页产品演示四个标签均可键盘切换。
- 演示切换前后容器高度变化不超过 2px。
- 桌面 hover 后卡片 transform 不为 `none`。
- 触摸视口不依赖 hover 才能看到操作。
- `prefers-reduced-motion: reduce` 时卡片 transform 为 `none`。
- 320px、640px、900px、1440px 无横向滚动。
- Hero 首屏能看到下一章节。
- 所有产品截图加载成功且非空白。
- 首页 FAQ 与 FAQ 页共享的问题文本一致。
- `www` 和 canonical 不写入不同主域名。

### 14.4 视觉截图

Playwright 必须生成：

- `home-desktop.png`
- `home-narrow.png`
- `home-mobile.png`
- `showcase-desktop.png`
- `docs-desktop.png`
- `download-mobile.png`

逐张检查：

- 文本不重叠。
- 卡片不套卡片。
- 截图清晰可读。
- 按钮不溢出。
- 页面不是单一米色或单一深色。
- 没有装饰性渐变球。

---

## 15. 实施顺序

### M1：全局 Token 与基础组件

- 调整颜色、阴影、容器和章节间距。
- 实现 InteractiveCard、RevealOnScroll、SectionHeading。
- 完成 reduced-motion。
- 调整 Header/Footer。

### M2：首页结构

- 重做 Hero。
- 新增可信度条和能力卡片。
- 新增 ProductShowcase。
- 新增工作流、重点功能带、场景和 FAQ。
- 新增最终 CTA。

### M3：内页统一

- Download、Docs、Changelog、FAQ、Security、Status、404。
- 只统一视觉语言，不改业务事实和版本数据。

### M4：截图与内容

- 生成 v0.4.4 脱敏演示截图。
- 检查图片像素中的敏感信息。
- 更新说明文字和 alt/caption。

### M5：完整测试

- TypeScript、Vitest、SSG、dist/link/secret checks。
- Playwright 全视口。
- Axe、键盘、reduced-motion、画布像素检查。

### M6：提交与发布

- 提交只包含 `website/` 和本设计直接相关的官网文档。
- 不提交旧版未跟踪脚本、旧 PDF 或 `patches_v024/`。
- 推送后确认 Cloudflare Pages 自动部署成功。
- 验证 `amberapp.asia` 200、`www` 301、文档深链接正常。

---

## 16. 完成标准

只有满足以下条件才算完成：

1. 首页不再只有 Hero 和三项能力条。
2. 第一屏展示真实 Amber 产品界面。
3. 用户 10 秒内能找到下载按钮和当前版本。
4. 核心功能卡片具备统一、克制的上浮反馈。
5. 产品演示支持账号、代理、共享、Codex 四个视图。
6. 移动端不依赖 hover，且无横向溢出。
7. reduced-motion 用户不会看到上浮和滚动动画。
8. 不复制 CCSwitch 的品牌、渐变光球、大圆角和分栏 Hero。
9. 不编造用户评价、统计数字或安全承诺。
10. 所有现有 SEO、Release、文档、链接、可访问性测试继续通过。
11. Cloudflare Pages 能从 GitHub `main` 自动部署。
12. `https://www.amberapp.asia/*` 继续 301 到根域名并保留路径与查询参数。

---

## 17. 另一个开发窗口的启动说明

仓库：

```text
D:\Study\other\sub2api\sub2api-desktop\Amber
```

当前基线：

```text
main: 18cc4c0
Amber release tag: v0.4.4
website production: https://amberapp.asia
```

开始前：

1. 阅读根目录 `AGENTS.md`。
2. 阅读本文件全文。
3. 检查 `git status`，保留用户现有未跟踪文件。
4. 建议创建独立分支 `website-visual-refresh`。
5. 只修改本文件允许的官网范围。
6. 不安装、卸载、启动或停止用户的 Amber。
7. 完成后先提供截图和测试结果，再决定是否合并和推送。

