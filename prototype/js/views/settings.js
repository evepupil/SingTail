// View 5: Multi-Tailnet Organizations, External Proxy Coexistence, Service & Backup
import { esc } from "../core/utils.js";

export function renderSettingsView(state) {
  const s = state.settings;

  const orgRowsHtml = state.orgs
    .map((org) => {
      const nodeCount = state.exitNodes.filter((n) => n.orgId === org.id).length;
      return `
        <div class="settings-row">
          <div style="min-width:0;">
            <div style="display:flex;align-items:center;gap:8px;">
              <span class="status-dot is-green"></span>
              <span style="font-weight:600;color:var(--text-primary);">${esc(org.name)}</span>
              <span class="tag-sharp tag-green">已授权 · ${nodeCount} 个出口</span>
            </div>
            <div class="font-mono-ui truncate" style="font-size:11.5px;color:var(--text-muted);margin-top:3px;">
              账号 ${esc(org.account)} · 本机节点 ${esc(org.localNodeName)} (${esc(org.localTailscaleIp)}) · 独立凭据目录 ${esc(org.stateDir)}
            </div>
          </div>
          <div style="display:flex;align-items:center;gap:6px;flex-shrink:0;">
            <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">同步于 ${esc(org.lastSync)}</span>
            <button type="button" class="btn-hairline btn-sm" data-action="sync-org">同步节点</button>
          </div>
        </div>
      `;
    })
    .join("");

  return `
    <div class="view-topbar">
      <div class="view-title-group">
        <h1 class="view-title">组织与系统设置</h1>
      </div>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-open-modal="export-config" data-testid="btn-export-config">⇩ 脱敏备份与导出</button>
        <button type="button" class="btn-lime" data-open-modal="add-org">+ 挂载新 Tailscale 组织</button>
      </div>
    </div>

    <div class="view-scroll">
      <div class="settings-stack">
        <!-- Block 1: Multi-Tailnet Organizations -->
        <section class="settings-block">
          <div class="settings-block-head">
            <span class="eyebrow-mono">已挂载的 Tailscale 组织 (${state.orgs.length})</span>
            <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">各组织独立保存 ts-state 凭据，切换出口免重登</span>
          </div>
          ${orgRowsHtml}
        </section>

        <!-- Block 2: External Proxy Coexistence & Fallback -->
        <section class="settings-block">
          <div class="settings-block-head">
            <span class="eyebrow-mono">外部代理协同与受限站点兜底</span>
            <span class="tag-sharp tag-cyan">云厂商 IP 受限应对</span>
          </div>

          <div class="settings-row">
            <div>
              <div style="font-weight:500;color:var(--text-primary);">一键让路切换至 Clash Verge</div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:2px;">先干净卸载虚拟网卡再拉起外部客户端，杜绝双端抢路由表断网</div>
            </div>
            <button type="button" class="btn-hairline" data-action="toggle-clash" data-testid="btn-settings-clash">
              ${state.clashHandover ? "⇄ 接管回 SingTail" : "⇄ 让路并启动 Clash Verge"}
            </button>
          </div>

          <div class="settings-row">
            <div>
              <div style="font-weight:500;color:var(--text-primary);">启用备用外部代理出口（供特定 AI / 流媒体规则调用）</div>
              <div class="font-mono-ui" style="font-size:11.5px;color:var(--text-muted);margin-top:2px;">
                监听地址：${esc(s.fallbackProxyAddr)} · 规则走向选「兜底代理」即转发至此端口
              </div>
            </div>
            <div style="display:flex;align-items:center;gap:8px;">
              <input
                type="text"
                class="input-sharp font-mono-ui"
                style="width:146px;"
                value="${esc(s.fallbackProxyAddr)}"
                data-setting-input="fallbackProxyAddr"
                aria-label="兜底外部代理地址"
              />
              <button
                type="button"
                class="sharp-switch"
                role="switch"
                aria-label="启用兜底外部代理"
                aria-checked="${s.fallbackProxyEnabled}"
                data-setting-toggle="fallbackProxyEnabled"
              >
                <span class="sharp-switch-thumb"></span>
              </button>
            </div>
          </div>
        </section>

        <!-- Block 3: Windows Service & Network Tuning -->
        <section class="settings-block">
          <div class="settings-block-head">
            <span class="eyebrow-mono">Windows 后台服务与底层网络调优</span>
            <span class="tag-sharp tag-green">不触碰 Windows 系统代理注册表</span>
          </div>

          <div class="settings-row">
            <div>
              <div style="font-weight:500;color:var(--text-primary);">系统服务开机静默自启与崩溃自愈</div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:2px;">以 SYSTEM 身份后台驻留，开机免弹 UAC 框，关闭界面不断网</div>
            </div>
            <button
              type="button"
              class="sharp-switch"
              role="switch"
              aria-label="开机静默自启"
              aria-checked="${s.autoStartService}"
              data-setting-toggle="autoStartService"
            >
              <span class="sharp-switch-thumb"></span>
            </button>
          </div>

          <div class="settings-row">
            <div>
              <div style="font-weight:500;color:var(--text-primary);">切换出口后自动实测公网 IP 并在异常时回滚</div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:2px;">经本地混合端口向双探测源核对出口 IP，超时立即恢复上一可用出口</div>
            </div>
            <button
              type="button"
              class="sharp-switch"
              role="switch"
              aria-label="自动核验公网IP"
              aria-checked="${s.autoVerifyIp}"
              data-setting-toggle="autoVerifyIp"
            >
              <span class="sharp-switch-thumb"></span>
            </button>
          </div>

          <div class="settings-row">
            <div>
              <div style="font-weight:500;color:var(--text-primary);">纯 IPv4 解析与虚拟网卡策略</div>
              <div style="font-size:12px;color:var(--text-muted);margin-top:2px;">无 IPv6 默认路由环境下跳过 AAAA 超时等待</div>
            </div>
            <button
              type="button"
              class="sharp-switch"
              role="switch"
              aria-label="纯 IPv4 策略"
              aria-checked="${s.ipv4Only}"
              data-setting-toggle="ipv4Only"
            >
              <span class="sharp-switch-thumb"></span>
            </button>
          </div>

          <div class="settings-row font-mono-ui" style="font-size:12px;">
            <span style="color:var(--text-muted);">TUN 网卡 MTU / 原生内网避让段 / 本地混合代理端口</span>
            <div style="display:flex;align-items:center;gap:8px;">
              <span class="tag-sharp">MTU ${s.mtu}</span>
              <span class="tag-sharp">排除 ${esc(s.bypassTailnetCidr)}</span>
              <span class="tag-sharp">127.0.0.1:${s.localMixedPort}</span>
            </div>
          </div>
        </section>
      </div>
    </div>
  `;
}
