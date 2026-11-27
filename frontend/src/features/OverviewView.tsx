import { useState } from "react";
import { TrafficCanvas } from "../components/TrafficCanvas";
import {
  type BreakdownTab,
  type ExitNode,
  type Organization,
  type TrafficPeriodKey,
  type TrafficReport,
} from "../types/contract";
import {
  filterExitNodesByOrg,
  formatTrafficGB,
  getActiveExit,
  getBreakdownColor,
  getExitIpDisplay,
  getLatencyMeta,
  type ModalKind,
} from "../state/selectors";

interface OverviewViewProps {
  theme: "dark" | "light";
  tunEnabled: boolean;
  coreAvailable: boolean;
  activeExitId: string;
  verifyingExitId?: string;
  bannerNotice?: { kind: "verifying" | "rollback"; text: string } | null;
  orgs: readonly Organization[];
  exitNodes: readonly ExitNode[];
  connectionsCount: number;
  traffic: TrafficReport;
  trafficPeriod: TrafficPeriodKey;
  pinging: boolean;
  onChangePeriod: (period: TrafficPeriodKey) => void;
  onSwitchExit: (exitId: string) => void;
  onPingNodes: () => void;
  onDismissBanner: () => void;
  onOpenModal: (modal: Exclude<ModalKind, null>) => void;
}

const PERIOD_TABS: ReadonlyArray<{ id: TrafficPeriodKey; label: string }> = [
  { id: "live", label: "实时" },
  { id: "day", label: "今日" },
  { id: "week", label: "近 7 天" },
  { id: "month30", label: "近 30 天" },
  { id: "monthly", label: "按月" },
];

