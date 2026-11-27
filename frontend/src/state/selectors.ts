import type {
  ExitNode,
  LiveConnection,
  Rule,
  RuleAction,
} from "../types/contract";

export type ViewId = "overview" | "rules" | "providers" | "connections" | "settings";
export type RuleFilterAction = "ALL" | RuleAction;
export type ModalKind =
  | "add-rule"
  | "pick-app"
  | "import-rules"
  | "add-org"
  | "add-provider"
  | "export-config"
  | null;

export interface CmdkItem {
  kind: "出口" | "视图" | "动作";
  label: string;
  sub: string;
  cmd: string;
}

const ORG_COLORS = [
  "var(--status-cyan)",
  "var(--accent-fill)",
  "var(--status-green)",
  "var(--status-amber)",
] as const;

export function getBreakdownColor(index: number): string {
  return ORG_COLORS[index % ORG_COLORS.length] ?? "var(--accent-fill)";
}

export function formatTrafficGB(gb: number): string {
  if (!Number.isFinite(gb) || gb <= 0) {
    return "0.0 GB";
  }
  if (gb >= 1024) {
    return `${(gb / 1024).toFixed(2)} TB`;
  }
  return `${gb.toFixed(1)} GB`;
}

export function formatRateKBps(kbps: number): string {
  if (!Number.isFinite(kbps) || kbps <= 0) {
    return "—";
  }
  if (kbps >= 1000) {
    return `${(kbps / 1024).toFixed(1)} MB/s`;
  }
  return `${Math.round(kbps)} KB/s`;
}

export function getActiveExit(
  exitNodes: readonly ExitNode[],
  activeExitId: string,
): ExitNode | undefined {
  return exitNodes.find((n) => n.id === activeExitId) ?? exitNodes[0];
}

export function filterExitNodesByOrg(
  exitNodes: readonly ExitNode[],
  orgFilter: string,
): ExitNode[] {
  if (orgFilter === "all") {
    return [...exitNodes];
  }
  return exitNodes.filter((n) => n.orgId === orgFilter);
}

export function filterRules(
  rules: readonly Rule[],
  actionFilter: RuleFilterAction,
  query: string,
): Rule[] {
  const q = query.trim().toLowerCase();
  return rules.filter((r) => {
    if (actionFilter !== "ALL" && r.action !== actionFilter) {
      return false;
    }
    if (!q) {
      return true;
    }
    return (
      r.payload.toLowerCase().includes(q) ||
      r.type.toLowerCase().includes(q)
    );
  });
}

export function countRulesByAction(
  rules: readonly Rule[],
): Record<RuleFilterAction, number> {
  const counts: Record<RuleFilterAction, number> = {
    ALL: rules.length,
    EXIT: 0,
    DIRECT: 0,
    FALLBACK_PROXY: 0,
    REJECT: 0,
  };
  for (const r of rules) {
    counts[r.action] += 1;
  }
  return counts;
}

export function canMoveRule(
  rules: readonly Rule[],
  ruleId: string,
  direction: -1 | 1,
): boolean {
  const idx = rules.findIndex((r) => r.id === ruleId);
  if (idx < 0) {
    return false;
  }
  const current = rules[idx];
  if (!current || current.locked) {
    return false;
  }
  const target = rules[idx + direction];
  if (!target || target.locked) {
    return false;
  }
  return true;
}

export function filterConnections(
  connections: readonly LiveConnection[],
  actionFilter: RuleFilterAction,
  query: string,
): LiveConnection[] {
  const q = query.trim().toLowerCase();
  return connections.filter((c) => {
    if (actionFilter !== "ALL" && c.routeAction !== actionFilter) {
      return false;
    }
    if (!q) {
      return true;
    }
    return (
      c.process.toLowerCase().includes(q) ||
      c.host.toLowerCase().includes(q) ||
      c.dstIp.toLowerCase().includes(q)
    );
  });
}

export function getLatencyMeta(
  ms: number,
  healthy = true,
): { text: string; tagClass: string } {
  if (!healthy) {
    return { text: "超时", tagClass: "tag-sharp tag-red" };
  }
  if (ms <= 70) {
    return { text: `${ms} ms`, tagClass: "tag-sharp tag-green" };
  }
  if (ms <= 165) {
    return { text: `${ms} ms`, tagClass: "tag-sharp tag-lime-outline" };
  }
  return { text: `${ms} ms`, tagClass: "tag-sharp tag-amber" };
}

export function getActionMeta(
  action: RuleAction,
  exitLabel = "出口",
): { text: string; tagClass: string } {
  switch (action) {
    case "EXIT":
      return { text: `↗ ${exitLabel}`, tagClass: "tag-sharp tag-lime-outline" };
    case "DIRECT":
      return { text: "→ 直连", tagClass: "tag-sharp tag-green" };
    case "FALLBACK_PROXY":
      return { text: "⇄ 本地代理", tagClass: "tag-sharp tag-cyan" };
    case "REJECT":
      return { text: "✕ 拦截", tagClass: "tag-sharp tag-red" };
  }
}

export function buildCmdkItems(
  exitNodes: readonly ExitNode[],
  query: string,
): CmdkItem[] {
  const q = query.trim().toLowerCase();
  const items: CmdkItem[] = [
    ...exitNodes
      .filter((n) => n.healthy)
      .map((n) => ({
        kind: "出口" as const,
        label: `${n.label} (${n.publicIp})`,
        sub: n.orgName,
        cmd: `exit:${n.id}`,
      })),
    { kind: "视图", label: "总览与出口", sub: "", cmd: "view:overview" },
    { kind: "视图", label: "分流规则", sub: "", cmd: "view:rules" },
    { kind: "视图", label: "规则集", sub: "", cmd: "view:providers" },
    { kind: "视图", label: "实时连接", sub: "", cmd: "view:connections" },
    { kind: "视图", label: "设置", sub: "", cmd: "view:settings" },
    { kind: "动作", label: "新建规则", sub: "", cmd: "modal:add-rule" },
    { kind: "动作", label: "导入规则文件", sub: "", cmd: "modal:import-rules" },
  ];

  if (!q) {
    return items;
  }
  return items.filter(
    (it) =>
      it.label.toLowerCase().includes(q) ||
      it.sub.toLowerCase().includes(q),
  );
}
