# Amber v0.4.2 连接码共享与云账户界面改造计划

> 文档状态：实施完成，Worker 已部署，待 GitHub Release 与版本门禁
> 目标版本：v0.4.2
> 基线版本：v0.4.1
> 开发分支：`v0.4.2-development`
> 修订日期：2026-07-19
> 核心模型：长期连接码 + 临时访问密码 + 每位使用者独立 Guest Key
> 体验目标：像远程连接软件一样简单，但保留 Amber 的独立授权、配额、代理和审计能力

## 实施状态（2026-07-19）

- Worker、Go Sidecar、桌面前端、文档与 NSIS v0.4.2 已完成。
- Claim 严格按“保存密钥 -> 建立禁用映射 -> 真实测试 -> 成功后启用”执行；失败状态可在收到的共享中重试，不会重复领取。
- D1 `0006_connect_codes.sql` 与 `0007_user_events.sql` 已通过本地 Worker 测试。
- Go 全套、定向 race、Rust、前端单元测试、Worker 测试、生产构建和 68 项双窗口 E2E 已通过。
- NSIS 已生成到 `src-tauri/target/release/bundle/nsis/Amber_0.4.2_x64-setup.exe`，打包过程没有安装或卸载 Amber。
- 待发布：配置生产 `SHARE_CONNECT_PEPPER` Secret、远程应用 D1 migration、部署 Worker、发布 GitHub Release，最后才将 `enforce_client_version` 改为 `true`。
- 隔离环境的 v0.4.1 -> v0.4.2 覆盖安装仍需在非用户生产环境执行；禁止用当前已安装 Amber 做该测试。

---

## 0. 方案结论

v0.4.2 将云共享的默认入口从“添加好友 -> 创建共享组 -> 邀请 -> 接受 -> 复制连接信息”调整为：

```text
共享者：开启共享 -> 复制连接信息
使用者：粘贴连接信息 -> 连接并使用
```

连接信息由两部分组成：

```text
Amber 连接码：572 814 639
临时访问密码：K7M4Q9
```

设计原则：

1. 连接码长期固定，类似远程软件的设备码，不作为秘密。
2. 临时访问密码短期有效，可以手动刷新，是领取权限的第二因素。
3. 同一连接码和密码可以允许一位或多位用户连接。
4. 每位连接成功的用户仍然获得独立 Guest Key、独立配额和独立控制状态。
5. 连接码和密码不能直接调用模型，只能在登录 Amber 后领取权限。
6. 使用者连接成功后自动加入 Amber 本地调度，不复制 Base URL/API Key。
7. 好友不再是共享前置条件，降为连接后的“常用联系人”。
8. 现有共享组、账号池、Worker Gateway、Owner Relay、配额和用量日志继续复用。
9. 普通界面不展示 recipient grant、key envelope、Worker path 等内部概念。
10. v0.4.2 使用可靠增量轮询，不加入完整 WebSocket 通知中心。

最终产品语言：

```text
把连接码和临时密码发给对方，对方输入后即可使用。
```

---

## 1. 为什么改成连接码模型

### 1.1 当前流程的问题

当前好友共享需要：

```text
交换 Friend Code
-> 发送好友申请
-> 对方接受好友
-> 创建共享组
-> 选择账号
-> 选择好友
-> 设置规则
-> 对方接受共享
-> 测试连接
-> 复制 Base URL/API Key 或手动配置
```

该流程适合长期权限管理，但不适合第一次给朋友临时使用。

### 1.2 从远程软件借鉴的内容

远程软件把复杂实现压缩为：

| 底层能力 | 用户看到的内容 |
| --- | --- |
| 设备注册和在线状态 | 设备码 |
| 身份验证 | 临时密码 |
| 中继和网络连接 | 连接按钮 |
| 权限生命周期 | 断开连接 |

Amber 对应压缩为：

| Amber 底层能力 | 用户看到的内容 |
| --- | --- |
| 共享组和账号池 | 我的共享 |
| recipient grant | 已连接用户 |
| Guest Key 和 envelope | 不展示 |
| Worker/Owner Relay | 云端可用/需要本机在线 |
| RPM、并发、配额 | 使用规则摘要 |
| 暂停和撤销 | 暂停/移除 |

### 1.3 Amber 不能照搬的部分

远程控制通常是一次实时会话，Amber 共享可能长期存在并消耗账号额度，因此不能：

- 多人共用同一个实际调用密钥。
- 默认永久开放连接。
- 使用简单密码但不做尝试限流。
- 密码正确后让匿名用户长期使用。
- 隐藏共享者无法撤销或限额的事实。

界面可以同样简单，后台必须保持逐用户独立授权。

---

## 2. 用户友好强制原则

### 2.1 两个任务，两个清晰区域

云共享首页只回答两个问题：

1. 别人怎样连接我的共享？
2. 我怎样连接别人的共享？

不要求用户先理解共享组、好友和设备。

### 2.2 普通流程不暴露技术术语

普通界面允许出现：

- 连接码。
- 临时密码。
- 共享账号。
- 可连接人数。
- 有效时间。
- 需要本机在线。
- 云端可用。

普通界面禁止出现：

- Guest Key。
- recipient grant。
- key envelope。
- worker_direct。
- owner_device。
- access key hash。
- Durable Object。

这些内容只存在于代码、开发文档和脱敏诊断中。

### 2.3 一次只要求一个主要决定

- 共享者首次只需选择“允许共享哪些账号”。
- 日常开启只需点击“开启共享”。
- 使用者只需填入连接信息并点击“连接并使用”。
- 高级配额和路由设置默认折叠。
- 删除、撤销和永久停止放入危险操作区。

### 2.4 默认安全，不默认共享全部账号

