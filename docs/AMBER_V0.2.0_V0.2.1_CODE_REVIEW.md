# Amber v0.2.0 / v0.2.1 代码全面评估报告

> 评估日期：2026-07-14  
> 评估方式：只读代码审查、静态检查、单元测试、构建验证  
> 评估范围：已发布的 `v0.2.0` 与当前本地 `v0.2.1` 候选工作区

## 1. 结论摘要

当前仓库中没有一个已经提交、打标签的 `v0.2.1` 版本。

- 当前分支：`v0.2.0-development`
- 当前 HEAD：`5b62f12`
- 已发布标签：`v0.2.0`，对应提交 `01aabdd`
- 本地 `package.json`、Cargo 和 Tauri 配置已改为 `0.2.1`
- 本地还存在一处未提交的 CORS 修改

因此，本报告把版本状态分成两部分：

1. `v0.2.0`：GitHub 已发布版本，可复现发布版行为。
2. `v0.2.1 candidate`：当前本地工作区，只能视为候选版本，尚未形成可追溯发布基线。

本次审查共识别出：

| 级别 | 数量 | 说明 |
| --- | ---: | --- |
| P1 高优先级 | 5 | 影响核心流程、数据正确性、Codex 可用性或数据安全 |
| P2 中优先级 | 10 | 会导致功能异常、错误状态、兼容性问题或恢复能力下降 |
| P3 低优先级 / 风险 | 8 | 边界验证、资源管理、异步竞争和可维护性问题 |

最需要优先处理的五项是：

1. `v0.2.0` CORS 导致导入提交和日志清空被浏览器拦截。
2. 批量导入在共享 `refresh_token` 时错误跳过第二个账号。
3. API Key 或端口变化后，Codex 配置仍显示已应用但实际已失效。
4. Chat Completions 的命名 `tool_choice` 和 `developer` 角色转换错误。
5. 数据目录迁移存在路径绕过和失败回滚期间的数据损坏风险。

## 2. 严重度定义

| 级别 | 定义 |
| --- | --- |
| P1 | 核心功能不可用、账号数据错误、配置静默失效、存在数据损坏或明显安全风险 |
| P2 | 特定条件下功能失败、状态错误、兼容性受损、恢复或故障转移不正确 |
| P3 | 低频边界问题、资源泄漏、诊断误报、测试不足或未来演进风险 |

## 3. P1 高优先级问题

### P1-01：v0.2.0 CORS 阻止导入提交和清空日志

#### 代码位置

- `core/internal/control/control.go:121-140`
- `src/api/control.ts:344-350`
- `src/api/control.ts:377-380`

#### 根本原因

已发布的 `v0.2.0` 只允许以下跨域请求头：

```text
Content-Type, X-Control-Token, Authorization
```

但前端导入提交还会发送：

```text
X-Import-Preview-SHA256
X-Validate-After-Import
```

清空日志会发送：

```text
X-Confirm-Clear
```

Tauri WebView 页面与 `http://127.0.0.1:<control-port>` 不同源。浏览器会先发送 OPTIONS 预检；发布版返回的 `Access-Control-Allow-Headers` 不包含这些请求头，因此浏览器会在请求进入 Go 控制接口之前直接阻止请求。

#### 影响

- 导入预览可以成功，但点击确认导入可能无响应或报网络错误。
- 清空日志请求可能无法到达后端。
- 后端单元测试不会发现问题，因为该故障发生在浏览器 CORS 层。

#### v0.2.1 候选状态

当前本地工作区已经添加所需请求头，但修改尚未提交。当前实现还会反射客户端提供的全部 `Access-Control-Request-Headers`，边界比实际需要更宽。

#### 建议

- 使用固定且完整的请求头白名单。
- 不需要反射任意请求头。
- 如保留动态响应，应添加 `Vary: Access-Control-Request-Headers`。
- 增加真实浏览器预检测试，覆盖导入提交和日志清空。

#### 验收条件

- WebView 中导入预览、提交、日志清空全部成功。
- OPTIONS 响应明确包含三个新增请求头。
- 未声明的自定义请求头不被无条件放行。

### P1-02：批量导入可能错误地把两个账号判定为一个

#### 代码位置

- `core/internal/account/import.go:146-180`
- `core/internal/store/migrations.go:397-412`
- `core/internal/account/import_test.go:56-77`

#### 根本原因

