import type { ExitNode, RoutingMode } from "../types/contract";
import { getLatencyMeta } from "../state/selectors";
import { SharpSwitch } from "./SharpSwitch";

interface TrayFlyoutProps {
  open: boolean;
  tunEnabled: boolean;
  routingMode: RoutingMode;
  activeExit: ExitNode | undefined;
  exitNodes: readonly ExitNode[];
  downNow: number;
  upNow: number;
  onToggleTun: () => void;
  onSetMode: (mode: RoutingMode) => void;
  onSwitchExit: (exitId: string) => void;
}

const MODES: ReadonlyArray<{ id: RoutingMode; label: string }> = [
  { id: "rule", label: "规则" },
  { id: "global", label: "全局" },
  { id: "direct", label: "直连" },
];

export function TrayFlyout({
  open,
  tunEnabled,
  routingMode,
  activeExit,
  exitNodes,
  downNow,
  upNow,
  onToggleTun,
  onSetMode,
  onSwitchExit,
}: TrayFlyoutProps) {
  const healthyNodes = exitNodes.filter((n) => n.healthy);

  return (
    <aside
      id="tray-flyout"
      className="tray-flyout"
      aria-label="系统托盘面板"
      hidden={!open}
    >
      <div className="tray-head">
        <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
          <span className={`status-dot ${tunEnabled ? "is-on" : ""}`} />
          <span style={{ fontWeight: 700, fontSize: "12.5px" }}>SingTail</span>
        </div>
        <SharpSwitch
          checked={tunEnabled}
          onToggle={onToggleTun}
          ariaLabel="总开关"
        />
      </div>

      <div
        style={{
          padding: "10px 12px",
          borderBottom: "1px solid var(--border-hairline)",
          background: "var(--bg-surface)",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            fontFamily: "var(--font-mono)",
            fontSize: "11.5px",
          }}
        >
          <span style={{ color: "var(--text-muted)" }}>
            {activeExit?.publicIp ?? "—"}
          </span>
          <span>
            <span style={{ color: "var(--accent-text)" }}>
              ↓{downNow.toFixed(1)}
            </span>{" "}
            /{" "}
            <span style={{ color: "var(--status-cyan)" }}>
              ↑{upNow.toFixed(1)}
            </span>{" "}
            MB/s
          </span>
        </div>

        <div className="mode-seg-group" style={{ marginTop: 8 }}>
          {MODES.map((m) => (
            <button
              key={m.id}
              type="button"
              className={`mode-seg-btn ${routingMode === m.id ? "is-active" : ""}`}
              onClick={() => onSetMode(m.id)}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>

      <div style={{ maxHeight: 172, overflowY: "auto" }}>
        {healthyNodes.map((n) => {
          const isSelected = n.id === activeExit?.id;
          const latency = getLatencyMeta(n.latencyMs, n.healthy);
          return (
            <button
              key={n.id}
              type="button"
              className={`tray-node-item ${isSelected ? "is-selected" : ""}`}
              onClick={() => onSwitchExit(n.id)}
            >
              <span className="truncate">
                {isSelected ? "● " : ""}
                {n.label}
              </span>
              <span className={latency.tagClass}>{latency.text}</span>
            </button>
          );
        })}
      </div>
    </aside>
  );
}
