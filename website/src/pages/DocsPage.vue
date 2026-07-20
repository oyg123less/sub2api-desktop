<script setup lang="ts">
import {
  CheckCircle2,
  CircleAlert,
  Info,
  ShieldAlert,
  TriangleAlert,
} from "lucide-vue-next";
import { ref } from "vue";
import ImageViewer from "../components/ImageViewer.vue";
import PageIntro from "../components/PageIntro.vue";
import { stableRelease } from "../config/releases";

const mobileToc = ref<HTMLDetailsElement | null>(null);
const currentVersion = `v${stableRelease.version}`;

const sections = [
  { id: "install", label: "安装与首次启动" },
  { id: "accounts", label: "导入与管理账号" },
  { id: "proxies", label: "代理配置" },
  { id: "service", label: "启动本地服务" },
  { id: "codex-local", label: "本地 Codex 接入" },
  { id: "ssh-host-key", label: "确认 SSH 主机密钥" },
  { id: "ssh-reverse", label: "SSH 反向隧道" },
  { id: "cloud", label: "云账号与同步" },
  { id: "sharing", label: `${currentVersion} 共享` },
  { id: "devices", label: "多设备路由" },
  { id: "workspaces", label: "独立工作区" },
  { id: "troubleshooting", label: "常见故障排查" },
] as const;

function closeMobileToc() {
  if (mobileToc.value) mobileToc.value.open = false;
}
</script>

