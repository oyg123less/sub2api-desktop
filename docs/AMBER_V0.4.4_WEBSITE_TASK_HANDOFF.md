# Amber v0.4.4 官网独立任务完整交接文档

> 交接对象：独立 Codex 官网开发任务
> 原始仓库：`D:\Study\other\sub2api\sub2api-desktop\Amber`
> 建议分支：`v0.4.4-website`
> 建议方式：独立 Git Worktree
> 编写日期：2026-07-20
> 当前正式版本：v0.4.3
> 目标版本：v0.4.4
> 文档性质：产品、代码、基础设施、凭据边界和官网开发的一次性交接

---

## 0. 新任务开始前必须知道的事情

该任务只负责 Amber 官方网站，不负责 Amber 客户端、Go Sidecar、Cloudflare Worker、D1、桌面云账户页面或安装包。

新任务开始后必须先完整阅读：

1. `D:\Study\other\sub2api\sub2api-desktop\Amber\AGENTS.md`
2. `D:\Study\other\sub2api\sub2api-desktop\Amber\docs\AMBER_V0.4.4_WORKSPACE_DEVICE_ROUTING_PLAN.md`
3. `D:\Study\other\sub2api\sub2api-desktop\Amber\docs\AMBER_V0.4.4_WEBSITE_TASK_HANDOFF.md`
4. `D:\Study\other\sub2api\sub2api-desktop\Amber\README.md`
5. `D:\Study\other\sub2api\sub2api-desktop\Amber\docs\USAGE.md`

前两份 v0.4.4 文档当前可能是原始工作区中的未提交文件。独立 Worktree 不一定自动包含它们，因此必须按上面的绝对路径只读打开，不要假设它们已存在于新分支。

### 0.1 文件所有权

官网任务允许：

- 新建和修改 `website/`。
- 在 `website/` 内建立独立 `package.json`、锁文件、源码、测试、静态资源和 Cloudflare Pages 配置。
- 只读使用现有 `app-icon.png`、`logo.svg`、`src-tauri/icons/` 和 `src/assets/docs/`。
- 在官网分支提交 `website/` 内的改动。

官网任务禁止修改：

- `core/`
- `cloud/`
- `src/`
- `src-tauri/`
- 根目录 `package.json` 和 `package-lock.json`
- `.github/workflows/`
- D1 migrations
- Amber 版本号
- NSIS/MSI 配置
- 当前已安装 Amber
- v0.4.4 核心开发计划

如官网确实需要核心仓库提供数据或素材，只记录需求并交回主任务，不跨边界修改。

### 0.2 发布边界

官网任务不得自行：

- 修改生产 DNS。
- 部署生产 Worker。
- 应用 D1 migration。
- 修改 Worker Secret。
- 提高最低客户端版本。
- 发布 GitHub Release。
- 上传或覆盖安装包。
- 安装、卸载、停止或重启用户当前 Amber。
- 把任何 Secret 写入前端、Markdown、Git、截图或日志。

官网任务交付源码、构建结果、截图、测试结果和提交哈希。生产合并与发布由主任务统一完成。

---

## 1. Amber 是什么

Amber 是 Windows 桌面本地网关工具，将用户自己持有的 ChatGPT OAuth 账号或 OpenAI 兼容 API 账号转换为本地 OpenAI 兼容接口。

默认本地地址：

```text
http://127.0.0.1:8080/v1
```

主要目标用户：

- 使用 Codex CLI 或 Codex VS Code 扩展的个人用户。
- 需要把 Windows 电脑上的账号、代理和网络出口提供给远程服务器 Codex 的用户。
- 管理多个自有账号、并发队列、代理和额度的用户。
- 希望通过可撤销共享把一个或多个账号授权给其他 Amber 用户的用户。

Amber 不是：

- OpenAI 官方产品。
- sub2api 官方桌面版。
- CCSwitch 官方产品。
- 公共售卖 API 平台。
- 无需授权的账号共享服务。
- 聊天或社交软件。

官网可以提到 Amber 参考了 sub2api 的网关能力和 CCSwitch 类工具的配置切换体验，但必须注明 Amber 是独立项目，不能暗示官方隶属、合作或背书。

### 1.1 当前功能