- 首次必须由共享者选择默认共享账号。
- Amber 可以排除异常账号，但不能自动加入未授权账号。
- OAuth 和绑定代理的账号自动使用本机回源。
- API Key 默认本机回源，明确确认后才能使用 Worker 直连。
- 临时密码默认 30 分钟有效、默认允许 1 人领取。
- 已连接用户不会因为刷新临时密码而被自动断开。

### 2.5 状态必须告诉用户下一步

错误不能只有红色 Toast。页面需要保留状态和操作：

```text
共享者设备未在线
这组账号通过共享者电脑访问 ChatGPT。
[重新测试]
```

### 2.6 首次配置与日常使用分开

首次开启共享：

```text
选择账号 -> 确认回源说明 -> 开启共享
```

以后开启共享：

```text
点击“开启共享”
```

不能为了宣传“一次点击”而静默完成首次凭据托管或 Codex 配置修改。

---

## 3. v0.4.2 范围

### 3.1 P0 必须完成

| 模块 | 内容 |
| --- | --- |
| 连接码 | 每个共享者一个默认长期连接码，可在高级设置中重置 |
| 临时密码 | 启用、刷新、过期、领取次数和安全校验 |
| 默认账号池 | 首次选择、后续复用、自动排除异常账号 |
| 多人连接 | 同一密码允许 1-N 人领取，每人独立 Guest Key |
| 连接并使用 | 校验、生成独立密钥、保存、测试、加入本地调度 |
| 主界面 | “我的共享”和“连接共享”两个任务面板 |
| 连接管理 | 查看、暂停、恢复、限额和移除已连接用户 |
| 收到的共享 | 查看状态、启停本地调度、测试、离开 |
| 网络路径 | 自动回源，明确显示是否需要共享者本机在线 |
| 可靠性 | 幂等、限流、超时查询、请求测试统一、事件增量刷新 |
| 兼容 | 保留现有好友和共享组数据，旧入口进入高级管理 |

### 3.2 P1 可在 P0 稳定后完成

- 把连接成功的用户添加为常用联系人。
- 从常用联系人快速复制连接信息。
- 连接信息文本智能粘贴和解析。
- Windows 系统通知。
- 多组账号池快捷切换。

### 3.3 明确不进入 v0.4.2

- 匿名设备无需登录即可连接。
- 固定永久访问密码。
- 每次连接由共享者实时审批。
- 完整 WebSocket 通知中心。
- 多设备自动故障接管。
- 好友聊天、群组和文件传输。
- 公开共享市场、搜索、收费或结算。
- 二维码。
- 自动安装、卸载或重启 Amber。

固定密码和逐次审批可在 v0.4.3 评估。v0.4.2 先把临时密码链路做稳。

---

## 4. 核心对象与状态

### 4.1 默认共享入口

每个云用户默认拥有一个共享入口：

```text
用户
└─ 默认共享入口
   ├─ 长期连接码
   ├─ 绑定的共享组/账号池
   ├─ 当前临时密码窗口
   └─ 已连接用户列表
```

连接码长期固定，用户暂停或恢复共享时不改变。

### 4.2 临时密码窗口

每次开启共享或刷新密码都创建一个新的密码窗口：

```text
active -> exhausted   达到允许领取人数
active -> expired     到达过期时间
active -> replaced    用户刷新密码
active -> stopped     用户暂停共享
```

刷新密码只影响后续连接，不撤销已经领取的独立权限。

### 4.3 已连接用户

每位连接用户对应现有的：

- `share_group_recipients` 一条 recipient grant。
- `share_access_keys` 一把独立 Guest Key。
- 独立 RPM、并发、配额和状态。
- 独立用量记录。

状态继续复用：

```text
active | paused | expired | revoked | left
```

连接码领取不创建 `pending` 状态，因为输入正确密码已经表达连接意图；领取完成后直接激活。

### 4.4 本地收到的共享

使用者本机保存：

- 加密后的 Guest Key。
- grant public ID。
- Worker Base URL。
- 是否加入本地调度。
- 健康状态和最近测试结果。

收到的共享不能再次作为共享账号转发。

---

## 5. 前端整体信息架构

### 5.1 云账户登录后的一级结构

```text
共享连接 | 常用联系人 | 高级管理
```

默认进入“共享连接”。

“高级管理”包含：

- 旧共享组。
- 账号池详细规则。
- 设备与本机回源。
- 云连接网络设置。
- 安全和同步诊断。
- 管理员入口（仅管理员）。

旧的 URL 查询参数继续兼容并跳转到对应区域。

### 5.2 页面头部

```text
云共享
通过 Amber Cloud 安全连接共享账号

云服务正常 · 本机回源在线 · 刚刚同步          [同步] [更多]
```

规则：

- 标题使用页面级字号，不使用营销式超大标题。
- 状态行保持单行，窄窗口自动换行。
- 同步是图标按钮并带 Tooltip。
- 网络、安全、退出等低频操作进入“更多”。
- 同步失败时状态行变为可点击的错误带，不另加重复卡片。

### 5.3 桌面布局

默认窗口下使用两个并列的功能面板：

```text
┌────────────────────────────────┬────────────────────────────────┐
│ 我的共享                       │ 连接他人的共享                 │
│                                │                                │
│ 连接码  572 814 639            │ 连接码                         │
│ 密码    ••••••        [显示]   │ [ 572 814 639              ]   │
│ 29 分钟后过期 · 可连接 3 人     │                                │
│                                │ 临时密码                       │
│ [复制连接信息] [刷新密码]       │ [ K7M4Q9                   ]   │
│                                │                                │
│ 2 个账号 · 需要本机在线         │ [粘贴连接信息] [连接并使用]    │
└────────────────────────────────┴────────────────────────────────┘

已连接我的共享 2 人                                      [查看全部]
我正在使用的共享 1 个                                    [查看全部]
```

两个面板是独立工具，不放入外层装饰卡片，也不相互嵌套。

### 5.4 窄窗口布局

小于 800px 时上下排列：

```text
我的共享
...

连接他人的共享
...

连接列表
```