<template>
  <PageIntro
    eyebrow="使用文档"
    title="从导入账号到 Codex 接入"
    :description="`本手册以当前稳定版 ${currentVersion} 为准，覆盖本地网关、代理、SSH、云同步、独立工作区与连接码共享。`"
  >
    <div class="docs-version-row">
      <span class="status-pill stable"><span class="status-dot" />当前稳定版 {{ currentVersion }}</span>
      <RouterLink to="/download">查看下载与校验信息</RouterLink>
    </div>
  </PageIntro>

  <div class="container docs-shell">
    <aside class="desktop-toc" aria-label="文档目录">
      <p>本页目录</p>
      <nav>
        <a v-for="(section, index) in sections" :key="section.id" :href="`#${section.id}`">
          <span>{{ String(index + 1).padStart(2, "0") }}</span>
          {{ section.label }}
        </a>
      </nav>
      <RouterLink class="toc-more" to="/faq">查看完整常见问题</RouterLink>
    </aside>

    <article class="docs-content">
      <details ref="mobileToc" class="mobile-toc">
        <summary>跳转到章节</summary>
        <nav aria-label="移动端文档目录">
          <a
            v-for="(section, index) in sections"
            :key="section.id"
            :href="`#${section.id}`"
            @click="closeMobileToc"
          >
            {{ String(index + 1).padStart(2, "0") }} · {{ section.label }}
          </a>
        </nav>
      </details>

      <div class="callout docs-scope-note">
        <Info :size="20" aria-hidden="true" />
        <div>
          <strong>本地使用无需注册云账号</strong>
          <p>导入账号、配置代理、启动本地服务、本地或 SSH Codex 接入、统计与日志均无需 Amber 云账号。OAuth 登录、SSH 连接和上游请求仍需对应网络可用；只有云同步、多设备和共享授权需要 Amber 云账号。</p>
        </div>
      </div>

      <div class="callout screenshot-note">
        <CircleAlert :size="20" aria-hidden="true" />
        <div>
          <strong>截图版本说明</strong>
          <p>本页产品图均来自 {{ currentVersion }} 真实界面。账号、连接码、用户 ID、本机路径、API Key、用量和费用等敏感信息已在图片像素中替换为不可用演示值。</p>
        </div>
      </div>

      <section id="install" class="doc-section">
        <p class="section-index">01</p>
        <h2>安装与首次启动</h2>
        <p class="section-lede">Amber 面向 Windows 10/11 x64。Windows 11 通常已经包含 WebView2；Windows 10 若打开后界面空白，应先安装当前版 Microsoft Edge WebView2 Runtime。</p>

        <ol class="doc-steps">
          <li>
            <strong>下载安装包</strong>
            <p>从本站下载页进入 {{ currentVersion }} GitHub Release，下载 <code>{{ stableRelease.installerName }}</code>，并核对页面提供的 SHA-256。</p>
          </li>
          <li>
            <strong>处理 SmartScreen 提示</strong>
            <p>若 Windows 显示“已保护你的电脑”，先确认文件名、来源和 SHA-256，再选择“更多信息”继续。不要忽略校验直接运行来源不明的安装包。</p>
          </li>
          <li>
            <strong>等待后台就绪</strong>
            <p>启动 Amber 后，左下角应显示“已就绪”。若持续启动中或启动失败，打开运行诊断检查数据目录、端口和 Sidecar 状态。</p>
          </li>
        </ol>

        <div class="callout warning">
          <TriangleAlert :size="20" aria-hidden="true" />
          <div>
            <strong>覆盖安装前先保留数据备份</strong>
            <p>{{ currentVersion }} 可以直接覆盖旧版本安装，一般会保留账号、代理和设置。不要先卸载来处理普通升级问题；重要数据仍应先备份。</p>
          </div>
        </div>
      </section>

      <section id="accounts" class="doc-section">
        <p class="section-index">02</p>
        <h2>导入与管理账号</h2>
        <p class="section-lede">进入“账号”，点击右上角“导入账号”。Amber 支持 ChatGPT OAuth、Base URL + API Key，以及单个或批量 JSON 文件。</p>

        <ImageViewer
          src="/screenshots/v044/accounts-v044.png"
          alt="Amber v0.4.4 账号列表与导入入口真实界面"
          caption="真实 v0.4.4 界面，账号身份、用量与费用已替换为演示值。点击图片可查看大图。"
        />

        <h3>选择合适的导入方式</h3>
        <dl class="doc-definition-list">
          <div>
            <dt>ChatGPT OAuth</dt>
            <dd>在浏览器完成登录与授权。授权超时或提示会话过期时，应重新发起流程，不要反复导入旧 Token。</dd>
          </div>
          <div>
            <dt>Base URL + API Key</dt>
            <dd>适用于 OpenAI 兼容上游。Base URL 通常以 <code>/v1</code> 结尾，可在保存前选择直连或一个已保存代理。</dd>
          </div>
          <div>
            <dt>JSON 文件</dt>
            <dd>支持单账号、账号数组、<code>{ "accounts": [...] }</code>，以及一次选择多个文件。提交前先检查新增、更新、跳过和冲突预览。</dd>
          </div>
        </dl>

        <h3>并发、队列和批处理</h3>
        <p>账号详情中的“最大并发”限制同时处理的请求数，“队列容量”控制并发占满后的等待数量。队列也满时，新请求会被明确拒绝，而不是无限等待。批量全选只覆盖当前页，跨页选择会保留并显示总数。</p>
      </section>

      <section id="proxies" class="doc-section">
        <p class="section-index">03</p>
        <h2>代理配置与批量应用</h2>
        <p class="section-lede">Amber 支持 HTTP、HTTPS 和 SOCKS5。代理保存后应先运行测试，分别查看 DNS、连接、TLS 和 HTTP 阶段，而不是只看最终成功或失败。</p>

        <ImageViewer
          src="/screenshots/v044/network-v044.png"
          alt="Amber v0.4.4 代理与网络出口真实界面"
          caption="真实 v0.4.4 界面，代理信息已替换为演示值。“应用到全部账号”是一次性写入现有账号。"
        />

        <ul class="doc-checklist">
          <li><CheckCircle2 :size="18" aria-hidden="true" />代理密码留空时可复用已保存值；只有明确清除才会删除。</li>
          <li><CheckCircle2 :size="18" aria-hidden="true" />批量应用后重新读取账号绑定摘要，确认实际写入结果。</li>
          <li><CheckCircle2 :size="18" aria-hidden="true" />代理测试成功不代表账号凭据、模型权限和上游策略一定可用。</li>
          <li><CheckCircle2 :size="18" aria-hidden="true" />不要求普通用户开启 TUN；先使用 Amber 自身的连接诊断定位失败阶段。</li>
        </ul>
      </section>

      <section id="service" class="doc-section">
        <p class="section-index">04</p>
        <h2>启动本地 OpenAI 兼容服务</h2>
        <p class="section-lede">回到仪表盘，点击“启动服务”。默认地址为 <code>http://127.0.0.1:8080/v1</code>，本地 API Key 由 Amber 生成，与上游 OpenAI API Key 不是同一项凭据。</p>

        <ImageViewer
          src="/screenshots/v044/dashboard-v044.png"
          alt="Amber v0.4.4 仪表盘与本地网关状态真实界面"
          caption="真实 v0.4.4 界面，API Key 与运营数据已替换为演示值。使用时以应用内实际值为准。"
        />

        <h3>先验证模型列表</h3>
        <p>仪表盘显示“运行中”后，在 PowerShell 中使用当前本地 API Key 检查模型列表：</p>
        <pre class="code-block" tabindex="0" aria-label="PowerShell 验证模型列表命令"><code>curl.exe http://127.0.0.1:8080/v1/models `
  -H "Authorization: Bearer sk-local-替换为你的密钥"</code></pre>

        <div class="callout warning">
          <TriangleAlert :size="20" aria-hidden="true" />
          <div>
            <strong>默认只监听本机</strong>
            <p>除非明确需要局域网访问，不要开启局域网监听，更不要把 8080 端口映射到公网。曾经泄露的本地 API Key 应立即重新生成。</p>
          </div>
        </div>
      </section>

      <section id="codex-local" class="doc-section">
        <p class="section-index">05</p>
        <h2>本地 Codex 接入</h2>
        <p class="section-lede">{{ currentVersion }} 提供“启动服务并注入”：Amber 会先启动本地网关，验证健康状态、API Key 和模型列表，再写入并回读 Codex 配置。</p>

        <ImageViewer
          src="/screenshots/v044/codex-injection-v044.png"
          alt="Amber v0.4.4 本机 Codex 一键接入真实界面"
          caption="真实 v0.4.4 本机接入界面，本机路径与备份时间已替换为演示值。"
        />

        <ol class="doc-steps compact">
          <li><strong>选择“启动服务并注入”</strong><p>Amber 会启动本地服务；启动失败、端口冲突或健康检查异常时不会写入配置。</p></li>
          <li><strong>选择模型并核对预览</strong><p>检查将写入的 <code>config.toml</code>、<code>auth.json</code> 路径、Base URL 和模型。</p></li>
          <li><strong>执行一键注入</strong><p>Amber 会先备份已有配置。应用后重新加载或重启 Codex，再发起最小请求。</p></li>
        </ol>

      </section>

      <section id="ssh-host-key" class="doc-section">
        <p class="section-index">06</p>
        <h2>首次连接时确认 SSH 主机密钥</h2>
        <p class="section-lede">主机密钥用于确认正在连接的是预期服务器。第一次测试远程目标时，Amber 会显示服务器 SHA-256 指纹并等待你在本机确认。</p>

        <ol class="doc-steps compact">
          <li><strong>点击“测试连接”</strong><p>Amber 使用表单中的主机、SSH 端口、用户名和认证信息连接远程服务器。</p></li>
          <li><strong>在可信渠道取得指纹</strong><p>通过云厂商控制台、服务器本地终端或管理员提供的可信记录核对，不要使用同一条可疑网络连接自行证明自己。</p></li>
          <li><strong>逐字比较 SHA-256</strong><p>只有指纹完全一致时，才在本机 Amber 点击“信任并继续”；不一致立即取消并检查主机地址或服务器变更。</p></li>
        </ol>

        <p>可在服务器可信终端列出 OpenSSH 主机公钥指纹：</p>
        <pre class="code-block" tabindex="0" aria-label="Linux 查看 SSH 主机指纹命令"><code>for key in /etc/ssh/ssh_host_*_key.pub; do
  ssh-keygen -lf "$key" -E sha256
