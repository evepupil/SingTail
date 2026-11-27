// View 5: Clean Settings (Multi-Tailnet Orgs, Local Proxy Forwarding, Startup)
import { esc } from "../core/utils.js";

export function renderSettingsView(state) {
  const s = state.settings;

  const orgRowsHtml = state.orgs
    .map((org) => {
      const nodeCount = state.exitNodes.filter((n) => n.orgId === org.id).length;
      return `
        <div class="settings-row">
          <div style="display:flex;align-items:center;gap:10px;min-width:0;">
            <span class="status-dot is-green"></span>
            <span style="font-weight:600;color:var(--text-primary);">${esc(org.name)}</span>
            <span class="font-mono-ui truncate" style="font-size:12px;color:var(--text-muted);">${esc(org.account)}</span>
            <span class="tag-sharp">${nodeCount} 节点</span>
          </div>
          <div style="display:flex;align-items:center;gap:8px;flex-shrink:0;">
            <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">${esc(org.lastSync)}</span>
            <button type="button" class="btn-hairline btn-sm" data-action="sync-org">同步</button>
          </div>
        </div>
      `;
    })
    .join("");

  return `
    <div class="view-topbar">
      <h1 class="view-title">设置</h1>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-open-modal="export-config" data-testid="btn-export-config">⇩ 导出配置</button>
        <button type="button" class="btn-lime" data-open-modal="add-org">+ 挂载组织</button>
      </div>
    </div>

    <div class="view-scroll">
      <div class="settings-stack">
        <section class="settings-block">
          <div class="settings-block-head">
            <span class="eyebrow-mono">Tailscale 组织 (${state.orgs.length})</span>
          </div>
          ${orgRowsHtml}
        </section>

        <section class="settings-block">
          <div class="settings-block-head">
            <span class="eyebrow-mono">常规与转发</span>
          </div>

          <div class="settings-row">
            <span style="font-weight:500;color:var(--text-primary);">本地代理转发地址（供「本地代理」规则调用）</span>
            <div style="display:flex;align-items:center;gap:8px;">
              <input
                type="text"
                class="input-sharp font-mono-ui"
                style="width:148px;"
                value="${esc(s.fallbackProxyAddr)}"
                data-setting-input="fallbackProxyAddr"
                aria-label="本地代理地址"
              />
              <button
                type="button"
                class="sharp-switch"
                role="switch"
                aria-label="启用本地代理转发"
                aria-checked="${s.fallbackProxyEnabled}"
                data-setting-toggle="fallbackProxyEnabled"
              >
                <span class="sharp-switch-thumb"></span>
              </button>
            </div>
          </div>

          <div class="settings-row">
            <span style="font-weight:500;color:var(--text-primary);">开机自启</span>
            <button
              type="button"
              class="sharp-switch"
              role="switch"
              aria-label="开机自启"
              aria-checked="${s.autoStartService}"
              data-setting-toggle="autoStartService"
            >
              <span class="sharp-switch-thumb"></span>
            </button>
          </div>

          <div class="settings-row">
            <span style="font-weight:500;color:var(--text-primary);">切换出口核验公网 IP</span>
            <button
              type="button"
              class="sharp-switch"
              role="switch"
              aria-label="切换出口核验公网 IP"
              aria-checked="${s.autoVerifyIp}"
              data-setting-toggle="autoVerifyIp"
            >
              <span class="sharp-switch-thumb"></span>
            </button>
          </div>
        </section>
      </div>
    </div>
  `;
}