凭据指纹生成逻辑优先使用 `refresh_token`：

```text
有 refresh_token -> hash("refresh:" + refresh_token)
否则            -> hash("access:" + access_token)
```

导入预览先检查凭据指纹是否重复，然后才比较已经签名验证的 `chatgpt_account_id`。如果两个导入记录共享刷新令牌，第二行会在身份比较之前直接被标记为 `skip`。

这意味着以下输入会出现错误：

```text
第 1 行：refresh_token = R，已验证账号 ID = A
第 2 行：refresh_token = R，已验证账号 ID = B
```

即使 `A != B`，第 2 行仍会被当作第 1 行的完全重复项。

#### 影响

- 导入两个账号后只显示一个账号。
- 再次导入相同内容仍可能只保留一个。
- 不同导出来源是否共享刷新令牌，会造成用户观察到“有时成功、有时失败”。

#### 为什么现有测试没有发现

`TestImportTwoDistinctVerifiedAccountsCommitsBoth` 使用了两个不同的 access token，且没有构造共享 refresh token 的情况，因此无法覆盖真实故障条件。

#### 建议

身份和凭据去重顺序应调整为：

1. 先验证并解析账号身份。
2. 如果两个记录具有不同的已验证账号 ID，不得仅凭凭据指纹跳过。
3. 相同指纹、不同已验证身份应标记为 `conflict`，而不是 `skip`。
4. 只有身份相同且凭据相同，才属于完全重复。

#### 必须增加的回归测试

- 相同 refresh token，不同已验证账号 ID：不得跳过第二行。
- 相同 refresh token，相同已验证账号 ID：应跳过或更新。
- 相同 access token，不同已验证账号 ID：应报告冲突。
- 无法验证身份、凭据相同：保持保守去重并给出明确警告。

### P1-03：Codex 配置在端口或 API Key 变化后静默失效

#### 代码位置

- `core/internal/control/control.go:351-358`
- `core/internal/control/codex.go:34-64`
- `core/internal/codexcfg/codexcfg.go:71-99`
- `core/internal/codexcfg/codexcfg.go:102-124`
- `src/views/Dashboard.vue:67-85`

#### 根本原因

Codex 配置把两个运行参数写入外部文件：

- `config.toml` 中保存 Amber API 端口。
- `auth.json` 中保存 Amber 本地 API Key。

但是：

- 重置 Key 只更新 Amber 设置，不同步 `auth.json`。
- 修改端口只更新 Amber 设置，不同步已应用的 `config.toml`。
- Codex 状态只检查 `model_provider == "sub2api"`，不检查当前 base URL、wire API 和 Key 是否仍然匹配。

#### 影响

- 重置 Key 后 Codex 返回 401。
- 修改端口后 Codex 继续连接旧端口。
- Amber 页面和诊断仍可能显示“Codex 已配置”。
- 用户容易把问题误判为代理、模型或上游服务故障。

#### 建议

- 判断 `Applied` 时校验当前 provider、base URL、wire API 和模型。
- 如果检测到 Amber provider 已应用，在 Key 或端口变化后提供原子同步。
- 同步失败时保留旧文件并返回明确错误，不能显示保存成功。
- 不要在用户使用其他 provider 时自动覆盖 Codex 文件。

#### 验收条件

- 重置 Key 后，已应用的 Codex 集成仍能立即请求。
- 修改端口并重启 API 后，Codex base URL 自动或显式同步。
- 文件与运行设置不一致时，状态显示“配置过期”，而不是“已应用”。

### P1-04：Chat Completions 工具调用和 developer 角色转换错误

#### 代码位置

- `core/internal/apicompat/chatcompletions_to_responses.go:91-100`
- `core/internal/apicompat/chatcompletions_to_responses.go:120-136`
- `core/internal/apicompat/chatcompletions_to_responses.go:495-518`

#### 问题 A：命名 tool_choice 结构错误

Chat Completions 的命名函数结构通常为：

```json
{
  "type": "function",
  "function": { "name": "search" }
}
```

Responses API 需要：

```json
{
  "type": "function",
  "name": "search"
}
```

当前代码对 `tool_choice` 原样透传，仅对旧版 `function_call` 做了正确转换。

#### 问题 B：developer 角色被降级为 user

消息转换只识别：

```text
system / user / assistant / tool / function
```

其他角色进入默认分支并按 `user` 转换。现代 OpenAI 客户端使用的 `developer` 消息因此失去高优先级指令语义。