- ChatGPT OAuth 账号导入。
- Base URL + API Key 账号导入。
- 单个 JSON、多个 JSON、单文件多账号导入。
- HTTP、HTTPS、SOCKS5 代理。
- 账号级代理绑定和批量代理配置。
- 账号测试和批量测试。
- 账号并发和等待队列。
- 账号健康、限流和自动恢复。
- 本地 `/v1/models`、`/v1/chat/completions`、`/v1/responses`。
- 模型目录和价格展示。
- 请求、Token、延迟、错误和费用统计。
- Codex 本地注入。
- SSH 远程直连注入。
- SSH 反向隧道注入。
- 主机密钥指纹确认。
- 加密云同步。
- 多账号共享池。
- 每位接收者独立 Guest Key、RPM、并发和额度。
- Owner Relay 本地回流。
- Worker-direct API 账号共享。

### 1.2 产品原则

1. 本地功能无需注册云账号。
2. 云同步、多设备和共享才需要云账号。
3. 默认仅监听本机，禁止默认暴露公网。
4. 用户凭据和代理密码属于最高敏感信息。
5. 官网只展示产品和文档，不处理用户凭据。
6. 共享必须可暂停、撤销、限流和审计。
7. 任何自动恢复都不能造成重复上游请求。

---

## 2. 当前仓库和版本状态

### 2.1 Git

```text
GitHub: https://github.com/oyg123less/sub2api-desktop
当前分支: main
当前提交: f52ecc30fc05d1e8ac19d179911742805a68fc42
短提交: f52ecc3
提交说明: style: satisfy Go formatting gate
```

当前主工作区存在用户历史未跟踪文件，官网任务不得删除、移动或提交它们：

- `build_form_fix.bat`
- `build_v024.bat`
- `build_v031.bat`
- `docs/AMBER_V0.2.0_DEVELOPMENT_SPEC.pdf`
- `docs/AMBER_V0.2.0_V0.2.1_CODE_REVIEW.md`
- `patches_v024/`

当前新建但尚未提交的 v0.4.4 计划：

- `docs/AMBER_V0.4.4_WORKSPACE_DEVICE_ROUTING_PLAN.md`
- `docs/AMBER_V0.4.4_WEBSITE_TASK_HANDOFF.md`

### 2.2 当前正式发布

```text
版本: v0.4.3
Release: https://github.com/oyg123less/sub2api-desktop/releases/tag/v0.4.3
安装包: Amber_0.4.3_x64-setup.exe
SHA-256: 724988948FD9B8E7CA8208C4D9046766CB9DAF79AB50AAA3014468DA5F1C7F12
```

当前 CI 成功记录：

<https://github.com/oyg123less/sub2api-desktop/actions/runs/29692627506>

官网在 v0.4.4 正式发布前必须继续把 v0.4.3 标为最新版本。不能提前把尚未发布的 v0.4.4 展示为可下载正式版。

### 2.3 版本演进

| 版本 | 主要内容 |
| --- | --- |
| v0.3.1 | 云同步与共享可靠性基础 |
| v0.3.2 | 账号面板、批量操作、并发队列和 UI 调整 |
| v0.3.3 | 同步幂等、超时安全和可靠推送 |
| v0.4.0 | 云账号、好友、多账号共享和 Owner Relay 初版 |
| v0.4.1 | 账号操作、统计费用、文档、云网络设置和稳定性 |
| v0.4.2 | 共享码 + 临时密码快速共享 |
| v0.4.3 | 云共享在云账户、账号页面、测试和 Codex 调用中的数据一致性 |
| v0.4.4 | 计划中：移除好友、工作区隔离、设备定向共享、可靠注入和官网 |

---

## 3. 当前技术架构

### 3.1 桌面端

```text
Vue 3 + TypeScript + Vite
        |
        v
Tauri v2 / Rust 桌面壳
        |
        v
Go Sidecar
        |
        +--> 本地 OpenAI 兼容 API
        +--> SQLite + 本机加密
        +--> 账号、代理、调度和统计
        +--> Codex 配置与 SSH 隧道
        +--> Amber Cloud 同步和 Owner Relay
```

关键目录：

```text
src/                        Vue 桌面前端
src/views/                  主要页面
src/components/cloud/       云账户与快速共享组件
src-tauri/                  Rust 桌面壳、资源和 NSIS
core/internal/store/        SQLite 与迁移
core/internal/gateway/      本地 OpenAI 网关和调度
core/internal/cloudsync/    云登录、同步、共享和 Relay
core/internal/codexcfg/     本地 Codex 配置
core/internal/codexremote/  SSH 远程注入和隧道
cloud/                      Cloudflare Worker
docs/                       使用和开发文档
tests/e2e/                  Playwright 测试
```

