# Amber 官网

Amber 官网是仓库内的独立 Vue 3 + TypeScript + Vite SSG 项目。源码、测试、静态资源和 Cloudflare Pages 配置全部位于 `website/`。

## 页面

- `/` 首页
- `/download` 下载与 SHA-256
- `/docs` 使用文档
- `/changelog` 更新日志
- `/faq` 常见问题
- `/security` 安全与隐私
- `/status` 服务状态页外壳

## 运行环境

必须使用仓库 `AGENTS.md` 指定的 Node.js 24：

```powershell
$NodeBin = 'C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin'
$Node = Join-Path $NodeBin 'node.exe'
$Npm = 'D:\Setup\Nodejs\nodejs\node_modules\npm\bin\npm-cli.js'
$env:Path = "$NodeBin;$env:Path"
$env:npm_config_cache = 'D:\Study\other\vet\.npm-cache'
```

首次安装依赖：

```powershell
& $Node $Npm ci --no-audit --no-fund
```

## 开发与验证

```powershell
# 开发服务器：http://127.0.0.1:4174
& $Node '.\node_modules\vite\bin\vite.js' --host 127.0.0.1 --port 4174 --strictPort

# TypeScript
& $Node '.\node_modules\vue-tsc\bin\vue-tsc.js' --noEmit

# Vitest
& $Node '.\node_modules\vitest\vitest.mjs' run

# 静态生成
& $Node '.\node_modules\vite-ssg\bin\vite-ssg.js' build

# 构建产物、链接和 Secret 扫描
& $Node '.\scripts\check-dist.mjs'
& $Node '.\scripts\check-links.mjs'
& $Node '.\scripts\check-secrets.mjs'

# Playwright 桌面、窄窗口和手机视口
& $Node '.\node_modules\@playwright\test\cli.js' test --workers=4

```

构建产物位于 `website/dist/`。`wrangler.toml` 只声明 Pages 构建目录，不包含生产绑定、域名或凭据。

## 版本数据

稳定版与即将发布版本只在 `src/config/releases.ts` 维护。版本、下载地址、文件大小、发布时间与 SHA-256 必须和正式 GitHub Release 完全一致。

当前稳定版为 v0.4.4，安装包由 GitHub Releases 直接托管。

## 截图

首页、产品演示和文档页使用 `public/screenshots/v044/` 中的 Amber v0.4.4 真实界面。原始截图不进入 `public/` 或 Git；只提交经过不可逆像素替换的输出文件。

`scripts/sanitize-product-assets.py` 负责遮盖账号身份、连接码、用户 ID、本机路径、API Key、用量和费用，并生成桌面、移动端及 Open Graph 素材。脚本需要 Pillow，原始截图路径通过命令行参数传入，不属于常规构建流程。

替换前必须直接检查图片像素中的邮箱、账号 ID、Token、API Key、Guest Key、代理凭据、服务器地址和设备名称。不能只在网页上使用 CSS 遮挡。

## 发布边界

官网构建不包含生产凭据。部署、DNS 与 GitHub Release 由独立发布流程执行，构建脚本不得调用私有 Worker 或管理 API。
