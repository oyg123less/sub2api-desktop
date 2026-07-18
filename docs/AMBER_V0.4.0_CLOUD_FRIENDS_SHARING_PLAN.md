# Amber v0.4.0 Cloud 2.0、好友与多账号共享开发文档

> 目标版本：v0.4.0
> 基线版本：v0.3.3
> 文档状态：设计冻结前评审稿
> 核心范围：云账户 UI 重构、好友关系、多账号共享组、接收方密钥交付、OAuth 本机回源
> 版本定位：Amber Cloud 2.0 + Friends + Share Groups + Owner Relay

## 1. 版本结论

v0.4.0 不再把“共享”作为单个账号的附属操作。共享入口、创建、接收、权限管理、用量和设备状态全部迁移到“云账户”页面，并以“共享组”作为唯一的新建模型。

一个共享组可以同时包含多个账号和多个好友，但每位好友必须拥有独立 Guest Key。共享者可以只暂停某位好友、单独修改其限流与额度、轮换其密钥或彻底撤销访问，不影响同组其他好友。

共享成功后不再显示二维码。接收者接受邀请后，在自己的“云账户 > 收到的共享”中直接获得可复制的 Base URL 和 API Key；密钥默认掩码显示，二维码组件和相关依赖在确认无其他引用后删除。

OAuth 账号不把 OAuth Token、代理信息、TLS 指纹或本机环境上传到 Worker。好友仍访问统一 Worker Base URL，Worker 完成鉴权、额度和路由，再通过加密 WebSocket 隧道把请求交给账号拥有者在线的 Amber Agent，由拥有者本机使用原有 TLS、代理、网络出口和 OAuth 环境访问 ChatGPT。

```text
好友客户端
  -> HTTPS + Guest Key
Amber Worker
  -> 好友鉴权、限流、配额、共享组路由
  -> WSS 加密隧道
账号拥有者的 Amber Agent
  -> 本机 TLS、代理、网络出口、OAuth
ChatGPT
```

## 2. 目标与非目标

### 2.1 必须完成

1. 重做云账户登录、注册、验证码与登录后工作台，消除现有多个视觉分支之间的割裂感。
2. 从账号页移除“创建共享”、旧共享管理和二维码结果弹窗。
3. 在云账户页提供“我的共享”“收到的共享”“好友”“设备”等清晰入口。
4. 支持一个共享组选择多个本地账号，并在创建后增删、启停单个账号。
5. 支持按 Friend Code 添加好友，不提供聊天、私信、动态或联系人推荐。
6. 支持给一个或多个已接受好友发送共享邀请；接收方必须主动接受或拒绝。
7. 每个“共享组 + 接收好友”使用独立 Guest Key，禁止多人共用同一密钥。
8. 共享者可对组或单个接收者执行暂停、恢复、撤销、删除、密钥轮换、请求额度、RPM、并发数和有效期管理。
9. OAuth 账号通过拥有者 Amber Agent 回源；拥有者离线时返回明确状态，不伪装为上游模型错误。
10. 保持 v0.3.3 旧 Guest Key 可用，并提供可识别、可管理、可迁移的“旧版共享”。
11. 好友、共享、密钥、配额、设备和审计操作必须有中英文文案、自动化测试与安全边界。

### 2.2 明确不做

- 不做好友聊天、留言、群聊、动态、在线社交状态或通讯录上传。
- 不允许接收者把共享继续转授权给第三人。
- 不把本机端口暴露到公网，不要求路由器端口映射，也不采用完全 P2P。
- 不上传或模拟拥有者完整 TLS 指纹、浏览器指纹、代理凭据或 OAuth Token。
- 不允许 Guest 请求指定任意上游 URL、任意本地账号或任意代理。
- 不在 v0.4.0 实现按金额结算；额度以请求数为主，令牌数只用于统计，待流式 usage 数据稳定后再考虑硬限制。
- 不为新共享提供匿名链接或不绑定好友的公共 Guest Key。
- 不把管理员面板与普通用户共享工作台混为同一层级。

## 3. 当前实现问题与改造边界

### 3.1 当前问题

- `src/views/Accounts.vue` 的共享入口位于账号详情内，只能对一个账号创建共享。
- 创建结果把 Base URL、Guest Key、共享码和二维码集中在弹窗中，适合临时人工转发，不适合长期权限治理。
- 当前 `share_grants` 将 `account_uid`、一个 Guest Key 和一份额度直接绑定，无法表达多账号、多好友和每人独立策略。
- `src/views/Cloud.vue` 同时承担认证、同步、账户摘要和管理员面板，登录后的普通用户工作台信息不足。
- 登录、注册、验证码是三个独立模板分支，注册过程缺少连续进度和明确返回路径。
- 当前 OAuth 共享固定返回 `oauth_device_relay_required`，没有设备在线状态和实际回源通道。

### 3.2 代码所有权调整

| 现有位置 | v0.4.0 调整 |
| --- | --- |
| 账号详情中的“共享”按钮 | 删除；账号页只负责账号本身、代理、状态、测试和并发队列 |
| `ShareQRCode.vue` | 无其他引用后删除 |
| `qrcode` / `@types/qrcode` | 无其他引用后从依赖中删除 |
| `Cloud.vue` 单文件大页面 | 拆成云账户外壳、认证、共享、好友、设备、安全等子组件 |
| `/control/cloud/shares` 单账号接口 | 保留兼容读取；新 UI 改用 share groups 接口 |
| Worker `share_grants` | 只承载 Legacy 兼容；新建数据进入 v0.4.0 表 |

账号页可以保留一条非操作性提示：“云端共享已移至云账户”，并提供跳转到 `/cloud?tab=shares` 的文本链接，但不得在账号页直接创建共享。

## 4. 产品语言与新手心智

UI 中统一使用以下词语，避免同一概念出现“授权、Grant、链接、共享码”等多套表达：

| 术语 | 对用户的含义 |
| --- | --- |
| 共享组 | 一组可以共同为好友处理请求的账号 |
| 我的共享 | 我创建并控制的共享组 |
| 收到的共享 | 好友发给我、我可以使用的共享 |
| 好友 | 可互相发送共享邀请的 Amber 用户，不包含聊天功能 |
| 访问密钥 | 接收者调用 Base URL 使用的独立 API Key |
| 本机回源 | 请求最终由账号拥有者在线的 Amber 发出 |
| Worker 直连 | 经明确授权后，由 Worker 使用 API Key 账号访问上游 |
| 暂停 | 可恢复，密钥暂时不能调用 |
| 撤销 | 不可恢复，当前密钥永久失效 |

所有危险动作必须说明影响对象。例如“暂停小林的访问”不能只显示“暂停”；“删除共享组”必须提示组内全部好友的现有密钥会立即失效。

## 5. 云账户整体信息架构

登录后使用一个稳定页面外壳，禁止把所有内容纵向堆在同一个长页面。

