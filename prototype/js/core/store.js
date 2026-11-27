// Reactive application store for SingTail Desktop Prototype
import {
  INITIAL_ORGS,
  INITIAL_EXIT_NODES,
  TRAFFIC_PERIODS,
  INITIAL_RULES,
  INITIAL_PROVIDERS,
  INITIAL_CONNECTIONS,
  RUNNING_APPS
} from "../data/mock-data.js";

const listeners = new Set();

export const state = {
  theme: "dark",
  activeView: "overview", // overview | rules | providers | connections | settings
  tunEnabled: true,
  routingMode: "rule", // rule | global | direct
  selectedOrgFilter: "all",
  activeExitId: "ts-us-macmini",
  verifyingExitId: null,
  bannerNotice: null,

  // Traffic analytics granularity & breakdown tab
  trafficPeriod: "live", // live | day | week | month30 | monthly
  breakdownTab: "node", // node | org
  trafficPeriods: structuredClone(TRAFFIC_PERIODS),

  orgs: structuredClone(INITIAL_ORGS),
  exitNodes: structuredClone(INITIAL_EXIT_NODES),
  rules: structuredClone(INITIAL_RULES),
  providers: structuredClone(INITIAL_PROVIDERS),
  connections: structuredClone(INITIAL_CONNECTIONS),
  runningApps: structuredClone(RUNNING_APPS),

  ruleFilterAction: "ALL",
  ruleSearch: "",
  connFilterAction: "ALL",
  connSearch: "",
  connPaused: false,
  selectedConnId: "c-106",

  downHistory: [
    1.8, 2.4, 3.1, 2.9, 4.6, 6.2, 5.4, 4.1, 3.3, 5.8, 7.9, 8.4,
    6.7, 5.2, 4.8, 6.1, 9.2, 11.4, 10.1, 7.6, 6.3, 5.9, 7.2, 8.8,
    7.4, 6.0, 5.5, 6.9, 8.1, 9.6, 8.3, 7.1, 6.4, 7.8, 8.5, 8.1
  ],
  upHistory: [
    0.3, 0.4, 0.5, 0.4, 0.8, 1.1, 0.9, 0.7, 0.6, 0.9, 1.3, 1.4,
    1.1, 0.8, 0.7, 1.0, 1.6, 1.9, 1.7, 1.2, 1.0, 0.9, 1.1, 1.4,
    1.2, 0.9, 0.8, 1.1, 1.3, 1.5, 1.3, 1.1, 1.0, 1.2, 1.4, 1.3
  ],

  settings: {
    autoStartService: true,
    autoVerifyIp: true,
    fallbackProxyEnabled: true,
    fallbackProxyAddr: "127.0.0.1:7897",
    autoUpdateRulesDays: 14
  },

  trayOpen: false,
  cmdkOpen: false,
  cmdkQuery: "",
  activeModal: null
};

export function subscribe(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function notify() {
  for (const fn of listeners) fn(state);
}

let toastTimer = null;
export function pushToast(message, isError = false) {
  const stack = document.getElementById("toast-stack");
  if (!stack) return;
  const item = document.createElement("div");
  item.className = `toast-item${isError ? " is-error" : ""}`;
  item.textContent = message;
  stack.replaceChildren(item);
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    item.remove();
  }, 2800);
}

export function getActiveExit() {
  return state.exitNodes.find((n) => n.id === state.activeExitId) || state.exitNodes[0];
}

export function setView(view) {
  state.activeView = view;
  state.cmdkOpen = false;
  notify();
}

export function toggleTheme() {
  state.theme = state.theme === "dark" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", state.theme);
  notify();
}

export function toggleTun() {
  state.tunEnabled = !state.tunEnabled;
  pushToast(state.tunEnabled ? "TUN 已开启" : "TUN 已关闭");
  notify();
}

export function setRoutingMode(mode) {
  state.routingMode = mode;
  notify();
}

export function setTrafficPeriod(period) {
  if (!state.trafficPeriods[period]) return;
  state.trafficPeriod = period;
  notify();
}

export function setBreakdownTab(tab) {
  state.breakdownTab = tab;
  notify();
}

export function switchExitNode(targetId) {
  if (state.verifyingExitId) return;
  const target = state.exitNodes.find((n) => n.id === targetId);
  if (!target || target.id === state.activeExitId) return;

  const previous = getActiveExit();
  state.verifyingExitId = target.id;
  state.bannerNotice = {
    kind: "verifying",
    text: `正在切换至 ${target.label} 并核验出口 IP…`
  };
  notify();

  setTimeout(() => {
    state.verifyingExitId = null;
    if (!target.healthy) {
      state.bannerNotice = {
        kind: "rollback",
        text: `${target.label} 连通核验超时，已回滚至 ${previous.label} (${previous.publicIp})`
      };
      pushToast(`核验超时，已回滚至 ${previous.label}`, true);
    } else {
      state.activeExitId = target.id;
      state.bannerNotice = null;
      pushToast(`已切换至 ${target.label} (${target.publicIp})`);
    }
    notify();
  }, 420);
}