#### 影响

- 强制指定工具的请求可能被上游拒绝。
- Agent 工具调用流程可能中断。
- developer 指令被当作普通用户内容，行为和安全边界发生变化。

#### 建议

- 为 Chat Completions `tool_choice` 增加显式结构转换和严格校验。
- 明确支持 `developer` 角色并转换成 Responses developer 消息。
- 未知角色应返回 400，不应静默降级为 user。

### P1-05：数据目录迁移存在数据损坏风险

#### 代码位置

- `src-tauri/src/lib.rs:199-217`
- `src-tauri/src/lib.rs:223-283`
- `src-tauri/src/lib.rs:285-296`

#### 根本原因

1. 目录包含关系使用 `PathBuf::starts_with`，但没有先执行 canonicalize。
2. Windows 路径大小写、`..`、junction 和符号链接可能绕过词法包含检查。
3. 新后端启动失败后的回滚流程忽略第二次 `wait_stopped` 结果。
4. 即使新后端可能仍在运行，也会继续删除新目录中的数据库、WAL、SHM 和 key。
5. 成功后移动旧目录作为备份时，重命名失败被忽略，仍向前端返回成功。

#### 影响

- 目标目录实际位于旧目录内部时，迁移完成后的旧目录重命名会同时移动正在使用的新数据库。
- 回滚期间可能删除仍被 sidecar 使用的数据文件。
- 迁移显示成功，但旧数据目录并未按预期备份或移动。

#### 建议

- 对源目录、目标目录及其最近存在的父目录执行 canonicalize。
- Windows 下按文件系统语义比较路径，并处理 junction/reparse point。
- sidecar 未确认停止时禁止删除任何数据库文件。
- 旧目录备份失败必须返回错误或明确警告。
- 迁移前后执行 SQLite `quick_check`，并验证账号和代理密文可解密。

## 4. P2 中优先级问题

### P2-01：运行中修改端口或 LAN 设置不会重绑服务

#### 代码位置

- `core/internal/apiserver/manager.go:39-82`
- `core/internal/control/control.go:166-185`
- `core/internal/control/control.go:251-320`
- `src/views/Settings.vue:111-128`

`Manager.Start()` 在服务已经运行时直接返回，不会检查监听端口或 `AllowLAN` 是否变化。设置页面保存后也不会自动执行 stop/start。

结果包括：

- 设置页面显示新端口，但实际服务仍监听旧端口。
- 开启 LAN 后状态返回 `host = 0.0.0.0`，但实际 listener 仍可能是 `127.0.0.1`。
- `lan_addresses` 无论是否允许 LAN 都会生成，可能向用户展示不可访问地址。

建议保存监听相关设置时执行受控重启，并区分“配置地址”和“当前实际监听地址”。

### P2-02：代理绑定与 OAuth 存在 fail-open

#### 代码位置

- `core/internal/control/control.go:583-600`
- `core/internal/store/accounts.go:261-264`
- `core/internal/gateway/engine.go:208-220`
- `core/internal/control/oauth.go:111-149`

当前接口不验证账号和代理是否存在。读取代理失败时，网关把代理当成 nil 并继续直连；OAuth 客户端构造失败时也回退到普通直连客户端。

当用户明确要求通过代理访问时，正确策略应是 fail-closed：返回代理错误，不能静默暴露真实出口。

### P2-03：Partial Settings API 会重置未提交字段

#### 代码位置

- `src/api/control.ts:337-339`
- `core/internal/control/control.go:251-315`

TypeScript 声明 `saveSettings(s: Partial<Settings>)`，但 Go 按完整 `Settings` 解码。缺失字段变为零值。

可能被重置的字段包括：

- `allow_lan`
- `inject_instructions`
- `auto_start_server`
- `auto_recovery`
- `log_retention_days`，零值在此处还代表永久保留

建议改成指针字段 Patch DTO，或者把前端接口改为只接受完整 Settings，并在后端拒绝缺失字段。

### P2-04：设置保存不是事务操作

#### 代码位置

- `core/internal/store/settings.go:174-205`
- `core/internal/config/holder.go:33-41`

所有设置通过 map 循环逐条 UPSERT。任何一条写入失败都会留下不可预测的部分更新，但 Holder 因 Save 返回错误而继续保留旧内存配置。

