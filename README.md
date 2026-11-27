# SingTail

面向自建 Tailscale 出口节点用户的 Windows / macOS 桌面客户端：把多组织接入、出口测速切换、国内外智能分流与流量统计整合成开箱即用的图形界面，内置 `sing-box 1.14.3` 内核与基础 `.srs` 规则包，告别命令行脚本与复杂网页控制台。

## 核心优势

1. **多组织出口自动发现与双模式接入**
   同时挂载个人 Tailnet 与公司/团队等多个 Tailscale 组织，支持通过官方网页扫码/点击授权登录（含自建 Headscale 控制地址），也支持直接导入本机已有的 `ts-state` 凭据目录；各组织登录状态分目录独立保存、互不覆盖，登录后自动汇总所有开启出口权限（Exit Node）的远端机器，显示直连打洞（P2P）或中继（DERP）状态与实时延迟，点一下即可跨组织切换。

2. **切换出口自动验 IP，不通自动回滚**
   每次切换出口节点时，后台自动通过本地混合代理端口核验实际公网出口 IP；如果节点超时或未生效，立即自动切回上一个可用出口并弹出横幅提醒，避免误切导致断网。

3. **国内外防污染精准分流**
   内置开箱即用的分流底盘：国内常见域名、国内 IP 段（含阿里云 `8.128.0.0/10` 修正段）和本机局域网直连不减速，境外域名自动经出口走加密 DNS 解析；支持直接点选电脑里正在运行的程序（如浏览器、游戏、IDE）一键加规则，也支持导入外部规则文本与 `.srs` 规则集订阅定时更新。

4. **与 Clash / Mihomo 无感共存**
   底层默认自动放行 `tailscaled.exe`、`verge-mihomo.exe`、`clash-verge.exe` 等本机常见代理客户端进程，防止虚拟网卡互相抓包套娃；同时支持填写一个本地备用代理端口（例如 `127.0.0.1:7897`），把特定域名单独分流给备用线路。

5. **多粒度历史流量与组织节点分布**
   基于轻量嵌入式 SQLite 账本，保留最近 48 小时分钟级明细并自动归并历史汇总，支持按「实时 / 今日 / 近 7 天 / 近 30 天 / 按月」查看上下行趋势曲线，以及各 Tailscale 组织、各出口节点的流量占比排行。

---

## 下载与使用

每次发布都会在 GitHub Releases 同时提供 Windows 与 macOS 共四份内置 `sing-box 1.14.3` 内核及 7 个基础 `.srs` 规则包的正式产物，按系统与习惯任选其一即可：

### Windows 方式一：标准安装包（推荐日常使用）

下载 `SingTail-windows-amd64-setup.exe` 并双击运行安装向导：
- 自动将程序、内置 `bin/sing-box.exe` 内核与 `rules/*.srs` 规则包安装至 `Program Files\SingTail`，并创建桌面与开始菜单快捷方式；
- 自动注册并启动 `SingTailService` 系统后台服务，开机免弹 UAC 框静默驻留，关闭窗口不断网；
- 需要卸载时，在系统「应用和功能」或开始菜单点击卸载即可自动停止并清理服务、内核与规则目录。

### Windows 方式二：绿色便携包（免安装解压即用）

下载 `SingTail-windows-amd64-portable.zip`，解压到任意目录即可使用：
- 压缩包内已包含 `singtail.exe`、`singtail-service.exe`、`bin/sing-box.exe` 与 `rules/*.srs`；
- 直接双击 `singtail.exe` 启动桌面客户端（未安装系统服务时，桌面端会自动拉起内置守护服务）；
- 如希望作为 Windows 后台服务开机静默自启，请右键以管理员身份运行目录内的 `install-service.bat`；
- 如需移除后台服务，右键以管理员身份运行 `uninstall-service.bat`。

### macOS 方式：DMG 镜像安装包（Apple Silicon / Intel）

根据芯片架构选择下载：
- Apple Silicon（M 系列芯片）：`SingTail-macos-arm64.dmg`
- Intel 芯片：`SingTail-macos-amd64.dmg`

安装与使用步骤：
- 双击打开 `.dmg` 镜像，将 `SingTail.app` 拖入右侧 `Applications` 快捷方式（`SingTail.app/Contents/Resources/` 已内置 `bin/sing-box` 与 `rules/*.srs`）；
- 如需注册或卸载 macOS 后台守护服务，可双击镜像内的 `install-service.command` / `uninstall-service.command`；
- 打开 `SingTail.app` 后，在主界面点击「挂载组织」即可选择「网页授权登录」（自动拉起浏览器完成 Tailscale / Headscale 授权并同步节点）或「导入已有凭据目录」（指定本机已有的 `ts-state` 目录路径）。

### 挂载 Tailscale 组织（真实 `tsnet` 登录）

1. **网页授权登录（默认）**：点击「+ 挂载 Tailscale 组织」，备注名与 Headscale 控制地址均可留空，点击「获取授权链接」后会自动打开浏览器授权页面，弹窗内每 1.5 秒自动同步授权状态，同意后自动拉取出口节点列表。
2. **导入已有凭据目录**：切换到「导入已有凭据目录」标签页，填入本机已有 `ts-state` 目录的绝对路径即可直接导入并同步节点。
3. **移除组织**：进入「设置」页面，在对应组织右侧点击「移除」即可卸载该组织凭据。

### 隐私与安全

- 所有 Tailscale 组织登录态与私钥（`ts-state-*`）仅保存在本机数据目录；
- 使用界面中的「导出脱敏配置」功能时，仅导出个人规则、规则集订阅与常规设置，自动剔除所有组织私钥与鉴权凭据，可放心分享备份。

---

## 本地开发与构建

### 环境要求

- Go 1.23+（支持 `CGO_ENABLED=0` 零依赖交叉编译 Windows 目标）
- Node.js 22+ 与 pnpm 9+

### 前端开发与检查

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend dev         # 启动本地前端开发预览
pnpm --dir frontend typecheck   # TypeScript 严格类型检查
pnpm --dir frontend test        # Vitest 单元测试
pnpm --dir frontend build       # 构建静态产物至 frontend/dist
```

### 一键远程门禁与双平台编译

在 macOS / Linux 开发或构建机上运行：

```bash
bash scripts/remote-gate.sh
```

该脚本会依次执行 `gofmt` 检查、前端严格检查/单测/构建、将 `frontend/dist` 同步到 `cmd/singtail-desktop/frontendDist`、`go vet ./...`、`go test -race ./...`，并使用 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` 交叉编译 `singtail-service.exe` 与 `singtail.exe`。

### 手动编译 Windows 二进制

```bash
pnpm --dir frontend build
rm -rf cmd/singtail-desktop/frontendDist && mkdir -p cmd/singtail-desktop/frontendDist
cp -R frontend/dist/. cmd/singtail-desktop/frontendDist/

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o singtail-service.exe ./cmd/singtail-service
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o singtail.exe ./cmd/singtail-desktop
```
