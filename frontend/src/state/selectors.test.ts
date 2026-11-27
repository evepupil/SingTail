import { describe, expect, it } from "vitest";
import type { ExitNode, LiveConnection, Rule } from "../types/contract";
import {
  buildCmdkItems,
  canMoveRule,
  countRulesByAction,
  filterConnections,
  filterExitNodesByOrg,
  filterRules,
  formatRateKBps,
  formatTrafficGB,
  getActionMeta,
  getActiveExit,
  getExitIpDisplay,
  getLatencyMeta,
} from "./selectors";

const SAMPLE_EXITS: ExitNode[] = [
  {
    id: "ts-us-macmini",
    orgId: "org-studio",
    orgName: "GolfJoy Studio",
    hostname: "us-la-macmini-m2",
    label: "洛杉矶 Mac mini",
    tailscaleIp: "100.123.114.28",
    publicIp: "68.225.41.109",
    region: "US · Los Angeles",
    relayMode: "直连",
    latencyMs: 138,
    online: true,
    healthy: true,
  },
  {
    id: "ts-hk-cloud",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "hk-edge-cvm-01",
    label: "香港轻量云",
    tailscaleIp: "100.91.77.63",
    publicIp: "43.153.82.19",
    region: "HK · Hong Kong",
    relayMode: "直连",
    latencyMs: 34,
    online: true,
    healthy: true,
  },
  {
    id: "ts-us-broken",
    orgId: "org-personal",
    orgName: "Personal Tailnet",
    hostname: "us-sjc-sandbox",
    label: "圣何塞节点",
    tailscaleIp: "100.91.201.99",
    publicIp: "203.0.113.88",
    region: "US · San Jose",
    relayMode: "中继",
    latencyMs: 285,
    online: true,
    healthy: false,
  },
];

const SAMPLE_RULES: Rule[] = [
  {
    id: "r-01",
    type: "PROCESS-NAME",
    payload: "tailscaled.exe",
    action: "DIRECT",
    hits: 2410,
    enabled: true,
    locked: true,
  },
  {
    id: "r-02",
    type: "PROCESS-NAME",
    payload: "YuanShen.exe",
    action: "DIRECT",
    hits: 964,
    enabled: true,
    locked: false,
  },
  {
    id: "r-03",
    type: "DOMAIN-SUFFIX",
    payload: "chatgpt.com, openai.com",
    action: "EXIT",
    hits: 1285,
    enabled: true,
    locked: false,
  },
  {
    id: "r-04",
    type: "RULE-SET",
    payload: "geosite-category-ads-all",
    action: "REJECT",
    hits: 729,
    enabled: true,
    locked: false,
  },
  {
    id: "r-05",
    type: "FINAL",
    payload: "默认兜底",
    action: "EXIT",
    hits: 6180,
    enabled: true,
    locked: true,
  },
];

const SAMPLE_CONNS: LiveConnection[] = [
  {
    id: "c-101",
    process: "Cursor.exe",
    host: "api.anthropic.com",
    dstIp: "160.79.104.25",
    port: 443,
    network: "TCP",
    matchedRule: "FINAL",
    routeAction: "EXIT",
    rttMs: 142,
    downKBps: 420.5,
    totalBytes: "3.8 MB",
    startedAt: "10:56:12",
  },
  {
    id: "c-102",
    process: "chrome.exe",
    host: "www.bilibili.com",
    dstIp: "119.96.135.82",
    port: 443,
    network: "TCP",
    matchedRule: "geosite-cn",
    routeAction: "DIRECT",
    rttMs: 19,
    downKBps: 3120.8,
    totalBytes: "68.4 MB",
    startedAt: "10:52:19",
  },
  {
    id: "c-103",
    process: "chrome.exe",
    host: "www.netflix.com",
    dstIp: "52.26.71.19",
    port: 443,
    network: "TCP",
    matchedRule: "netflix.com",
    routeAction: "FALLBACK_PROXY",
    rttMs: 56,
    downKBps: 2490,
    totalBytes: "42.0 MB",
    startedAt: "10:49:33",
  },
];

describe("formatTrafficGB", () => {
  it("formats values below 1 TB in GB with 1 decimal", () => {
    expect(formatTrafficGB(0)).toBe("0.0 GB");
    expect(formatTrafficGB(18.44)).toBe("18.4 GB");
    expect(formatTrafficGB(1023.9)).toBe("1023.9 GB");
  });

  it("formats values at or above 1024 GB in TB with 2 decimals", () => {
    expect(formatTrafficGB(1024)).toBe("1.00 TB");
    expect(formatTrafficGB(1842.5)).toBe("1.80 TB");
  });

  it("guards negative or non-finite inputs", () => {
    expect(formatTrafficGB(-5)).toBe("0.0 GB");
    expect(formatTrafficGB(Number.NaN)).toBe("0.0 GB");
  });
});

describe("formatRateKBps", () => {
  it("formats zero and positive KB/s or MB/s", () => {
    expect(formatRateKBps(0)).toBe("—");
    expect(formatRateKBps(420.5)).toBe("421 KB/s");
    expect(formatRateKBps(1840)).toBe("1.8 MB/s");
  });
});

