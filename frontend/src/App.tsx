import { useCallback, useEffect, useRef, useState } from "react";
import { CmdkPalette } from "./components/CmdkPalette";
import { Modals } from "./components/Modals";
import { SharpSwitch } from "./components/SharpSwitch";
import { TrayFlyout } from "./components/TrayFlyout";
import { ConnectionsView } from "./features/ConnectionsView";
import { OverviewView } from "./features/OverviewView";
import { ProvidersView } from "./features/ProvidersView";
import { RulesView } from "./features/RulesView";
import { SettingsView } from "./features/SettingsView";
import {
  type AddOrgPayload,
  type AddProviderPayload,
  type AddRulePayload,
  daemonClient,
} from "./services/daemonClient";
import {
  getActiveExit,
  type ModalKind,
  type ViewId,
} from "./state/selectors";
import type {
  DashboardSnapshot,
  RoutingMode,
  RuleAction,
  SanitizedExport,
  SystemSettings,
  TrafficPeriodKey,
} from "./types/contract";

interface WailsRuntime {
  WindowMinimise?: () => void;
  WindowToggleMaximise?: () => void;
  WindowHide?: () => void;
}

interface ToastState {
  id: number;
  message: string;
  isError: boolean;
}

const MODES: ReadonlyArray<{ id: RoutingMode; label: string }> = [
  { id: "rule", label: "规则" },
  { id: "global", label: "全局" },
  { id: "direct", label: "直连" },
];

function getInitialTheme(): "dark" | "light" {
  try {
    const saved = localStorage.getItem("singtail-theme");
    if (saved === "light" || saved === "dark") {
      return saved;
    }
  } catch {
    // Ignore storage access errors
  }
  return document.documentElement.getAttribute("data-theme") === "light"
    ? "light"
    : "dark";
}

function getWailsRuntime(): WailsRuntime | undefined {
  return (window as unknown as { runtime?: WailsRuntime }).runtime;
}