```text
┌──────────────────────────────────────────────────────────────────────┐
│ 云账户                    Relay 在线 · 刚刚同步     [立即同步] [···] │
│ Astin · 普通成员                                                   │
├──────────────────────────────────────────────────────────────────────┤
│ 概览 | 我的共享 | 收到的共享 2 | 好友 1 | 设备 | 安全 | 管理员*      │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│                         当前标签内容                                 │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

标签定义：

| 标签 | 核心内容 |
| --- | --- |
| 概览 | 待处理事项、共享健康、Relay 状态、同步状态和最近活动 |
| 我的共享 | 创建与管理共享组、好友权限、成员账号、额度和用量 |
| 收到的共享 | 接受/拒绝邀请，查看 Base URL、访问密钥、状态和个人用量 |
| 好友 | Friend Code、好友申请、好友列表、删除与拉黑 |
| 设备 | 本机回源开关、当前设备、主设备、在线状态和能力 |
| 安全 | 主密码、登录会话、身份密钥状态和安全说明 |
| 管理员 | 仅管理员可见，仍需 `ADMIN_API_KEY` 二次解锁 |

页面级规则：

- 顶部身份、Relay 状态和导航位置保持稳定，标签切换不导致页面整体跳动。
- 待处理数量只在确实存在时显示，最大显示为 `99+`。
- 主操作每页最多一个：如“新建共享组”或“添加好友”。次级操作放在行尾或更多菜单。
- 空状态必须给出下一步按钮，不写大段产品说明。
- 管理员入口只对 `role=admin` 渲染，普通用户 DOM 中也不应存在管理员控制内容。

## 6. 认证页面 UI 重设计

### 6.1 统一认证容器

登录、注册、邮箱验证和成功状态都在同一个居中容器中切换。容器宽度建议 `min(460px, calc(100vw - 32px))`，圆角不超过 8px；字段全部纵向排列，不再出现左右两列输入框。

```text
              Amber Cloud
       在设备间同步并安全共享账号

       ┌──────────┬──────────┐
       │   登录   │   注册   │
       └──────────┴──────────┘

       邮箱
       [ astin@example.com             ]

       主密码
       [ ••••••••••••              👁 ]

       [             登录              ]
```

注册流程在同一容器中连续切换：

```text
步骤 1  创建账户  ───  2 验证邮箱  ───  3 完成

验证码已发送至 as***@qq.com        [修改邮箱]

          [ 1 ][ 8 ][ 4 ][ 2 ][ 0 ][ 6 ]

          [          验证          ]
          54 秒后可重新发送
```

### 6.2 交互要求

- 6 位验证码使用六格视觉组件，但语义上保持一个支持粘贴和 `autocomplete=one-time-code` 的输入控件。
- 粘贴 6 位数字后自动填满；填写最后一位后不自动提交，避免误操作。
- “修改邮箱”回到注册步骤并保留用户已确认的非密码字段；密码必须重新输入。
- 重发按钮显示倒计时，服务端仍执行独立频率限制。
- 密码规则在输入时逐项反馈，提交错误定位到具体字段，不只显示 Toast。
- Turnstile 加载失败时在原位置提供重试，不能让注册按钮永久无解释地禁用。
- 验证成功后在同一容器显示成功状态，再进入“概览”，不闪现空白页。
- 认证页不显示同步、共享或管理员控件。

## 7. 概览页设计

概览不是营销首页，也不是卡片堆叠。使用紧凑状态带、待办列表和最近活动三段布局。

```text
共享状态       Relay 设备       本月成功请求       云同步
3 个运行中     当前设备在线      1,284              已同步
────────────────────────────────────────────────────────
待处理
• 小林请求添加你为好友                              [处理]
• Team Pool 邀请等待你接受                          [查看]
────────────────────────────────────────────────────────
我的共享                         最近活动
Team Pool  3账号  2好友  正常     小林使用 gpt-5.6 · 2分钟前
Backup    1账号  已暂停           当前设备上线 · 8分钟前
```

状态优先级为：安全风险 > 邀请待处理 > Relay 离线 > 共享降级 > 同步失败 > 正常。相同问题不在多个区域重复显示。

## 8. 好友功能设计

### 8.1 添加方式

每个用户拥有随机、可轮换的 Friend Code，例如 `AMB-7K4P-N9Q2`。只能精确输入完整 Friend Code 查找，禁止按邮箱模糊搜索和批量枚举。

添加流程：

1. 用户点击“添加好友”。
2. 输入完整 Friend Code。
3. Worker 返回脱敏后的公开资料：显示名、头像首字母和 Friend Code，不返回邮箱。
4. 用户确认发送申请。
5. 接收方在“好友”标签与概览待办中接受或拒绝。
6. 接受后双方才可以互相发送共享邀请。

不提供申请附言，避免功能逐步演变为聊天系统。

### 8.2 好友页面

```text
好友                                            [添加好友]
你的 Friend Code  AMB-7K4P-N9Q2        [复制] [重新生成]

收到的申请 1
┌ 小林 · AMB-82PQ-X7K3              [拒绝] [接受] ┐

全部好友 4                    [搜索好友] [状态 ▾]
┌ L  小林        已共享 2 个组      [发起共享] [···] ┐
│ A  Alex         暂无共享          [发起共享] [···] │
```

好友行的更多菜单只包含：设置备注名、删除好友、拉黑。没有聊天图标或消息入口。

### 8.3 好友状态机

```text
无关系
  -> pending（已发送申请）
      -> accepted（双方成为好友）
      -> declined（接收方拒绝）
      -> cancelled（发送方撤回）
      -> expired（30 天未处理）

accepted
  -> removed（任一方删除）
  -> blocked（任一方拉黑）
```

规则：

- 同一对用户只能存在一个有效申请或好友关系。
- 不能添加自己，不能绕过拉黑重新申请。
- 删除好友时默认立即暂停双方之间仍有效的共享，并弹出二次选择：“仅暂停并删除好友”或“同时撤销全部密钥”。
- 拉黑必须撤销双方全部现有密钥，并阻止后续申请与共享邀请。
- 重新生成 Friend Code 只影响未来添加，不删除已有好友。
- 申请频率建议限制为每用户每日 20 次、对同一目标每日 3 次；错误响应不能泄露目标是否拉黑了自己。

## 9. 我的共享 UI 设计

### 9.1 列表

```text
我的共享                                        [新建共享组]
[搜索名称或好友]  [全部状态 ▾]  [全部回源方式 ▾]