主要桌面页面：

- `Dashboard.vue`
- `Accounts.vue`
- `Proxies.vue`
- `Statistics.vue`
- `Models.vue`
- `Cloud.vue`
- `Codex.vue`
- `Shop.vue`
- `Docs.vue`
- `Settings.vue`

### 3.2 Cloudflare Worker

当前 Worker：

```text
Worker name: amber-cloud-api
当前公开入口: https://amber-cloud-api.484486528.workers.dev
目标自定义入口: https://api.amberapp.asia
当前 /health 版本: 0.4.3
```

`api.amberapp.asia` 尚未作为生产 API 入口完成配置。官网任务不得假设它已经可用。

当前 Cloudflare 资源定义在 `cloud/wrangler.toml`：

```text
D1 binding: DB
D1 database: amber-cloud
D1 database id: bab3a69b-ad29-4008-8089-31ca999c43dd

KV binding: SESSIONS
KV id: c258f96bc12a4b5682f8451be452b4ed
KV preview id: 1f85dbcea8884104a31544088df407b5

Durable Object: OWNER_RELAY / OwnerRelay
Durable Object: SHARE_ACCESS / ShareAccessCoordinator
Durable Object: SHARE_CONNECT_GUARD / ShareConnectGuard
```

D1 migrations 当前到：

```text
0001_m1.sql
0002_m2_sharing.sql
0003_m3_idempotency.sql
0004_cloud_friends_share_groups.sql
0005_email_delivery_events.sql
0006_connect_codes.sql
0007_user_events.sql
0008_v043_client_version.sql
```

当前 `0008_v043_client_version.sql` 将最低和最新客户端版本都设为 `0.4.3`。

### 3.3 云端当前能力

- 邮箱注册和验证码。
- 登录、refresh token 轮换、退出和强制注销。
- D1 用户、保险库、共享和审计元数据。
- KV 会话与验证状态。
- 加密保险库同步。
- 好友 API，v0.4.4 计划停用前端依赖。
- 分享组和接收授权。
- 共享码、临时密码和领取保护。
- Owner Relay WebSocket。
- 共享请求 RPM、并发和额度协调。
- QQ SMTP 国内邮箱优先，Resend 作为其他邮箱发送渠道。
- 最低客户端版本门禁。

### 3.4 当前文档可能过时的内容

`cloud/README.md` 仍含部分早期 v0.4.0 描述，例如好友和 Owner Relay 尚未完成等历史说明。官网不能直接整段复制，必须以当前代码、v0.4.3 修复文档和 v0.4.4 计划为准。

根 `README.md` 和 `docs/USAGE.md` 当前仍描述好友功能。v0.4.4 核心开发完成前，官网需要把新共享流程标记为“v0.4.4 规划”或使用可替换内容，不得提前误导当前 v0.4.3 用户。

---

## 4. v0.4.4 已确定的产品方向

详细实现见：

`docs/AMBER_V0.4.4_WORKSPACE_DEVICE_ROUTING_PLAN.md`

官网必须理解以下结论。

### 4.1 本地功能不要求注册

无需云账号即可：

- 导入账号。
- 配置代理。
- 启动本地服务。
- 使用本地 Codex 注入。
- 使用 SSH 远程注入。
- 查看本地统计和日志。

需要云账号的功能：

- 云同步和备份。
- 发起共享。
- 接收长期共享。
- 多设备同步。
- 管理共享授权。

官网不能把 Amber 描述成“必须注册后才能使用”。

### 4.2 移除好友功能

v0.4.4 计划从产品界面和主流程移除：

- 好友标签页。
- 好友申请。
- Friend Code。
- 共享前选择好友。
- 等待好友接受。

新流程：

```text
共享者：选择账号 -> 开始共享 -> 发送共享码和临时密码
接收者：输入共享码和临时密码 -> 连接并使用
```

每位接收者仍然拥有独立 Guest Key、限流、并发、额度、暂停和删除权限。

### 4.3 同机多云账号工作区隔离

v0.4.4 计划采用每个云用户独立工作区：

