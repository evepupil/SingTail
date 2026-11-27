// View 1: Overview, Telemetry Strip, Live Throughput Canvas & Multi-Org Exit Node Matrix
import { getActiveExit } from "../core/store.js";
import { esc, renderLatencyTag } from "../core/utils.js";

export function renderOverviewView(state) {
  const activeExit = getActiveExit();
  const downNow = state.tunEnabled ? state.downHistory[state.downHistory.length - 1] : 0;
  const upNow = state.tunEnabled ? state.upHistory[state.upHistory.length - 1] : 0;

  const filteredNodes =
    state.selectedOrgFilter === "all"
      ? state.exitNodes
      : state.exitNodes.filter((n) => n.orgId === state.selectedOrgFilter);

  const bannerHtml = state.bannerNotice
    ? `<div class="inline-banner is-${esc(state.bannerNotice.kind)}" role="status">
        <span>${esc(state.bannerNotice.text)}</span>
        ${
          state.clashHandover
            ? `<button type="button" class="btn-hairline btn-sm" data-action="toggle-clash">接管回 SingTail</button>`
            : `<button type="button" class="btn-hairline btn-sm" data-action="dismiss-banner">知道了</button>`
        }
      </div>`
    : "";

  const orgTabsHtml = [
    `<button type="button" class="seg-tab ${state.selectedOrgFilter === "all" ? "is-active" : ""}" data-org-filter="all">全部组织 (${state.exitNodes.length})</button>`,
    ...state.orgs.map((org) => {
      const count = state.exitNodes.filter((n) => n.orgId === org.id).length;
      return `<button type="button" class="seg-tab ${state.selectedOrgFilter === org.id ? "is-active" : ""}" data-org-filter="${esc(org.id)}">${esc(org.name)} (${count})</button>`;
    })
  ].join("");

  const cardsHtml = filteredNodes
    .map((node) => {
      const isSelected = node.id === state.activeExitId;
      const isVerifying = node.id === state.verifyingExitId;
      const isRelay = node.relayMode.includes("DERP");
      return `
        <button
          type="button"
          class="exit-card ${isSelected ? "is-selected" : ""} ${!node.online ? "is-offline" : ""}"
          data-switch-exit="${esc(node.id)}"
          data-testid="exit-card-${esc(node.id)}"
        >
          <div class="exit-card-head">
            <div style="min-width:0;">
              <div class="exit-node-title">
                <span class="status-dot ${isSelected ? "is-on" : node.healthy ? "is-green" : "is-amber"}"></span>
                <span class="truncate">${esc(node.label)}</span>
              </div>
              <div class="font-mono-ui truncate" style="font-size:11.5px;color:var(--text-subtle);margin-top:2px;">
                ${esc(node.hostname)} · ${esc(node.orgName)}
              </div>
            </div>
            <div style="display:flex;align-items:center;gap:4px;flex-shrink:0;">
              ${
                isVerifying
                  ? `<span class="tag-sharp tag-amber">核验中…</span>`
                  : isSelected
                  ? `<span class="tag-sharp tag-lime">当前出口</span>`
                  : ""
              }
              ${renderLatencyTag(node.latencyMs, node.healthy)}
            </div>
          </div>

          <div class="exit-meta-grid">
            <div>
              <div class="exit-kv-label">公网出口 IP</div>
              <div class="truncate" style="color:var(--text-primary);">${esc(node.publicIp)}</div>
            </div>
            <div>
              <div class="exit-kv-label">Tailscale 地址</div>
              <div class="truncate">${esc(node.tailscaleIp)}</div>
            </div>
            <div>
              <div class="exit-kv-label">隧道链路</div>
              <div class="truncate" style="color:${isRelay ? "var(--status-amber)" : "var(--status-green)"};">${esc(node.relayMode)}</div>
            </div>
            <div>
              <div class="exit-kv-label">区域 / 线路</div>
              <div class="truncate">${esc(node.region)}</div>
            </div>
          </div>
        </button>
      `;
    })
    .join("");

  return `
    <div class="view-topbar">
      <div class="view-title-group">
        <h1 class="view-title">总览与出口</h1>
        <span class="tag-sharp ${state.tunEnabled ? "tag-green" : ""}">${state.tunEnabled ? "TUN 已接管" : "已停用"}</span>
      </div>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="ping-nodes" data-testid="btn-ping-nodes">↻ 探测延迟</button>
        <button type="button" class="btn-lime" data-open-modal="add-org" data-testid="btn-add-org">+ 挂载新组织</button>
      </div>
    </div>

    <div class="view-scroll">
      ${bannerHtml}

      <!-- 4-Cell Flush Architectural Bento Metrics -->
      <section class="bento-strip" aria-label="关键指标">
        <div class="bento-cell is-striped">
          <div class="eyebrow-mono">当前生效出口</div>
          <div class="metric-value truncate" style="font-size:17px;font-family:var(--font-sans);">${esc(activeExit.label)}</div>
          <div class="metric-sub">
            <span class="font-mono-ui truncate">${esc(activeExit.orgName)}</span>
            <span class="tag-sharp tag-lime-outline">${esc(activeExit.relayMode)}</span>
          </div>
        </div>

        <div class="bento-cell">
          <div class="eyebrow-mono">实测公网 IP</div>
          <div class="metric-value selectable-text" id="metric-public-ip">${esc(activeExit.publicIp)}</div>
          <div class="metric-sub">
            <span class="truncate">${esc(activeExit.region)}</span>
            <span class="tag-sharp tag-green">已核验</span>
          </div>
        </div>

        <div class="bento-cell">
          <div class="eyebrow-mono">实时吞吐速率</div>
          <div class="metric-value">
            <span id="live-down-val" style="color:var(--accent-text);">↓ ${downNow.toFixed(1)}</span>
            <span style="font-size:13px;color:var(--text-subtle);font-weight:500;">MB/s</span>
          </div>
          <div class="metric-sub font-mono-ui">
            <span id="live-up-val" style="color:var(--status-cyan);">↑ ${upNow.toFixed(1)} MB/s</span>
            <span>活跃 ${state.connections.length} 条</span>
          </div>
        </div>

        <div class="bento-cell">
          <div class="eyebrow-mono">分流与防污染</div>
          <div class="metric-value">${state.rules.filter((r) => r.enabled).length} 条规则</div>
          <div class="metric-sub font-mono-ui">
            <span>国内 223.5.5.5</span>
            <span class="tag-sharp tag-cyan">境外 DoH</span>
          </div>
        </div>
      </section>

      <!-- Dual-Series Throughput Sparkline -->
      <section class="sparkline-panel">
        <div class="sparkline-head">
          <div style="display:flex;align-items:center;gap:12px;">
            <span class="eyebrow-mono">吞吐量波形 (近 36 秒)</span>
            <span class="font-mono-ui" style="font-size:11px;color:var(--accent-text);">■ 下行流量</span>
            <span class="font-mono-ui" style="font-size:11px;color:var(--status-cyan);">■ 上行流量</span>
          </div>
          <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">MTU ${state.settings.mtu} · IPv4 Only · 排除 ${esc(state.settings.bypassTailnetCidr)}</span>
        </div>
        <div class="sparkline-canvas-wrap">
          <canvas id="sparkline-canvas" class="sparkline-canvas"></canvas>
        </div>
      </section>

      <!-- Organization Filter & Exit Nodes Matrix -->
      <div class="section-bar">
        <div class="seg-tabs" role="tablist" aria-label="按组织筛选">
          ${orgTabsHtml}
        </div>
        <span class="font-mono-ui" style="font-size:11.5px;color:var(--text-subtle);">点击任意节点立即热切并核验公网出口</span>
      </div>

      <section class="exit-grid" aria-label="组织出口节点列表">
        ${cardsHtml}
      </section>
    </div>
  `;
}