done</code></pre>

        <div class="callout warning">
          <ShieldAlert :size="20" aria-hidden="true" />
          <div>
            <strong>“等待确认”是在等当前用户操作</strong>
            <p>服务器不会自动替你确认。若没有看到确认界面，重新点击“测试连接”或“一键注入”，不要在未知指纹上继续等待或盲目信任。</p>
          </div>
        </div>
      </section>

      <section id="ssh-reverse" class="doc-section">
        <p class="section-index">07</p>
        <h2>SSH 反向隧道与远程 Codex</h2>
        <p class="section-lede">当远程服务器不能直接访问上游，但 Windows 电脑上的 Amber 能够通过本机网络或代理访问时，选择“反向隧道（回流本机）”。</p>

        <div class="route-flow" aria-label="SSH 反向隧道请求路径">
          <span>远程 Codex</span><b aria-hidden="true">→</b>
          <span>服务器本地端口</span><b aria-hidden="true">→</b>
          <span>SSH 反向隧道</span><b aria-hidden="true">→</b>
          <span>本机 Amber</span><b aria-hidden="true">→</b>
          <span>本机代理与账号</span>
        </div>

        <h3>远端准备条件</h3>
        <ul>
          <li>SSH 服务允许 TCP 转发，通常需要 <code>AllowTcpForwarding yes</code>。</li>
          <li>选定的远程回流端口未被其他进程占用。</li>
          <li>SSH 用户可以创建并写入自己的 <code>~/.codex</code>。</li>
          <li>普通个人回流不需要开启 <code>GatewayPorts</code>。</li>
        </ul>

        <div class="callout danger">
          <CircleAlert :size="20" aria-hidden="true" />
          <div>
            <strong>本机离线时，反向隧道立即失去上游路径</strong>
            <p>远程 Codex 请求最终仍由本机 Amber 发出。本机休眠、Amber 退出、服务停止或 SSH 断开都会导致请求失败。</p>
          </div>
        </div>
      </section>

      <section id="cloud" class="doc-section">
        <p class="section-index">08</p>
        <h2>云账号注册与加密同步</h2>
        <p class="section-lede">云账号用于同步、备份、多设备和共享，不是使用本地网关的前置条件。注册时需要邮箱验证码，并应使用密码管理器保存云主密码。</p>

        <h3>同步失败时按阶段诊断</h3>
        <ol class="diagnostic-grid">
          <li><strong>DNS</strong><span>域名是否解析，系统或代理 DNS 是否返回可用地址。</span></li>
          <li><strong>TCP</strong><span>目标端口能否建立连接，防火墙和代理是否拦截。</span></li>
          <li><strong>TLS</strong><span>证书、SNI 和系统时间是否正常。</span></li>
          <li><strong>HTTP</strong><span>服务是否返回预期状态，而不是代理登录页或错误页。</span></li>
        </ol>
        <p>在“连接设置”中依次尝试系统代理、Amber 已保存代理或直连，运行网络探测，成功后应用并重试。退出云账号不会修复网络链路，只会清除当前会话。</p>

        <div class="callout warning">
          <TriangleAlert :size="20" aria-hidden="true" />
          <div>
            <strong>优先使用应用内置云入口</strong>
            <p>{{ currentVersion }} 首选 <code>https://api.amberapp.asia</code>，并为幂等请求提供 Workers 域名回退。不要手工写入来历不明的云地址。</p>
          </div>
        </div>
      </section>

      <section id="sharing" class="doc-section">
        <p class="section-index">09</p>
        <h2>当前 {{ currentVersion }} 的共享流程</h2>
        <p class="section-lede">{{ currentVersion }} 使用“连接码 + 临时密码”快速共享，不要求先添加好友。使用前，双方都需要登录各自的 Amber 云账号。</p>

        <ImageViewer
          src="/screenshots/v044/cloud-sharing-v044.png"
          alt="Amber v0.4.4 云账户、加密同步与受控共享真实界面"
          caption="真实 v0.4.4 界面，账户、连接码与用户标识已替换为演示值；连接后会为接收者建立独立授权。"
        />

        <div class="sharing-columns">
          <div>
            <h3>共享者</h3>
            <ol>
              <li>在云账户中选择愿意共享的一个或多个账号。</li>
              <li>点击“开始共享”，设置领取人数和临时密码有效期。</li>
              <li>把连接码与临时密码通过可信渠道发送给接收者。</li>
              <li>在已连接用户中单独暂停、限流或移除授权。</li>
            </ol>
          </div>
          <div>
            <h3>接收者</h3>
            <ol>
              <li>登录自己的云账号，打开连接他人共享的入口。</li>
              <li>输入或粘贴连接码与临时密码。</li>
              <li>点击“连接并使用”，等待真实路径测试完成。</li>
              <li>成功后在账号页面使用收到的共享。</li>
            </ol>
          </div>
        </div>

        <div class="callout">
          <Info :size="20" aria-hidden="true" />
          <div>
            <strong>连接码和临时密码不是模型 API Key</strong>
            <p>它们只用于领取授权。每位接收者会获得独立 Guest Key；暂停或删除一个接收者，不应影响其他接收者。</p>
          </div>
        </div>

        <div class="upcoming-note">
          <h3>每位接收者独立管理</h3>
          <p>连接成功后产生独立授权和 Guest Key。共享者可以单独暂停、限流或移除某位接收者，不影响同一共享中的其他人。</p>
        </div>
      </section>

      <section id="devices" class="doc-section">
        <p class="section-index">10</p>
        <h2>{{ currentVersion }} 的多设备定向路由</h2>
        <p class="section-lede">新共享默认固定到创建它的电脑。共享者可显式添加最多两台具备目标账号且健康的备用设备，其他在线设备不会自动接管。</p>

        <div class="callout danger">
          <CircleAlert :size="20" aria-hidden="true" />
          <div>
            <strong>备用设备必须具备相同账号与可用网络</strong>
            <p>只在线并不代表可以承载共享。备用设备还必须拥有目标账号、账号处于启用和健康状态，并具备可用代理与并发容量。</p>
          </div>
        </div>

        <h3>路由与故障转移边界</h3>
        <ul class="doc-checklist">
          <li><CheckCircle2 :size="18" aria-hidden="true" />主设备正常时，共享始终保持其账号、代理和网络出口。</li>
          <li><CheckCircle2 :size="18" aria-hidden="true" />主设备在上游开始前不可用时，才按明确优先级尝试合格备用设备。</li>
          <li><CheckCircle2 :size="18" aria-hidden="true" />上游请求开始后不跨设备重放，避免重复扣费或重复执行。</li>
        </ul>

      </section>

      <section id="workspaces" class="doc-section">
        <p class="section-index">11</p>
        <h2>{{ currentVersion }} 的独立云账号工作区</h2>
        <p class="section-lede">{{ currentVersion }} 为每个云用户使用独立工作区，隔离普通账号、代理、同步队列、Guest Key、日志、设置与 SSH 目标。</p>

        <div class="callout danger">
          <ShieldAlert :size="20" aria-hidden="true" />
          <div>
            <strong>退出登录不会删除工作区或改变归属</strong>
            <p>再次登录同一用户会打开原工作区；登录另一用户时必须切换或创建其工作区，不会复用上一用户的数据。</p>
          </div>
        </div>

        <div class="upcoming-note">
          <h3>旧数据库归属不明确时只读恢复</h3>
          <p>检测到多个历史用户、归属不明同步队列或异常元数据时，Amber 不会猜测或自动合并，而是打开只读恢复工作区。确认归属后再显式导出需要的账号和代理。</p>
        </div>
      </section>

      <section id="troubleshooting" class="doc-section last-section">
        <p class="section-index">12</p>
        <h2>常见故障排查</h2>
        <p class="section-lede">先记录发生时间、HTTP 状态、请求 ID、实际端口和失败阶段。日志与截图必须先移除邮箱、账号 ID、API Key、Token、代理凭据、服务器地址和设备名称。</p>

        <div class="troubleshooting-list">
          <details open>
            <summary>Codex 返回 502 Bad Gateway</summary>
            <div>
              <p>{{ currentVersion }} 最常见原因是已经写入 <code>127.0.0.1:8080/v1</code>，但 Amber 本地服务没有启动。</p>
              <ol>
                <li>打开 Amber，在仪表盘点击“启动服务”。</li>
                <li>确认状态为“运行中”，并用 <code>/v1/models</code> 验证。</li>
                <li>核对 Codex 配置中的端口，再重新发起请求。</li>
              </ol>
            </div>
          </details>
          <details>
            <summary>无法连接 127.0.0.1:8080</summary>
            <div><p>检查服务是否启动、设置中的端口是否改变，以及客户端是否把 <code>/v1</code> 拼成了 <code>/v1/v1</code>。若端口被占用，先确认占用进程，不要直接关闭未知服务。</p></div>
          </details>
          <details>
            <summary>账号测试成功，但 Codex 仍不可用</summary>
            <div><p>账号测试只证明 Amber 可以访问该账号，不证明本地网关正在运行。依次检查服务状态、<code>/v1/models</code>、本地 API Key、Codex Base URL 和配置重新加载。</p></div>
          </details>
          <details>
            <summary>代理测试成功，但账号请求失败</summary>
            <div><p>继续检查账号凭据、模型权限、上游限制和账号绑定的实际代理。Clash Fake-IP 或 TUN 环境还可能让 DNS 与真实连接路径不同，应查看分阶段诊断。</p></div>
          </details>
          <details>
            <summary>SSH 已连接，但远程配置写入失败</summary>
            <div><p>使用同一 SSH 用户确认 <code>$HOME</code>、<code>~/.codex</code> 权限、磁盘空间和远程端口。主机密钥变化时必须重新通过可信渠道核对，不能仅为消除错误而删除记录。</p></div>
          </details>
          <details>
            <summary>云同步显示 DNS、TCP、TLS 或 HTTP 失败</summary>
            <div><p>打开云连接设置，分别测试系统代理、已保存代理和直连。根据失败阶段修复解析、连接、防火墙、证书或服务响应；不要通过退出登录掩盖网络错误。</p></div>
          </details>
          <details>
            <summary>共享者设备离线</summary>
            <div><p>OAuth 共享和本机回流依赖共享者配置的主设备或合格备用设备。恢复设备、网络、Relay 和目标账号后重试；未显式配置的设备不会自动接管。</p></div>
          </details>
        </div>

        <div class="docs-next">
          <div>
            <h3>需要按现象继续排查？</h3>
            <p>常见问题页补充了端口冲突、TUN、工作区、设备回流和安全边界说明。</p>
          </div>
          <RouterLink class="button button-secondary" to="/faq">查看常见问题</RouterLink>
        </div>
      </section>
    </article>
  </div>