```text
AmberData/
├── 本地工作区
└── workspaces/
    ├── 用户A/sub2api.db + key
    └── 用户B/sub2api.db + key
```

关键行为：

- 未登录前的本地工作区可以由用户显式绑定给一个云账号。
- 绑定后永久属于该账号。
- 退出登录不删除数据，也不解除归属。
- 另一个云账号必须创建或打开独立工作区。
- 不同用户不共享账号、代理、同步队列、Guest Key、日志和 SSH 目标。
- 旧数据库出现多个历史用户时不自动猜测归属。

### 4.4 同账号多设备定向共享

当前 v0.4.3 按用户 Owner Relay 的在线主设备优先选择，不能保证该设备拥有目标账号。

v0.4.4 计划改为：

- 新共享默认绑定创建共享的具体设备。
- 同账号其他设备不会随机接管。
- 用户可显式配置备用设备。
- 备用设备必须拥有目标 `account_uid` 且代理、健康和并发均可用。
- 上游请求开始前允许安全故障转移。
- `upstream_started` 后禁止跨设备重放。

官网可以用“共享由你指定的电脑提供服务”解释，不要使用 Durable Object、account_uid 等内部术语。

### 4.5 Codex 一键接入修复

v0.4.3 及更早版本允许用户在本地 API 服务未启动时注入 `127.0.0.1:8080/v1`，随后 Codex 返回 502。

v0.4.4 计划把按钮改为：

```text
启动服务并注入
```

只有本地服务启动、健康检查和 `/v1/models` 验证通过后才写 Codex 配置。

官网 FAQ 必须解释旧版本 502 的应急处理：先启动 Amber 服务，再使用 Codex。

---

## 5. 官网目标与信息架构

### 5.1 域名规划

```text
amberapp.asia           Amber 官网
www.amberapp.asia       重定向到官网
api.amberapp.asia       Amber Cloud Worker
docs.amberapp.asia      在线文档
download.amberapp.asia  跳转 GitHub Release
status.amberapp.asia    服务状态
mail.amberapp.asia      邮件发件域名
```

当前已知状态：

- `amberapp.asia` 已购买，注册商为阿里云。
- 用户此前使用阿里云云解析 DNS。
- 邮件相关 DNS 已配置并已能发送验证码。
- `mail.amberapp.asia` 已用于 Resend 发件身份。
- 国内邮箱当前优先通过 QQ SMTP 发送。
- `api.amberapp.asia` 尚未完成 Worker Custom Domain。
- 官网和文档站尚未部署。
- 任何 DNS 变更都必须由主任务与用户确认，官网任务不能操作。

### 5.2 网站技术方案

推荐：

- 独立 `website/`。
- Vue 3 + TypeScript + Vite。
- 静态生成或纯静态路由。
- 部署目标 Cloudflare Pages。
- 官网构建产物 `website/dist/`。
- 官网自己的 `package.json` 和锁文件。
- 图标使用 `lucide-vue-next`。
- 不依赖桌面应用运行时。
- 不从浏览器调用私有管理 API。

不得把根桌面应用 Vite 配置直接复用为官网配置，避免两个产品的依赖和构建相互影响。

### 5.3 页面清单

#### 首页

- H1 使用产品名 `Amber`。
- 辅助文案说明 Windows 本地账号网关、Codex 接入和可控共享。
- 第一屏使用真实 Amber 软件截图，不用无关科技背景。
- 主按钮：下载最新版、查看使用文档。
- 第一屏显示当前稳定版本，v0.4.4 发布前为 v0.4.3。
- 首屏下方露出下一部分内容。

#### 下载页

- 当前版本。
- Windows x64。
- 发布时间。
- 安装包大小。
- SHA-256。
- GitHub Release 链接。
- 覆盖安装说明。
- Windows SmartScreen 说明。
- 历史版本入口。

下载文件继续由 GitHub Release 托管，官网不能让 Worker 代理 EXE。

#### 使用文档

- 安装与首次启动。
- 账号导入。
- 代理配置与批量应用。
- 启动本地服务。
- 本地 Codex 注入。
- SSH 主机密钥确认。
- SSH 反向隧道。
- 云账号注册和同步。
- 共享码和临时密码。
- 多设备承载说明。
- 工作区切换说明。
- 常见错误处理。

文档必须有左侧目录、清晰小标题、大图、图片点击放大、重点和警告样式。不能把多张小截图并排堆放。

