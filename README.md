<div align="center">

<img src="app-icon.png" width="120" alt="Amber logo"/>

# Amber · 琥珀

**把你的 ChatGPT 订阅变成本地 OpenAI 兼容 API 的桌面网关**

Tauri v2 · Vue 3 · Go

</div>

---

## 简介

Amber（琥珀）是一个 Windows 桌面应用，将 ChatGPT 订阅账号转换为本地 OpenAI 兼容 API，供个人开发和工具接入使用。核心网关逻辑基于开源项目 [sub2api](https://github.com/search?q=sub2api)（LGPL v3）改造，仅限个人使用。

## 功能

- **仪表盘**：网关运行状态、请求统计一览
- **账号管理**：添加 / 批量导入 ChatGPT 账号，OAuth 登录，自动故障切换
- **代理管理**：为账号配置上游代理并测试连通性
- **统计**：请求量、Token 用量、日志查看
- **Codex 接入**：一键生成 Codex CLI 远程配置，可直接复制
- **设置**：API 端口、密钥、加密存储等

## 技术架构

```
┌────────────────────────────────────────┐
│           Tauri v2 桌面应用             │
│  ┌──────────────┐   ┌───────────────┐  │
│  │ Vue 3 前端    │──▶│  Rust 壳       │  │
│  │ (Vite + TS)  │   │  (窗口/托盘)    │  │
│  └──────────────┘   └──────┬────────┘  │
│                            │ 启动/守护   │
│                     ┌──────▼────────┐  │
│                     │  Go Sidecar   │  │
│                     │  本地网关核心   │  │
│                     └──────┬────────┘  │
└────────────────────────────┼───────────┘
                             ▼
              http://127.0.0.1:<端口>/v1/...
              （OpenAI 兼容 API，本地调用）
```

- `src/` — Vue 3 前端
- `src-tauri/` — Rust 壳与打包配置
- `core/` — Go sidecar 源码（账号、代理、网关、存储、OAuth、Codex 配置等模块）

## 从源码构建

### 前置依赖

| 工具 | 版本 |
|---|---|
| Node.js | ≥ 18 |
| Rust（含 MSVC 工具链） | stable |
| Go | ≥ 1.21（仅需重新编译 sidecar 时） |

### 1. 编译 Go sidecar

```powershell
cd core
$env:CGO_ENABLED="0"; $env:GOOS="windows"; $env:GOARCH="amd64"
go build -o ..\src-tauri\binaries\sub2api-sidecar-x86_64-pc-windows-msvc.exe .\cmd\sidecar
```

> sidecar 二进制不入库（见 `.gitignore`），克隆仓库后必须先完成这一步，否则打包报错 `resource path 'binaries\...' doesn't exist`。

### 2. 安装前端依赖并打包

```powershell
# 国内网络建议先设置镜像（一次性）：
# npm config set registry https://registry.npmmirror.com
# Cargo 镜像见下方“常见问题”

npm install

# 如果系统残留了失效的 127.0.0.1 代理配置，先执行：
$env:NO_PROXY="*"; $env:no_proxy="*"

npm run tauri build
```

产物：

- 安装包（NSIS）：`src-tauri\target\release\bundle\nsis\Amber_0.1.0_x64-setup.exe`
- 安装包（MSI）：`src-tauri\target\release\bundle\msi\Amber_0.1.0_x64_en-US.msi`

### 开发模式

```powershell
npm run tauri dev
```

## 常见问题

**Q：打包时报 `Could not connect to index.crates.io ... via 127.0.0.1`？**
系统里残留了指向 `127.0.0.1` 的代理配置。执行 `$env:NO_PROXY="*"` 后再打包；国内网络建议配置 Cargo 镜像（`~/.cargo/config.toml`）：

```toml
[source.crates-io]
replace-with = "rsproxy-sparse"

[source.rsproxy-sparse]
registry = "sparse+https://rsproxy.cn/index/"

[net]
git-fetch-with-cli = true
```

**Q：打开页面几秒后内容区变空白？**
Windows WebView2 的 GPU 合成 bug，本项目已在启动时注入 `--disable-gpu` 规避（见 `src-tauri/src/lib.rs`）。

## 许可

仅限个人使用。核心网关部分基于 sub2api，遵循 LGPL v3。