建议把全部设置更新放入一个 SQLite 事务，事务提交成功后再替换内存快照。

### P2-05：账号代理故障不会触发账号故障转移

#### 代码位置

- `core/internal/gateway/engine.go:125-178`
- `core/internal/gateway/engine.go:205-284`

当前只对 429 和鉴权错误尝试下一个账号。第一账号的代理连接失败会归类为普通 upstream error，并立即结束整个请求。

如果代理故障发生在 token refresh 阶段，还可能被记录为账号鉴权失败，使账号进入 `refresh_failed` 退避状态。

建议区分：

- 账号绑定代理故障：可以尝试下一账号，但不能改写为鉴权失败。
- 全局上游故障：根据幂等性和是否已经输出响应决定是否重试。
- 客户端取消：立即停止，不重试。

### P2-06：Token 刷新边界处理不一致

#### 代码位置

- `core/internal/account/manager.go:76-98`
- `core/internal/account/manager.go:115-125`
- `core/internal/account/oauth.go:93-103`

问题包括：

- 普通自动刷新在 `expires_in == 0` 时默认一小时。
- 手动强制刷新没有这一回退，会写入立即过期时间。
- OAuth 重新登录更新已有账号时，如果响应没有 refresh token，会覆盖掉已有 refresh token。

建议统一 token 合并规则：缺失的新字段保留旧值，`expires_in` 缺失时使用统一、安全的默认值。

### P2-07：429 恢复时间可能选择错误窗口

#### 代码位置

- `core/internal/gateway/ratelimit.go:77-89`

代码始终优先使用 secondary reset 时间，没有判断 429 是由 primary 还是 secondary 窗口触发。如果 primary 窗口已经耗尽而 secondary 较早恢复，账号会被过早重新加入调度并重复产生 429。

建议根据使用率和窗口状态选择对应 reset，无法确定时采用更保守的最大有效重置时间。

### P2-08：原生 Responses 请求可能丢失数字精度

#### 代码位置

- `core/internal/gateway/responses.go:23-79`
- `core/internal/gateway/responses.go:205-254`

请求先被反序列化为 `map[string]any`。Go 的 `encoding/json` 会把 JSON number 解码为 `float64`，因此嵌套输入中大于 `2^53` 的整数可能在重新序列化时被修改。

此外，原生 Responses SSE 收到失败终止事件后，会输出 Chat 风格的错误 envelope，而不是原样 Responses 事件或规范的 Responses error event，严格客户端可能无法解析。

建议使用 `json.Decoder.UseNumber()` 或保留 `json.RawMessage`，并为 Responses 流单独实现错误事件格式。

### P2-09：省略 model 时响应返回空模型名

#### 代码位置

- `core/internal/gateway/engine.go:101-134`
- `core/internal/gateway/stream.go:15-31`
- `core/internal/gateway/stream.go:83-86`

请求省略 model 时，网关正确使用默认模型转发，但响应模型字段使用原始 `requestedModel`，因此返回空字符串。

建议分别保留：

- 用户原始请求模型，用于审计。
- 实际解析模型，用于响应和运行日志。

### P2-10：OAuth 诊断在首次登录后永久误报

#### 代码位置

- `core/internal/control/oauth.go:68-83`
- `core/internal/diagnostics/service.go:316-329`

OAuth listener 第一次启动后不会停止，持续占用 `127.0.0.1:1455`。诊断检查却通过再次监听 1455 判断端口是否可用，因此首次登录后会把 Amber 自己的正常 listener 误判为端口冲突。

诊断应查询 OAuth coordinator 自身状态，或者向本地 callback listener 执行专用健康检查。

## 5. P3 低优先级问题和风险

### P3-01：控制接口吞掉数据库错误

`/control/status` 忽略 `ListAccounts` 错误并报告账号数为 0；统计接口忽略 Daily、ByModel、LogHealth 错误并返回 HTTP 200。故障会被伪装成“没有数据”。

相关位置：`core/internal/control/control.go:166-185`、`core/internal/control/control.go:743-764`。

### P3-02：账号和代理写接口缺少存在性及值域验证

存在以下情况：

- 负数 ID 可以通过 `pathID`。
- 删除不存在账号或代理可能仍返回成功。
- 绑定不存在代理 ID 不报错。
- 代理类型、host、port 在创建和更新时未完整校验。

相关位置：`core/internal/control/control.go:642-689`、`core/internal/control/control.go:828-835`。

