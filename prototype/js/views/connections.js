// View 4: Minimalist Live Connections + Right-hand Inspector
import { getActiveExit } from "../core/store.js";
import { esc, renderActionBadge } from "../core/utils.js";

export function renderConnectionsView(state) {
  const activeExit = getActiveExit();
  const q = state.connSearch.trim().toLowerCase();

  const filtered = state.connections.filter((c) => {
    if (state.connFilterAction !== "ALL" && c.routeAction !== state.connFilterAction) {
      return false;
    }
    if (!q) return true;
    return (
      c.process.toLowerCase().includes(q) ||
      c.host.toLowerCase().includes(q) ||
      c.dstIp.toLowerCase().includes(q)
    );
  });

  const selected =
    filtered.find((c) => c.id === state.selectedConnId) ||
    state.connections.find((c) => c.id === state.selectedConnId) ||
    filtered[0] ||
    null;

  const filterTabs = [
    { id: "ALL", label: `全部 (${state.connections.length})` },
    { id: "EXIT", label: "出口" },
    { id: "DIRECT", label: "直连" },
    { id: "FALLBACK_PROXY", label: "本地代理" },
    { id: "REJECT", label: "拦截" }
  ];

  const formatRate = (kb) => {
    if (kb <= 0) return "—";
    if (kb >= 1000) return `${(kb / 1024).toFixed(1)} MB/s`;
    return `${Math.round(kb)} KB/s`;
  };

  const rowsHtml = filtered
    .map((c) => {
      const isSel = selected && c.id === selected.id;
      return `
        <tr class="${isSel ? "is-selected" : ""}" data-select-conn="${esc(c.id)}" style="cursor:pointer;" data-testid="conn-row-${esc(c.id)}">
          <td class="font-mono-ui" style="width:136px;color:var(--text-primary);font-weight:500;">${esc(c.process)}</td>
          <td class="font-mono-ui" title="${esc(c.host)}:${c.port}">${esc(c.host)}<span style="color:var(--text-subtle);">:${c.port}</span></td>
          <td style="width:150px;" title="${esc(c.matchedRule)}"><span class="font-mono-ui" style="font-size:11.5px;color:var(--text-muted);">${esc(c.matchedRule)}</span></td>
          <td style="width:142px;">${renderActionBadge(c.routeAction, activeExit.label)}</td>
          <td class="font-mono-ui" style="width:86px;text-align:right;color:var(--accent-text);">${formatRate(c.downKBps)}</td>
          <td class="font-mono-ui" style="width:74px;text-align:right;color:var(--text-subtle);">${esc(c.totalBytes)}</td>
        </tr>
      `;
    })
    .join("");

  const inspectorHtml = selected
    ? `
      <div class="inspector-section" style="background:var(--bg-code);">
        <div class="font-mono-ui truncate selectable-text" style="font-size:14px;font-weight:600;color:var(--text-primary);">
          ${esc(selected.host)}:${selected.port}
        </div>
        <div style="display:flex;align-items:center;gap:6px;margin-top:6px;">
          <span class="tag-sharp">${esc(selected.network)}</span>
          ${renderActionBadge(selected.routeAction, activeExit.label)}
        </div>
      </div>

      <div class="inspector-section">
        <div class="inspector-kv-list" style="margin-top:0;">
          <div class="inspector-kv-row"><span>进程</span><span>${esc(selected.process)}</span></div>
          <div class="inspector-kv-row"><span>目标 IP</span><span>${esc(selected.dstIp)}:${selected.port}</span></div>
          <div class="inspector-kv-row"><span>命中规则</span><span>${esc(selected.matchedRule)}</span></div>
          <div class="inspector-kv-row"><span>延迟</span><span>${selected.rttMs ? `${selected.rttMs} ms` : "—"}</span></div>
          <div class="inspector-kv-row"><span>累计流量</span><span>${esc(selected.totalBytes)}</span></div>
          <div class="inspector-kv-row"><span>时间</span><span>${esc(selected.startedAt)}</span></div>
        </div>
      </div>

      <div class="inspector-section">
        <div class="eyebrow-mono" style="margin-bottom:8px;">域名规则</div>
        <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;">
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:EXIT">↗ 走出口</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:DIRECT">→ 直连</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:FALLBACK_PROXY">⇄ 本地代理</button>
          <button type="button" class="btn-danger btn-sm" data-quick-rule="${esc(selected.id)}:domain:REJECT">✕ 拦截</button>
        </div>

        <div class="eyebrow-mono" style="margin:12px 0 8px;">进程规则 (${esc(selected.process)})</div>
        <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;">
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:process:EXIT">↗ 走出口</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:process:DIRECT">→ 直连</button>
        </div>
      </div>
    `
    : `<div class="empty-box">点击连接查看详情</div>`;

  return `
    <div class="view-topbar">
      <h1 class="view-title">实时连接</h1>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="toggle-conn-pause">${state.connPaused ? "▶ 恢复" : "⏸ 暂停"}</button>
      </div>
    </div>

    <div class="section-bar">
      <div class="seg-tabs" role="tablist">
        ${filterTabs
          .map(
            (t) =>
              `<button type="button" class="seg-tab ${state.connFilterAction === t.id ? "is-active" : ""}" data-conn-filter="${t.id}">${esc(t.label)}</button>`
          )
          .join("")}
      </div>
      <input
        id="conn-search-input"
        type="search"
        class="input-sharp"
        style="width:220px;"
        placeholder="过滤进程 / 域名 / IP…"
        value="${esc(state.connSearch)}"
      />
    </div>

    <div class="split-workbench">
      <div class="view-scroll">
        ${
          filtered.length
            ? `<table class="data-table" aria-label="实时连接">
                <thead>
                  <tr>
                    <th style="width:136px;">进程</th>
                    <th>目标地址</th>
                    <th style="width:150px;">规则</th>
                    <th style="width:142px;">走向</th>
                    <th style="width:86px;text-align:right;">速率</th>
                    <th style="width:74px;text-align:right;">流量</th>
                  </tr>
                </thead>
                <tbody>${rowsHtml}</tbody>
              </table>`
            : `<div class="empty-box">
                <div>无匹配连接</div>
                <button type="button" class="btn-hairline" data-action="reset-conn-filter">清空筛选</button>
              </div>`
        }
      </div>

      <aside class="inspector-pane" aria-label="连接详情">
        ${inspectorHtml}
      </aside>
    </div>
  `;
}