export function drawSparklineCanvas(state) {
  const canvas = document.getElementById("sparkline-canvas");
  if (!canvas) return;
  const rect = canvas.parentElement.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return;

  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.floor(rect.width * dpr);
  canvas.height = Math.floor(rect.height * dpr);
  const ctx = canvas.getContext("2d");
  ctx.scale(dpr, dpr);

  const w = rect.width;
  const h = rect.height;
  ctx.clearRect(0, 0, w, h);

  const isLight = state.theme === "light";
  const gridColor = isLight ? "rgba(27,26,23,0.08)" : "rgba(255,255,255,0.06)";
  const downStroke = isLight ? "#2d5a00" : "#ceff00";
  const downFill = isLight ? "rgba(45,90,0,0.12)" : "rgba(206,255,0,0.14)";
  const upStroke = isLight ? "#0369a1" : "#38bdf8";

  // Architectural horizontal + vertical reference grid
  ctx.strokeStyle = gridColor;
  ctx.lineWidth = 1;
  for (let i = 1; i <= 2; i++) {
    const y = Math.round((h / 3) * i) + 0.5;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(w, y);
    ctx.stroke();
  }
  for (let i = 1; i < 6; i++) {
    const x = Math.round((w / 6) * i) + 0.5;
    ctx.beginPath();
    ctx.moveTo(x, 0);
    ctx.lineTo(x, h);
    ctx.stroke();
  }

  const maxVal = Math.max(12, ...state.downHistory, ...state.upHistory);
  const padTop = 8;
  const padBot = 6;
  const usableH = h - padTop - padBot;

  const drawSeries = (arr, stroke, fill) => {
    if (arr.length < 2) return;
    const stepX = w / (arr.length - 1);
    ctx.beginPath();
    arr.forEach((v, idx) => {
      const x = idx * stepX;
      const y = padTop + usableH - (v / maxVal) * usableH;
      if (idx === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 1.6;
    ctx.stroke();

    if (fill) {
      ctx.lineTo(w, h);
      ctx.lineTo(0, h);
      ctx.closePath();
      ctx.fillStyle = fill;
      ctx.fill();
    }
  };

  drawSeries(state.downHistory, downStroke, downFill);
  drawSeries(state.upHistory, upStroke, null);
}