### P3-03：日志和统计查询参数缺少上限

虽然负数 limit 会在 Store 中恢复为默认值，但非常大的 limit 仍会被接受。日志导出还会一次性把最多一百万行加载进内存。

相关位置：`core/internal/control/control.go:730-781`、`core/internal/store/logs.go:23-64`。

### P3-04：OAuth pending 状态不会清理

已完成、失败或用户放弃的 OAuth state 永久保存在内存 map 中。重复回调没有原子“已领取”状态，多个回调可能重复执行 token exchange，后到的失败结果还可能覆盖先前成功结果。

相关位置：`core/internal/control/oauth.go:16-65`、`core/internal/control/oauth.go:86-140`。

### P3-05：OAuth 回调 HTML 没有转义动态错误文本

token exchange 错误文本可能包含上游响应内容，并被直接写入 HTML。经用户代理或受控上游返回恶意内容时，可能造成 callback 页面脚本注入。

相关位置：`core/internal/control/oauth.go:120-129`、`core/internal/control/oauth.go:152-168`。

### P3-06：前端异步轮询可能发生旧响应覆盖新响应

多个页面使用 `setInterval(async () => ...)`，没有请求序列号或 AbortController。请求超过轮询间隔时会重叠，后返回的旧请求可能覆盖新状态。

相关位置：

- `src/App.vue:47-63`
- `src/views/Dashboard.vue:104-130`
- `src/views/Accounts.vue:278-297`
- `src/views/Statistics.vue:35-47`

### P3-07：sidecar spawn 存在检查与登记竞争窗口

`spawn()` 先单独检查 `active_run == 0`，释放锁后才分配并写入 run ID。两个并发调用可能都通过检查并启动两个 sidecar，后一个 child 句柄覆盖前一个。

另外，版本兼容逻辑允许任何以 `-dev` 结尾的 sidecar，不要求其基础版本与桌面版本匹配，可能隐藏开发环境中的陈旧二进制。

相关位置：`src-tauri/src/supervisor.rs:187-234`、`src-tauri/src/supervisor.rs:307-310`。

### P3-08：5.6 模型接入仍是静态目录，不是能力检测

当前实现已经硬编码：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

但没有：

- 向上游发现账号实际可用模型。
- 按账号缓存模型能力。
- 默认 5.6 不可用时回退。
- 区分“目录支持”和“当前账号获得灰度权限”。

新安装默认直接使用 `gpt-5.6-sol`。如果账号尚未获得该模型权限，初次测试和网关请求可能失败。

相关位置：`core/internal/openai/catalog.go:3-39`、`core/internal/openai/modelmap.go:8-69`。

## 6. 并发和状态一致性补充

### 6.1 手动禁用账号可能被在途请求重新激活

成功请求会无条件调用 `RecordAccountSuccess`，该方法直接把状态设置为 active。如果用户在请求执行期间手动禁用账号，请求完成后可能重新把账号改回 active。

相关位置：`core/internal/gateway/engine.go:145-149`、`core/internal/store/accounts.go:213-220`。

### 6.2 账号失败次数更新不是原子操作

`RecordAccountFailure` 先 SELECT 当前次数，再单独 UPDATE。多个并发失败可能读取相同旧值并覆盖彼此，导致退避级别低于实际失败次数。

相关位置：`core/internal/store/accounts.go:223-239`。

### 6.3 同时执行相同导入可能产生重复账号

预览匹配和事务写入不是同一个数据库事务，凭据指纹也不是唯一索引。两个并发导入请求可能都预览为 create，然后顺序写入相同凭据。

当前单窗口 UI 通过 loading 状态降低了发生概率，但控制 API 本身没有保证。

## 7. 测试和验证结果

### 7.1 已通过

- `scripts/check-version.ps1`：版本文件一致，当前本地版本为 `0.2.1`
- `gofmt -l core`：通过
- `go vet ./...`：通过
- `go test -race ./...`：通过
- `cargo fmt --check`：通过
- `cargo clippy --all-targets -- -D warnings`：通过
- `cargo test`：6 项通过
- Vue TypeScript 检查：通过
- Vite 生产构建：通过
- Vitest：2 项通过

### 7.2 未完成

Playwright E2E 在本机运行 184 秒后超时，没有获得有效测试结果。测试过程中启动的 Playwright 和 Vite 进程已清理。