要求：

- 输入框、按钮和文本不得横向溢出。
- 连接码保持分组显示，但允许完整复制。
- 操作按钮在窄窗口可换行，不缩小文字。
- 面板最小高度稳定，加载状态不推动后续内容跳动。

### 5.5 视觉规则

- 功能面板最大圆角 8px。
- 不使用渐变球、装饰光斑或营销插画。
- 连接码使用等宽字体，建议 20-24px，不使用 Hero 字号。
- 临时密码默认遮罩，复制不要求先显示。
- 使用现有 Icon/Lucide 图标。
- 复制、显示、刷新使用图标或图标加文字。
- 危险操作使用克制的危险色，不能与主按钮竞争。
- Hover 只做轻微 `translateY(-1px)` 或边框变化，不明显放大。
- 支持 `prefers-reduced-motion`。

---

## 6. “我的共享”面板设计

### 6.1 未配置状态

```text
我的共享

选择允许他人使用的账号，Amber 将为你生成连接码。

[设置共享账号]
```

该状态只有一个主要按钮。

### 6.2 已配置但未开启

```text
我的共享                                      已暂停

连接码 572 814 639
默认共享账号 · 2 个账号                        [更改]

临时密码将在开启后生成。

允许连接  [ - ] 1 [ + ] 人
有效时间  30 分钟

[开启共享]
```

默认值：

- 允许连接 1 人。
- 有效时间 30 分钟。
- RPM 30。
- 并发 2。
- 总配额不限。

RPM、并发和总配额不在主面板显示，进入“使用规则”。

### 6.3 已开启状态

```text
我的共享                                      可连接

连接码
572 814 639                                   [复制]

临时访问密码
••••••                                        [显示] [复制]

29:41 后过期 · 还可连接 3 人

[复制连接信息]                    [刷新密码]

2 个共享账号 · 需要本机在线                    [详情]

[暂停全部共享]
```

交互说明：

- “复制连接信息”复制连接码、密码、有效时间和 Amber 使用提示。
- “刷新密码”立即让旧密码停止新连接，不影响已连接用户。
- “暂停全部共享”暂停入口和所有 active recipient，属于确认操作。
- 恢复共享时恢复可恢复 recipient，并创建新的临时密码。

### 6.4 密码展示

- 默认遮罩，避免录屏或他人旁观泄露。
- 点击“显示”仅在当前页面会话中显示。
- 页面离开、应用最小化或 30 秒后自动重新遮罩。
- 复制密码不要求显示。
- 复制成功使用短 Toast，不改变按钮宽度。

### 6.5 复制连接信息格式

```text
Amber 共享连接
连接码：572 814 639
临时密码：K7M4Q9
有效期：29 分钟

在 Amber 的“连接他人的共享”中粘贴后，点击“连接并使用”。
```

不包含 Guest Key、Base URL 或用户邮箱。

### 6.6 刷新密码确认

刷新密码不是危险操作，不弹长篇确认。按钮下方说明：

```text
刷新后旧密码不能再连接，已连接用户不受影响。
```

点击后直接刷新并显示复制成功入口。

### 6.7 暂停和删除语义

- `刷新密码`：只阻止旧密码的新领取。
- `暂停全部共享`：暂停新领取和所有已连接用户，可恢复。
- `移除用户`：永久撤销该用户当前 Key。
- `删除共享入口`：删除默认连接入口并撤销所有用户，不可恢复，放在高级危险区。

禁止使用含义模糊的“关闭”同时承担多种行为。

---

## 7. 首次共享账号设置

### 7.1 单页设置，不使用五步向导

```text
设置共享账号

选择允许他人使用的账号

[x] ChatGPT A      正常       使用本机网络
[x] ChatGPT B      正常       使用代理 HK-01
[ ] Team API       正常       凭据保留本机
[ ] Test API       已禁用     不可选择

共享方式
智能均衡

! 选中的账号包含 OAuth/代理账号。
  使用期间，本机 Amber 需要保持在线。

[取消]                           [保存并开启]
```

### 7.2 账号行信息

每行只显示：

- 账号显示名。
- OAuth/API Key 类型图标。
- 健康状态。
- 网络摘要：本机网络、代理名称或云端可用。
- 选择框。

不显示完整 Client UID、token、代理密码或 TLS 参数。

### 7.3 自动回源规则

| 账号条件 | 默认回源 | 用户文案 |
| --- | --- | --- |
| OAuth | Owner Relay | 使用本机网络 |
| 绑定代理 | Owner Relay | 使用代理“名称” |
| 需要 TLS/本地出口 | Owner Relay | 使用本机环境 |
| API Key，未确认云托管 | Owner Relay | 凭据保留本机 |
| API Key，已确认云托管 | Worker 直连 | 云端可用，无需本机在线 |

普通设置中不显示 Owner Relay/Worker Direct 术语。

### 7.4 异常账号

- 已禁用、待验证、严重认证失败不可选择。
- 临时限额账号可以选择，但显示“暂时限额”。
- 代理已经删除的绑定账号不可选择，并提供“前往代理设置”。
- 未完成云同步的账号不可选择，并提供“立即同步”。
- 部分账号异常不阻止保存其他正常账号。

### 7.5 修改账号池

修改账号池不改变连接码和现有 recipient Key。

保存前显示影响：

```text
将移除 1 个账号并加入 2 个账号。
当前已连接用户会自动使用更新后的账号池。
```

如果移除后没有可用账号，禁止保存。

---

## 8. “连接他人的共享”面板设计

### 8.1 空闲状态

```text
连接他人的共享

连接码
[ 572 814 639                         ]

临时访问密码
[ K7M4Q9                              ] [显示]

[粘贴连接信息]              [连接并使用]
```

### 8.2 输入体验

连接码：

- 只接受数字。
- 自动格式化为 `XXX XXX XXX`。
- 粘贴不带空格的 9 位数字也能识别。
- 输入完成后不自动提交。