#### 更新日志

- 当前稳定版。
- 历史版本。
- 新增、修复、兼容性和已知限制分栏。
- 未发布 v0.4.4 使用“即将发布”状态，不伪装为正式版本。

#### 常见问题

至少包含：

- 127.0.0.1:8080 连接失败。
- 502 Bad Gateway。
- 本地服务未启动。
- 端口冲突。
- 代理和 TUN。
- 账号测试成功但 Codex 不可用。
- SSH 反向隧道为什么要求本机 Amber 在线。
- 主机密钥如何确认。
- 云同步 DNS/TCP/TLS/HTTP 失败。
- 共享者设备离线。
- 同账号多设备走哪台设备。
- 切换云账号后数据放在哪里。

#### 安全与隐私

- 本地数据范围。
- 云同步数据范围。
- 共享路径。
- 设备回流说明。
- 日志和截图脱敏要求。
- 如何撤销共享。
- 如何删除本地工作区。
- 不夸大端到端加密和匿名性。

#### 服务状态页外壳

- Amber Cloud API。
- 登录与注册。
- 验证码邮件。
- Owner Relay。
- 当前事件和维护公告。

当前任务只实现页面和状态数据接口边界，不自行接入生产监控或修改 Worker。

### 5.4 视觉方向

- 延续 Amber 客户端的安静、工具化风格。
- 白色和浅灰为主背景。
- 深灰正文。
- 琥珀色作为品牌强调。
- 绿色只表示健康和成功。
- 红色只表示危险和错误。
- 避免大面积橙棕色造成单一色调。
- 不使用渐变球、光斑、无关 3D 装饰。
- 卡片圆角不超过 8px。
- 使用真实软件截图。
- 页面必须支持桌面、窄窗口和手机。
- 图片支持点击查看原图。
- 所有按钮文字必须完整显示。
- 官网第一屏不能做成左右卡片式营销模板。

### 5.5 官网文案原则

可以写：

- Windows 本地 OpenAI 兼容网关。
- 管理自己的 ChatGPT OAuth 和 OpenAI 兼容账号。
- 为 Codex 提供本地和远程接入。
- 可撤销、可限流的账号共享。
- 参考 sub2api 和 CCSwitch 类工具的工作流。

禁止写：

- OpenAI 官方。
- sub2api 官方桌面版。
- CCSwitch 官方合作版。
- 绝对防封。
- 绝对匿名。
- 永久免费且无任何限制，除非产品策略正式确认。
- 所有数据官方都无法看到，除非经过代码和架构审计确认。
- v0.4.4 已发布，直到 Release 实际存在。

---

## 6. 可复用品牌与截图资产

### 6.1 品牌

- `app-icon.png`
- `logo.svg`
- `src-tauri/icons/icon.png`
- `src-tauri/icons/icon.ico`
- `src-tauri/icons/128x128.png`
- `src-tauri/icons/128x128@2x.png`

### 6.2 当前文档截图

目录：`src/assets/docs/`

- `dashboard.png`
- `accounts.png`
- `account-details.png`
- `import.png`
- `proxies.png`
- `statistics.png`
- `models.png`
- `cloud-register.png`
- `cloud-workspace.png`
- `codex.png`
- `codex-local.png`
- `codex-host-key.png`
- `settings.png`

注意：

- 这些截图主要来自 v0.4.1-v0.4.3。
- `cloud-workspace.png` 可能包含即将移除的好友界面，不应作为 v0.4.4 最终截图。
- v0.4.4 云账户、工作区、设备定向共享和“启动服务并注入”完成后，需要主任务提供新截图。
- 当前阶段官网可以完成布局和图片组件，但应使用明确的可替换资源映射。
- 如果截图来自用户真实登录账号，邮箱、账号 ID、Token、Guest Key、Base URL、代理地址和设备名称必须打码。

---

## 7. 凭据和授权交接

### 7.1 绝对规则

任何真实凭据都不得出现在：

- 本文档。
- `website/` 源码。
- Vite `VITE_*` 变量。
- Git 提交。
- GitHub Issue。
- README。
- 构建日志。
- Playwright 截图。
- 浏览器 localStorage/sessionStorage。
- 聊天交接文本。

Vite 的 `VITE_*` 会被编译进浏览器 JavaScript，因此只能放公开配置，绝不能放 Secret。

