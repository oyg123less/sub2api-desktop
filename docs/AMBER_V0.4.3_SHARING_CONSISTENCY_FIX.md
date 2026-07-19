# Amber v0.4.3 云共享一致性修复与发布说明

> 目标版本：v0.4.3  
> 修复范围：桌面前端、Go Sidecar、本地 SQLite、Amber Cloud Worker、测试与发布工具  
> 核心目标：云账户、账户页和实际 Codex 调用始终使用同一份当前共享授权

## 1. 问题摘要

v0.4.2 存在两个互相独立的共享入口：

1. 云账户页面从 `cloud_received_keys` 读取当前 Guest Key，因此测试可以成功。
2. 账户页面只展示普通 `accounts`。用户需要手工复制 Base URL 和 Guest Key 再导入，导入后形成一份不会自动轮换的普通 API Key 账户。

共享者删除后重新创建共享时，Worker 会签发新的 Guest Key，旧 Key 会被撤销。此时云账户测试使用新 Key，而账户页和 Codex 可能仍使用旧 Key，最终返回：

```text
HTTP 401
share_access_revoked
```

通用 401 分类又会把该错误当成 ChatGPT OAuth 登录失效，导致界面错误提示“需要重新登录”。

## 2. 修复后的单一数据流

```text
Amber Cloud 当前授权
        |
        v
cloud_received_keys + cloud_received_account_links
        |
        +--> 云账户轻量测试
        |
        +--> 账户页受管云共享行
        |
        +--> 批量账户测试
        |
        +--> 本地网关调度与 Codex 实际请求
```

账户页不再要求用户复制并长期维护 Guest Key。已接收共享会以负数合成 ID 暴露给本机控制 API，避免与普通 SQLite 账户 ID 冲突，但不会复制进 `accounts` 表。

## 3. 受管云共享账户

受管账户增加以下只读来源信息：

| 字段 | 含义 |
| --- | --- |
| `source=cloud_share` | 该行来自 Amber Cloud，不是手工 API 账户 |
| `cloud_grant_id` | 当前接收授权 ID |
| `cloud_owner_name` | 共享者名称 |
| `cloud_group_name` | 共享组名称 |

账户页面行为如下：

| 操作 | v0.4.3 行为 |
| --- | --- |
| 单个测试 | 走与 Codex 相同的完整网关链路，使用当前 Guest Key 和当前代理 |
| 批量测试 | 普通账户与云共享可一起测试 |
| 开启/关闭 | 修改本设备的云共享调度开关 |
| 绑定代理 | 修改被共享者本机访问 Worker 所用代理 |
| 全局代理 | 同时覆盖普通账户和当前云共享账户 |
| 并发/配额 | 只读，由共享者策略统一管理 |
| 删除 | 跳转云账户管理，不允许当普通账户删除 |

## 4. Guest Key 轮换与撤销

- `SaveCloudReceivedKey` 继续以 `(user_id, grant_public_id)` 为唯一来源，新版本覆盖旧版本。
- 账户列表、批量测试和网关调度均在执行时从当前记录解密，不缓存手工副本。
- 远端共享状态变为非 `active`/`paused` 后，本地链接和加密 Guest Key 一并删除。
- 对历史手工导入的旧 `sk-amber-*` 账户不做静默覆盖或删除；它在首次收到 `share_access_revoked` 后会被停用并显示正确说明，避免误删用户数据。

## 5. 测试与开关分离

连接测试只更新健康状态，不再隐式改变用户的本地启用开关：

- 临时网络失败不会自动关闭共享。
- 测试成功不会把用户手动关闭的共享重新打开。
- 明确的 `share_access_revoked` 属于严重鉴权异常，会自动停止该共享参与后续调度。

连接码首次接收仍会执行初始测试。初始测试失败时保持未启用，用户修复网络并重新测试成功后可手动开启。

## 6. 错误分类

Sidecar 会解析 Worker 的结构化错误：

```json
{
  "error": {
    "code": "share_access_revoked",
    "message": "The shared access was revoked."
  }
}
```

该错误不再归类为 `account_unauthorized`，也不会展示 ChatGPT“重新登录”操作。账户状态原因统一为 `share_access_revoked`，中英文界面均引导用户前往云账户重新获取共享。

## 7. 界面调整