export function App() {
  const [theme, setTheme] = useState<"dark" | "light">(getInitialTheme);
  const [snapshot, setSnapshot] = useState<DashboardSnapshot | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [activeView, setActiveView] = useState<ViewId>("overview");
  const [trafficPeriod, setTrafficPeriod] = useState<TrafficPeriodKey>("live");
  const [trayOpen, setTrayOpen] = useState(false);
  const [cmdkOpen, setCmdkOpen] = useState(false);
  const [activeModal, setActiveModal] = useState<ModalKind>(null);
  const [connPaused, setConnPaused] = useState(false);
  const [pinging, setPinging] = useState(false);
  const [refreshingProviders, setRefreshingProviders] = useState(false);
  const [toast, setToast] = useState<ToastState | null>(null);

  const toastTimerRef = useRef<number | undefined>(undefined);
  const pollInFlightRef = useRef(false);
  const periodRef = useRef<TrafficPeriodKey>(trafficPeriod);
  const connPausedRef = useRef(connPaused);
  const versionRef = useRef(0);

  periodRef.current = trafficPeriod;
  connPausedRef.current = connPaused;

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    try {
      localStorage.setItem("singtail-theme", theme);
    } catch {
      // Ignore storage access errors
    }
  }, [theme]);

  const showToast = useCallback((message: string, isError = false) => {
    window.clearTimeout(toastTimerRef.current);
    setToast({ id: Date.now(), message, isError });
    toastTimerRef.current = window.setTimeout(() => {
      setToast(null);
    }, 2800);
  }, []);

  useEffect(() => {
    return () => {
      window.clearTimeout(toastTimerRef.current);
    };
  }, []);

  const mergeSnapshot = useCallback(
    (incoming: DashboardSnapshot, preservePeriodTraffic = false) => {
      setSnapshot((prev) => {
        const nextTraffic =
          preservePeriodTraffic &&
          prev &&
          incoming.traffic.period !== periodRef.current
            ? prev.traffic
            : incoming.traffic;
        const nextConnections =
          connPausedRef.current && prev ? prev.connections : incoming.connections;
        return {
          ...incoming,
          traffic: nextTraffic,
          connections: nextConnections,
        };
      });
    },
    [],
  );

  const loadSnapshot = useCallback(
    async (period: TrafficPeriodKey) => {
      const reqVersion = ++versionRef.current;
      try {
        const data = await daemonClient.getSnapshot(period);
        if (versionRef.current !== reqVersion || periodRef.current !== period) {
          return;
        }
        setLoadError(null);
        mergeSnapshot(data, false);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : "无法连接守护服务";
        setLoadError(msg);
      }
    },
    [mergeSnapshot],
  );

  useEffect(() => {
    void loadSnapshot("live");
  }, [loadSnapshot]);

  // Silent 2s polling in live view or active connections view
  useEffect(() => {
    const shouldPoll =
      trafficPeriod === "live" || (activeView === "connections" && !connPaused);
    if (!shouldPoll) {
      return;
    }

    const timer = window.setInterval(() => {
      if (pollInFlightRef.current) {
        return;
      }
      const reqPeriod = periodRef.current;
      const reqVersion = versionRef.current;
      pollInFlightRef.current = true;

      void daemonClient
        .getSnapshot(reqPeriod)
        .then((data) => {
          if (versionRef.current !== reqVersion || periodRef.current !== reqPeriod) {
            return;
          }
          setLoadError(null);
          mergeSnapshot(data, false);
        })
        .catch(() => {
          // Keep current snapshot on transient poll errors
        })
        .finally(() => {
          pollInFlightRef.current = false;
        });
    }, 2000);

    return () => {
      window.clearInterval(timer);
    };
  }, [trafficPeriod, activeView, connPaused, mergeSnapshot]);

  // Global keyboard shortcuts (Ctrl+K, Esc)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCmdkOpen((prev) => !prev);
      } else if (e.key === "Escape") {
        setCmdkOpen(false);
        setActiveModal(null);
        setTrayOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  const runMutation = useCallback(
    async (
      action: () => Promise<DashboardSnapshot>,
      toastText?: string,
      nextView?: ViewId,
    ): Promise<DashboardSnapshot | null> => {
      const reqVersion = ++versionRef.current;
      try {
        const data = await action();
        mergeSnapshot(data, true);
        const activePeriod = periodRef.current;
        if (data.traffic.period !== activePeriod) {
          void daemonClient
            .getSnapshot(activePeriod)
            .then((refreshed) => {
              if (
                versionRef.current !== reqVersion ||
                periodRef.current !== activePeriod
              ) {
                return;
              }
              mergeSnapshot(refreshed, false);
            })
            .catch(() => {});
        }
        if (nextView) {
          setActiveView(nextView);
        }
        if (toastText) {
          showToast(toastText);
        }
        return data;
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : "操作失败";
        showToast(msg, true);
        return null;
      }
    },
    [mergeSnapshot, showToast],
  );

  const handleChangePeriod = useCallback(
    (period: TrafficPeriodKey) => {
      setTrafficPeriod(period);
      periodRef.current = period;
      void loadSnapshot(period);
    },
    [loadSnapshot],
  );

  const handleToggleTun = useCallback(() => {
    if (!snapshot) {
      return;
    }
    const nextEnabled = !snapshot.tunEnabled;
    void runMutation(
      () => daemonClient.setTun(nextEnabled),
      nextEnabled ? "TUN 已开启" : "TUN 已关闭",
    );
  }, [snapshot, runMutation]);

  const handleSetMode = useCallback(
    (mode: RoutingMode) => {
      void runMutation(() => daemonClient.setMode(mode));
    },
    [runMutation],
  );

  const handleSwitchExit = useCallback(
    async (exitId: string) => {
      if (!snapshot) {
        return;
      }
      if (snapshot.verifyingExitId || exitId === snapshot.activeExitId) {
        return;
      }
      const target = snapshot.exitNodes.find((n) => n.id === exitId);
      if (!target) {
        return;
      }

      setSnapshot((prev) =>
        prev
          ? {
              ...prev,
              verifyingExitId: target.id,
              bannerNotice: {
                kind: "verifying",
                text: `正在切换至 ${target.label} 并核验出口 IP…`,
              },
            }
          : prev,
      );

      const data = await runMutation(() => daemonClient.switchExit(exitId));
      if (!data) {
        setSnapshot((prev) =>
          prev
            ? {
                ...prev,
                verifyingExitId: undefined,
                bannerNotice: null,
              }
            : prev,
        );
        return;
      }

      if (data.bannerNotice?.kind === "rollback") {
        showToast(data.bannerNotice.text, true);
      } else {
        const current = getActiveExit(data.exitNodes, data.activeExitId);
        if (current) {
          showToast(`已切换至 ${current.label} (${current.publicIp})`);
        }
      }
    },
    [snapshot, runMutation, showToast],
  );

  const handlePingNodes = useCallback(() => {
    if (pinging) {
      return;
    }
    setPinging(true);
    void runMutation(() => daemonClient.pingExits(), "延迟已刷新").finally(() => {
      setPinging(false);
    });
  }, [pinging, runMutation]);

  const handleDismissBanner = useCallback(() => {
    setSnapshot((prev) => (prev ? { ...prev, bannerNotice: null } : prev));
    void runMutation(() => daemonClient.dismissBanner());
  }, [runMutation]);

  const handleAddRule = useCallback(
    async (payload: AddRulePayload): Promise<boolean> => {
      const data = await runMutation(
        () => daemonClient.addRule(payload),
        `已添加规则：${payload.payload}`,
        "rules",
      );
      return data !== null;
    },
    [runMutation],
  );

  const handleImportRules = useCallback(
    async (rawText: string): Promise<boolean> => {
      const data = await runMutation(
        () => daemonClient.importRules(rawText),
        "规则导入完成",
        "rules",
      );
      return data !== null;
    },
    [runMutation],
  );

  const handleToggleRule = useCallback(
    (ruleId: string) => {
      void runMutation(() => daemonClient.toggleRule(ruleId));
    },
    [runMutation],
  );

  const handleMoveRule = useCallback(
    (ruleId: string, direction: -1 | 1) => {
      void runMutation(() => daemonClient.moveRule(ruleId, direction));
    },
    [runMutation],
  );

  const handleDeleteRule = useCallback(
    (ruleId: string) => {
      void runMutation(() => daemonClient.deleteRule(ruleId));
    },
    [runMutation],
  );

  const handleQuickRule = useCallback(
    (connId: string, matchKind: "domain" | "process", action: RuleAction) => {
      void runMutation(
        () => daemonClient.quickRule({ connId, matchKind, action }),
        "已写入分流规则",
      );
    },
    [runMutation],
  );

  const handleRefreshProviders = useCallback(() => {
    if (refreshingProviders) {
      return;
    }
    setRefreshingProviders(true);
    void runMutation(
      () => daemonClient.refreshProviders(),
      "规则集已全部更新",
    ).finally(() => {
      setRefreshingProviders(false);
    });
  }, [refreshingProviders, runMutation]);

  const handleAddProvider = useCallback(
    async (payload: AddProviderPayload): Promise<boolean> => {
      const data = await runMutation(
        () => daemonClient.addProvider(payload),
        `已添加订阅：${payload.tag}`,
        "providers",
      );
      return data !== null;
    },
    [runMutation],
  );

  const handleAddOrg = useCallback(
    async (payload: AddOrgPayload): Promise<boolean> => {
      const data = await runMutation(
        () => daemonClient.addOrg(payload),
        `已挂载组织：${payload.name}`,
      );
      return data !== null;
    },
    [runMutation],
  );

  const handleSyncOrg = useCallback(
    (orgId: string) => {
      void runMutation(() => daemonClient.syncOrg(orgId), "已同步");
    },
    [runMutation],
  );

  const handleUpdateSettings = useCallback(
    (next: SystemSettings) => {
      void runMutation(() => daemonClient.updateSettings(next));
    },
    [runMutation],
  );

  const handleFetchExport = useCallback((): Promise<SanitizedExport> => {
    return daemonClient.exportConfig();
  }, []);

  const handleWindowMinimize = () => {
    const rt = getWailsRuntime();
    if (rt?.WindowMinimise) {
      rt.WindowMinimise();
    } else {
      setTrayOpen((prev) => !prev);
    }
  };

  const handleWindowMaximize = () => {
    getWailsRuntime()?.WindowToggleMaximise?.();
  };

  const handleWindowClose = () => {
    const rt = getWailsRuntime();
    if (rt?.WindowHide) {
      rt.WindowHide();
    } else {
      setTrayOpen((prev) => !prev);
    }
  };

  const activeExit = snapshot
    ? getActiveExit(snapshot.exitNodes, snapshot.activeExitId)
    : undefined;
  const downNow =
    snapshot && snapshot.tunEnabled
      ? (snapshot.traffic.liveDownHistory.at(-1) ?? 0)
      : 0;
  const upNow =
    snapshot && snapshot.tunEnabled
      ? (snapshot.traffic.liveUpHistory.at(-1) ?? 0)
      : 0;

  const navItems: ReadonlyArray<{
    id: ViewId;
    label: string;
    count: number;
  }> = [
    {
      id: "overview",
      label: "总览与出口",
      count: snapshot?.exitNodes.length ?? 0,
    },
    { id: "rules", label: "分流规则", count: snapshot?.rules.length ?? 0 },
    {
      id: "providers",
      label: "规则集",
      count: snapshot?.providers.length ?? 0,
    },
    {
      id: "connections",
      label: "实时连接",
      count: snapshot?.connections.length ?? 0,
    },
    { id: "settings", label: "设置", count: snapshot?.orgs.length ?? 0 },
  ];

  const renderNavIcon = (id: ViewId) => {
    switch (id) {
      case "overview":
        return (
          <svg
            className="rail-nav-icon"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            aria-hidden="true"
          >
            <rect x="2" y="2" width="5" height="5" />
            <rect x="9" y="2" width="5" height="5" />
            <rect x="2" y="9" width="5" height="5" />
            <rect x="9" y="9" width="5" height="5" />
          </svg>
        );
      case "rules":
        return (
          <svg
            className="rail-nav-icon"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            aria-hidden="true"
          >
            <path d="M2 4h12M2 8h8M2 12h10" />
            <circle cx="13" cy="8" r="1.5" />
          </svg>
        );
      case "providers":
        return (
          <svg
            className="rail-nav-icon"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            aria-hidden="true"
          >
            <path d="M2.5 3.5h11v9h-11z" />
            <path d="M2.5 6.5h11M6 6.5v6" />
          </svg>
        );
      case "connections":
        return (
          <svg
            className="rail-nav-icon"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            aria-hidden="true"
          >
            <path d="M2 10l3-4 3 5 3-6 3 5" />
          </svg>
        );
      case "settings":
        return (
          <svg
            className="rail-nav-icon"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            aria-hidden="true"
          >
            <circle cx="8" cy="8" r="2.5" />
            <path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4" />
          </svg>
        );
    }
  };

  return (
    <>
      <div className="desktop-window">
        <header className="window-titlebar">
          <div className="titlebar-brand">
            <span className="brand-mark">ST</span>
            <span className="brand-wordmark font-pixel-brand">
              SING<span>TAIL</span>
            </span>
          </div>

          <div className="titlebar-center">
            <button
              type="button"
              className="cmdk-trigger"
              data-testid="btn-open-cmdk"
              onClick={() => setCmdkOpen(true)}
            >
              <span>⌕ 搜索出口、规则…</span>
              <span className="font-mono-ui" style={{ fontSize: "10.5px" }}>
                Ctrl+K
              </span>
            </button>
          </div>

          <div className="titlebar-right">
            <div className="titlebar-tool-group">
              <button
                id="btn-toggle-tray"
                type="button"
                className={`btn-hairline btn-sm${trayOpen ? " is-active" : ""}`}
                data-testid="btn-toggle-tray"
                onClick={() => setTrayOpen((prev) => !prev)}
              >
                ◫ 托盘
              </button>
              <button
                id="btn-toggle-theme"
                type="button"
                className="btn-hairline btn-sm font-mono-ui"
                data-testid="btn-toggle-theme"
                onClick={() =>
                  setTheme((prev) => (prev === "dark" ? "light" : "dark"))
                }
              >
                {theme === "dark" ? "◐ 暗色" : "◑ 亮色"}
              </button>
            </div>

            <div className="win-controls" aria-label="窗口控制">
              <button
                type="button"
                className="win-ctrl-btn"
                aria-label="最小化"
                onClick={handleWindowMinimize}
              >
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 10 10"
                  stroke="currentColor"
                  strokeWidth="1.2"
                  aria-hidden="true"
                >
                  <path d="M1 5h8" />
                </svg>
              </button>
              <button
                type="button"
                className="win-ctrl-btn"
                aria-label="最大化"
                onClick={handleWindowMaximize}
              >
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 10 10"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.2"
                  aria-hidden="true"
                >
                  <rect x="1.5" y="1.5" width="7" height="7" />
                </svg>
              </button>
              <button
                type="button"
                className="win-ctrl-btn is-close"
                aria-label="关闭"
                onClick={handleWindowClose}
              >
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 10 10"
                  stroke="currentColor"
                  strokeWidth="1.2"
                  aria-hidden="true"
                >
                  <path d="M1.5 1.5l7 7M8.5 1.5l-7 7" />
                </svg>
              </button>
            </div>
          </div>
        </header>

        <div className="window-body">
          <aside id="sidebar-rail" className="sidebar-rail">
            <div className="rail-engine-box">
              <div className="engine-top-row">
                <div className="engine-status-label">
                  <span
                    className={`status-dot ${snapshot?.tunEnabled ? "is-on" : ""}`}
                  />
                  <span>{snapshot?.tunEnabled ? "TUN 已开启" : "已关闭"}</span>
                </div>
                <SharpSwitch
                  checked={Boolean(snapshot?.tunEnabled)}
                  onToggle={handleToggleTun}
                  disabled={!snapshot}
                  ariaLabel="总开关"
                  testId="switch-master-tun"
                />
              </div>

              <div className="mode-seg-group" role="group" aria-label="分流模式">
                {MODES.map((m) => (
                  <button
                    key={m.id}
                    type="button"
                    disabled={!snapshot}
                    className={`mode-seg-btn ${snapshot?.routingMode === m.id ? "is-active" : ""}`}
                    onClick={() => handleSetMode(m.id)}
                  >
                    {m.label}
                  </button>
                ))}
              </div>

              <div className="engine-exit-mini">
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 4,
                  }}
                >
                  <span
                    className="truncate"
                    style={{
                      fontSize: "12.5px",
                      fontWeight: 600,
                      color: "var(--text-primary)",
                    }}
                  >
                    {activeExit?.label ?? "—"}
                  </span>
                  <span
                    className="font-mono-ui"
                    style={{
                      fontSize: "10.5px",
                      color: "var(--accent-text)",
                      flexShrink: 0,
                    }}
                  >
                    {activeExit ? `${activeExit.latencyMs} ms` : "—"}
                  </span>
                </div>
                <div
                  className="font-mono-ui truncate"
                  style={{ fontSize: 11, color: "var(--text-muted)" }}
                >
                  {activeExit?.publicIp ?? "—"}
                </div>
              </div>
            </div>

            <nav className="rail-nav" aria-label="主导航">
              {navItems.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={`rail-nav-btn ${activeView === item.id ? "is-active" : ""}`}
                  data-testid={`nav-${item.id}`}
                  onClick={() => setActiveView(item.id)}
                >
                  <span className="rail-nav-left">
                    {renderNavIcon(item.id)}
                    <span className="truncate">{item.label}</span>
                  </span>
                  <span className="rail-count">{item.count}</span>
                </button>
              ))}
            </nav>
          </aside>

          <main id="workspace-main" className="workspace-main">
            {!snapshot ? (
              <div className="empty-box">
                <div>{loadError ? loadError : "正在连接守护服务…"}</div>
                {loadError && (
                  <button
                    type="button"
                    className="btn-hairline"
                    onClick={() => void loadSnapshot(trafficPeriod)}
                  >
                    重试
                  </button>
                )}
              </div>
            ) : activeView === "overview" ? (
              <OverviewView
                theme={theme}
                tunEnabled={snapshot.tunEnabled}
                activeExitId={snapshot.activeExitId}
                verifyingExitId={snapshot.verifyingExitId}
                bannerNotice={snapshot.bannerNotice}
                orgs={snapshot.orgs}
                exitNodes={snapshot.exitNodes}
                connectionsCount={snapshot.connections.length}
                traffic={snapshot.traffic}
                trafficPeriod={trafficPeriod}
                pinging={pinging}
                onChangePeriod={handleChangePeriod}
                onSwitchExit={(id) => void handleSwitchExit(id)}
                onPingNodes={handlePingNodes}
                onDismissBanner={handleDismissBanner}
                onOpenModal={setActiveModal}
              />
            ) : activeView === "rules" ? (
              <RulesView
                rules={snapshot.rules}
                activeExitLabel={activeExit?.label ?? "出口"}
                onToggleRule={handleToggleRule}
                onMoveRule={handleMoveRule}
                onDeleteRule={handleDeleteRule}
                onOpenModal={setActiveModal}
              />
            ) : activeView === "providers" ? (
              <ProvidersView
                providers={snapshot.providers}
                refreshing={refreshingProviders}
                onRefreshAll={handleRefreshProviders}
                onOpenModal={setActiveModal}
              />
            ) : activeView === "connections" ? (
              <ConnectionsView
                connections={snapshot.connections}
                activeExitLabel={activeExit?.label ?? "出口"}
                paused={connPaused}
                onTogglePause={() => setConnPaused((prev) => !prev)}
                onQuickRule={handleQuickRule}
              />
            ) : (
              <SettingsView
                orgs={snapshot.orgs}
                exitNodes={snapshot.exitNodes}
                settings={snapshot.settings}
                onSyncOrg={handleSyncOrg}
                onUpdateSettings={handleUpdateSettings}
                onOpenModal={setActiveModal}
              />
            )}
          </main>
        </div>
      </div>

      <TrayFlyout
        open={trayOpen}
        tunEnabled={Boolean(snapshot?.tunEnabled)}
        routingMode={snapshot?.routingMode ?? "rule"}
        activeExit={activeExit}
        exitNodes={snapshot?.exitNodes ?? []}
        downNow={downNow}
        upNow={upNow}
        onToggleTun={handleToggleTun}
        onSetMode={handleSetMode}
        onSwitchExit={(id) => void handleSwitchExit(id)}
      />

      <Modals
        activeModal={activeModal}
        runningApps={snapshot?.runningApps ?? []}
        onClose={() => setActiveModal(null)}
        onAddRule={handleAddRule}
        onImportRules={handleImportRules}
        onAddOrg={handleAddOrg}
        onAddProvider={handleAddProvider}
        onFetchExport={handleFetchExport}
        onToast={showToast}
      />

      <CmdkPalette
        open={cmdkOpen}
        exitNodes={snapshot?.exitNodes ?? []}
        onClose={() => setCmdkOpen(false)}
        onSelectExit={(id) => void handleSwitchExit(id)}
        onSelectView={setActiveView}
        onOpenModal={setActiveModal}
      />

      <div id="toast-stack" className="toast-stack" aria-live="polite">
        {toast && (
          <div
            key={toast.id}
            className={`toast-item${toast.isError ? " is-error" : ""}`}
          >
            {toast.message}
          </div>
        )}
      </div>
    </>
  );
}

export default App;