</template>

<style scoped>
.docs-version-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  margin-top: 24px;
}

.docs-version-row a {
  font-size: 14px;
  font-weight: 700;
}

.docs-shell {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  align-items: start;
  gap: 52px;
  padding-block: 58px 100px;
}

.desktop-toc {
  position: sticky;
  top: calc(var(--header-height) + 24px);
  max-height: calc(100vh - var(--header-height) - 48px);
  overflow-y: auto;
  padding-right: 12px;
}

.desktop-toc > p {
  margin-bottom: 13px;
  color: var(--ink-soft);
  font-size: 12px;
  font-weight: 760;
  text-transform: uppercase;
}

.desktop-toc nav {
  display: grid;
  border-left: 1px solid var(--border);
}

.desktop-toc nav a {
  display: grid;
  grid-template-columns: 26px minmax(0, 1fr);
  gap: 6px;
  padding: 7px 10px 7px 14px;
  color: var(--ink-soft);
  font-size: 13px;
  line-height: 1.4;
  text-decoration: none;
}

.desktop-toc nav a:hover,
.desktop-toc nav a:focus-visible {
  background: var(--amber-soft);
  color: var(--amber-dark);
}

.desktop-toc nav span {
  color: var(--amber-dark);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 11px;
}

.toc-more {
  display: inline-block;
  margin: 18px 0 0 14px;
  font-size: 13px;
  font-weight: 700;
}