临时密码：

- 自动转换为大写。
- 使用排除易混淆字符的 Base32 字符集。
- 支持密码管理器和粘贴。
- 默认遮罩，提供显示按钮。

键盘：

- Enter 在两个字段合法时执行连接。
- 错误聚焦到对应字段。
- 不用 Toast 代替字段错误。

### 8.3 一键粘贴连接信息

“粘贴连接信息”在用户点击后读取剪贴板：

- 能解析 Amber 标准复制文本。
- 能解析 `572814639 K7M4Q9`。
- 能解析带空格和换行的两段内容。
- 解析成功后填入两个字段，不自动连接。
- 解析失败时在按钮下说明支持格式。
- 不在页面加载时静默读取剪贴板。

### 8.4 连接中的状态

按钮区域保持固定高度，显示阶段：

```text
正在验证连接信息…
正在创建独立访问权限…
正在测试连接…
正在加入 Amber…
```

不使用不确定的无限旋转；每个阶段设置超时并可取消。

用户取消只取消未开始的后续阶段。上游测试已经开始时不自动重放。

### 8.5 连接成功

```text
连接成功

小林的共享 · 2 个账号
需要共享者设备在线
已加入 Amber，可以直接使用。

[查看已连接共享]
```

如果本机 Codex 尚未接入 Amber：

```text
[一键接入本机 Codex]
[稍后]
```

### 8.6 连接失败

错误保留在面板内：

| 原因 | 用户文案 | 操作 |
| --- | --- | --- |
| 连接码不存在 | 连接信息不正确或已经失效 | 检查连接码 |
| 密码错误 | 临时密码不正确 | 重新输入 |
| 密码过期 | 临时密码已经过期 | 向共享者获取新密码 |
| 人数已满 | 本次允许连接人数已用完 | 联系共享者刷新密码 |
| 共享暂停 | 共享者暂时关闭了共享 | 联系共享者 |
| 频率限制 | 尝试次数过多 | 显示可重试时间 |
| 云连接失败 | 当前网络无法连接 Amber Cloud | 打开网络诊断 |

不得区分“连接码存在但密码错误”和更多可用于枚举账号的详细信息。

---

## 9. 已连接用户管理

### 9.1 首页摘要

```text
正在使用我的共享  2 人                         [查看全部]

小林    正在使用 · 128/不限 · 刚刚              [暂停]
Alex    已暂停 · 54/500 · 2 小时前               [恢复]
```

默认只显示最近 3 人。

### 9.2 完整列表

每行显示：

- 云显示名。
- 是否为常用联系人。
- 状态。
- 已用/总配额。
- 最近使用时间。
- 暂停/恢复。
- 更多菜单。

更多菜单：

- 调整使用规则。
- 刷新该用户密钥（高级）。
- 添加为常用联系人。
- 永久移除。

### 9.3 单用户规则

编辑面板只显示：

- 每分钟请求数。
- 最大并发。
- 总请求额度。
- 有效期。

保存只影响当前用户。

### 9.4 用户身份

连接者必须登录 Amber Cloud，因此共享者可以看到：

- 显示名。
- 脱敏 Friend Code 或连接身份 ID。
- 首次连接时间。
- 最近使用时间。

不显示邮箱、IP、设备硬件信息或完整密钥。

---

## 10. 我正在使用的共享

### 10.1 首页摘要

```text
我正在使用的共享  1 个                         [查看全部]

小林的共享    可用 · 本机回源 · 128/不限         [关闭]
```

### 10.2 完整列表

每行显示：

- 共享者显示名。
- 共享名称。
- 可用状态。
- 本机回源/云端可用的用户语言说明。
- 个人用量和配额。
- 是否参与本地调度。
- 测试按钮。
- 更多菜单。

更多菜单：

- 连接信息。
- 查看状态详情。
- 离开共享。

### 10.3 加入本地调度

连接成功后创建本地映射：

```text
cloud_received_account_links
- id
- cloud_user_id
- grant_public_id
- enabled
- display_name
- scheduler_priority
- health_status
- health_message
- last_checked_at
- created_at
- updated_at
```

Guest Key 继续只保存在现有加密的 `cloud_received_keys` 表中。

### 10.4 调度规则

- 连接并测试成功后默认启用。
- 账号页面显示为“云共享”来源。
- 用户可以关闭或重新测试。
- 不能修改代理、Base URL 或 Key。
- 不能导出凭据。
- 不能再次加入自己的共享账号池。
- 共享者暂停/撤销后立即停止调度。
- 共享者离线属于临时失败，不标记本地认证失败。
- 云共享失败不自动关闭其他本地账号。

### 10.5 连接信息

高级连接信息仍提供 Base URL 和 Guest Key，用于：

- 不经过 Amber 本地调度的兼容客户端。
- 远程服务器直接配置。
- 手动故障排查。

默认遮罩 Key，并明确说明直接使用将绕过本地账号调度。

---

## 11. 常用联系人

### 11.1 定位

好友功能改名或在 UI 中表达为“常用联系人”，不再是共享前置条件。

作用：

- 记录经常连接的用户。
- 显示备注名。
- 快速查看双方历史共享。
- 快速复制当前连接信息。

不增加聊天和留言。

### 11.2 连接后添加

连接成功后可选显示：

```text
经常使用对方的共享？
[添加为常用联系人]
```

该操作继续复用现有好友申请与接受机制。

不阻塞连接成功结果，不反复提示。

### 11.3 兼容现有好友

- 现有好友全部显示在常用联系人中。
- 现有好友关系、备注和黑名单不迁移丢失。
- 被拉黑用户不能通过连接码领取权限。
- 删除联系人默认不撤销既有共享，界面提供单独选择。

---

## 12. 高级管理

高级管理保留现有专业能力：