本机系统 Node 为 `18.18.2`，直接执行 npm 时还受到全局缓存目录 `D:\Setup\Nodejs\node_cache` 的 EPERM 影响。改用工作区 Node `24.14.0` 并直接调用工具入口后，类型检查、构建和 Vitest 均通过。

### 7.3 Go 测试覆盖率

| 包 | 覆盖率 |
| --- | ---: |
| `internal/apicompat` | 0.0% |
| `internal/control` | 4.9% |
| `internal/openai` | 8.8% |
| `internal/apiserver` | 13.0% |
| `internal/account` | 51.4% |
| `internal/store` | 50.8% |
| `internal/transport` | 60.0% |
| `internal/gateway` | 66.0% |
| `internal/diagnostics` | 66.5% |
| `internal/codexcfg` | 64.3% |

最危险的测试缺口是：

1. 约 15 万行级字符规模的 `apicompat` 目录没有任何测试覆盖。
2. 控制 API 只有代理测试相关覆盖，没有设置、CORS、导入、Codex 和日志接口测试。
3. 前端只有 ConfirmModal 的两个键盘交互测试。
4. E2E 只检查应用外壳和两个导航链接，没有覆盖真实工作流。

## 8. 建议开发顺序

### 第一阶段：阻止核心回归

1. 完成并提交固定白名单 CORS 修复。
2. 修正导入身份与凭据去重顺序。
3. 修复 Codex Key、端口和状态一致性。
4. 修复命名 tool choice 和 developer 角色转换。
5. 加固数据目录迁移和回滚。

### 第二阶段：代理和运行状态可靠性

1. 所有明确配置代理的路径改为 fail-closed。
2. 设置端口和 LAN 变化时受控重启 API listener。
3. 修复代理故障分类和跨账号 failover。
4. 统一 token 合并与过期时间处理。
5. 修复 429 窗口选择。

### 第三阶段：接口一致性和测试

1. Settings 改为严格 PUT 或真正 PATCH。
2. 设置保存改为 SQLite 事务。
3. Responses 请求保留 JSON 数字精度。
4. 修复 OAuth 诊断和 pending 生命周期。
5. 为 control、apicompat 和核心前端流程补充测试。

## 9. v0.2.1 发布最低验收矩阵

| 场景 | 预期结果 |
| --- | --- |
| WebView 批量导入两个不同账号 | 两个账号都进入数据库和账号列表 |
| 两行共享 refresh token、身份不同 | 不得静默跳过第二行 |
| 导入预览后修改内容再提交 | 返回 preview hash mismatch |
| 清空日志 | OPTIONS 和 DELETE 都成功 |
| API 运行时修改端口 | 旧端口关闭，新端口可访问 |
| API 运行时开启 LAN | 实际监听地址与状态一致 |
| 重置 API Key 后使用 Codex | Codex 继续可用或明确提示配置过期 |
| 修改端口后使用 Codex | config.toml 不再指向旧端口 |
| 指定命名 tool_choice | 上游收到 Responses 格式对象 |
| developer 消息 | 保持 developer 指令层级 |
| 第一账号代理离线 | 按策略尝试下一个可用账号 |
| OAuth 指定无效代理 | 返回错误，不允许静默直连 |
| primary 配额耗尽 | 不在 primary reset 前恢复账号 |
| 迁移到嵌套目录或 junction | 操作被拒绝，不改动原数据 |
| 新数据目录启动失败 | 原目录、指针和数据库完整恢复 |
| Responses 输入包含大整数 | 转发后数值完全一致 |
| 请求省略 model | 响应返回实际默认模型名 |
| 首次 OAuth 登录后运行诊断 | OAuth listener 不误报端口冲突 |

## 10. 发布判断

当前代码不建议直接作为 `v0.2.1` 发布。

至少应先完成所有 P1 项，以及 P2-01、P2-02、P2-03、P2-05 和 P2-06。否则即使版本号和安装包更新成功，仍可能继续出现：

- 两账号只导入一个。
- Codex 配置显示正常但实际 401 或连接失败。
- LAN 设置显示成功但不可访问。
- 代理故障时静默直连或无法切换账号。
- 工具调用客户端在指定函数时失败。
- 数据目录迁移失败后无法安全恢复。

建议修复完成后建立正式 `v0.2.1` 提交和标签，确保源码、sidecar、Tauri、安装包和 GitHub Release 使用同一个可追溯版本。