.docs-content {
  width: 100%;
  min-width: 0;
  max-width: 968px;
}

.mobile-toc {
  display: none;
}

.docs-scope-note {
  margin-bottom: 20px;
  background: var(--teal-soft);
}

.doc-section {
  padding-block: 64px;
  border-bottom: 1px solid var(--border);
  scroll-margin-top: calc(var(--header-height) + 20px);
}

.doc-section:first-of-type {
  padding-top: 44px;
}

.last-section {
  padding-bottom: 0;
  border-bottom: 0;
}

.section-index {
  margin-bottom: 8px;
  color: var(--amber-dark);
  font-family: "Cascadia Code", Consolas, monospace;
  font-size: 13px;
  font-weight: 750;
}

.doc-section h2 {
  margin-bottom: 16px;
  font-size: 31px;
}

.doc-section h3 {
  margin: 34px 0 12px;
}

.section-lede {
  margin-bottom: 30px;
  color: var(--ink-soft);
  font-size: 18px;
  line-height: 1.75;
}

.doc-section :deep(.image-viewer) {
  margin-block: 30px 38px;
}

.doc-section :deep(.image-button) {
  box-shadow: 0 10px 28px rgba(28, 34, 30, 0.1);
}

@media (hover: hover) and (pointer: fine) {
  .doc-section :deep(.image-expand) {
    opacity: 0;
    transform: translateY(4px);
    transition:
      opacity var(--motion-fast) ease,
      transform var(--motion-fast) var(--ease-out);
  }

  .doc-section :deep(.image-button:hover .image-expand),
  .doc-section :deep(.image-button:focus-visible .image-expand) {
    opacity: 1;
    transform: none;
  }
}