### 7.2 以前在对话中出现过的敏感凭据

此前用户曾在对话中粘贴过以下真实值：

- GitHub Personal Access Token。
- Cloudflare API Token。
- Turnstile Secret。
- Resend API Key。
- QQ SMTP 授权码。

这些值不得复制到本交接文档或新任务。由于它们曾以明文出现在对话中，应在 v0.4.4 正式生产发布前轮换。

Turnstile Site Key 是公开客户端配置，不属于 Secret，但仍应从统一公开配置读取，不在多个页面重复硬编码。

### 7.3 Worker Secret 名称

当前代码可能使用：

| Secret | 用途 | 轮换注意事项 |
| --- | --- | --- |
| `JWT_SECRET` | Access/refresh 会话签名与派生 | 轮换会使现有会话失效，应安排维护窗口 |
| `TURNSTILE_SECRET` | 注册 Turnstile 服务端验证 | 与对应 Widget 配套轮换 |
| `RESEND_API_KEY` | 非国内邮箱验证码发送 | 可直接创建新 Key 后切换 |
| `RESEND_WEBHOOK_SECRET` | Resend Webhook 签名校验 | 与 Webhook 配置配套更新 |
| `QQ_SMTP_USER` | 国内邮箱发件账号 | 属于个人信息，按 Secret 管理 |
| `QQ_SMTP_AUTH_CODE` | QQ SMTP 授权码 | 已在对话暴露，应重新生成 |
| `ADMIN_API_KEY` | 管理员面板第二因素 | 轮换后更新管理员保存的值 |
| `SHARE_KMS_KEY` | Worker-direct 共享凭据 AES-GCM 加密 | 禁止直接轮换，否则历史密文无法解密；必须做双密钥迁移 |
| `SHARE_CONNECT_PEPPER` | 临时共享密码哈希 Pepper | 轮换会让当前临时密码失效，可在维护窗口执行 |

当前 `cloud/src/types.ts` 把 `SHARE_KMS_KEY` 和 `SHARE_CONNECT_PEPPER` 定义为可选，但生产共享功能实际依赖它们。

### 7.4 非 Secret 的生产绑定

以下内容已在 `wrangler.toml` 中公开配置，不等于访问凭据：

- Worker 名称。
- D1 database ID。
- KV namespace ID。
- Durable Object binding。
- `RESEND_FROM` 发件身份。
- compatibility date。

这些 ID 可以出现在仓库，但不能据此认为新任务有生产操作权限。

### 7.5 当前 CLI 授权状态

2026-07-20 在非交互命令环境执行 `wrangler secret list` 时失败，原因是当前环境没有设置 `CLOUDFLARE_API_TOKEN`。因此：

- 不能确认生产环境现有 Secret 名称是否全部齐全。
- 不能假设此前粘贴的 Cloudflare Token 仍有效。
- 官网任务不应尝试部署。
- 主任务正式部署前必须使用新 Token 执行 `wrangler secret list` 和部署前审计。

当前环境也没有可用的 `gh` 命令。GitHub 发布由主任务通过现有 Git 凭据或 CI 完成，官网任务不得依赖 `gh`。

### 7.6 正确的授权方式

生产操作必须使用：

- 操作系统环境变量。
- Wrangler 交互式 `secret put`。
- GitHub Actions Secret/Variable。
- 用户密码管理器。
- Cloudflare 控制台中的 Secret。

不要把 Token 作为 PowerShell 命令参数写入历史记录。不要在工具输出中打印环境变量。

建议将 Cloudflare 权限拆分：

- Worker/D1/KV 部署 Token，由主任务持有。
- Pages 部署 Token，仅在官网正式发布阶段使用。
- DNS 修改 Token 单独保管，官网开发任务不持有。

### 7.7 生产轮换顺序建议

在 v0.4.4 发布前由主任务执行：

1. 撤销旧 GitHub PAT，改用新细粒度 Token 或 GitHub Actions。
2. 撤销旧 Cloudflare API Token，创建最小权限 Token。
3. 轮换 Turnstile Secret 并验证注册。
4. 轮换 Resend API Key 并发送真实测试邮件。
5. 重新生成 QQ SMTP 授权码并测试 QQ、163 等国内邮箱。
6. 评估并轮换 `ADMIN_API_KEY`。
7. 如需轮换 `JWT_SECRET`，提前通知所有用户重新登录。
8. 不直接轮换 `SHARE_KMS_KEY`，先实现密文迁移或双 Key 解密。
9. 轮换 `SHARE_CONNECT_PEPPER` 后让旧临时共享密码统一失效。
10. 运行 Secret 扫描，确认仓库和构建产物没有真实值。