┌ Team Pool                                  运行中  [暂停] [···] ┐
│ 3 个账号 · 2 位好友 · 本机回源优先 · 30 RPM                 │
│ 账号  ChatGPT-1  ChatGPT-2  Backup API                       │
│ 好友  小林  128/1000      Alex  54/500                       │
│ 最近请求 2 分钟前                  本月成功率 98.7%           │
└──────────────────────────────────────────────────────────────┘
```

共享组使用整行信息块，而不是密集表格或多层嵌套卡片。悬停只做轻微边框、阴影与 `translateY(-1px)`，不使用明显放大造成布局抖动。

每行固定展示：组名、组状态、账号数、好友数、默认策略、各好友额度摘要、最近请求和健康状态。暂停、恢复是可见按钮；编辑、复制组 ID、删除放在更多菜单。

### 9.2 创建共享组向导

点击“新建共享组”打开宽度约 760px 的分步对话框或页面内抽屉。顶部步骤条与关闭按钮固定，内容区域独立滚动，底部“上一步/下一步”固定，避免滚动后找不到关闭或提交按钮。

#### 第 1 步：名称与用途

- 共享组名称，必填，2 至 40 字符。
- 可选说明，最多 120 字符，只对共享者自己和受邀好友显示。
- 明确提示：“每位好友将获得独立密钥，你可以单独控制。”

#### 第 2 步：选择账号

- 支持搜索、全选当前筛选结果和逐个多选。
- 每行显示账号名、类型、当前状态、代理、回源方式和并发占用。
- 严重异常、已禁用、未完成云同步的账号不可选，并在行内说明原因。
- OAuth 账号固定为“本机回源”。
- API Key 账号可选择“本机回源”或“Worker 直连”；Worker 直连必须单独确认云端凭据托管。
- 至少选择一个账号。

#### 第 3 步：选择好友

- 只显示已接受好友，支持多选和搜索。
- 每位好友显示当前已经接收的共享组数量，避免重复邀请。
- 至少选择一个好友；无好友时直接提供“先添加好友”的次级流程，返回后保留前两步内容。

#### 第 4 步：访问规则

- 路由策略：智能均衡（默认）或按顺序故障转移。
- 每分钟请求数 RPM：默认 30，范围 1 至 600。
- 最大并发：默认 2，范围 1 至 20。
- 总请求额度：默认不限，范围 1 至 1,000,000。
- 有效期：默认永不过期，最长可设置未来一年。
- 规则先作为组默认值，再允许在创建后对单个好友覆盖。

#### 第 5 步：确认并创建

- 按“账号、好友、访问规则、回源要求”四段汇总。
- 若包含本机回源账号，醒目提示：“你的 Amber Agent 在线时好友才能使用这些账号。”
- 创建按钮必须幂等；重复点击或超时重试不得生成重复共享组或多把有效密钥。
- 创建完成后跳转到共享组详情，不弹二维码，不要求用户手工转发密钥。

### 9.3 共享组详情

使用右侧宽抽屉或独立详情路由，头部固定显示组名、状态、暂停/恢复和关闭按钮。内容标签：

| 标签 | 内容 |
| --- | --- |
| 账号 | 成员账号、优先级、权重、回源方式、健康、启停与移除 |
| 好友 | 邀请状态、密钥前缀、个人规则、用量、暂停、轮换和撤销 |
| 用量 | 按好友/账号/模型筛选的请求趋势、成功率和延迟 |
| 活动 | 谁在何时邀请、接受、暂停、改额度、轮换或撤销 |

账号操作规则：

- 停用账号只影响当前共享组，不改变账号页中的全局启用状态。
- 移除账号前检查是否为最后一个可用账号；若是，提示共享组将进入“无可用账号”。
- 本地账号发生严重异常时，运行时路由立即排除；UI 显示“账号异常”，但不偷偷删除成员关系。
- 账号恢复后可自动重新参与路由，但因用户手工停用的账号不得自动恢复。

好友操作规则：

- “暂停访问”保留密钥，可恢复。
- “轮换密钥”生成新密钥并立即使旧密钥失效；接收方客户端自动取得新密钥信封并显示“密钥已更新”。
- “撤销访问”永久作废当前密钥与邀请；再次共享必须创建新邀请和新密钥。
- “移出共享组”与撤销访问等价，但保留审计记录。
- 组级暂停立即覆盖全部好友；恢复组时不恢复此前被单独暂停或撤销的好友。

## 10. 收到的共享 UI 设计

### 10.1 待接收邀请

```text
收到的共享                                      待处理 2

┌ Team Pool                                        来自 小林 ┐
│ 3 个可用账号 · 智能均衡 · 30 RPM · 2 并发                 │
│ 需要好友设备在线：是       有效期：2026-12-31              │
│                                      [拒绝] [接受共享]     │
└────────────────────────────────────────────────────────────┘
```

接收者只能看到共享组公开摘要，不能看到拥有者账号邮箱、OAuth Token、API Key、代理地址、网络出口或设备 IP。

### 10.2 已接受共享

```text
┌ Team Pool                         可用 · 拥有者设备在线 ┐
│ 来自 小林     本月 128 / 1000     30 RPM · 2 并发       │
│ Base URL  https://api.amberapp.asia/v1          [复制] │
│ API Key   sk-amber-••••••••••••7H2K      [显示] [复制] │
│                                  [测试连接] [查看用量] │
└─────────────────────────────────────────────────────────┘
```

要求：

- API Key 默认掩码，复制不要求先显示明文。
- 不显示二维码，不要求好友从聊天工具接收密钥。
- 测试连接使用低成本固定请求，并明确区分“密钥无效、拥有者离线、没有可用账号、上游失败”。
- 暂停、过期、撤销、拥有者离线、额度耗尽分别使用不同状态与解决提示。
- 接收者可以“隐藏共享”或“离开共享”；离开会撤销自己的密钥，不影响其他好友。
- 密钥轮换后自动同步新信封；解密失败时显示“在此设备重新解锁云保险库”，不能回退展示旧密钥。

## 11. 设备与本机回源 UI

设备页只展示与回源、安全有关的信息，不复制系统诊断页。

```text
本机回源
[开启] 允许好友通过选定共享组使用此设备

当前设备  Astin-PC        在线 · 主设备
能力      OAuth / API Key / 本地代理 / 流式响应
活动请求  1               最近心跳 6 秒前