.doc-steps {
  display: grid;
  margin: 24px 0 30px;
  padding: 0;
  list-style: none;
  counter-reset: doc-step;
}

.doc-steps li {
  position: relative;
  min-width: 0;
  padding: 19px 0 19px 54px;
  border-top: 1px solid var(--border);
  counter-increment: doc-step;
}

.doc-steps li:last-child {
  border-bottom: 1px solid var(--border);
}

.doc-steps li::before {
  position: absolute;
  top: 18px;
  left: 0;
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  border-radius: 50%;
  background: var(--amber-soft);
  color: var(--amber-dark);
  content: counter(doc-step);
  font-size: 13px;
  font-weight: 800;
}

.doc-steps strong {
  display: block;
  margin-bottom: 4px;
}

.doc-steps p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.doc-steps.compact li {
  padding-block: 15px;
}

.doc-steps.compact li::before {
  top: 14px;
}

.doc-definition-list {
  margin: 0 0 34px;
  border-top: 1px solid var(--border);
}

.doc-definition-list > div {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  gap: 24px;
  padding-block: 18px;
  border-bottom: 1px solid var(--border);
}

.doc-definition-list dt {
  font-weight: 750;
}

.doc-definition-list dd {
  margin: 0;
  color: var(--ink-soft);
}