export function OverviewView({
  theme,
  tunEnabled,
  coreAvailable,
  activeExitId,
  verifyingExitId,
  bannerNotice,
  orgs,
  exitNodes,
  connectionsCount,
  traffic,
  trafficPeriod,
  pinging,
  onChangePeriod,
  onSwitchExit,
  onPingNodes,
  onDismissBanner,
  onOpenModal,
}: OverviewViewProps) {
  const [breakdownTab, setBreakdownTab] = useState<BreakdownTab>("node");
  const [selectedOrgFilter, setSelectedOrgFilter] = useState<string>("all");

  const activeExit = getActiveExit(exitNodes, activeExitId);
  const exitIpMeta = getExitIpDisplay(activeExit);
  const downNow = tunEnabled ? (traffic.liveDownHistory.at(-1) ?? 0) : 0;
  const upNow = tunEnabled ? (traffic.liveUpHistory.at(-1) ?? 0) : 0;
  const totalPeriodGB =
    traffic.totalGB > 0 ? traffic.totalGB : traffic.totalDownGB + traffic.totalUpGB;

  const orgStats = [...traffic.byOrg]
    .sort((a, b) => b.totalGB - a.totalGB)
    .map((item, idx) => ({
      ...item,
      color: getBreakdownColor(idx),
    }));

  const nodeStats = [...traffic.byNode].sort((a, b) => b.totalGB - a.totalGB);
  const effectiveOrgFilter =
    selectedOrgFilter === "all" || orgs.some((o) => o.id === selectedOrgFilter)
      ? selectedOrgFilter
      : "all";
  const filteredNodes = filterExitNodesByOrg(exitNodes, effectiveOrgFilter);

  return (
    <>
      <div className="view-topbar">
        <h1 className="view-title">总览与出口</h1>
        <div className="view-actions">
          <button
            type="button"
            className="btn-hairline"
            disabled={pinging}
            data-testid="btn-ping-nodes"
            onClick={onPingNodes}
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              aria-hidden="true"
            >
              <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9M13.5 2.5v3h-3" />
            </svg>
            <span>{pinging ? "测速中…" : "测速"}</span>
          </button>
          <button
            type="button"
            className="btn-lime"
            data-testid="btn-add-org"
            onClick={() => onOpenModal("add-org")}
          >
            + 挂载组织
          </button>
        </div>
      </div>

      <div className="view-scroll">
        {!coreAvailable && (
          <div
            className="inline-banner is-verifying"
            role="status"
            data-testid="banner-missing-core"
          >
            <span>未检测到 sing-box 内核，TUN 与分流暂不可用</span>
          </div>
        )}

        {bannerNotice && (
          <div className={`inline-banner is-${bannerNotice.kind}`} role="status">
            <span>{bannerNotice.text}</span>
            <button
              type="button"
              className="btn-hairline btn-sm"
              onClick={onDismissBanner}
            >
              关闭
            </button>
          </div>
        )}

        <section className="bento-strip" aria-label="核心状态">
          <div className="bento-cell is-striped">
            <div className="eyebrow-mono">当前出口</div>
            <div
              className="metric-value truncate"
              style={{ fontSize: 17, fontFamily: "var(--font-sans)" }}
            >
              {activeExit?.label ?? "未选择"}
            </div>
            <div className="metric-sub">
              <span className="font-mono-ui truncate">
                {activeExit?.orgName ?? "—"}
              </span>
              <span className="tag-sharp tag-lime-outline">
                {activeExit ? `${activeExit.latencyMs} ms` : "—"}
              </span>
            </div>
          </div>

          <div className="bento-cell">
            <div className="eyebrow-mono">出口公网 IP</div>
            <div className="metric-value selectable-text" id="metric-public-ip">
              {exitIpMeta.ipText}
            </div>
            <div className="metric-sub">
              <span className="truncate">{activeExit?.region || "—"}</span>
              <span className={exitIpMeta.badgeClass}>
                {exitIpMeta.badgeText}
              </span>
            </div>
          </div>

          <div className="bento-cell">
            <div className="eyebrow-mono">实时速率</div>
            <div className="metric-value">
              <span id="live-down-val" style={{ color: "var(--accent-text)" }}>
                ↓ {downNow.toFixed(1)}
              </span>{" "}
              <span
                style={{
                  fontSize: "12.5px",
                  color: "var(--text-subtle)",
                  fontWeight: 500,
                }}
              >
                MB/s
              </span>
            </div>
            <div className="metric-sub font-mono-ui">
              <span id="live-up-val" style={{ color: "var(--status-cyan)" }}>
                ↑ {upNow.toFixed(1)} MB/s
              </span>
              <span>连接 {connectionsCount}</span>
            </div>
          </div>
        </section>

        <section className="analytics-split" aria-label="流量统计与分布">
          <div className="analytics-pane">
            <div className="analytics-head">
              <div className="seg-tabs" role="tablist" aria-label="时间粒度">
                {PERIOD_TABS.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    className={`seg-tab ${trafficPeriod === t.id ? "is-active" : ""}`}
                    data-testid={`period-${t.id}`}
                    onClick={() => onChangePeriod(t.id)}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
              <div
                className="font-mono-ui"
                style={{
                  fontSize: 12,
                  display: "flex",
                  alignItems: "center",
                  gap: 12,
                }}
              >
                <span>
                  总计{" "}
                  <strong style={{ color: "var(--text-primary)", fontSize: 14 }}>
                    {formatTrafficGB(totalPeriodGB)}
                  </strong>
                </span>
                <span style={{ color: "var(--accent-text)" }}>
                  ↓ {formatTrafficGB(traffic.totalDownGB)}
                </span>
                <span style={{ color: "var(--status-cyan)" }}>
                  ↑ {formatTrafficGB(traffic.totalUpGB)}
                </span>
              </div>
            </div>

            <TrafficCanvas traffic={traffic} theme={theme} />
          </div>

          <div className="analytics-pane">
            <div className="analytics-head">
              <span className="eyebrow-mono">流量分布 ({traffic.label})</span>
              <div className="seg-tabs" role="tablist" aria-label="分布维度">
                <button
                  type="button"
                  className={`seg-tab ${breakdownTab === "node" ? "is-active" : ""}`}
                  data-testid="breakdown-node"
                  onClick={() => setBreakdownTab("node")}
                >
                  按节点
                </button>
                <button
                  type="button"
                  className={`seg-tab ${breakdownTab === "org" ? "is-active" : ""}`}
                  data-testid="breakdown-org"
                  onClick={() => setBreakdownTab("org")}
                >
                  按组织
                </button>
              </div>
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <div className="org-stacked-bar">
                {orgStats.map((o) => (
                  <div
                    key={o.id}
                    style={{ width: `${o.percent}%`, background: o.color }}
                    title={`${o.name}: ${formatTrafficGB(o.totalGB)}`}
                  />
                ))}
              </div>
              <div className="org-legend-row">
                {orgStats.map((o) => (
                  <span
                    key={o.id}
                    className="truncate"
                    style={{ display: "inline-flex", alignItems: "center", gap: 5 }}
                  >
                    <i
                      style={{
                        display: "inline-block",
                        width: 7,
                        height: 7,
                        background: o.color,
                      }}
                    />
                    <span style={{ color: "var(--text-muted)" }}>{o.name}</span>
                    <strong style={{ color: "var(--text-primary)", fontWeight: 600 }}>
                      {o.percent.toFixed(0)}%
                    </strong>
                  </span>
                ))}
              </div>
            </div>

            <div className="dist-list">
              {breakdownTab === "org"
                ? orgStats.map((o) => (
                    <div key={o.id} className="dist-item">
                      <div className="dist-item-top">
                        <span
                          className="truncate"
                          style={{ fontWeight: 500, color: "var(--text-primary)" }}
                        >
                          {o.name}
                        </span>
                        <span
                          className="font-mono-ui"
                          style={{ color: "var(--text-body)" }}
                        >
                          {formatTrafficGB(o.totalGB)}{" "}
                          <span style={{ color: "var(--text-subtle)" }}>
                            ({o.percent.toFixed(1)}%)
                          </span>
                        </span>
                      </div>
                      <div className="dist-bar-track">
                        <div
                          className="dist-bar-fill"
                          style={{
                            width: `${o.totalGB > 0 ? Math.max(2, o.percent) : 0}%`,
                            background: o.color,
                          }}
                        />
                      </div>
                    </div>
                  ))
                : nodeStats.map((n) => (
                    <div key={n.id} className="dist-item">
                      <div className="dist-item-top">
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 6,
                            minWidth: 0,
                          }}
                        >
                          <span
                            className="truncate"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {n.name}
                          </span>
                          {n.subLabel && (
                            <span
                              className="tag-sharp"
                              style={{ height: 17, fontSize: 10 }}
                            >
                              {n.subLabel}
                            </span>
                          )}
                        </div>
                        <span
                          className="font-mono-ui"
                          style={{ flexShrink: 0, color: "var(--text-body)" }}
                        >
                          {formatTrafficGB(n.totalGB)}{" "}
                          <span style={{ color: "var(--text-subtle)" }}>
                            ({n.percent.toFixed(1)}%)
                          </span>
                        </span>
                      </div>
                      <div className="dist-bar-track">
                        <div
                          className="dist-bar-fill"
                          style={{
                            width: `${n.totalGB > 0 ? Math.max(2, n.percent) : 0}%`,
                          }}
                        />
                      </div>
                    </div>
                  ))}
            </div>
          </div>
        </section>

        {orgs.length === 0 ? (
          <section
            className="empty-box"
            aria-label="出口节点"
            data-testid="empty-orgs-guide"
          >
            <div
              style={{
                fontSize: 15,
                fontWeight: 600,
                color: "var(--text-primary)",
              }}
            >
              尚未挂载 Tailscale 组织
            </div>
            <div style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
              登录 Tailscale 账号后将自动拉取组织内所有可用的出口节点
            </div>
            <button
              type="button"
              className="btn-lime"
              data-testid="btn-empty-add-org"
              onClick={() => onOpenModal("add-org")}
            >
              + 挂载 Tailscale 组织
            </button>
          </section>
        ) : (
          <>
            <div className="section-bar">
              <div className="seg-tabs" role="tablist" aria-label="组织筛选">
                <button
                  type="button"
                  className={`seg-tab ${effectiveOrgFilter === "all" ? "is-active" : ""}`}
                  onClick={() => setSelectedOrgFilter("all")}
                >
                  全部 ({exitNodes.length})
                </button>
                {orgs.map((org) => {
                  const count = exitNodes.filter((n) => n.orgId === org.id).length;
                  return (
                    <button
                      key={org.id}
                      type="button"
                      className={`seg-tab ${effectiveOrgFilter === org.id ? "is-active" : ""}`}
                      onClick={() => setSelectedOrgFilter(org.id)}
                    >
                      {org.name} ({count})
                    </button>
                  );
                })}
              </div>
            </div>

            {filteredNodes.length === 0 ? (
              <div className="empty-box" data-testid="empty-exits-box">
                <div>暂无可用出口节点</div>
              </div>
            ) : (
              <section className="exit-grid" aria-label="出口节点">
                {filteredNodes.map((node) => {
                  const isSelected = node.id === activeExitId;
                  const isVerifying = node.id === verifyingExitId;
                  const latency = getLatencyMeta(node.latencyMs, node.healthy);
                  const nodeDisplayIp =
                    node.publicIp && node.publicIp !== "-"
                      ? node.publicIp
                      : node.tailscaleIp || "待探测";
                  return (
                    <button
                      key={node.id}
                      type="button"
                      className={`exit-card ${isSelected ? "is-selected" : ""} ${!node.online ? "is-offline" : ""}`}
                      data-testid={`exit-card-${node.id}`}
                      onClick={() => onSwitchExit(node.id)}
                    >
                      <div className="exit-card-head">
                        <div className="exit-node-title">
                          <span
                            className={`status-dot ${isSelected ? "is-on" : node.healthy ? "is-green" : "is-amber"}`}
                          />
                          <span className="truncate">{node.label}</span>
                        </div>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 4,
                            flexShrink: 0,
                          }}
                        >
                          {isVerifying ? (
                            <span className="tag-sharp tag-amber">核验中</span>
                          ) : isSelected ? (
                            <span className="tag-sharp tag-lime">当前</span>
                          ) : null}
                          <span className={latency.tagClass}>{latency.text}</span>
                        </div>
                      </div>

                      <div
                        style={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          gap: 8,
                          paddingTop: 8,
                          borderTop: "1px solid var(--border-hairline)",
                          fontFamily: "var(--font-mono)",
                          fontSize: "11.5px",
                          color: "var(--text-muted)",
                        }}
                      >
                        <span className="truncate">
                          {node.orgName} ·{" "}
                          <strong style={{ color: "var(--text-primary)", fontWeight: 500 }}>
                            {nodeDisplayIp}
                          </strong>
                        </span>
                        <span
                          style={{
                            color:
                              node.relayMode === "中继"
                                ? "var(--status-amber)"
                                : "var(--status-green)",
                            flexShrink: 0,
                          }}
                        >
                          {node.relayMode}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </section>
            )}
          </>
        )}
      </div>
    </>
  );
}