- 共享组列表和详情。
- 多账号权重、优先级和路由策略。
- 好友独立规则。
- 用量和审计。
- Owner Relay 设备。
- 云连接代理。
- 安全与同步诊断。
- 管理员面板。

### 12.1 默认共享组

连接码绑定一个特殊的默认共享组，但仍使用现有共享组表：

- UI 标记为“默认连接共享”。
- 在普通页面只显示账号和状态摘要。
- 在高级管理中可以查看完整账号、recipient 和用量。
- 不允许从高级页面意外删除而不提示连接码影响。

### 12.2 旧五步向导

旧共享组创建向导在 v0.4.2 保留为“高级创建共享组”。

不再作为首页主入口，不立即删除以保护现有高级用户。

---

## 13. 数据库设计

### 13.1 D1 migration 0006

建议新增：

```sql
CREATE TABLE share_connect_endpoints (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  connection_code TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK(status IN ('active','paused','deleted')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(owner_id)
);

CREATE TABLE share_connect_windows (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  endpoint_id INTEGER NOT NULL REFERENCES share_connect_endpoints(id) ON DELETE CASCADE,
  password_version INTEGER NOT NULL,
  password_salt TEXT NOT NULL,
  password_verifier TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('active','exhausted','expired','replaced','stopped')),
  max_claims INTEGER NOT NULL,
  claimed_count INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(endpoint_id,password_version)
);

CREATE UNIQUE INDEX idx_connect_window_active
  ON share_connect_windows(endpoint_id) WHERE status='active';

CREATE TABLE share_connect_claims (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  window_id INTEGER NOT NULL REFERENCES share_connect_windows(id) ON DELETE CASCADE,
  recipient_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(window_id,recipient_user_id),
  UNIQUE(recipient_user_id,idempotency_key)
);
```

最终 SQL 需要遵循现有 migration 的 CHECK、索引和审计规范。

### 13.2 复用现有表

连接成功时继续写入：

- `share_group_recipients`。
- `share_access_keys`。
- `share_audit_log`。
- `share_request_reservations_v2`。
- `share_usage_log_v2`。

连接码不创建第二套请求网关。

### 13.3 不要求好友关系

现有 `share_group_recipients` 直接绑定 `recipient_id`，数据库层不要求 friendship。

新 claim 接口：

- 根据当前登录用户确定 `recipient_id`。
- 禁止共享者连接自己的连接码。
- 检查双方黑名单。
- 不调用现有 `resolveRecipient(friendship_id)`。
- 直接创建 active recipient grant。

### 13.4 本地 schema 14

新增：

```text
cloud_connect_host_state
- user_id PRIMARY KEY
- endpoint_public_id
- connection_code
- active_window_public_id
- password_cipher
- password_expires_at
- max_claims
- created_at
- updated_at

cloud_received_account_links
- user_id
- grant_public_id
- enabled
- display_name
- scheduler_priority
- health_status
- health_message
- last_checked_at
- created_at
- updated_at
```

临时密码使用安装密钥加密保存，确保 Amber 重启后仍能在有效期内显示当前密码。

本地数据库不保存密码明文。

---

## 14. 密码与连接码安全

### 14.1 连接码

- 9 位数字，显示为 `XXX XXX XXX`。
- 连接码是公开标识，不作为安全秘密。
- Worker 随机生成并处理唯一冲突。
- 高级设置可以重置连接码。
- 重置只阻止旧码的新连接，不自动撤销已有 recipient。

### 14.2 临时密码

- 6 位 Base32 字符。
- 排除 `0/O/1/I/L` 等易混淆字符。
- 默认 30 分钟有效。
- 默认 1 次领取，最多建议 20 次。
- 共享者可以选择 10 分钟、30 分钟、2 小时。
- 密码不进入 URL、日志、事件、诊断或审计详情。

### 14.3 服务端验证

增加 Worker Secret：

```text
SHARE_CONNECT_PEPPER
```

Worker 保存：

```text
HMAC-SHA256(pepper, endpoint_public_id || salt || normalized_password)
```

要求：

- 使用恒定时间比较。
- Secret 不写入代码或 git。
- Owner 创建/刷新时密码通过 TLS 传递，服务端只在请求内存中短暂存在。
- Recipient claim 时同样只用于验证，不写日志。

### 14.4 尝试限流

新增 Durable Object `ShareConnectGuard`，按以下维度限制：

- 连接码。
- 登录用户。
- IP 哈希。

建议规则：

- 同一用户连续失败 5 次，锁定 10 分钟。
- 同一连接码一分钟最多 20 次失败。
- 同一 IP 一小时最多 100 次失败。
- 成功连接后清理用户短期失败计数。
- 错误响应不泄露连接码是否存在、密码是否过期或目标用户身份。

### 14.5 并发领取

最后一个名额必须原子处理：

```text
检查窗口 active
-> 检查 claimed_count < max_claims
-> 检查用户未领取
-> 增加 claimed_count
-> 创建 recipient/key/claim
```

使用 D1 batch + CHECK/触发器或 Durable Object 串行化，禁止超发。

### 14.6 Guest Key

由使用者本机生成：

- 本机保留 Guest Key 明文并立即加密保存。
- 向 Worker 提交哈希、前缀和加密给自己的 envelope。
- Worker 插入 active `share_access_keys`。
- 使用者其他设备可继续通过 envelope 恢复。
- Worker、D1 和共享者永远拿不到 Guest Key 明文。

---

## 15. API 设计

### 15.1 Owner 侧 Worker API

```text
GET    /v1/connect/host
PUT    /v1/connect/host/accounts
POST   /v1/connect/host/start
POST   /v1/connect/host/rotate-password
POST   /v1/connect/host/pause
POST   /v1/connect/host/resume
POST   /v1/connect/host/reset-code
GET    /v1/connect/host/recipients
PATCH  /v1/connect/host/recipients/{recipient_id}
DELETE /v1/connect/host/recipients/{recipient_id}
```