.doc-checklist {
  display: grid;
  gap: 12px;
  margin: 24px 0 0;
  padding: 0;
  list-style: none;
}

.doc-checklist li {
  display: grid;
  grid-template-columns: 22px minmax(0, 1fr);
  gap: 10px;
}

.doc-checklist svg {
  margin-top: 4px;
  color: var(--green);
}

.code-block {
  margin: 18px 0 28px;
}

.upcoming-note {
  margin-top: 32px;
  padding: 24px 0 4px 24px;
  border-left: 4px solid var(--teal);
}

.upcoming-note h3 {
  margin: 13px 0 7px;
}

.upcoming-note p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

.route-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
  margin: 26px 0 34px;
  padding-block: 18px;
  border-block: 1px solid var(--border);
}

.route-flow span {
  padding: 7px 10px;
  border-radius: 5px;
  background: var(--teal-soft);
  color: var(--teal);
  font-size: 13px;
  font-weight: 700;
}

.route-flow b {
  color: var(--ink-soft);
}

.diagnostic-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 22px 0 30px;
  padding: 0;
  border: 1px solid var(--border);
  list-style: none;
}

.diagnostic-grid li {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  gap: 14px;
  padding: 18px;
  border-bottom: 1px solid var(--border);
}

.diagnostic-grid li:nth-child(odd) {
  border-right: 1px solid var(--border);
}

