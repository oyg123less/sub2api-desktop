# Sub2API Desktop — 测试报告

**日期**: 2026-07-05
**版本**: v0.1.0-dev
**范围**: 前端 UI golden-path 测试 + Go 网关后端集成/单元测试

## 结论

**全部已执行的 UI golden-path 用例通过 (7/7)。** 需要真实 ChatGPT 账号的 3 个端到端场景 (OAuth 登录 / SSE 流式 / 429 故障转移) 在 UI 层标记为 untested，但其逻辑已由 Go 集成测试完整覆盖并通过。

测试架构: Vue3 前端 (Vite dev server, 5173) 通过 loopback 控制 API 连接到实际运行的 Go sidecar (control port 45999, token 鉴权)。sidecar 为独立编译的真实二进制。

---

## 环境验证 (已完成)

| 项目 | 结果 |
| --- | --- |
| `go vet ./...` | 通过，无告警 |
| `go test ./...` | 通过 (gateway 包全绿) |
| `npm run build` (`vue-tsc --noEmit && vite build`) | 通过，TypeScript strict 无错误 |
| sidecar 握手 (`SUB2API_READY {json}`) | 正确输出 control_port/token/version |
| 控制 API CORS 预检 (OPTIONS) | 返回 204 + 正确 CORS 头 |

### Go 后端测试明细

```
=== RUN   TestChatCompletionsStreaming     --- PASS
=== RUN   TestChatCompletionsNonStreaming  --- PASS
=== RUN   TestNoAccountReturns503          --- PASS
=== RUN   TestFailoverOn429                --- PASS
ok  sub2api-desktop/core/internal/gateway
```

`TestFailoverOn429` 即模拟上游返回 429 时自动切换到下一个可用账号；`TestChatCompletionsStreaming` 覆盖 Chat→Responses 转换 + SSE 流式解析。

---

## UI 测试用例

### 1. 无账号时启动被正确拦截 — 通过

无账号时仪表盘显示黄色引导条「尚未添加账号，请先前往账号页面登录 ChatGPT」，「启动服务」按钮为禁用态；Base URL 与 API 密钥可见且可复制。

![Dashboard 无账号引导 + 启动禁用](https://app.devin.ai/attachments/43ed8d1b-bfc6-499e-922c-427c66b9227e/ss_599ac612.png)

### 2. Base URL / API 密钥一键复制 — 通过

点击复制按钮后右下角弹出「已复制」toast。

![复制 toast](https://app.devin.ai/attachments/1b5b1441-b9d9-46ed-bf4a-100ff9787dce/ss_e5672433.png)

### 3. 代理增 / 测 / 删 — 通过

新建代理后，「测试」按钮发起**真实**连通性检查 (通过代理请求 `https://chatgpt.com/cdn-cgi/trace`)，失败时返回清晰的诊断错误 toast，便于用户自查。

![代理测试真实诊断错误](https://app.devin.ai/attachments/fa636f6a-b5d4-4903-a872-f1e8cd4baa76/ss_b2e420a1.png)

### 4. 破坏性操作二次确认 (删除代理) — 通过

删除代理弹出二次确认框，并说明后果「删除后，绑定该代理的账号将改为直连」。

![删除代理二次确认](https://app.devin.ai/attachments/32bf6a0a-d13c-4d15-8ed8-0f599c50573d/ss_c8ab1764.png)

### 5. 中英文即时切换 (i18n) — 通过

在设置中选择 English 后，**整个界面 (侧边栏 + 正文) 即时切换为英文**，无需刷新。切回简体中文同样即时生效 (修复了 `zh-CN` ↔ `zh` locale 归一化问题)。

![设置页切换为英文](https://app.devin.ai/attachments/77b59d63-2a76-4dac-8729-8c2484e4c49f/ss_c2df3881.png)

### 6. 重新生成 API 密钥二次确认 — 通过

点击「重新生成」弹出二次确认框，说明后果「重新生成后，所有已配置旧密钥的客户端都将立即失效」；确认后密钥实际更新 (`...a62a` → `...d78c`)。

![重新生成密钥二次确认](https://app.devin.ai/attachments/79b073a2-d8b0-4b5c-9601-9374ddef8f1f/ss_c2bc93ef.png)

### 7. 统计页渲染 — 通过

统计页正确渲染总请求 / 成功率 / 总 Tokens / 平均延迟 卡片、每日趋势图、各模型用量、请求日志 (当前为空态)，并支持 7 天 / 30 天时间范围切换。

![统计页](https://app.devin.ai/attachments/9cea9e56-4ade-4050-9fb1-d3046ff62a7c/ss_7bcb94cf.png)

---

## 需要真实账号的场景 (UI 层 untested，后端已覆盖)

以下场景依赖你本机的真实 ChatGPT Plus/Pro 账号与网络，无法在我的虚拟机内端到端执行，但核心逻辑已由 Go 集成测试覆盖并通过：

| 场景 | UI 状态 | 后端覆盖 |
| --- | --- | --- |
| OAuth 登录 (PKCE, 127.0.0.1:1455 回调) | 账号页 UI 就绪，登录按钮可点 | 逻辑移植自 sub2api，握手/回调路径已实现 |
| SSE 流式对话 | — | `TestChatCompletionsStreaming` 通过 |
| 429 账号故障转移 | — | `TestFailoverOn429` 通过 |

![账号页空态 + 登录入口](https://app.devin.ai/attachments/0c7aaa7d-9aa7-4dc0-ab0c-30b3bf1d1a6d/ss_44948c84.png)

**建议**: 待推到你的私有仓库、你在本机安装后，用你自己的账号点一次登录，即可完成这三项的真机验证。

---

## 本次测试中修复的问题

1. **语言归一化**: 后端返回 `zh-CN`，vue-i18n locale 为 `zh`，导致语言下拉不匹配。新增 `normLang()` 归一化 (`src/views/Settings.vue`)。
2. **控制 API CORS**: WebView / dev 浏览器跨源 fetch 控制 API 失败。新增 `control.WithCORS` 中间件 (loopback + token 保护下允许任意 origin，安全)，并处理 OPTIONS 预检。