### 15.2 Recipient 侧 Worker API

```text
POST /v1/connect/claim
```

请求包含：

```json
{
  "connection_code": "572814639",
  "temporary_password": "K7M4Q9",
  "idempotency_key": "...",
  "key_material": {
    "key_prefix": "sk-amber-...",
    "guest_key_hash": "...",
    "key_envelope": "...",
    "envelope_context": "...",
    "recipient_key_version": 1
  }
}
```

响应不返回 Guest Key，只返回 grant、共享摘要和 Base URL。

### 15.3 本地 Control API

```text
GET  /control/cloud/connect/host
PUT  /control/cloud/connect/host/accounts
POST /control/cloud/connect/host/start
POST /control/cloud/connect/host/rotate-password
POST /control/cloud/connect/host/pause
POST /control/cloud/connect/host/resume
POST /control/cloud/connect/claim-and-use
GET  /control/cloud/connect/recipients
GET  /control/cloud/connect/received
PATCH /control/cloud/connect/received/{grant_id}
```

前端不能直接处理 Guest Key 生成、哈希、envelope 和本地加密保存。

### 15.4 幂等

以下操作要求 Idempotency-Key：

- 首次创建默认共享入口。
- 开启密码窗口。
- 刷新密码。
- Claim 并使用。
- 单用户密钥轮换。

客户端超时后先按幂等键查询结果，不直接生成新对象。

---

## 16. 自动回源与代理

### 16.1 路径

```text
使用者 Amber
  -> 按使用者“云连接设置”访问 Worker
  -> Worker 验证独立 Guest Key
  -> 选择共享账号
     -> Worker 直连：Cloudflare 出口
     -> 本机回源：共享者 Amber -> 账号绑定代理/TLS/网络出口
```

### 16.2 用户说明

主界面只显示：

- `需要本机在线`。
- `云端可用，无需本机在线`。
- `混合模式，部分账号需要本机在线`。

详情中显示：

- `本机回源：使用共享者账号的代理和网络出口`。
- `云端直连：使用 Amber Cloud 网络出口`。

### 16.3 使用者自己的代理

- 使用者的云连接代理只影响“使用者 -> Worker”。
- 使用者给自己账号绑定的代理不参与共享账号最终访问。
- Owner Relay 使用共享者账号绑定的代理。
- Worker 直连不使用双方本地代理。

该说明进入帮助 Tooltip 和使用文档，避免用户误解测试路径。

---

## 17. 统一连接测试

### 17.1 单一请求构造器

账号页测试、连接码领取测试、收到的共享测试和真实调用必须复用统一构造器或共享协议结构。

固定要求：

- 模型来自统一模型目录。
- Responses `input` 使用消息数组。
- Codex 后端使用 `stream: true`。
- `Accept` 与请求体一致。
- 不发送上游不支持的参数。
- Worker 旧客户端兼容只针对明确旧格式。

### 17.2 Claim 后测试

连接并使用流程：

1. Claim 成功。
2. 本地保存 Guest Key。
3. 创建 disabled 本地映射。
4. 发送一次最小真实测试。
5. 测试成功后启用映射。
6. 测试失败则保留映射为 `needs_attention`，允许重试，不重复 Claim。

### 17.3 不安全重试边界

- 密码校验和 Claim 在幂等键下可安全重试。
- 上游测试 POST 可能已经开始时不自动重放。
- Worker 连接失败且确认未进入上游时可以短重试。
- 超时后查询用量/请求 ID，不盲目再次测试。

---

## 18. 邀请与状态刷新

连接码模式不需要发出共享邀请，但仍需要及时刷新：

- 新用户连接成功。
- 用户暂停、离开或被移除。
- 共享者暂停/恢复。
- Owner Relay 离线/恢复。
- 配额耗尽。

### 18.1 v0.4.2 方案

使用：

```text
D1 cloud_user_events
+ cursor 增量拉取
+ 自适应轮询
+ Windows 通知（P1）
```

轮询：

- 云共享页面前台：5 秒。
- 应用前台其他页面：15 秒。
- 应用后台：60 秒。
- 网络恢复：立即补拉。
- 手动同步：立即拉取。

### 18.2 防打扰

- 同对象重复状态合并。
- 当前页面已经显示的事件不弹系统通知。
- Relay 短暂抖动超过阈值后才提示。
- 普通用量增长不通知。
- 密码过期只更新面板，不弹系统通知。

---

## 19. 前端组件与文件改造

当前 `CloudWorkspace.vue` 已承担过多职责。v0.4.2 建议按真实业务边界拆分，但不创建无意义抽象。

### 19.1 组件建议

```text
src/components/cloud/
├─ CloudWorkspace.vue              页面外壳、导航、状态加载
├─ CloudConnectHome.vue            连接码共享主页面
├─ HostSharePanel.vue              我的共享
├─ JoinSharePanel.vue              连接他人的共享
├─ HostRecipientsList.vue          使用我的共享
├─ ReceivedConnectionsList.vue     我正在使用的共享
├─ ShareAccountPicker.vue          首次/修改账号池
├─ SharePolicyEditor.vue           单用户高级规则
├─ CloudAdvancedWorkspace.vue      旧共享组、设备、安全入口
├─ ConnectionStatus.vue            统一状态与动作
├─ connectionText.ts               连接信息格式化/解析
└─ workspaceCache.ts               缓存扩展
```

### 19.2 文件职责

- `CloudWorkspace.vue` 不直接实现密码、Claim 或 recipient 业务。
- `HostSharePanel.vue` 只接收状态和发出用户命令。
- 密码原文只存在必要的响应状态和加密本地存储，不写入普通缓存。
- `workspaceCache.ts` 禁止缓存临时密码、Guest Key 和 Base URL 密钥参数。
- `connectionText.ts` 必须有独立单元测试。
- `src/api/control.ts` 增加明确类型，不使用大范围 `Record<string, unknown>`。
- `src/i18n.ts` 中英文同步增加，禁止前端硬编码中文错误。