其他设备
Laptop-2                 离线 · 3 天前              [移除]
```

规则：

- 首次创建包含本机回源账号的共享时，引导开启 Agent，不在后台静默开启。
- 已开启后，Amber 登录云账户且本地网关可用时自动重连；用户显式关闭后不得自动开启。
- 多设备时允许指定主设备和备用设备。同一个请求只交给一个设备。
- 设备被移除或会话撤销后，现有 WebSocket 必须立即关闭。
- 应用退出、休眠、网络切换时更新离线状态；UI 对瞬时断线使用短暂“正在重连”，超过 30 秒才显示离线。
- 不安装、卸载、停止或重启用户的 Amber 来验证该功能；测试使用独立开发进程和模拟 Agent。

## 12. 多账号路由规则

### 12.1 账号可用性

一个账号只有同时满足以下条件才进入候选集：

1. 共享组处于 `active`。
2. 该账号成员关系为 `enabled`。
3. 账号未被全局关闭，未处于严重异常、刷新失败或有效限额期。
4. 账号支持请求的端点和模型。
5. 本机回源账号存在具备该账号的在线 Agent；Worker 直连账号存在有效托管凭据。
6. 账号当前并发和本地等待队列未超过账号页已配置限制。

### 12.2 路由策略

`balanced` 为默认策略：先选最低优先级数值，再在同级账号中按健康、剩余额度、当前 in-flight 和最近失败进行加权选择。

`failover` 按用户设置的顺序尝试，只在请求尚未送达上游时切换下一个账号。

严禁在以下情况自动重放到另一个账号：Agent 已发送 `upstream_started`、已经收到上游响应头、或无法确定上游是否收到请求。此时返回 `relay_result_unknown`，避免模型请求被重复执行。

### 12.3 混合回源

- OAuth：强制本机回源。
- 带本地代理的账号：默认本机回源。
- 自定义 Base URL：默认本机回源；只有通过 Worker 上游白名单校验且用户明确同意时才允许 Worker 直连。
- 官方 API Key：可选择 Worker 直连或本机回源。
- 组内本机账号离线时，可以回退到可用的 Worker 直连账号；没有可用回退时返回 `owner_device_offline`。

## 13. 好友与共享状态机

### 13.1 共享组

```text
draft -> active <-> paused -> deleted
           |
           -> degraded（仍可自动恢复到 active）
```

- `degraded` 是计算状态，不作为人工写入值；表示部分账号异常或 Relay 不完整，但仍可能处理请求。
- `deleted` 后全部接收者密钥立即撤销，业务数据不可恢复；审计元数据保留 30 天。

### 13.2 接收者授权

```text
pending
  -> active <-> paused
  -> declined
  -> expired

active/paused
  -> revoked
  -> left
```

- `pending` 密钥哈希和加密信封可以预创建，但 Gateway 必须拒绝调用。
- `paused` 可恢复并继续使用同一密钥。
- `revoked`、`declined`、`expired`、`left` 均不可恢复；重新共享必须产生新授权和新密钥。
- 组暂停是覆盖状态，不改写接收者自身状态。

### 13.3 密钥

```text
prepared -> active -> replaced
                    -> revoked
                    -> expired
```

同一个接收者授权最多只有一把 `active` 密钥。轮换在单个原子事务中激活新密钥并将旧密钥标记为 `replaced`。

## 14. 密钥生成、交付与存储

### 14.1 原则

- Worker 数据库只保存 Guest Key 哈希、短前缀和面向接收者的加密信封，不保存可直接使用的明文 Guest Key。
- Guest Key 由共享者 Amber 本地生成，格式建议为 `sk-amber-<43 chars base64url>`。
- Owner UI 只显示密钥前缀和状态，不提供查看好友完整密钥的功能；控制权通过暂停、轮换和撤销实现。
- 接收者 Amber 解密密钥后存入本地加密数据库，默认掩码展示。
- Guest 请求通过 HTTPS 把密钥提交给 Gateway，Worker 因执行鉴权必然能在请求内存中接触密钥，但不得写入日志、D1、KV、异常详情或分析事件。

### 14.2 用户身份加密密钥

每个云账户首次升级后生成一对 X25519 身份密钥：

- 公钥保存在 `friend_profiles.encryption_public_key`，供好友加密访问密钥。
- 私钥由云保险库密钥加密后保存为 `friend_profiles.encryption_private_cipher`；该字段只能由本人 Profile API 读取，好友查询永不返回，Worker 无法解密。使用独立字段而不是旧 `settings` vault 项，避免 v0.3.3 客户端更新设置时丢弃未知身份密钥字段。
- 多设备登录并解锁云保险库后可以取得同一身份私钥。
- 身份密钥轮换必须先为所有收到的有效共享重包裹密钥信封，失败时不允许删除旧私钥。

### 14.3 密钥信封格式

使用 X25519 + HKDF-SHA256 + AES-256-GCM：

```json
{
  "version": 1,
  "algorithm": "X25519-HKDF-SHA256-AES-256-GCM",
  "ephemeral_public_key": "base64url",
  "salt": "base64url",
  "nonce": "base64url",
  "ciphertext": "base64url"
}
```

客户端在加密前生成全局唯一的 `envelope_context`（`ctx_` + 至少 160 位随机值），Worker 对其执行唯一约束。AAD 固定为：

```text
amber-share-key-v1|<envelope_context>|<recipient_key_version>
```

Worker 必须校验信封字段长度、版本、唯一上下文和接收者公钥版本，但不尝试解密。使用客户端预生成上下文，避免信封错误依赖尚未由 Worker 创建的共享组或授权 ID。

## 15. D1 Schema 4 设计

建议新增迁移 `cloud/migrations/0004_cloud_friends_share_groups.sql`。所有时间使用 UTC ISO-8601；所有公开 ID 使用不可枚举随机 ID，不向客户端暴露自增主键。

### 15.1 好友

```sql
CREATE TABLE friend_profiles (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  friend_code TEXT NOT NULL UNIQUE,
  encryption_public_key TEXT NOT NULL,
  encryption_private_cipher TEXT NOT NULL,
  encryption_key_version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE friend_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  sender_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  receiver_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pair_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','accepted','declined','cancelled','expired')),
  created_at TEXT NOT NULL,
  responded_at TEXT,
  expires_at TEXT NOT NULL,
  CHECK(sender_id <> receiver_id)
);

CREATE UNIQUE INDEX idx_friend_requests_pending_pair
  ON friend_requests(pair_key) WHERE status='pending';

CREATE TABLE friendships (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  user_low_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  user_high_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK(status IN ('active','removed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(user_low_id, user_high_id)
);

CREATE TABLE friendship_aliases (
  friendship_id INTEGER NOT NULL REFERENCES friendships(id) ON DELETE CASCADE,
  owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  alias TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(friendship_id, owner_user_id)
);

CREATE TABLE friend_blocks (
  blocker_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  blocked_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY(blocker_id, blocked_id)
);
```

应用层必须把用户对规范化为 `user_low_id < user_high_id`，并用相同顺序生成 `pair_key`；`pending` 条件唯一索引负责拦截双方同时发起或重复提交的申请。

### 15.2 共享组与成员账号

```sql
CREATE TABLE share_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK(status IN ('active','paused','deleted')),
  route_policy TEXT NOT NULL CHECK(route_policy IN ('balanced','failover')),
  default_rpm INTEGER NOT NULL,
  default_concurrency INTEGER NOT NULL,
  default_quota_requests INTEGER NOT NULL DEFAULT 0,
  default_expires_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE TABLE share_group_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  account_uid TEXT NOT NULL,
  account_type TEXT NOT NULL CHECK(account_type IN ('oauth','api_key')),
  relay_mode TEXT NOT NULL CHECK(relay_mode IN ('owner_device','worker_direct')),
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1,
  token_cipher TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(group_id, account_uid)
);
```

`token_cipher` 仅允许 `worker_direct` 使用，并继续由 `SHARE_KMS_KEY` 加密。OAuth 行必须为 `NULL`。账号展示名和邮箱从拥有者已解密的 vault 数据取得，不复制到接收者可见的数据结构。

### 15.3 接收者与访问密钥

```sql
CREATE TABLE share_group_recipients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  generation INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL CHECK(status IN ('pending','active','paused','declined','expired','revoked','left')),
  rpm_limit INTEGER NOT NULL,
  concurrency_limit INTEGER NOT NULL,
  quota_requests INTEGER NOT NULL DEFAULT 0,
  used_requests INTEGER NOT NULL DEFAULT 0,
  reserved_requests INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  created_at TEXT NOT NULL,
  accepted_at TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE(group_id, recipient_id, generation)
);