---

## 8. 构建环境和命令

### 8.1 强制 Node 24

仓库禁止使用系统 Node 18。必须使用：

```text
C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe
```

官网任务应在 `website/` 内通过该 Node 直接执行本地 JS 入口，例如：

```powershell
& 'C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' '.\node_modules\vite\bin\vite.js' build

& 'C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' '.\node_modules\vitest\vitest.mjs' run

& 'C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' '.\node_modules\@playwright\test\cli.js' test
```

不得调用裸 `node`、`npm` 或 `npx` 进行项目验证。

依赖安装如必须调用 npm CLI，也应使用 Node 24 直接运行 npm 的 CLI 入口，并在交付中记录确切命令。

### 8.2 官网开发服务器

官网完成初版后应启动独立开发服务器，不能占用桌面应用默认 Vite `5173`。建议使用：

```text
http://127.0.0.1:4174
```

如端口已占用，选择其他端口并在交付中说明。

### 8.3 验证

至少执行：

- TypeScript 类型检查。
- Vitest。
- 生产构建。
- Playwright 桌面视口。
- Playwright 手机视口。
- 链接检查。
- 图片加载检查。
- 键盘导航检查。
- 基本无障碍检查。
- 页面标题、描述和 canonical 检查。
- 不含 Secret 的静态扫描。

---

## 9. 官网数据和发布配置

官网不应把版本号和下载信息散落硬编码在多个组件。建议建立单一公开配置：

```ts
export interface PublicReleaseInfo {
  version: string;
  releaseUrl: string;
  installerName: string;
  sha256: string;
  publishedAt: string;
  status: "stable" | "upcoming";
}
```

初始稳定数据使用 v0.4.3。v0.4.4 正式发布后由主任务只修改这一处公开配置。

官网运行时不得为了显示版本信息请求管理员 API、D1 或带鉴权 Worker 接口。

状态页可以读取未来提供的公开、只读、无凭据状态 JSON，但在该接口正式存在前使用静态“尚未接入监控”状态，不伪造实时数据。

---

## 10. SEO 和项目关系说明

建议标题方向：

```text
Amber - Windows 本地 OpenAI 兼容网关与 Codex 接入工具
```

可以自然出现：

- sub2api desktop
- Codex remote access
- OpenAI compatible gateway
- ChatGPT OAuth account management
- CCSwitch workflow

必须配套说明：

```text
Amber 是独立开源项目，与 OpenAI、sub2api、CCSwitch 不存在官方隶属或背书关系。
```

不要为蹭关键词制造虚假兼容、官方合作或商标归属表述。

建议加入：

- `SoftwareApplication` 结构化数据。
- Open Graph。
- Twitter/X Card。
- sitemap。
- robots.txt。
- canonical URL。
- 中英文页面语言标记。

---

## 11. 隐私和截图规范

官网不能出现：

- 真实邮箱完整地址。
- QQ SMTP 发件账号。
- ChatGPT account ID。
- Access/Refresh Token。
- API Key。
- Guest Key。
- 本地 API Key。
- Cloudflare Token。
- GitHub PAT。
- Admin Key。
- 代理用户名和密码。
- SSH 密码和私钥。
- 真实服务器 IP。
- 未经用户允许的设备名称。

截图脱敏后仍需检查图片像素，不能只用 CSS 在网页上遮挡，因为原图仍可下载。

官网表单如未来增加反馈功能，必须单独评审隐私、反滥用和数据存储。本任务不要自行添加收集邮箱、账号或日志的表单。

---

## 12. 当前已知问题和官网表述

### 12.1 v0.4.1/v0.4.2/v0.4.3 服务未启动后注入

现象：

```text
unexpected status 502 Bad Gateway: Unknown error
url: http://127.0.0.1:8080/v1/responses
```

已确认常见原因：用户注入了 Codex 配置，但没有启动 Amber 本地服务。

当前应急步骤：

1. 打开 Amber。
2. 在仪表盘启动服务。
3. 确认显示运行中。
4. 重新使用 Codex。

v0.4.4 将提供“启动服务并注入”。