### 19.3 页面状态管理

Host 状态：

```text
unconfigured | paused | starting | active | exhausted | degraded | error
```

Join 状态：

```text
idle | validating | claiming | testing | linking | success | error
```

所有异步动作使用单独 busy key，不能一个全局 busy 锁住无关区域。

### 19.4 布局稳定性

- 面板使用固定最小高度。
- 加载 Skeleton 尺寸与真实内容一致。
- 密码显示/隐藏不改变列宽。
- Toast 不承载长错误。
- 模态框头部和底部操作固定，内容区独立滚动。
- 关闭按钮始终可见，不随内容滚动消失。
- 滚动条不覆盖顶部圆角。

### 19.5 i18n 文案

中文核心文案：

- 我的共享。
- 连接他人的共享。
- 连接码。
- 临时访问密码。
- 开启共享。
- 复制连接信息。
- 刷新密码。
- 连接并使用。
- 正在使用我的共享。
- 我正在使用的共享。
- 需要本机在线。
- 云端可用。

英文需要使用相同语义，不直译内部术语：

- My share。
- Connect to a share。
- Connection code。
- Temporary password。
- Start sharing。
- Copy connection details。
- Connect and use。

---

## 20. 隐私与日志

禁止进入日志、缓存、事件和诊断：

- 临时密码。
- Guest Key。
- 上游 token/API Key。
- key envelope 明文内容。
- 代理密码。

允许记录：

- endpoint public ID。
- 连接码后 3 位或哈希。
- password window public ID。
- recipient public ID。
- 状态码、错误类别和耗时。
- 是否 Owner Relay/Worker Direct（诊断内部）。

审计事件：

- 开启共享。
- 刷新密码。
- 连接成功。
- 暂停/恢复共享。
- 调整单用户规则。
- 移除用户。
- 重置连接码。

审计详情禁止包含秘密。

---

## 21. 开发里程碑

### M0：原型和协议冻结

- [ ] 冻结 v0.4.1 成功共享链路回归基线。
- [ ] 制作桌面双面板静态原型。
- [ ] 制作窄窗口上下排列原型。
- [ ] 覆盖未配置、暂停、开启、连接中、成功和错误状态。
- [ ] 在默认 Amber 窗口尺寸评审。
- [ ] 确认普通界面不出现内部术语。
- [ ] 原型评审通过前不开始 D1 migration。

### M1：Worker 连接码协议

- [ ] D1 migration 0006。
- [ ] `SHARE_CONNECT_PEPPER` Secret 配置说明。
- [ ] ShareConnectGuard Durable Object。
- [ ] Host 查询、账号池、开启、刷新、暂停、恢复接口。
- [ ] Claim 接口。
- [ ] 原子人数限制。
- [ ] 每人独立 recipient 和 access key。
- [ ] 黑名单、自连接和过期校验。
- [ ] 幂等和审计。

### M2：本地存储与 Cloud Manager

- [ ] 本地 schema 14。
- [ ] 临时密码加密存储。
- [ ] Host 状态和账号池管理。
- [ ] 本地 Guest Key 生成和 envelope。
- [ ] Claim-and-use 流程。
- [ ] `cloud_received_account_links`。
- [ ] 超时结果恢复。
- [ ] 凭据清理和离开共享。

### M3：网关调度与统一测试

- [ ] 云共享来源接入调度器。
- [ ] 禁止云共享再次分享。
- [ ] 统一测试请求构造。
- [ ] Owner Relay/Worker Direct 状态映射。
- [ ] 临时离线退避。
- [ ] 暂停、撤销和配额状态同步。
- [ ] 请求日志正确归属云共享来源。

### M4：前端主界面

- [ ] CloudWorkspace 页面外壳收敛。
- [ ] HostSharePanel。
- [ ] JoinSharePanel。
- [ ] 一键粘贴解析。
- [ ] 首次账号池选择。
- [ ] 已连接用户摘要和列表。
- [ ] 收到的共享摘要和列表。
- [ ] 状态/错误统一组件。
- [ ] 中英文 i18n。
- [ ] 键盘、焦点、响应式和 reduced-motion。

### M5：高级管理和兼容

- [ ] 现有好友改为常用联系人表达。
- [ ] 旧共享组进入高级管理。
- [ ] 旧路由和查询参数兼容。
- [ ] v0.4.1 Worker/客户端兼容。
- [ ] 云连接代理和设备管理入口保留。
- [ ] 管理员治理增加连接入口状态，不显示秘密。

### M6：事件、文档与发布

- [ ] D1 用户事件和 cursor。
- [ ] 自适应轮询。
- [ ] Windows 通知（P1）。
- [ ] 更新使用文档。
- [ ] 连接码安全说明。
- [ ] 代理/回源路径说明。
- [ ] Go、前端、Worker、E2E 全部门禁。
- [ ] D1 migration 远程预检。
- [ ] NSIS v0.4.2 打包。
- [ ] 隔离环境覆盖安装测试。

开发和打包禁止安装、卸载、停止或重启用户当前运行的 Amber。打包完成后只汇报产物路径、版本和校验值。

---

## 22. 测试矩阵

### 22.1 Host

- 首次选择单个/多个账号。
- 账号包含 OAuth、API Key、代理和异常状态。
- 开启共享。
- 重启 Amber 后恢复有效密码显示。
- 密码过期。
- 刷新密码。
- 允许 1、3、20 人连接。
- 暂停、恢复和删除共享入口。
- 修改账号池不影响现有 Key。

### 22.2 Join

- 手动输入。
- 粘贴纯数字和密码。
- 粘贴标准连接文本。
- 输入错误、过期、人数已满和频率限制。
- Claim 超时后重试幂等。
- Claim 成功、测试失败后重新测试。
- 已连接用户重复输入同一窗口。
- 共享者尝试连接自己。
- 黑名单用户连接。