CREATE UNIQUE INDEX idx_share_recipients_current
  ON share_group_recipients(group_id, recipient_id)
  WHERE status IN ('pending','active','paused');

CREATE TABLE share_access_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  key_version INTEGER NOT NULL,
  key_prefix TEXT NOT NULL,
  guest_key_hash TEXT NOT NULL UNIQUE,
  key_envelope TEXT NOT NULL,
  envelope_context TEXT NOT NULL UNIQUE,
  recipient_key_version INTEGER NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('prepared','active','replaced','revoked','expired')),
  created_at TEXT NOT NULL,
  activated_at TEXT,
  revoked_at TEXT,
  UNIQUE(recipient_grant_id, key_version)
);

CREATE UNIQUE INDEX idx_share_access_keys_active
  ON share_access_keys(recipient_grant_id) WHERE status='active';
```

### 15.4 设备、用量与审计

```sql
CREATE TABLE share_devices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  device_public_key TEXT NOT NULL,
  capabilities TEXT NOT NULL,
  is_primary INTEGER NOT NULL DEFAULT 0,
  revoked INTEGER NOT NULL DEFAULT 0,
  last_seen_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_share_devices_primary
  ON share_devices(user_id) WHERE is_primary=1 AND revoked=0;

CREATE TABLE share_device_sessions (
  id TEXT PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  connected_at TEXT NOT NULL,
  last_heartbeat_at TEXT NOT NULL,
  disconnected_at TEXT,
  close_reason TEXT
);