### 12.2 云同步失败

部分国内用户无法连接 `workers.dev`。当前桌面端支持云连接设置和 DNS/TCP/TLS/HTTP 诊断。v0.4.4 计划使用 `api.amberapp.asia` 作为首选入口，但域名尚未配置完成。

### 12.3 多设备共享

v0.4.3 仍按主设备优先，不是最终设备定向方案。官网在 v0.4.4 发布前不能宣称已经支持可靠主备设备切换。

### 12.4 多云账号工作区

v0.4.3 本地普通账号和同步队列没有完整按云用户隔离。官网当前版本 FAQ 应建议用户不要在同一数据目录中反复切换不同云账号。v0.4.4 完成后再更新为独立工作区说明。

### 12.5 好友功能

v0.4.3 仍存在好友功能。v0.4.4 计划移除。官网应通过版本条件渲染对应文案，避免 v0.4.3 用户看不到实际存在的入口说明。

---

## 13. 官网任务实施顺序

### 阶段 W1：独立工程和设计系统

- 创建 `website/`。
- 建立独立依赖和构建。
- 定义颜色、字体、间距和响应式变量。
- 建立公开版本配置。
- 建立图片查看器和文档布局组件。

### 阶段 W2：首页和下载

- 首页真实产品首屏。
- 功能工作流。
- 下载页。
- v0.4.3 稳定版本数据。
- v0.4.4 upcoming 状态。

### 阶段 W3：文档和 FAQ

- 使用文档导航。
- 大图和点击放大。
- 502、服务启动、代理、SSH、云同步和共享说明。
- 版本差异提示。

### 阶段 W4：安全、更新和状态

- 安全说明。
- 更新日志。
- 服务状态页外壳。
- 项目独立性声明。

### 阶段 W5：响应式和测试

- 桌面、窄窗口和手机。
- Playwright 截图。
- 图片像素和非空检查。
- 链接和 SEO。
- Secret 扫描。

### 阶段 W6：交付

- 提交官网分支。
- 汇报提交哈希。
- 汇报构建、测试和截图结果。
- 列出等待主任务提供的 v0.4.4 最终截图与版本信息。
- 不部署生产。

---

## 14. 官网任务验收标准

1. 所有官网代码都位于 `website/`。
2. 没有修改核心任务拥有的目录。
3. 首页第一屏明确展示 Amber 和真实产品界面。
4. 当前下载仍是 v0.4.3，未提前发布 v0.4.4。
5. 本地使用无需注册的产品边界表达正确。
6. 好友功能按版本准确描述。
7. 工作区和设备定向共享说明与 v0.4.4 计划一致。
8. 文档图片足够大并可点击放大。
9. 手机和桌面页面无文字溢出、遮挡和布局跳动。
10. 不包含任何真实 Secret 或用户敏感信息。
11. 生产构建和测试全部通过。
12. 没有部署生产、修改 DNS 或操作已安装 Amber。

---

## 15. 向主任务交付时的汇报格式

官网任务完成后应提供：

```text
分支：v0.4.4-website
提交：<commit hash>
目录：website/
开发服务器：http://127.0.0.1:<port>

已完成页面：
- 首页
- 下载
- 文档
- 更新日志
- FAQ
- 安全
- 状态页外壳

验证：
- TypeScript
- Vitest
- Vite build
- Playwright desktop
- Playwright mobile
- Link check
- Secret scan

等待核心任务提供：
- v0.4.4 最终版本号和发布日期
- v0.4.4 安装包名称、大小和 SHA-256
- v0.4.4 最终云账户截图
- v0.4.4 工作区切换截图
- v0.4.4 设备定向共享截图
- v0.4.4 启动服务并注入截图
```

---

## 16. 最后提醒

- 该官网任务的目标是独立交付可合并的网站，不是接管 v0.4.4 核心开发。
- 不要从以前的对话或日志中复制任何真实授权码。
- 不要把 Cloudflare、GitHub、Resend、QQ SMTP 或管理员凭据写入文档。
- 当前 Wrangler 非交互环境未认证，不能擅自部署。
- 当前正式产品是 v0.4.3，v0.4.4 仍是开发计划。
- 官网可以先完成结构和当前版本内容，最终截图与下载信息在核心功能冻结后替换。
- 所有生产操作由主任务在完成安全审计和凭据轮换后统一执行。