### 22.3 多人隔离

- 同一密码 20 位用户并发领取。
- 最后一个名额不超发。
- 每位用户 Guest Key 哈希不同。
- 暂停 A 不影响 B。
- A 配额耗尽不影响 B。
- 刷新密码不影响 A/B。
- 永久移除 A 后 5 秒内阻止新请求。

### 22.4 代理与回源

- OAuth 经共享者本机回源。
- 绑定代理账号使用共享者账号代理。
- Worker 直连使用 Cloudflare 出口。
- 使用者云连接直连、系统代理、指定代理。
- 共享者离线/恢复。
- 混合账号池部分可用。

### 22.5 本地调度

- 连接成功加入调度。
- 用户关闭/开启云共享来源。
- 暂停、撤销和配额变化同步。
- 云共享不能再次共享。
- 导出不包含凭据。
- 云共享失败不关闭本地账号。
- 账号页可显示和测试云共享来源。

### 22.6 UI

- 默认桌面窗口。
- 1280x720、1024x700、800x700、窄窗口。
- 中英文最长文案。
- 125%/150% Windows 缩放。
- 键盘完整流程。
- 屏幕阅读器标签。
- 密码显示/遮罩和应用最小化。
- 滚动条、圆角、固定关闭按钮。
- 加载和错误状态无布局跳动。

### 22.7 安全

- D1、KV、Worker 日志无密码/Key。
- 本地数据库密码加密。
- 连接码枚举保护。
- 恒定时间密码比较。
- 并发名额安全。
- Idempotency-Key 重放。
- 用户/连接码/IP 三层限流。
- 管理员无法取得秘密。

---

## 23. 验收标准

### 23.1 使用体验

- 已完成首次配置的共享者点击一次“开启共享”即可生成可发送的连接信息。
- “复制连接信息”一次复制连接码和密码。
- 使用者可通过一次粘贴和一次“连接并使用”完成连接。
- 普通流程无需添加好友。
- 普通流程无需复制 Base URL/API Key。
- 同一临时密码支持设置多位连接者。
- 普通界面不出现内部协议术语。
- 所有错误都说明下一步。

### 23.2 功能

- 每位使用者独立 Guest Key、配额和控制状态。
- 连接成功后自动成为 Amber 本地调度来源。
- 共享者可以单独暂停、限流和移除用户。
- 刷新密码不影响已连接用户。
- 暂停全部共享可恢复。
- 现有好友和共享组数据不丢失。

### 23.3 可靠性

- Claim、开启和刷新密码全部幂等。
- 20 人并发领取不超发。
- 暂停/撤销后 5 秒内阻止新请求。
- Owner Relay 恢复后无需重新领取。
- 测试和真实调用使用统一请求结构。
- 网络恢复后状态自动刷新。

### 23.4 安全

- 临时密码只以 verifier 形式存在于 D1。
- Guest Key 不以明文存在于 Worker/D1/日志。
- 本地密码和 Guest Key 使用安装密钥加密。
- 连接码不能单独领取权限。
- 未登录用户不能领取。
- 黑名单和自连接被阻止。
- 管理员不能查看秘密。

### 23.5 性能

- 有缓存时云共享首屏 300ms 内显示。
- 正常网络下开启共享 2 秒内完成。
- 正常网络下 Claim 在不含上游测试时 2 秒内完成。
- 状态增量刷新不重载整页。
- 100 位已连接用户支持分页管理。

---

## 24. 迁移与回滚

- D1 新增表，不修改现有 recipient/access key 语义。
- 默认共享入口首次使用时创建，不为所有旧用户批量创建。
- 现有共享组和收到的共享继续显示在高级管理。
- 现有好友关系迁移为常用联系人展示，不改变数据库关系。
- Worker 保留旧好友邀请和共享组 API。
- 桌面 schema 13 升级到 14 前按现有机制备份。
- 旧桌面版本看到新表时不能启动失败。
- 回滚 Worker 时新连接码入口应暂停，不能错误放行。
- 发布前在预览 D1 完整执行 migration、回滚演练和生产备份。

---

## 25. 发布门禁

必须通过：

- `go test ./...`
- 共享、调度、cloudsync 定向 `go test -race`
- 前端 Vitest
- Vue TypeScript 检查
- Vite production build
- Worker TypeScript 检查
- Worker Vitest
- D1 migration 本地测试
- D1 preview 远程测试
- Playwright 连接码完整流程
- Playwright 默认/窄窗口截图检查
- 连接码并发领取测试
- Owner Relay 离线/恢复测试
- Secret/日志扫描
- v0.4.1 到 v0.4.2 数据迁移测试
- 隔离环境 NSIS 覆盖安装测试

Node.js 必须使用仓库规定的 Node 24，禁止调用系统 Node 18。

---

## 26. 最终用户路径

### 共享者首次使用

```text
设置共享账号
-> 保存并开启
-> 复制连接信息
```

### 共享者日常使用

```text
开启共享
-> 复制连接信息
```

### 单人连接

```text
粘贴连接信息
-> 连接并使用
```

### 多人连接

```text
共享者把“允许连接人数”设为 N
-> 多人使用同一连接码和临时密码
-> 每人获得独立权限
```

### 密码泄露或不希望继续发放

```text
刷新密码
-> 旧密码停止新连接
-> 已连接用户继续正常使用
```

### 暂停所有人

```text
暂停全部共享
-> 新连接被阻止
-> 已连接用户全部暂停
-> 以后可以恢复
```

v0.4.2 的成功不以新增多少入口衡量，而以用户是否能够把 Amber 当成“输入连接码和密码即可使用”的工具衡量。复杂的共享组、独立密钥、代理、回源、配额和审计全部保留，但不阻塞普通用户完成连接。