.diagnostic-grid li:nth-last-child(-n + 2) {
  border-bottom: 0;
}

.diagnostic-grid strong {
  color: var(--amber-dark);
}

.diagnostic-grid span {
  color: var(--ink-soft);
  font-size: 14px;
}

.sharing-columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  margin: 30px 0;
  border: 1px solid var(--border);
  background: var(--border);
}

.sharing-columns > div {
  padding: 24px;
  background: var(--surface);
}

.sharing-columns h3 {
  margin-top: 0;
}

.sharing-columns ol {
  margin-bottom: 0;
  padding-left: 21px;
}

.sharing-columns li + li {
  margin-top: 8px;
}

.troubleshooting-list {
  border-top: 1px solid var(--border);
}

.troubleshooting-list details {
  border-bottom: 1px solid var(--border);
}

.troubleshooting-list summary {
  position: relative;
  padding: 18px 42px 18px 0;
  font-weight: 740;
  cursor: pointer;
}

.troubleshooting-list summary::after {
  position: absolute;
  top: 18px;
  right: 8px;
  color: var(--amber-dark);
  content: "+";
  font-size: 20px;
  line-height: 1;
}

.troubleshooting-list details[open] summary::after {
  content: "−";
}

.troubleshooting-list details > div {
  padding: 0 34px 20px 0;
  color: var(--ink-soft);
}

.troubleshooting-list details p:last-child,
.troubleshooting-list details ol:last-child {
  margin-bottom: 0;
}

.docs-next {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 28px;
  margin-top: 42px;
  padding-top: 28px;
  border-top: 1px solid var(--border);
}

.docs-next h3 {
  margin: 0 0 5px;
}

.docs-next p {
  margin-bottom: 0;
  color: var(--ink-soft);
}

@media (max-width: 960px) {
  .docs-shell {
    grid-template-columns: 1fr;
    gap: 0;
    padding-top: 34px;
  }

  .desktop-toc {
    display: none;
  }

  .mobile-toc {
    display: block;
    margin-bottom: 22px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
  }

  .mobile-toc summary {
    padding: 13px 16px;
    color: var(--ink);
    font-weight: 730;
    cursor: pointer;
  }

  .mobile-toc nav {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 4px 10px 12px;
    border-top: 1px solid var(--border);
  }

  .mobile-toc nav a {
    padding: 9px 8px;
    color: var(--ink-soft);
    font-size: 13px;
    text-decoration: none;
  }
}

@media (max-width: 640px) {
  .docs-shell {
    padding-bottom: 72px;
  }

  .mobile-toc nav {
    grid-template-columns: 1fr;
  }

  .doc-section {
    padding-block: 52px;
  }

  .doc-section h2 {
    font-size: 27px;
  }

  .section-lede {
    font-size: 16px;
  }

  .doc-definition-list > div,
  .diagnostic-grid,
  .sharing-columns {
    grid-template-columns: 1fr;
  }

  .doc-definition-list > div {
    gap: 5px;
  }

  .diagnostic-grid li,
  .diagnostic-grid li:nth-child(odd),
  .diagnostic-grid li:nth-last-child(-n + 2) {
    border-right: 0;
    border-bottom: 1px solid var(--border);
  }

  .diagnostic-grid li:last-child {
    border-bottom: 0;
  }

  .route-flow {
    align-items: stretch;
  }

  .route-flow span {
    flex: 1 1 42%;
    text-align: center;
  }

  .route-flow b {
    display: none;
  }

  .upcoming-note {
    padding-left: 16px;
  }

  .docs-next {
    display: grid;
  }

  .docs-next .button {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .doc-section :deep(.image-expand) {
    opacity: 1;
    transform: none;
    transition: none;
  }
}
</style>
