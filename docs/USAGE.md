# Amber · 琥珀 使用教程

> 本教程与软件内「使用文档」页面内容一致，涵盖从添加账号到在客户端调用的完整流程。

Amber 把你自己的 ChatGPT 订阅，在本地转成一个 OpenAI 兼容接口（`http://127.0.0.1:8080/v1`），让 Cherry Studio、Cursor、ChatBox 等工具像用 OpenAI API 一样直接调用。所有数据只保存在你本机，令牌加密存储。

**快速路径**：添加账号 → （可选）配置代理 → 启动服务 → 在客户端填入地址与密钥

---

## 1. 添加 ChatGPT 账号

进入「账号」页面，有两种方式添加账号：

1. 点击右上角「登录 ChatGPT 账号」，会打开浏览器走标准 OAuth 登录，在官网授权后自动回填账号。
2. 如果你已从别的工具导出了令牌，点击「批量导入」，粘贴 JSON 即可一次导入多个账号。

![账号页右上角：登录 / 批量导入](../src/assets/docs/accounts.png)

## 2. 批量导入（JSON 格式）

在弹窗中粘贴 JSON，支持两种结构：

- 账号数组：`[ {…}, {…} ]`
- 对象：`{ "accounts": [ … ] }`

每条至少要有 `access_token` 或 `refresh_token`；若提供 `id_token`，会自动解析出邮箱、账号 ID 和套餐。

按 `chatgpt_account_id` 去重：已存在的账号会更新令牌，而不会重复创建。导入完成后会提示「新增 / 更新 / 跳过」的数量。

![粘贴 JSON 后点击「批量导入」](../src/assets/docs/import.png)

## 3. （可选）配置代理

如果需要通过代理访问，进入「代理」页面点击「添加代理」，支持 HTTP / HTTPS / SOCKS5，凭据加密保存。

添加后可点「测试」检测连通性与延迟；回到「账号」页面即可为每个账号单独绑定代理。

![代理页右上角：添加代理](../src/assets/docs/proxies.png)

## 4. 启动服务

回到「仪表盘」，确认至少已添加一个账号后，点击右上角「启动服务」。

启动后即可复制页面上的「接口地址 (Base URL)」和「API 密钥」，这两项就是客户端要填的内容。

![仪表盘：启动服务 + 复制 Base URL / API 密钥](../src/assets/docs/dashboard.png)

## 5. 在客户端中使用

在 Cherry Studio / Cursor / ChatBox 等工具中，模型服务商选择「OpenAI 兼容 (OpenAI-Compatible)」。

- **Base URL**：`http://127.0.0.1:8080/v1`
- **API Key**：仪表盘上的本地密钥（形如 `sk-local-…`）

保存后即可像调用 OpenAI 一样发起对话，请求会自动转发到你的 ChatGPT 账号。

## 6. 远程开发接入（SSH 隧道）

**场景**：网关跑在你本地 Windows，但你用 VS Code Remote-SSH 连到远程服务器、在远程用 Codex 插件。此时插件跑在远程，直接填 `127.0.0.1:8080` 是连不上的（那是远程的本地）。

**解决办法**：用 SSH「反向端口转发」把远程的 8080 转发回你 Windows 的 8080。这样远程访问 `http://127.0.0.1:8080/v1` 就等于访问你本地的网关，App 无需任何改动。

方式一：命令行反向隧道（连接时加 `-R`，注意是 `-R` 不是 `-L`）：

```bash
ssh -R 8080:127.0.0.1:8080 user@远程服务器
```

方式二（推荐）：写进 `~/.ssh/config`，VS Code Remote-SSH 连上后自动转发：

```
Host myserver
    HostName 远程IP
    User user
    RemoteForward 8080 127.0.0.1:8080
```

验证：在远程终端执行下面的 curl，能返回模型列表即成功（把 `sk-local-…` 换成仪表盘上的密钥）：

```bash
curl http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-local-…"
```

> 提示：远程 sshd 需 `AllowTcpForwarding yes`（默认开启）；仅自用时无需 `GatewayPorts`。

## 7. 查看统计

「统计」页面展示请求量、成功率、Token 消耗与平均延迟趋势，并可查看每条请求日志，方便排查问题。

![统计页：请求量 / 成功率 / Token / 延迟](../src/assets/docs/statistics.png)

## 8. 设置说明

![设置页：端口 / 密钥 / 反封号](../src/assets/docs/settings.png)

- **监听端口**：本地 API 服务的端口，改动后需重启服务。
- **本地 API 密钥**：客户端连接使用的密钥，可随时「重新生成」（旧密钥立即失效）。
- **反封号**：建议保持「注入 Codex 指令」和「TLS 指纹伪装」开启，可降低账号被识别/封禁的风险。
- **默认模型**：当客户端请求的模型不是 `gpt-5*` 系列时使用的回退模型。语言可在此切换中文 / English。

---

## ⚠️ 注意事项

本工具仅供个人自用，请勿分发或商用。使用第三方转发存在账号被 OpenAI 限制的风险，任何「反封号」措施都不能保证绝对安全，请自行评估。