CREATE TABLE share_device_challenges (
  challenge_hash TEXT PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE share_request_reservations_v2 (
  id TEXT PRIMARY KEY,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  access_key_id INTEGER NOT NULL REFERENCES share_access_keys(id) ON DELETE CASCADE,
  group_account_id INTEGER REFERENCES share_group_accounts(id) ON DELETE SET NULL,
  state TEXT NOT NULL CHECK(state IN ('pending','settled','released')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE TABLE share_usage_log_v2 (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  group_account_id INTEGER REFERENCES share_group_accounts(id) ON DELETE SET NULL,
  device_id INTEGER REFERENCES share_devices(id) ON DELETE SET NULL,
  route_mode TEXT NOT NULL,
  model TEXT,
  status INTEGER NOT NULL,
  error_code TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  latency_ms INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE share_audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER REFERENCES share_groups(id) ON DELETE SET NULL,
  actor_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_public_id TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE cloud_mutation_receipts (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  operation TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_status INTEGER NOT NULL,
  response_body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY(user_id, operation, idempotency_key)
);
```

`pair_key` 使用两个用户 ID 排序后的稳定形式，保证双方同时发申请时也只能产生一个 `pending` 申请。接收者撤销、拒绝或离开后，再次邀请会创建更高 `generation` 的新授权；旧行保持终态用于审计，不能被重新激活。

`cloud_mutation_receipts` 为创建共享组、邀请好友和密钥轮换保存七天幂等回执。同一用户、操作与幂等键绑定不同请求哈希时返回 `409 idempotency_key_reused`。

配额预占沿用 v0.3.3 的数据库触发器思路：只有 `active`、未过期、组未暂停且 `used + reserved < quota` 时才能创建 `pending` 预占。成功结算 `settled`，明确失败和未发往上游的错误执行 `released`；未知结果不重复发送，并按协议是否收到 `upstream_started` 决定结算策略。迁移末尾创建必要的 owner、recipient、status、expiry 和 usage 索引，并执行 `UPDATE schema_version SET version=4`。

## 16. REST API 设计

所有写接口要求认证、JSON 大小限制、严格字段白名单和 `Idempotency-Key`。列表统一使用 `limit` 与不可伪造游标，不返回数据库自增 ID。

### 16.1 Profile 与好友

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/profile` | 当前公开资料、Friend Code 和身份公钥状态 |
| PATCH | `/v1/profile` | 修改显示名 |
| POST | `/v1/profile/friend-code/rotate` | 轮换 Friend Code |
| GET | `/v1/friends` | 好友列表 |
| GET | `/v1/friend-requests` | 收到和发出的申请 |
| POST | `/v1/friend-requests` | 通过完整 Friend Code 发起申请 |
| POST | `/v1/friend-requests/:id/accept` | 接受 |
| POST | `/v1/friend-requests/:id/decline` | 拒绝 |
| POST | `/v1/friend-requests/:id/cancel` | 撤回 |
| PATCH | `/v1/friends/:id` | 修改自己的备注名 |
| DELETE | `/v1/friends/:id` | 删除好友并处理现有共享 |
| POST | `/v1/friends/:id/block` | 拉黑并撤销双方共享 |
| DELETE | `/v1/friends/:id/block` | 解除拉黑，不自动恢复好友 |

### 16.2 共享组

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/share-groups` | 我的共享组列表 |
| POST | `/v1/share-groups` | 幂等创建共享组、账号、好友邀请和密钥信封 |
| GET | `/v1/share-groups/:id` | 详情 |
| PATCH | `/v1/share-groups/:id` | 名称、说明、状态、路由和默认规则 |
| DELETE | `/v1/share-groups/:id` | 删除组并撤销全部密钥 |
| POST | `/v1/share-groups/:id/accounts` | 添加账号 |
| PATCH | `/v1/share-groups/:id/accounts/:accountId` | 启停、顺序、权重、回源方式 |
| DELETE | `/v1/share-groups/:id/accounts/:accountId` | 移除账号 |
| POST | `/v1/share-groups/:id/recipients` | 邀请一个或多个好友 |
| PATCH | `/v1/share-groups/:id/recipients/:recipientId` | 暂停、恢复、规则覆盖 |
| DELETE | `/v1/share-groups/:id/recipients/:recipientId` | 撤销并移出 |
| POST | `/v1/share-groups/:id/recipients/:recipientId/keys/rotate` | 原子轮换密钥 |
| GET | `/v1/share-groups/:id/usage` | 所有者用量聚合与明细 |
| GET | `/v1/share-groups/:id/audit` | 组活动审计 |

### 16.3 接收方

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/received-shares` | 待接受和已接受共享 |
| POST | `/v1/received-shares/:id/accept` | 接受并激活预创建密钥 |
| POST | `/v1/received-shares/:id/decline` | 拒绝 |
| POST | `/v1/received-shares/:id/leave` | 离开并撤销自己的密钥 |
| GET | `/v1/received-shares/:id/key-envelope` | 获取当前密钥信封 |
| GET | `/v1/received-shares/:id/usage` | 接收者自己的用量 |

### 16.4 设备

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/v1/devices` | 设备与最近在线状态 |
| POST | `/v1/devices` | 注册设备和能力 |
| PATCH | `/v1/devices/:id` | 重命名、设为主设备 |
| DELETE | `/v1/devices/:id` | 撤销设备并断开 Relay |
| POST | `/v1/devices/:id/challenge` | 签发 60 秒有效的一次性 Relay 挑战 |
| GET Upgrade | `/v1/relay/connect` | Amber Agent 建立认证 WSS |

本地控制端提供对应 `/control/cloud/profile`、`/friends`、`/share-groups`、`/received-shares`、`/devices` 接口。Vue 只访问本地控制端；本地 Go 服务负责会话刷新、身份密钥、密钥信封、凭据读取和 WebSocket 生命周期。

## 17. WebSocket 本机回源协议

### 17.1 Cloudflare 组件

新增两个 Durable Object：

- `ShareAccessCoordinator`：按访问授权 ID 分片，执行 RPM、并发、短期预占和请求状态机。
- `OwnerRelay`：按拥有者 user ID 分片，管理设备 WebSocket、心跳、能力快照和请求流。

`wrangler.toml` 必须增加 Durable Object binding 和迁移声明。普通 Worker 不直接持有仅存在于单实例内存中的设备连接映射。

### 17.2 握手

设备使用 Ed25519 签名密钥。私钥只保存在本地加密数据库，公钥在设备注册时上传。连接前，Agent 先通过认证的 `POST /v1/devices/:id/challenge` 取得 32 字节随机挑战和过期时间，再签名以下规范化内容：

```text
SHA-256("amber-relay-v1|<device_id>|<challenge>|<expires_at>")
```

Amber Agent 使用云会话和一次性设备证明连接：

```text
GET /v1/relay/connect?device_id=<public_id>&protocol=1
Authorization: Bearer <cloud_access_token>
X-Amber-Device-Challenge: <base64url challenge>
X-Amber-Device-Challenge-Expires: <UTC ISO-8601>
X-Amber-Device-Proof: <base64url Ed25519 signature>
Upgrade: websocket
```

Worker 先验证挑战哈希、设备归属、过期时间和 Ed25519 签名，再以 `UPDATE ... WHERE consumed_at IS NULL AND expires_at > now` 原子消费挑战。更新行数不是 1 时拒绝连接。无论握手后续是否成功，同一挑战都不得再次使用；过期和已消费记录异步清理。仅有 Access Token、仅有设备 ID 或重放旧握手均不能建立 Relay。

连接后消息包含统一字段：

```json
{
  "protocol": 1,
  "type": "hello",
  "message_id": "random-id",
  "device_id": "dev_...",
  "capabilities": ["oauth", "api_key", "proxy", "streaming"],
  "account_snapshot_version": 42
}
```

Worker 返回 `hello_ack` 后设备才视为在线。心跳间隔 20 秒，60 秒未收到心跳判定离线；网络恢复使用指数退避并加随机抖动重连。

### 17.3 请求事件

```text
relay_request
  -> relay_accepted
  -> upstream_started
  -> response_start
  -> response_chunk (0..N)
  -> response_end

任意阶段可返回 relay_error
```

`relay_request` 只包含受允许字段：请求 ID、共享组公开 ID、账号 UID、`responses` 或 `chat/completions` 端点、模型、经过过滤的请求头、超时和 Base64URL 请求体。Guest 不能传入上游 URL、代理、Authorization 或账号 UID。

响应按不超过 64 KiB 的块发送，包含连续序号。Worker 使用有限窗口 ACK 实现背压；未确认块超过窗口时 Agent 暂停读取上游，防止内存无限增长。

### 17.4 超时与断线语义

| 阶段 | 建议超时 | 行为 |
| --- | --- | --- |
| 分配在线设备 | 3 秒 | 可选择备用设备 |
| Agent 接受请求 | 5 秒 | 未送上游，可安全换设备/账号 |
| 等待上游响应头 | 30 秒 | 已开始上游时不得自动重放 |
| 流式空闲 | 90 秒 | 终止隧道并返回超时 |
| 总请求 | 30 分钟 | 关闭请求并记录审计 |

如果在 `upstream_started` 之前断线，配额释放且允许安全故障转移。如果在其后断线，返回 `relay_result_unknown`，记录未知结果，不自动重放。

### 17.5 Agent 防御性校验

- 只接受 Worker 签发且属于当前登录用户的请求。
- 账号 UID 必须位于本机已同步的该共享组允许列表中。
- 只允许固定端点和 `POST`。
- 忽略 Guest 注入的 Authorization、Host、Cookie、代理和上游 URL。
- 继续使用 Amber 现有账号状态、代理绑定、最大并发和访问队列。
- 所有日志对 Token、Guest Key、请求正文和响应正文脱敏。

## 18. 错误码与用户提示

| 错误码 | HTTP | UI 提示 |
| --- | ---: | --- |
| `friend_code_not_found` | 404 | 未找到该 Friend Code，请检查完整代码 |
| `friend_request_exists` | 409 | 已经发送申请或你们已经是好友 |
| `friendship_required` | 403 | 只能向已接受的好友发起共享 |
| `friend_action_unavailable` | 409 | 当前好友状态不允许此操作 |
| `share_invitation_pending` | 403 | 请先在 Amber 云账户中接受共享 |
| `share_group_paused` | 403 | 共享者已暂停此共享组 |
| `share_access_paused` | 403 | 共享者已暂停你的访问 |
| `share_access_revoked` | 401 | 访问密钥已撤销，请联系共享者重新邀请 |
| `share_access_expired` | 403 | 共享已过期 |
| `share_quota_exhausted` | 429 | 本共享的请求额度已用完 |
| `share_rate_limited` | 429 | 请求过快，请在指定秒数后重试 |
| `share_concurrency_limited` | 429 | 当前并发已满，请稍后重试 |
| `share_no_eligible_account` | 503 | 共享组暂时没有可用账号 |
| `owner_device_offline` | 503 | 共享者设备离线，本机回源暂不可用 |
| `owner_relay_busy` | 503 | 共享者设备繁忙，请稍后重试 |
| `relay_timeout` | 504 | 本机回源响应超时 |
| `relay_result_unknown` | 502 | 连接中断，结果未知；为避免重复请求未自动重试 |
| `relay_protocol_mismatch` | 426 | 共享者需要升级 Amber |
| `shared_model_unsupported` | 400 | 共享组中的账号不支持该模型 |
| `key_envelope_unreadable` | 409 | 此设备无法解锁访问密钥，请重新解锁云保险库 |
| `key_rotation_required` | 409 | 访问密钥已更新，请同步后重试 |

Gateway 错误保持 OpenAI 风格 JSON，但不得把拥有者邮箱、账号 UID、设备 ID或上游 HTML 暴露给接收者。

## 19. 旧共享迁移

### 19.1 兼容原则

- Schema 4 迁移不删除、不改写 `share_grants` 和 `share_usage_log`。
- 旧 `/v1/responses` Guest Key 继续通过 Legacy 查询路径鉴权。
- v0.4.0 UI 把每条旧授权映射为“旧版单账号共享”，只允许暂停、恢复、查看用量和撤销，不允许继续创建同类共享。
- 旧密钥只有哈希，Worker 无法重新展示明文，也不能自动发送给好友。

### 19.2 转换流程

1. 用户在旧版共享上点击“迁移到共享组”。
2. 选择组名、好友和新规则。
3. 本地生成每位好友的独立新密钥与信封。
4. 新共享创建并由好友接受。
5. UI 询问是否立即撤销旧密钥；默认建议撤销，但不自动执行。

转换不是数据库就地改写，因为旧共享不知道真实使用者，强行绑定好友会造成权限归属错误。

## 20. 管理员能力调整

管理员面板仍需云账户 `role=admin` 与 `ADMIN_API_KEY` 双重验证。新增：

- 查看共享组 owner、状态、账号数、接收者数、成功率与最近活动，不显示账号凭据和完整 Guest Key。
- 暂停或撤销违规共享组/单个访问授权。
- 查看好友申请滥用统计和封禁用户。
- 查看 Relay 在线设备数量、协议版本分布和错误率，不显示用户 IP 明细给普通运营界面。
- 所有管理员动作写入 `admin_audit`，包含原因和目标公开 ID。

普通用户绝不能看到管理员标签或二次验证输入框。

## 21. 视觉、响应式与无障碍规范

### 21.1 视觉

- 沿用 `var(--bg)`、`var(--surface)`、`var(--border)`、`var(--text)` 等现有变量。
- 中性背景为主，绿色只表示健康，琥珀色表示等待/降级，红色表示危险，蓝色只用于可交互主操作，避免单一色相覆盖整页。
- 卡片和对话框圆角不超过 8px；不使用渐变球、装饰光斑或营销 Hero。
- 标题尺寸匹配工作台，不使用超大字号。
- Hover 动画 120 至 180ms，位移不超过 1px；尊重 `prefers-reduced-motion`。
- 密钥、Base URL、Friend Code 使用等宽字体；长文本必须截断并提供复制，不得溢出容器。

### 21.2 响应式

| 宽度 | 行为 |
| --- | --- |
| `>= 1180px` | 完整标签、共享组摘要双区布局、详情宽抽屉 |
| `800px - 1179px` | 标签可横向滚动，筛选器换行，共享组元数据精简 |
| `< 800px` | 单列；列表行改为纵向；对话框全宽；固定底部操作栏 |

最小验收窗口包含 1024x720、1280x800、1440x900 和 390x844。任何按钮、密钥、错误信息和标签不得相互覆盖。

### 21.3 无障碍

- 标签、步骤条、对话框、菜单和六位验证码使用正确 ARIA 语义。
- 全流程支持键盘操作，焦点进入对话框后受控，关闭后返回触发按钮。
- 不只依靠颜色表达状态，所有 Badge 同时显示文字或图标。
- 动态邀请、密钥轮换和 Relay 断线使用 `aria-live=polite`，危险错误使用 `assertive`。
- 正文与背景至少 4.5:1，对大文本至少 3:1。

## 22. 前后端模块拆分建议

### 22.1 Vue

```text
src/views/Cloud.vue                      路由壳、认证态和标签选择
src/components/cloud/CloudAuthPanel.vue  登录/注册/验证码连续流程
src/components/cloud/CloudHeader.vue     身份、同步和 Relay 状态
src/components/cloud/CloudOverview.vue
src/components/cloud/ShareGroupList.vue
src/components/cloud/ShareGroupWizard.vue
src/components/cloud/ShareGroupDetail.vue
src/components/cloud/ReceivedShares.vue
src/components/cloud/FriendsPanel.vue
src/components/cloud/DevicesPanel.vue
src/components/cloud/SecurityPanel.vue
src/components/cloud/AdminPanel.vue
```

不要继续把全部逻辑加入现有 `Cloud.vue`。共享组、好友和设备分别建立 typed store/composable；请求状态按资源隔离，避免一个全局 `busy` 使整个页面所有操作同时失效。

### 22.2 Go 本地服务

- `cloudsync`：扩展 profile、friends、groups、received shares 和 devices 客户端。
- `control`：增加对应本地 API，统一校验与错误映射。
- `relay` 新包：设备身份、WebSocket、重连、请求状态机、流式桥接与背压。
- `store`：保存身份私钥密文、接收的 Guest Key、Relay 用户设置和待提交幂等操作。
- 复用现有账号选择、代理绑定、并发队列和上游请求实现，不复制第二套 HTTP 客户端。

### 22.3 Worker

- `friend-routes.ts`
- `share-group-routes.ts`
- `received-share-routes.ts`
- `device-routes.ts`
- `share-group-gateway.ts`
- `durable/share-access.ts`
- `durable/owner-relay.ts`
- `share-gateway.ts` 保留 Legacy，逐步把公共鉴权与响应过滤抽成复用模块。

## 23. 开发里程碑

### M0：协议冻结与迁移基础

- 冻结本文术语、状态机、API 和错误码。
- 完成 Schema 4 正向迁移、回滚兼容说明和旧数据快照测试。
- 增加 feature flags：`friends_enabled`、`share_groups_enabled`、`owner_relay_enabled`。
- Worker 先部署兼容旧客户端的数据库与只读接口。

### M1：Cloud 2.0 UI 与认证连续流程

- 拆分 `Cloud.vue`，完成稳定页头、标签外壳和响应式布局。
- 重做登录、注册、验证码、成功状态。
- 完成概览、空状态、加载骨架和错误边界。
- 保持现有同步、主密码和管理员功能可用。

### M2：好友与身份密钥

- 生成/同步 X25519 身份密钥。
- 完成 Friend Code、申请、接受、拒绝、撤回、删除、拉黑和频控。
- 完成好友 UI、待办与安全审计。

### M3：多账号共享组控制面

- 完成共享组、成员账号、接收者、独立策略与密钥信封。
- 完成五步创建向导、详情抽屉、暂停/恢复/撤销/轮换和用量。
- 完成收到的共享、接受/拒绝、Base URL/API Key 展示与测试连接。
- 从账号页移除旧入口和二维码，加入 Legacy 管理与迁移流程。

### M4：OAuth Owner Relay

- 完成 Durable Objects、设备注册、WSS 握手、心跳和重连。
- 完成请求/响应流、背压、超时、断线语义和本地账号路由。
- 接入本地代理、网络出口、OAuth 刷新和账号并发队列。

### M5：配额、观测与管理员能力

- 完成 RPM、并发、总请求额度的原子预占和结算。
- 完成组/好友/账号维度用量、成功率、延迟和审计。
- 完成管理员暂停、撤销、滥用统计和 Relay 版本观测。

### M6：迁移、压测与发布

- 完成旧共享兼容和人工转换流程。
- 执行故障注入、跨设备、窄窗口、长流和升级回滚测试。
- 关闭 feature flag 前完成真实两用户端到端验收。
- 打包 v0.4.0，但不得自动安装、卸载、停止或启动用户已安装的 Amber。

## 24. 测试矩阵

### 24.1 前端

- 登录、注册、验证码始终在同一容器，无空白过渡。
- 验证码粘贴、重发倒计时、修改邮箱和 Turnstile 失败恢复。
- 好友申请全部状态与未读数量。
- 向导返回上一步时数据不丢失；重复提交只创建一个组。
- 多账号、多好友选择和单好友规则覆盖。
- 共享详情长内容滚动时关闭按钮与底部操作固定。
- Base URL、密钥和错误文本在所有目标窗口不溢出。
- 普通用户没有管理员 DOM；接收者看不到拥有者敏感字段。
- Playwright 更新旧 `sharing.spec.ts`，删除二维码断言，新增双方浏览器上下文的邀请与接收流程。

### 24.2 Worker 与 D1

- 好友用户对唯一性、双方并发接受、撤回/接受竞态和拉黑绕过。
- 未成为好友不能创建邀请；删除好友按选择暂停或撤销相关授权。
- 每位好友密钥哈希唯一，同一授权最多一把 active key。
- 密钥轮换原子化：任何时刻不能同时存在两把可用 key，也不能出现两把都不可用的中间状态。
- 组暂停、好友暂停、过期、额度、RPM 和并发判定组合测试。
- v0.3.3 Legacy key 在 Schema 4 后仍可调用。
- 管理员接口不能返回信封明文、Guest Key、Token 或请求正文。

### 24.3 Relay

- Agent 在线/离线/重连、主设备切换、备用设备故障转移。
- OAuth、API Key、本地代理和自定义 Base URL 的允许路径。
- 断线发生在 `relay_accepted` 前、`upstream_started` 前后、响应头后和流式中段。
- `upstream_started` 后绝不自动重放。
- 4 MiB 请求边界、64 KiB 响应块、慢客户端背压和 30 分钟长流。
- 伪造账号 UID、上游 URL、Authorization、设备证明和旧协议版本被拒绝。
- Agent 严重异常账号被排除，恢复后按用户启停状态决定是否重新加入。

### 24.4 质量命令

发布前至少通过：

```text
npm run build
npm run test
npm run test:e2e
cd cloud && npm run typecheck
cd cloud && npm run test
go test ./...
go test -race ./...
cargo test --manifest-path src-tauri/Cargo.toml
cargo clippy --manifest-path src-tauri/Cargo.toml -- -D warnings
```

再执行独立开发环境中的 Worker/Agent 双用户端到端测试和 NSIS 打包校验。测试不得操作用户已安装的 Amber。

## 25. 发布与回滚

推荐顺序：

1. 备份远端 D1，应用 Schema 4；迁移只新增表、索引和兼容字段。
2. 部署支持 v0.3.3 Legacy 路径的 v0.4.0 Worker，feature flags 默认关闭。
3. 验证旧 v0.3.3 客户端同步和旧 Guest Key。
4. 小范围开启 Friends 与 Share Groups，观察错误率和重复记录。
5. 小范围开启 Owner Relay，只允许测试账号。
6. 完成真实两用户测试后发布桌面 v0.4.0。
7. 最后逐步对生产用户开启新功能。

回滚 Worker 时不得删除 Schema 4 表；旧 Worker 会忽略新增表。桌面客户端回滚到 v0.3.3 后不能管理新共享组，但新 Guest Key 是否继续工作由 Worker feature flag 控制。紧急情况下只暂停新建与 Relay，不撤销已有密钥，除非确认存在安全风险。

## 26. 完成定义

v0.4.0 只有在以下条件全部满足时才算完成：

1. 账号页不能再创建共享，云账户是唯一共享入口，项目中不再展示共享二维码。
2. 注册、验证码和成功状态在同一视觉容器连续完成，登录后云账户工作台在目标窗口无溢出、重叠或空白页。
3. 两个真实云用户可以通过 Friend Code 成为好友，且没有任何聊天入口。
4. 共享者可以在一个共享组中选择至少两个账号和两个好友，每位好友得到不同 Guest Key。
5. 接收者接受后可在自己的云账户中复制 Base URL 和密钥并成功测试，无需共享者人工转发。
6. 共享者可以单独暂停、恢复、限流、改额度、轮换和撤销任一好友密钥，不影响其他好友。
7. OAuth 请求在拥有者 Agent 在线时通过本机网络成功流式返回；离线时稳定返回 `owner_device_offline`。
8. 任何 `upstream_started` 后的断线都不会触发自动重放。
9. Worker、D1、Vue、Go、Rust 和 Playwright 测试全部通过，Legacy v0.3.3 Guest Key 兼容测试通过。
10. 日志、D1、KV、错误响应和管理员 UI 中不存在 OAuth Token、API Key 明文、完整 Guest Key、代理凭据和请求/响应正文。
11. 生成可校验的 v0.4.0 NSIS 安装包，但开发与测试过程未自动安装、卸载、停止或启动用户当前 Amber。

## 27. 实施前必须再次确认的产品参数

以下参数不改变架构，可以在正式编码前以配置默认值冻结：

- 默认 RPM：建议 30。
- 默认好友最大并发：建议 2。
- 好友申请有效期：建议 30 天。
- 待接受共享有效期：建议 7 天。
- 删除共享审计保留期：建议 30 天。
- 是否允许 API Key 账号选择 Worker 直连：建议允许，但默认本机回源且逐账号明确确认。
- Base URL 正式域名：建议固定为 `https://api.amberapp.asia/v1`；在 DNS 和 Worker custom domain 完成前继续使用部署 origin 动态返回。

这些值应进入 Worker 配置或平台设置，禁止散落硬编码在 Vue、Go 和 Worker 多处。
