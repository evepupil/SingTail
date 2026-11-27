// View 4: Surge-style Live Activity Stream + Right-hand Deep Connection Inspector
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
      c.dstIp.toLowerCase().includes(q) ||
      c.matchedRule.toLowerCase().includes(q)
    );
  });

  const selected =
    filtered.find((c) => c.id === state.selectedConnId) ||
    state.connections.find((c) => c.id === state.selectedConnId) ||
    filtered[0] ||
    null;

  const filterTabs = [
    { id: "ALL", label: `全部 (${state.connections.length})` },
    { id: "EXIT", label: "走出口" },
    { id: "DIRECT", label: "直连" },
    { id: "FALLBACK_PROXY", label: "兜底代理" },
    { id: "REJECT", label: "已拦截" }
  ];

  const rowsHtml = filtered
    .map((c) => {
      const isSel = selected && c.id === selected.id;
      return `
        <tr class="${isSel ? "is-selected" : ""}" data-select-conn="${esc(c.id)}" style="cursor:pointer;" data-testid="conn-row-${esc(c.id)}">
          <td class="font-mono-ui" style="width:128px;color:var(--text-primary);font-weight:500;">${esc(c.process)}</td>
          <td class="font-mono-ui" title="${esc(c.host)}:${c.port}">${esc(c.host)}<span style="color:var(--text-subtle);">:${c.port}</span></td>
          <td style="width:168px;" title="${esc(c.matchedRule)}"><span class="font-mono-ui" style="font-size:11.5px;color:var(--text-muted);">${esc(c.matchedRule)}</span></td>
          <td style="width:138px;">${renderActionBadge(c.routeAction, activeExit.hostname)}</td>
          <td class="font-mono-ui" style="width:94px;text-align:right;color:var(--accent-text);">
            ${c.downKBps > 0 ? `${c.downKBps.toFixed(1)} KB/s` : "—"}
          </td>
          <td class="font-mono-ui" style="width:78px;text-align:right;color:var(--text-subtle);">${esc(c.totalBytes)}</td>
        </tr>
      `;
    })
    .join("");

  const inspectorHtml = selected
    ? `
      <div class="inspector-section" style="background:var(--bg-code);">
        <div class="eyebrow-mono">连接检视器 (Inspector)</div>
        <div class="font-mono-ui truncate selectable-text" style="font-size:14px;font-weight:600;color:var(--text-primary);margin-top:4px;">
          ${esc(selected.host)}:${selected.port}
        </div>
        <div style="display:flex;align-items:center;gap:6px;margin-top:6px;flex-wrap:wrap;">
          <span class="tag-sharp">${esc(selected.network)} · ${esc(selected.protocol)}</span>
          ${renderActionBadge(selected.routeAction, activeExit.hostname)}
        </div>
      </div>

      <div class="inspector-section">
        <div class="eyebrow-mono">链路与解析明细</div>
        <div class="inspector-kv-list">
          <div class="inspector-kv-row"><span>发起进程</span><span>${esc(selected.process)}</span></div>
          <div class="inspector-kv-row"><span>目标地址</span><span>${esc(selected.dstIp)}:${selected.port}</span></div>
          <div class="inspector-kv-row"><span>命中规则</span><span>${esc(selected.matchedRule)}</span></div>
          <div class="inspector-kv-row"><span>DNS 解析源</span><span>${esc(selected.dnsServer)}</span></div>
          <div class="inspector-kv-row"><span>DNS 耗时</span><span>${selected.dnsMs} ms</span></div>
          <div class="inspector-kv-row"><span>链路往返延迟</span><span>${selected.rttMs} ms</span></div>
          <div class="inspector-kv-row"><span>累计流量</span><span>${esc(selected.totalBytes)}</span></div>
          <div class="inspector-kv-row"><span>建立时间</span><span>${esc(selected.startedAt)}</span></div>
        </div>
      </div>

      <div class="inspector-section">
        <div class="eyebrow-mono" style="margin-bottom:8px;">一键写入分流规则（按域名）</div>
        <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;">
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:EXIT" data-testid="btn-quick-domain-exit">↗ 域名走出口</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:DIRECT" data-testid="btn-quick-domain-direct">→ 域名设直连</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:domain:FALLBACK_PROXY">⇄ 走兜底代理</button>
          <button type="button" class="btn-danger btn-sm" data-quick-rule="${esc(selected.id)}:domain:REJECT">✕ 拦截此域名</button>
        </div>

        <div class="eyebrow-mono" style="margin:12px 0 8px;">一键写入分流规则（按程序 ${esc(selected.process)}）</div>
        <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;">
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:process:EXIT">↗ 该程序走出口</button>
          <button type="button" class="btn-hairline btn-sm" data-quick-rule="${esc(selected.id)}:process:DIRECT" data-testid="btn-quick-proc-direct">→ 该程序直连</button>
        </div>
      </div>
    `
    : `<div class="empty-box">选择左侧任意连接查看解析与隧道详情</div>`;

  return `
    <div class="view-topbar">
      <div class="view-title-group">
        <h1 class="view-title">实时连接</h1>
        <span class="tag-sharp ${state.connPaused ? "tag-amber" : "tag-green"}">${state.connPaused ? "已暂停刷新" : "实时采集中"}</span>
      </div>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="toggle-conn-pause">${state.connPaused ? "▶ 恢复流" : "⏸ 暂停流"}</button>
      </div>
    </div>

    <div class="section-bar">
      <div class="seg-tabs" role="tablist" aria-label="按路由结果筛选">
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
        style="width:230px;"
        placeholder="过滤程序名 / 域名 / IP…"
        value="${esc(state.connSearch)}"
      />
    </div>

    <div class="split-workbench">
      <div class="view-scroll">
        ${
          filtered.length
            ? `<table class="data-table" aria-label="实时连接列表">
                <thead>
                  <tr>
                    <th style="width:128px;">进程名</th>
                    <th>目标域名 / 端口</th>
                    <th style="width:168px;">命中规则</th>
                    <th style="width:138px;">实际走向</th>
                    <th style="width:94px;text-align:right;">下行速率</th>
                    <th style="width:78px;text-align:right;">累计</th>
                  </tr>
                </thead>
                <tbody>${rowsHtml}</tbody>
              </table>`
            : `<div class="empty-box">
                <div>当前筛选条件下暂无活动连接</div>
                <button type="button" class="btn-hairline" data-action="reset-conn-filter">清空过滤条件</button>
              </div>`
        }
      </div>

      <aside class="inspector-pane" aria-label="连接详情检视器">
        ${inspectorHtml}
      </aside>
    </div>
  `;
}