- 账户行增加“云共享”标签和“由谁共享”说明。
- 云共享行保留测试、开关、代理和详情入口。
- 普通删除按钮替换为“在云账户中管理”。
- 选中云共享时禁用批量删除，但保留批量测试。
- 详情页隐藏本机不可修改的并发与队列输入框，并解释策略归属。
- 仪表盘账户数量包含受管云共享，只有云共享时不再误报“没有账户”。

## 8. 回归测试矩阵

| 层级 | 场景 | 预期 |
| --- | --- | --- |
| Store | Guest Key 从 v1 轮换到 v2 | 列表和调度只返回 v2 |
| Store | 本机关闭后执行成功测试 | 健康变为正常，开关保持关闭 |
| Store | 共享暂停 | 显示暂停且不参与调度 |
| Store | 全局绑定/清除代理 | 普通账户与云共享同时更新 |
| Cloud Sync | 云账户测试 | 使用当前 Key、选定代理和客户端版本头 |
| Cloud Sync | 远端撤销 | 本地链接和加密 Key 被清除 |
| Gateway | 当前 Key 完整链路测试 | Responses 请求成功且账户保持正常 |
| Gateway | Worker 返回撤销错误 | 精确分类并自动停用共享 |
| Control API | 账户列表与状态计数 | 云共享可见且计数正确 |
| Control API | 云共享删除/限额修改 | 返回 `cloud_share_managed` |
| Frontend | 桌面与窄窗口 | 受管标签、详情、测试和批量限制正确 |
| E2E | 历史旧 Key 行 | 显示共享失效，不显示“重新登录” |

发布门禁包括：Go test/vet/race、Worker TypeScript 与 Vitest、Vue TypeScript 与 Vitest、生产 Vite 构建、Rust fmt/clippy/test、Playwright 全量用例、版本一致性检查和密钥扫描。

## 9. 发布顺序

1. 完成全部本地回归。
2. 构建 v0.4.3 Sidecar 和 NSIS 安装包。
3. 校验安装包版本、文件名和 SHA-256。
4. 推送提交与 `v0.4.3` tag。
5. 创建 GitHub v0.4.3 Release 并上传安装包。
6. 部署 Worker 代码和 D1 `0008_v043_client_version.sql`。
7. 验证 `/health` 返回 `0.4.3`，并验证旧客户端收到 426、新客户端通过门禁。

该顺序保证最低版本提高前，用户已经可以下载 v0.4.3。

## 10. 回滚策略

- 桌面端：保留 v0.4.2 Release，不覆盖旧 tag；必要时回退 GitHub latest 指向。
- Worker：代码可回滚到上一部署；共享网关接口协议未做破坏性修改。
- 门禁：将 `minimum_client_version` 和 `latest_client_version` 临时恢复为 `0.4.2` 即可解除升级限制。
- 本地数据库：本次不增加本地 schema，v0.4.2 数据可直接由 v0.4.3 打开。

## 11. 验收标准

- 重新共享后，用户无需复制或重新导入 Base URL/Guest Key。
- 云账户、账户页和实际 Codex 调用使用同一当前授权。
- 账户页测试结果代表真实网关链路。
- 撤销授权不会提示 ChatGPT 重新登录。
- 测试失败不会因临时网络问题偷偷修改本地开关。
- 只有云共享账户时，仪表盘和服务启动判断仍正确。
- 安装包只生成到 `src-tauri/target/release/bundle/nsis`，打包过程不操作已安装 Amber。

## 12. 2026-07-19 发布验证记录

- Go：`go test ./...`、`go vet ./...`、`go test -race ./...` 通过。
- Worker：TypeScript 检查通过，8 个测试文件、51 个用例通过。
- 前端：TypeScript 检查通过，10 个测试文件、40 个用例通过，生产构建通过。
- Rust：`cargo fmt --check`、Clippy `-D warnings` 通过，9 个用例通过。
- Playwright：生产 `vite preview` 下，桌面和紧凑视口共 72 个用例通过。
- 版本一致性：root package、lock、Tauri、Cargo、Sidecar、Worker 均为 `0.4.3`。
- 密钥扫描：未发现 GitHub、Cloudflare、Resend 或 QQ SMTP 真实凭据写入仓库。
- NSIS：`Amber_0.4.3_x64-setup.exe`，7,966,422 bytes。
- SHA-256：`724988948FD9B8E7CA8208C4D9046766CB9DAF79AB50AAA3014468DA5F1C7F12`。
