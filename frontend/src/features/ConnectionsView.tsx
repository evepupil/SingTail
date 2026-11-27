import { useState } from "react";
import {
  filterConnections,
  formatRateKBps,
  getActionMeta,
  type RuleFilterAction,
} from "../state/selectors";
import type { LiveConnection, RuleAction } from "../types/contract";

interface ConnectionsViewProps {
  connections: readonly LiveConnection[];
  activeExitLabel: string;
  paused: boolean;
  onTogglePause: () => void;
  onQuickRule: (
    connId: string,
    matchKind: "domain" | "process",
    action: RuleAction,
  ) => void;
}

export function ConnectionsView({
  connections,
  activeExitLabel,
  paused,
  onTogglePause,
  onQuickRule,
}: ConnectionsViewProps) {
  const [filterAction, setFilterAction] = useState<RuleFilterAction>("ALL");
  const [search, setSearch] = useState("");
  const [selectedConnId, setSelectedConnId] = useState<string>("");

  const filtered = filterConnections(connections, filterAction, search);
  const selected =
    filtered.find((c) => c.id === selectedConnId) ??
    connections.find((c) => c.id === selectedConnId) ??
    filtered[0] ??
    null;

  const filterTabs: ReadonlyArray<{ id: RuleFilterAction; label: string }> = [
    { id: "ALL", label: `全部 (${connections.length})` },
    { id: "EXIT", label: "出口" },
    { id: "DIRECT", label: "直连" },
    { id: "FALLBACK_PROXY", label: "本地代理" },
    { id: "REJECT", label: "拦截" },
  ];

  const selectedActionMeta = selected
    ? getActionMeta(selected.routeAction, activeExitLabel)
    : null;

  return (
    <>
      <div className="view-topbar">
        <h1 className="view-title">实时连接</h1>
        <div className="view-actions">
          <button
            type="button"
            className="btn-hairline"
            onClick={onTogglePause}
          >
            {paused ? "▶ 恢复" : "⏸ 暂停"}
          </button>
        </div>
      </div>

      <div className="section-bar">
        <div className="seg-tabs" role="tablist">
          {filterTabs.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`seg-tab ${filterAction === t.id ? "is-active" : ""}`}
              onClick={() => setFilterAction(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <input
          id="conn-search-input"
          type="search"
          className="input-sharp"
          style={{ width: 220 }}
          placeholder="过滤进程 / 域名 / IP…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <div className="split-workbench">
        <div className="view-scroll">
          {filtered.length > 0 ? (
            <table className="data-table" aria-label="实时连接">
              <thead>
                <tr>
                  <th style={{ width: 136 }}>进程</th>
                  <th>目标地址</th>
                  <th style={{ width: 150 }}>规则</th>
                  <th style={{ width: 142 }}>走向</th>
                  <th style={{ width: 86, textAlign: "right" }}>速率</th>
                  <th style={{ width: 74, textAlign: "right" }}>流量</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => {
                  const isSel = selected?.id === c.id;
                  const meta = getActionMeta(c.routeAction, activeExitLabel);
                  return (
                    <tr
                      key={c.id}
                      className={isSel ? "is-selected" : undefined}
                      style={{ cursor: "pointer" }}
                      data-testid={`conn-row-${c.id}`}
                      onClick={() => setSelectedConnId(c.id)}
                    >
                      <td
                        className="font-mono-ui"
                        style={{
                          width: 136,
                          color: "var(--text-primary)",
                          fontWeight: 500,
                        }}
                      >
                        {c.process}
                      </td>
                      <td
                        className="font-mono-ui"
                        title={`${c.host}:${c.port}`}
                      >
                        {c.host}
                        <span style={{ color: "var(--text-subtle)" }}>
                          :{c.port}
                        </span>
                      </td>
                      <td style={{ width: 150 }} title={c.matchedRule}>
                        <span
                          className="font-mono-ui"
                          style={{ fontSize: "11.5px", color: "var(--text-muted)" }}
                        >
                          {c.matchedRule}
                        </span>
                      </td>
                      <td style={{ width: 142 }}>
                        <span className={meta.tagClass}>{meta.text}</span>
                      </td>
                      <td
                        className="font-mono-ui"
                        style={{
                          width: 86,
                          textAlign: "right",
                          color: "var(--accent-text)",
                        }}
                      >
                        {formatRateKBps(c.downKBps)}
                      </td>
                      <td
                        className="font-mono-ui"
                        style={{
                          width: 74,
                          textAlign: "right",
                          color: "var(--text-subtle)",
                        }}
                      >
                        {c.totalBytes}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          ) : (
            <div className="empty-box">
              <div>无匹配连接</div>
              <button
                type="button"
                className="btn-hairline"
                onClick={() => {
                  setFilterAction("ALL");
                  setSearch("");
                }}
              >
                清空筛选
              </button>
            </div>
          )}
        </div>

        <aside className="inspector-pane" aria-label="连接详情">
          {selected && selectedActionMeta ? (
            <>
              <div
                className="inspector-section"
                style={{ background: "var(--bg-code)" }}
              >
                <div
                  className="font-mono-ui truncate selectable-text"
                  style={{
                    fontSize: 14,
                    fontWeight: 600,
                    color: "var(--text-primary)",
                  }}
                >
                  {selected.host}:{selected.port}
                </div>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 6,
                    marginTop: 6,
                  }}
                >
                  <span className="tag-sharp">{selected.network}</span>
                  <span className={selectedActionMeta.tagClass}>
                    {selectedActionMeta.text}
                  </span>
                </div>
              </div>

              <div className="inspector-section">
                <div className="inspector-kv-list" style={{ marginTop: 0 }}>
                  <div className="inspector-kv-row">
                    <span>进程</span>
                    <span>{selected.process}</span>
                  </div>
                  <div className="inspector-kv-row">
                    <span>目标 IP</span>
                    <span>
                      {selected.dstIp}:{selected.port}
                    </span>
                  </div>
                  <div className="inspector-kv-row">
                    <span>命中规则</span>
                    <span>{selected.matchedRule}</span>
                  </div>
                  <div className="inspector-kv-row">
                    <span>延迟</span>
                    <span>{selected.rttMs ? `${selected.rttMs} ms` : "—"}</span>
                  </div>
                  <div className="inspector-kv-row">
                    <span>累计流量</span>
                    <span>{selected.totalBytes}</span>
                  </div>
                  <div className="inspector-kv-row">
                    <span>时间</span>
                    <span>{selected.startedAt}</span>
                  </div>
                </div>
              </div>

              <div className="inspector-section">
                <div className="eyebrow-mono" style={{ marginBottom: 8 }}>
                  域名规则
                </div>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(2,minmax(0,1fr))",
                    gap: 6,
                  }}
                >
                  <button
                    type="button"
                    className="btn-hairline btn-sm"
                    onClick={() => onQuickRule(selected.id, "domain", "EXIT")}
                  >
                    ↗ 走出口
                  </button>
                  <button
                    type="button"
                    className="btn-hairline btn-sm"
                    onClick={() => onQuickRule(selected.id, "domain", "DIRECT")}
                  >
                    → 直连
                  </button>
                  <button
                    type="button"
                    className="btn-hairline btn-sm"
                    onClick={() =>
                      onQuickRule(selected.id, "domain", "FALLBACK_PROXY")
                    }
                  >
                    ⇄ 本地代理
                  </button>
                  <button
                    type="button"
                    className="btn-danger btn-sm"
                    onClick={() => onQuickRule(selected.id, "domain", "REJECT")}
                  >
                    ✕ 拦截
                  </button>
                </div>

                <div className="eyebrow-mono" style={{ margin: "12px 0 8px" }}>
                  进程规则 ({selected.process})
                </div>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(2,minmax(0,1fr))",
                    gap: 6,
                  }}
                >
                  <button
                    type="button"
                    className="btn-hairline btn-sm"
                    onClick={() => onQuickRule(selected.id, "process", "EXIT")}
                  >
                    ↗ 走出口
                  </button>
                  <button
                    type="button"
                    className="btn-hairline btn-sm"
                    onClick={() => onQuickRule(selected.id, "process", "DIRECT")}
                  >
                    → 直连
                  </button>
                </div>
              </div>
            </>
          ) : (
            <div className="empty-box">点击连接查看详情</div>
          )}
        </aside>
      </div>
    </>
  );
}
