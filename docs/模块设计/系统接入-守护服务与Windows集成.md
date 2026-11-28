> - **模块定位**：系统接入层 — 提供带本机鉴权的守护进程控制网关 API、Windows 系统服务注册管理、系统托盘与本机运行中进程枚举
> - **对应代码**：`internal/daemon/` · `internal/syswin/` · `cmd/singtail-service/`
> - **所属里程碑**：[M2](../roadmap.md#m2核心业务层多组织与内核编排-系统接入层) · [M5](../roadmap.md#2-里程碑总览)
> - **上游架构**：[架构设计.md](../架构设计.md#2-架构分层表)
> - **当前状态**：已完成
> - **最近更新**：2026-11-28

# 模块设计：守护服务网关与 Windows 系统集成

## 1. 职责与边界

- **负责的事**：
  1. 本地控制网关（`internal/daemon`）：在 `127.0.0.1` 提供统一 JSON 控制接口（查询全量仪表盘快照、切换 TUN / 模式 / 出口、查询各粒度流量统计、增删改排导入规则、更新规则集、查询实时连接、挂载新组织、导出脱敏配置），并可托管静态前端资源供开发验收。
  2. 本机鉴权保护：支持本地随机密钥头 `X-SingTail-Token` 校验与严格同源检查，防止外部恶意网页跨站请求本机服务。
  3. Windows 平台能力（`internal/syswin`，通过 Go 构建标签 `//go:build windows` 与非 Windows 兜底分离）：Windows 系统服务注册/启动/卸载、枚举当前正在运行的 `.exe` 进程名与路径、纯 Go Win32 系统托盘通知图标（零 CGO）。
- **不负责的事**：不包含分流规则计算或 SQLite SQL 逻辑，全部委托给下层模块。

## 2. 结构与数据流

- `internal/daemon/server.go` & `traffic_tracker.go`：HTTP 路由注册、鉴权中间件、1 秒后台遥测采样与出口专属增量分离。
- `internal/syswin/process_windows.go` & `process_stub.go`：Windows 下调用 `CreateToolhelp32Snapshot`（纯 Go `golang.org/x/sys/windows`）枚举活动进程；非 Windows 下返回标准快照供跨平台测试与预览。
- `internal/syswin/service_common.go`、`service_windows.go`、`service_darwin.go` & `service_stub.go`：基于 `golang.org/x/sys/windows/svc` 与 read-only SCM API 实现后台服务状态探测、安装/启动/自启策略同步、以及按需 UAC 提权执行。
- `cmd/singtail-service/main.go`：守护服务可执行入口（支持 `-install`、`-start`、`-uninstall`、便携目录数据绑定与后台服务宿主模式）。

## 3. 关键决策

- **平台相关代码严格通过 `_windows.go` / `_stub.go` 隔离**：既保证在 Windows 真机调用原生无 CGO 的系统接口，又保证在 `cicd mac` 上既能原生跑通所有集成测试，又能用 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` 零报错编译出真实的 Windows 服务 `.exe`。

## 4. 当前实现

- `internal/daemon/server.go` & `traffic_tracker.go`：默认零模拟数据生产模式启动，提供组织授权、凭据导入、组织移除、浏览器唤起与 `privileged` 状态上报；每 1 秒采样的流量统计仅分离记入真正经过出口节点的字节增量，排除直连/本地代理/拦截干扰并支持按需同步系统服务开机自启配置。
- `internal/syswin/` & `cmd/singtail-desktop/`：桌面端启动若检测到本机 4189 端口无响应且未安装/未运行系统服务，自动发起一次 UAC 授权（GUI 无闪黑窗）完成 `SingTailService` 安装（便携目录自动绑定同级 `data/`）并拉起后台服务，后续开机自启后普通双击桌面图标零弹窗直连；若用户拒绝提权，仍可进入普通模式并支持在横幅按钮或点击 TUN 开关时随时一键授权热切换至后台服务。
- `cmd/singtail-service/main.go`：支持跨平台默认数据目录解析、`-listen`、`-data-dir`、`-web-dir`、`-install`、`-start`、`-uninstall` 及后台服务宿主模式。
- `internal/daemon/server_test.go` & `internal/syswin/syswin_test.go`：全量单测与竞态测试全部通过。

## 5. 验证方式

在 `cicd mac` 执行：
```bash
go test -v -race ./internal/daemon/... ./internal/syswin/...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/singtail-service
```
验收断言：
1. 守护服务所有 REST 接口端到端读写响应码与 JSON 结构符合 `internal/contract` 契约。
2. Windows 目标交叉编译零警告产出 `singtail-service.exe`。

## 6. 待扩展项

- 支持通过 Windows 命名管道（Named Pipe）替代回环端口进一步收紧ACL权限。

## 7. 改动历史

| 日期 | 说明 |
|---|---|
| 2026-11-27 | 初版模块设计定稿 |
| 2026-11-27 | 开关在出口检查完成后才确认开启，失败时保留关闭状态。 |
| 2026-11-28 | 桌面直接托管网络时同样静默启动内核。 |
| 2026-11-28 | 修复直连模式误覆盖出口公网 IP 的问题，并增加桌面端对旧版后台服务的识别提示。 |
| 2026-11-28 | 流量采样改为 1 秒分离出口专属流量、桌面启动/开 TUN 支持按需 UAC 自动安装并拉起 `SingTailService` 且统一便携数据目录、同步设置页开机自启策略。 |
