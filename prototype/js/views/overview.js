// View 1: Overview, Multi-Granularity Traffic Analytics, Org/Node Distribution & Exit Matrix
import { getActiveExit } from "../core/store.js";
import { esc, renderLatencyTag } from "../core/utils.js";

function formatGB(gb) {
  if (gb >= 1024) {
    return `${(gb / 1024).toFixed(2)} TB`;
  }
  return `${gb.toFixed(1)} GB`;
}

export function renderOverviewView(state) {
  const activeExit = getActiveExit();
  const downNow = state.tunEnabled ? state.downHistory[state.downHistory.length - 1] : 0;
  const upNow = state.tunEnabled ? state.upHistory[state.upHistory.length - 1] : 0;

  const periodCfg = state.trafficPeriods[state.trafficPeriod] || state.trafficPeriods.live;
  const totalPeriodGB = periodCfg.totalDownGB + periodCfg.totalUpGB;

  // Compute organization & per-node traffic distribution for selected period
  const nodeStats = state.exitNodes
    .filter((n) => (periodCfg.nodeUsageGB[n.id] ?? 0) > 0 || n.healthy)
    .map((n) => {
      const gb = periodCfg.nodeUsageGB[n.id] ?? 0;
      const pct = totalPeriodGB > 0 ? (gb / totalPeriodGB) * 100 : 0;
      return { ...n, gb, pct };
    })
    .sort((a, b) => b.gb - a.gb);

  const orgStats = state.orgs
    .map((org, idx) => {
      const gb = state.exitNodes
        .filter((n) => n.orgId === org.id)
        .reduce((sum, n) => sum + (periodCfg.nodeUsageGB[n.id] ?? 0), 0);
      const pct = totalPeriodGB > 0 ? (gb / totalPeriodGB) * 100 : 0;
      return {
        ...org,
        gb,
        pct,
        color: idx === 0 ? "var(--status-cyan)" : "var(--accent-fill)"
      };
    })
    .sort((a, b) => b.gb - a.gb);

  const filteredNodes =
    state.selectedOrgFilter === "all"
      ? state.exitNodes
      : state.exitNodes.filter((n) => n.orgId === state.selectedOrgFilter);

  const bannerHtml = state.bannerNotice
    ? `<div class="inline-banner is-${esc(state.bannerNotice.kind)}" role="status">
        <span>${esc(state.bannerNotice.text)}</span>
        <button type="button" class="btn-hairline btn-sm" data-action="dismiss-banner">关闭</button>
      </div>`
    : "";

  const periodTabs = [
    { id: "live", label: "实时" },
    { id: "day", label: "今日" },
    { id: "week", label: "近 7 天" },
    { id: "month30", label: "近 30 天" },
    { id: "monthly", label: "按月" }
  ];

  const orgTabsHtml = [
    `<button type="button" class="seg-tab ${state.selectedOrgFilter === "all" ? "is-active" : ""}" data-org-filter="all">全部 (${state.exitNodes.length})</button>`,
    ...state.orgs.map((org) => {
      const count = state.exitNodes.filter((n) => n.orgId === org.id).length;
      return `<button type="button" class="seg-tab ${state.selectedOrgFilter === org.id ? "is-active" : ""}" data-org-filter="${esc(org.id)}">${esc(org.name)} (${count})</button>`;
    })
  ].join("");

  const cardsHtml = filteredNodes
    .map((node) => {
      const isSelected = node.id === state.activeExitId;
      const isVerifying = node.id === state.verifyingExitId;
      return `
        <button
          type="button"
          class="exit-card ${isSelected ? "is-selected" : ""}"
          data-switch-exit="${esc(node.id)}"
          data-testid="exit-card-${esc(node.id)}"
        >
          <div class="exit-card-head">
            <div class="exit-node-title">
              <span class="status-dot ${isSelected ? "is-on" : node.healthy ? "is-green" : "is-amber"}"></span>
              <span class="truncate">${esc(node.label)}</span>
            </div>
            <div style="display:flex;align-items:center;gap:4px;flex-shrink:0;">
              ${
                isVerifying
                  ? `<span class="tag-sharp tag-amber">核验中</span>`
                  : isSelected
                  ? `<span class="tag-sharp tag-lime">当前</span>`
                  : ""
              }
              ${renderLatencyTag(node.latencyMs, node.healthy)}
            </div>
          </div>

          <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;padding-top:8px;border-top:1px solid var(--border-hairline);font-family:var(--font-mono);font-size:11.5px;color:var(--text-muted);">
            <span class="truncate">${esc(node.orgName)} · <strong style="color:var(--text-primary);font-weight:500;">${esc(node.publicIp)}</strong></span>
            <span style="color:${node.relayMode === "中继" ? "var(--status-amber)" : "var(--status-green)"};flex-shrink:0;">${esc(node.relayMode)}</span>
          </div>
        </button>
      `;
    })
    .join("");

  const breakdownRowsHtml =
    state.breakdownTab === "org"
      ? orgStats
          .map(
            (o) => `
            <div class="dist-item">
              <div class="dist-item-top">
                <span class="truncate" style="font-weight:500;color:var(--text-primary);">${esc(o.name)}</span>
                <span class="font-mono-ui" style="color:var(--text-body);">${formatGB(o.gb)} <span style="color:var(--text-subtle);">(${o.pct.toFixed(1)}%)</span></span>
              </div>
              <div class="dist-bar-track">
                <div class="dist-bar-fill" style="width:${Math.max(2, o.pct)}%;background:${o.color};"></div>
              </div>
            </div>
          `
          )
          .join("")
      : nodeStats
          .map(
            (n) => `
            <div class="dist-item">
              <div class="dist-item-top">
                <div style="display:flex;align-items:center;gap:6px;min-width:0;">
                  <span class="truncate" style="color:var(--text-primary);">${esc(n.label)}</span>
                  <span class="tag-sharp" style="height:17px;font-size:10px;">${esc(n.orgName)}</span>
                </div>
                <span class="font-mono-ui" style="flex-shrink:0;color:var(--text-body);">${formatGB(n.gb)} <span style="color:var(--text-subtle);">(${n.pct.toFixed(1)}%)</span></span>
              </div>
              <div class="dist-bar-track">
                <div class="dist-bar-fill" style="width:${n.gb > 0 ? Math.max(2, n.pct) : 0}%;"></div>
              </div>
            </div>
          `
          )
          .join("");

  return `
    <div class="view-topbar">
      <h1 class="view-title">总览与出口</h1>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="ping-nodes" data-testid="btn-ping-nodes">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9M13.5 2.5v3h-3"/></svg>
          <span>测速</span>
        </button>
        <button type="button" class="btn-lime" data-open-modal="add-org" data-testid="btn-add-org">+ 挂载组织</button>
      </div>
    </div>

    <div class="view-scroll">
      ${bannerHtml}

      <!-- Clean 3-Cell Key Telemetry -->
      <section class="bento-strip" aria-label="核心状态">
        <div class="bento-cell is-striped">
          <div class="eyebrow-mono">当前出口</div>
          <div class="metric-value truncate" style="font-size:17px;font-family:var(--font-sans);">${esc(activeExit.label)}</div>
          <div class="metric-sub">
            <span class="font-mono-ui truncate">${esc(activeExit.orgName)}</span>
            <span class="tag-sharp tag-lime-outline">${activeExit.latencyMs} ms</span>
          </div>
        </div>

        <div class="bento-cell">
          <div class="eyebrow-mono">出口公网 IP</div>
          <div class="metric-value selectable-text" id="metric-public-ip">${esc(activeExit.publicIp)}</div>
          <div class="metric-sub">
            <span class="truncate">${esc(activeExit.region)}</span>
            <span class="tag-sharp tag-green">已连通</span>
          </div>
        </div>

        <div class="bento-cell">
          <div class="eyebrow-mono">实时速率</div>
          <div class="metric-value">
            <span id="live-down-val" style="color:var(--accent-text);">↓ ${downNow.toFixed(1)}</span>
            <span style="font-size:12.5px;color:var(--text-subtle);font-weight:500;">MB/s</span>
          </div>
          <div class="metric-sub font-mono-ui">
            <span id="live-up-val" style="color:var(--status-cyan);">↑ ${upNow.toFixed(1)} MB/s</span>
            <span>连接 ${state.connections.length}</span>
          </div>
        </div>
      </section>

      <!-- Multi-Granularity Traffic Trend + Org/Node Distribution -->
      <section class="analytics-split" aria-label="流量统计与分布">
        <!-- Left: Time Granularity + Total Usage + Trend Canvas -->
        <div class="analytics-pane">
          <div class="analytics-head">
            <div class="seg-tabs" role="tablist" aria-label="时间粒度">
              ${periodTabs
                .map(
                  (t) =>
                    `<button type="button" class="seg-tab ${state.trafficPeriod === t.id ? "is-active" : ""}" data-traffic-period="${t.id}" data-testid="period-${t.id}">${esc(t.label)}</button>`
                )
                .join("")}
            </div>
            <div class="font-mono-ui" style="font-size:12px;display:flex;align-items:center;gap:12px;">
              <span>总计 <strong style="color:var(--text-primary);font-size:14px;">${formatGB(totalPeriodGB)}</strong></span>
              <span style="color:var(--accent-text);">↓ ${formatGB(periodCfg.totalDownGB)}</span>
              <span style="color:var(--status-cyan);">↑ ${formatGB(periodCfg.totalUpGB)}</span>
            </div>
          </div>

          <div class="sparkline-canvas-wrap">
            <canvas id="sparkline-canvas" class="sparkline-canvas"></canvas>
          </div>
        </div>

        <!-- Right: Traffic Distribution across Organizations & Exit Nodes -->
        <div class="analytics-pane">
          <div class="analytics-head">
            <span class="eyebrow-mono">流量分布 (${esc(periodCfg.label)})</span>
            <div class="seg-tabs" role="tablist" aria-label="分布维度">
              <button type="button" class="seg-tab ${state.breakdownTab === "node" ? "is-active" : ""}" data-breakdown-tab="node" data-testid="breakdown-node">按节点</button>
              <button type="button" class="seg-tab ${state.breakdownTab === "org" ? "is-active" : ""}" data-breakdown-tab="org" data-testid="breakdown-org">按组织</button>
            </div>
          </div>

          <!-- Stacked Organization Share Bar -->
          <div style="display:flex;flex-direction:column;gap:6px;">
            <div class="org-stacked-bar">
              ${orgStats
                .map(
                  (o) => `<div style="width:${o.pct}%;background:${o.color};" title="${esc(o.name)}: ${formatGB(o.gb)}"></div>`
                )
                .join("")}
            </div>
            <div class="org-legend-row">
              ${orgStats
                .map(
                  (o) => `
                  <span class="truncate" style="display:inline-flex;align-items:center;gap:5px;">
                    <i style="display:inline-block;width:7px;height:7px;background:${o.color};"></i>
                    <span style="color:var(--text-muted);">${esc(o.name)}</span>
                    <strong style="color:var(--text-primary);font-weight:600;">${o.pct.toFixed(0)}%</strong>
                  </span>
                `
                )
                .join("")}
            </div>
          </div>

          <!-- Ranked Breakdown List -->
          <div class="dist-list">
            ${breakdownRowsHtml}
          </div>
        </div>
      </section>

      <!-- Exit Nodes Filter & Cards -->
      <div class="section-bar">
        <div class="seg-tabs" role="tablist" aria-label="组织筛选">
          ${orgTabsHtml}
        </div>
      </div>

      <section class="exit-grid" aria-label="出口节点">
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
  const labelColor = isLight ? "#625e56" : "#8a8a95";
  const downStroke = isLight ? "#2d5a00" : "#ceff00";
  const downBar = isLight ? "#4d8a00" : "#ceff00";
  const downFill = isLight ? "rgba(45,90,0,0.12)" : "rgba(206,255,0,0.14)";
  const upStroke = isLight ? "#0369a1" : "#38bdf8";

  // Horizontal grid lines
  ctx.strokeStyle = gridColor;
  ctx.lineWidth = 1;
  const bottomLabelSpace = state.trafficPeriod === "live" ? 8 : 22;
  const chartH = h - bottomLabelSpace;

  for (let i = 1; i <= 3; i++) {
    const y = Math.round((chartH / 4) * i) + 0.5;
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(w, y);
    ctx.stroke();
  }

  if (state.trafficPeriod === "live") {
    for (let i = 1; i < 6; i++) {
      const x = Math.round((w / 6) * i) + 0.5;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, chartH);
      ctx.stroke();
    }

    const maxVal = Math.max(12, ...state.downHistory, ...state.upHistory);
    const padTop = 10;
    const usableH = chartH - padTop;

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
      ctx.lineWidth = 1.7;
      ctx.stroke();

      if (fill) {
        ctx.lineTo(w, chartH);
        ctx.lineTo(0, chartH);
        ctx.closePath();
        ctx.fillStyle = fill;
        ctx.fill();
      }
    };

    drawSeries(state.downHistory, downStroke, downFill);
    drawSeries(state.upHistory, upStroke, null);
    return;
  }

  // Historical Stacked Bar Chart (day / week / month30 / monthly)
  const cfg = state.trafficPeriods[state.trafficPeriod];
  if (!cfg || !cfg.labels) return;

  const n = cfg.labels.length;
  const totals = cfg.downSeries.map((d, i) => d + (cfg.upSeries[i] || 0));
  const maxTotal = Math.max(1, ...totals) * 1.15;
  const padX = 18;
  const slotW = (w - padX * 2) / n;
  const barW = Math.min(28, Math.max(10, Math.floor(slotW * 0.48)));

  ctx.font = '10.5px "Geist Mono", Consolas, monospace';
  ctx.textAlign = "center";

  for (let i = 0; i < n; i++) {
    const downV = cfg.downSeries[i] || 0;
    const upV = cfg.upSeries[i] || 0;
    const downH = Math.max(2, (downV / maxTotal) * (chartH - 12));
    const upH = Math.max(1, (upV / maxTotal) * (chartH - 12));

    const cx = padX + slotW * i + slotW / 2;
    const x = Math.round(cx - barW / 2);
    const yDown = Math.round(chartH - downH);
    const yUp = Math.round(yDown - upH - 1);

    ctx.fillStyle = downBar;
    ctx.fillRect(x, yDown, barW, downH);

    ctx.fillStyle = upStroke;
    ctx.fillRect(x, yUp, barW, upH);

    ctx.fillStyle = labelColor;
    ctx.fillText(cfg.labels[i], cx, h - 6);
  }
}