export function pingAllExitNodes() {
  for (const node of state.exitNodes) {
    if (!node.healthy) {
      node.latencyMs = 299;
      continue;
    }
    const delta = Math.round((Math.random() - 0.5) * 10);
    node.latencyMs = Math.max(18, node.latencyMs + delta);
  }
  pushToast("延迟已刷新");
  notify();
}

export function addCustomRule({ type, payload, action }) {
  const cleanPayload = payload.trim();
  if (!cleanPayload) return false;
  state.rules.splice(2, 0, {
    id: `r-${Date.now()}`,
    type,
    payload: cleanPayload,
    action,
    hits: 0,
    enabled: true,
    locked: false
  });
  pushToast(`已添加规则：${cleanPayload}`);
  notify();
  return true;
}

export function importRulesFromText(rawText) {
  const lines = rawText
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#") && !l.startsWith("//"));

  const parsed = [];
  for (const line of lines) {
    const parts = line.split(",").map((s) => s.trim());
    if (parts.length >= 3) {
      const rawType = parts[0].toUpperCase();
      const validTypes = ["DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "PROCESS-NAME", "IP-CIDR", "RULE-SET"];
      const type = validTypes.includes(rawType) ? rawType : "DOMAIN-SUFFIX";
      const payload = parts[1];
      const rawAction = parts[2].toUpperCase();
      const action = ["EXIT", "DIRECT", "REJECT", "FALLBACK_PROXY"].includes(rawAction)
        ? rawAction
        : rawAction === "PROXY"
        ? "EXIT"
        : "DIRECT";
      if (payload) {
        parsed.push({
          id: `r-imp-${Date.now()}-${parsed.length}`,
          type,
          payload,
          action,
          hits: 0,
          enabled: true,
          locked: false
        });
      }
    }
  }
  if (parsed.length > 0) {
    state.rules.splice(2, 0, ...parsed);
    pushToast(`已导入 ${parsed.length} 条规则`);
    notify();
  } else {
    pushToast("未识别到有效规则", true);
  }
  return parsed.length;
}

export function toggleRuleEnabled(ruleId) {
  const rule = state.rules.find((r) => r.id === ruleId);
  if (!rule || rule.locked) return;
  rule.enabled = !rule.enabled;
  notify();
}

export function moveRule(ruleId, direction) {
  const idx = state.rules.findIndex((r) => r.id === ruleId);
  if (idx < 0) return;
  const targetIdx = idx + direction;
  if (targetIdx < 2 || targetIdx >= state.rules.length - 2) return;
  const [item] = state.rules.splice(idx, 1);
  state.rules.splice(targetIdx, 0, item);
  notify();
}

export function deleteRule(ruleId) {
  const idx = state.rules.findIndex((r) => r.id === ruleId);
  if (idx < 0 || state.rules[idx].locked) return;
  state.rules.splice(idx, 1);
  notify();
}

export function quickRouteFromConnection(connId, matchKind, action) {
  const conn = state.connections.find((c) => c.id === connId);
  if (!conn) return;
  const isProcess = matchKind === "process";
  const type = isProcess ? "PROCESS-NAME" : "DOMAIN-SUFFIX";
  const payload = isProcess ? conn.process : conn.host.split(".").slice(-2).join(".");
  addCustomRule({ type, payload, action });
  conn.routeAction = action;
  conn.matchedRule = payload;
  notify();
}

export function refreshAllProviders() {
  const now = new Date();
  const stamp = `11-27 ${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  for (const p of state.providers) {
    p.updatedAt = stamp;
  }
  pushToast("规则集已全部更新");
  notify();
}

export function addProvider({ tag, sourceName, url }) {
  if (!tag.trim() || !url.trim()) return false;
  state.providers.unshift({
    id: `p-${Date.now()}`,
    tag: tag.trim(),
    sourceName: sourceName.trim() || "Custom",
    url: url.trim(),
    entryCount: 412,
    updatedAt: "刚刚"
  });
  pushToast(`已添加订阅：${tag.trim()}`);
  notify();
  return true;
}

export function addOrganization({ name, account, nodeHostname, publicIp, region }) {
  const orgId = `org-${Date.now()}`;
  const cleanName = name.trim() || "New Tailnet";
  state.orgs.push({
    id: orgId,
    name: cleanName,
    account: account.trim() || "admin@team.org",
    localTailscaleIp: "100.108.64.19",
    stateDir: `data/ts-state-${orgId}`,
    lastSync: "刚刚"
  });

  const newNodeId = `ts-${Date.now()}`;
  state.exitNodes.unshift({
    id: newNodeId,
    orgId,
    orgName: cleanName,
    hostname: nodeHostname.trim() || "eu-fra-exit-01",
    label: `${region.trim() || "法兰克福"} 出口`,
    tailscaleIp: "100.108.12.1",
    publicIp: publicIp.trim() || "88.198.44.102",
    region: region.trim() || "DE · Frankfurt",
    relayMode: "直连",
    latencyMs: 164,
    online: true,
    healthy: true
  });

  for (const p of Object.values(state.trafficPeriods)) {
    p.nodeUsageGB[newNodeId] = 0;
  }

  pushToast(`已挂载组织：${cleanName}`);
  notify();
  return true;
}
