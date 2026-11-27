// Strict TypeScript mirror of internal/contract/types.go

export type RoutingMode = "rule" | "global" | "direct";
export type RuleAction = "EXIT" | "DIRECT" | "FALLBACK_PROXY" | "REJECT";
export type RuleType =
  | "DOMAIN-SUFFIX"
  | "DOMAIN-KEYWORD"
  | "PROCESS-NAME"
  | "IP-CIDR"
  | "RULE-SET"
  | "FINAL";
export type TrafficPeriodKey = "live" | "day" | "week" | "month30" | "monthly";
export type BreakdownTab = "node" | "org";

export interface Organization {
  id: string;
  name: string;
  account: string;
  localTailscaleIp: string;
  stateDir?: string;
  lastSync: string;
}

export interface ExitNode {
  id: string;
  orgId: string;
  orgName: string;
  hostname: string;
  label: string;
  tailscaleIp: string;
  publicIp: string;
  region: string;
  relayMode: string;
  latencyMs: number;
  online: boolean;
  healthy: boolean;
}

export interface Rule {
  id: string;
  type: RuleType;
  payload: string;
  action: RuleAction;
  hits: number;
  enabled: boolean;
  locked: boolean;
}

export interface RuleProvider {
  id: string;
  tag: string;
  sourceName: string;
  url: string;
  entryCount: number;
  updatedAt: string;
}

export interface RunningApp {
  process: string;
  path: string;
}

export interface LiveConnection {
  id: string;
  process: string;
  host: string;
  dstIp: string;
  port: number;
  network: string;
  matchedRule: string;
  routeAction: RuleAction;
  rttMs: number;
  downKBps: number;
  totalBytes: string;
  startedAt: string;
}

export interface TrafficBreakdownItem {
  id: string;
  name: string;
  subLabel: string;
  downBytes: number;
  upBytes: number;
  totalGB: number;
  percent: number;
}

export interface TrafficReport {
  period: TrafficPeriodKey;
  label: string;
  totalDownGB: number;
  totalUpGB: number;
  totalGB: number;
  labels: string[];
  downSeries: number[];
  upSeries: number[];
  liveDownHistory: number[];
  liveUpHistory: number[];
  byNode: TrafficBreakdownItem[];
  byOrg: TrafficBreakdownItem[];
}

export interface BannerNotice {
  kind: "verifying" | "rollback";
  text: string;
}

export interface SystemSettings {
  autoStartService: boolean;
  autoVerifyIp: boolean;
  fallbackProxyEnabled: boolean;
  fallbackProxyAddr: string;
  autoUpdateRulesDays: number;
}

export interface DashboardSnapshot {
  tunEnabled: boolean;
  routingMode: RoutingMode;
  activeExitId: string;
  verifyingExitId?: string;
  bannerNotice?: BannerNotice | null;
  orgs: Organization[];
  exitNodes: ExitNode[];
  rules: Rule[];
  providers: RuleProvider[];
  connections: LiveConnection[];
  runningApps: RunningApp[];
  settings: SystemSettings;
  traffic: TrafficReport;
}

export interface SanitizedExport {
  version: string;
  exportedAt: string;
  customRules: Array<{
    type: RuleType;
    payload: string;
    action: RuleAction;
    enabled: boolean;
  }>;
  providers: Array<{
    tag: string;
    sourceName: string;
    url: string;
  }>;
  settings: SystemSettings;
}