describe("exit selectors", () => {
  it("returns active exit or falls back to first node", () => {
    expect(getActiveExit(SAMPLE_EXITS, "ts-hk-cloud")?.id).toBe("ts-hk-cloud");
    expect(getActiveExit(SAMPLE_EXITS, "missing")?.id).toBe("ts-us-macmini");
    expect(getActiveExit([], "ts-hk-cloud")).toBeUndefined();
  });

  it("filters exit nodes by orgId", () => {
    expect(filterExitNodesByOrg(SAMPLE_EXITS, "all")).toHaveLength(3);
    expect(filterExitNodesByOrg(SAMPLE_EXITS, "org-personal")).toHaveLength(2);
    expect(filterExitNodesByOrg(SAMPLE_EXITS, "org-studio")).toHaveLength(1);
  });

  it("assigns latency badge severity by health and threshold", () => {
    expect(getLatencyMeta(34, true)).toEqual({ text: "34 ms", tagClass: "tag-sharp tag-green" });
    expect(getLatencyMeta(138, true)).toEqual({ text: "138 ms", tagClass: "tag-sharp tag-lime-outline" });
    expect(getLatencyMeta(220, true)).toEqual({ text: "220 ms", tagClass: "tag-sharp tag-amber" });
    expect(getLatencyMeta(40, false)).toEqual({ text: "超时", tagClass: "tag-sharp tag-red" });
  });

  it("displays tailscaleIp and pending badge when publicIp is empty", () => {
    const firstExit = SAMPLE_EXITS[0];
    expect(firstExit).toBeDefined();
    if (!firstExit) return;

    expect(getExitIpDisplay(undefined)).toEqual({
      ipText: "—",
      badgeText: "离线",
      badgeClass: "tag-sharp tag-red",
    });
    expect(getExitIpDisplay({ ...firstExit, publicIp: "" })).toEqual({
      ipText: "100.123.114.28",
      badgeText: "待探测",
      badgeClass: "tag-sharp tag-amber",
    });
    expect(getExitIpDisplay(firstExit)).toEqual({
      ipText: "68.225.41.109",
      badgeText: "已连通",
      badgeClass: "tag-sharp tag-green",
    });
    expect(getExitIpDisplay({ ...firstExit, online: false })).toEqual({
      ipText: "68.225.41.109",
      badgeText: "离线",
      badgeClass: "tag-sharp tag-red",
    });
  });
});

describe("rule selectors", () => {
  it("filters rules by action and search query", () => {
    expect(filterRules(SAMPLE_RULES, "ALL", "")).toHaveLength(5);
    expect(filterRules(SAMPLE_RULES, "DIRECT", "")).toHaveLength(2);
    expect(filterRules(SAMPLE_RULES, "ALL", "OPENAI")).toHaveLength(1);
    expect(filterRules(SAMPLE_RULES, "EXIT", "process-name")).toHaveLength(0);
    expect(filterRules(SAMPLE_RULES, "ALL", "rule-set")).toHaveLength(1);
  });

  it("counts rules by action", () => {
    expect(countRulesByAction(SAMPLE_RULES)).toEqual({
      ALL: 5,
      EXIT: 2,
      DIRECT: 2,
      FALLBACK_PROXY: 0,
      REJECT: 1,
    });
  });

  it("prevents moving locked rules or crossing locked boundaries", () => {
    expect(canMoveRule(SAMPLE_RULES, "r-01", 1)).toBe(false);
    expect(canMoveRule(SAMPLE_RULES, "r-02", -1)).toBe(false);
    expect(canMoveRule(SAMPLE_RULES, "r-02", 1)).toBe(true);
    expect(canMoveRule(SAMPLE_RULES, "r-04", 1)).toBe(false);
    expect(canMoveRule(SAMPLE_RULES, "r-04", -1)).toBe(true);
  });

  it("formats rule action badge metadata", () => {
    expect(getActionMeta("EXIT", "香港轻量云")).toEqual({
      text: "↗ 香港轻量云",
      tagClass: "tag-sharp tag-lime-outline",
    });
    expect(getActionMeta("FALLBACK_PROXY")).toEqual({
      text: "⇄ 本地代理",
      tagClass: "tag-sharp tag-cyan",
    });
  });
});

describe("connection and cmdk selectors", () => {
  it("filters connections by action and process/host/ip query", () => {
    expect(filterConnections(SAMPLE_CONNS, "ALL", "")).toHaveLength(3);
    expect(filterConnections(SAMPLE_CONNS, "DIRECT", "bilibili")).toHaveLength(1);
    expect(filterConnections(SAMPLE_CONNS, "ALL", "160.79.")).toHaveLength(1);
    expect(filterConnections(SAMPLE_CONNS, "FALLBACK_PROXY", "cursor")).toHaveLength(0);
  });

  it("excludes unhealthy exits from cmdk items and filters by query", () => {
    const allItems = buildCmdkItems(SAMPLE_EXITS, "");
    expect(allItems.some((it) => it.cmd === "exit:ts-us-broken")).toBe(false);
    expect(allItems.some((it) => it.cmd === "exit:ts-hk-cloud")).toBe(true);

    const filtered = buildCmdkItems(SAMPLE_EXITS, "GolfJoy");
    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.cmd).toBe("exit:ts-us-macmini");
  });
});
