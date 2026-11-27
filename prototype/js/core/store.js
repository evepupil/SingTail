// Reactive application store for SingTail Desktop Prototype
import {
  INITIAL_ORGS,
  INITIAL_EXIT_NODES,
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
  clashHandover: false,
  routingMode: "rule", // rule | global | direct
  selectedOrgFilter: "all",
  activeExitId: "ts-us-macmini",
  verifyingExitId: null,
  bannerNotice: null, // { kind: 'rollback' | 'verifying' | 'clash', text: string }

  orgs: structuredClone(INITIAL_ORGS),
  exitNodes: structuredClone(INITIAL_EXIT_NODES),
  rules: structuredClone(INITIAL_RULES),
  providers: structuredClone(INITIAL_PROVIDERS),
  connections: structuredClone(INITIAL_CONNECTIONS),
  runningApps: structuredClone(RUNNING_APPS),

  // Filters & UI selections
  ruleFilterAction: "ALL",
  ruleSearch: "",
  connFilterAction: "ALL",
  connSearch: "",
  connPaused: false,
  selectedConnId: "c-101",

  // Telemetry sparkline history (36 points, MB/s)
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

  // System & Fallback settings
  settings: {
    autoStartService: true,
    autoVerifyIp: true,
    mtu: 1280,
    ipv4Only: true,
    bypassTailnetCidr: "100.64.0.0/10",
    localMixedPort: 7899,
    cnDns: "223.5.5.5",
    exitDoh: "https://1.1.1.1/dns-query",
    fallbackProxyEnabled: true,
    fallbackProxyAddr: "127.0.0.1:7897",
    clashExePath: "C:\\Program Files\\Clash Verge\\clash-verge.exe",
    autoUpdateRulesDays: 14
  },

  // Floating overlays
  trayOpen: false,
  cmdkOpen: false,
  cmdkQuery: "",
  activeModal: null // null | 'add-rule' | 'pick-app' | 'import-rules' | 'add-org' | 'add-provider' | 'export-config'
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
  }, 3000);
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
  if (state.clashHandover) {
    state.clashHandover = false;
    state.tunEnabled = true;
    state.bannerNotice = null;
    pushToast("已退出外部代理让路，SingTail 虚拟网卡重新接管");
    notify();
    return;
  }
  state.tunEnabled = !state.tunEnabled;
  pushToast(state.tunEnabled ? "虚拟网卡 singbox 已挂载" : "虚拟网卡已卸载，恢复系统普通直连");
  notify();
}

export function setRoutingMode(mode) {
  state.routingMode = mode;
  const labelMap = { rule: "规则分流", global: "全局出口", direct: "全部直连" };
  pushToast(`分流模式已切为：${labelMap[mode]}`);
  notify();
}

export function switchExitNode(targetId) {
  if (state.verifyingExitId) return;
  const target = state.exitNodes.find((n) => n.id === targetId);
  if (!target) return;
  if (target.id === state.activeExitId) {
    pushToast(`当前已在出口：${target.label}`);
    return;
  }

  const previous = getActiveExit();
  state.verifyingExitId = target.id;
  state.bannerNotice = {
    kind: "verifying",
    text: `正在切换至 ${target.hostname} (${target.tailscaleIp}) 并实测核验出口公网 IP…`
  };
  notify();

  setTimeout(() => {
    state.verifyingExitId = null;
    if (!target.healthy) {
      state.bannerNotice = {
        kind: "rollback",
        text: `出口 ${target.hostname} 公网连通核验超时，已自动回滚至 ${previous.label} (${previous.publicIp})`
      };
      pushToast(`核验失败，已安全回滚至 ${previous.hostname}`, true);
    } else {
      state.activeExitId = target.id;
      state.bannerNotice = null;
      pushToast(`已切换至 ${target.label} · 实测公网 IP ${target.publicIp}`);
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
    const delta = Math.round((Math.random() - 0.5) * 12);
    node.latencyMs = Math.max(18, node.latencyMs + delta);
  }
  pushToast("已完成全部组织出口节点延迟探测");
  notify();
}

export function toggleClashHandover() {
  state.clashHandover = !state.clashHandover;
  if (state.clashHandover) {
    state.tunEnabled = false;
    state.bannerNotice = {
      kind: "clash",
      text: "已安全卸载 SingTail 虚拟网卡并拉起 Clash Verge；点击右侧「接管回 SingTail」可随时切回。"
    };
    pushToast("已让路并拉起 Clash Verge");
  } else {
    state.tunEnabled = true;
    state.bannerNotice = null;
    pushToast("已优雅关闭 Clash Verge 并恢复 SingTail 接管");
  }
  notify();
}

export function addCustomRule({ type, payload, action, note }) {
  const cleanPayload = payload.trim();
  if (!cleanPayload) return false;
  const newRule = {
    id: `r-${Date.now()}`,
    type,
    payload: cleanPayload,
    action,
    note: note.trim() || "用户自定义规则",
    hits: 0,
    enabled: true,
    locked: false
  };
  // Insert right after locked head rules (index 2) so custom rules take top priority
  state.rules.splice(2, 0, newRule);
  pushToast(`已添加高优先级规则：${type} · ${cleanPayload}`);
  notify();
  return true;
}

export function importRulesFromText(rawText) {
  const lines = rawText
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#") && !l.startsWith("//"));

  let added = 0;
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
          id: `r-imp-${Date.now()}-${added}`,
          type,
          payload,
          action,
          note: "规则文件导入",
          hits: 0,
          enabled: true,
          locked: false
        });
        added += 1;
      }
    }
  }
  if (added > 0) {
    state.rules.splice(2, 0, ...parsed);
    pushToast(`校验通过，成功导入 ${added} 条分流规则`);
    notify();
  } else {
    pushToast("未识别到有效规则行，请检查格式", true);
  }
  return added;
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
  const [removed] = state.rules.splice(idx, 1);
  pushToast(`已移除规则：${removed.payload}`);
  notify();
}

export function quickRouteFromConnection(connId, matchKind, action) {
  const conn = state.connections.find((c) => c.id === connId);
  if (!conn) return;
  const isProcess = matchKind === "process";
  const type = isProcess ? "PROCESS-NAME" : "DOMAIN-SUFFIX";
  const payload = isProcess ? conn.process : conn.host.split(".").slice(-2).join(".");
  addCustomRule({
    type,
    payload,
    action,
    note: "从实时连接检视器快捷创建"
  });
  conn.routeAction = action;
  conn.matchedRule = `${type} · ${payload}`;
  notify();
}

export function refreshAllProviders() {
  const now = new Date();
  const stamp = `2026-11-27 ${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  for (const p of state.providers) {
    p.updatedAt = stamp;
    p.verified = true;
  }
  pushToast(`已更新并反编译校验全部 ${state.providers.length} 个规则集`);
  notify();
}

export function addProvider({ tag, sourceName, url }) {
  if (!tag.trim() || !url.trim()) return false;
  state.providers.unshift({
    id: `p-${Date.now()}`,
    tag: tag.trim(),
    format: "SRS Binary",
    sourceName: sourceName.trim() || "自定义远程订阅",
    url: url.trim(),
    entryCount: 412,
    updatedAt: "刚刚",
    badge: "订阅规则集",
    verified: true
  });
  pushToast(`已添加规则订阅：${tag.trim()}`);
  notify();
  return true;
}

export function addOrganization({ name, account, nodeHostname, publicIp, region }) {
  const orgId = `org-${Date.now()}`;
  const cleanName = name.trim() || "New Team Tailnet";
  state.orgs.push({
    id: orgId,
    name: cleanName,
    account: account.trim() || "admin@team.org",
    tailnetDomain: `${cleanName.toLowerCase().replace(/[^a-z0-9]/g, "")}.ts.net`,
    localNodeName: `singtail-${cleanName.toLowerCase().slice(0, 6)}-win`,
    localTailscaleIp: "100.108.64.19",
    stateDir: `data/ts-state-${orgId}`,
    authStatus: "authenticated",
    lastSync: "刚刚"
  });

  state.exitNodes.unshift({
    id: `ts-${Date.now()}`,
    orgId,
    orgName: cleanName,
    hostname: nodeHostname.trim() || "eu-fra-exit-01",
    label: `${region.trim() || "法兰克福"} 团队出口`,
    tailscaleIp: "100.108.12.1",
    publicIp: publicIp.trim() || "88.198.44.102",
    region: region.trim() || "DE · Frankfurt",
    ispType: "Hetzner",
    relayMode: "P2P 直连",
    latencyMs: 164,
    online: true,
    healthy: true
  });

  pushToast(`已完成独立端口授权并挂载组织：${cleanName}`);
  notify();
  return true;
}

export function tickTelemetry() {
  if (!state.tunEnabled) {
    state.downHistory.push(0);
    state.downHistory.shift();
    state.upHistory.push(0);
    state.upHistory.shift();
    notify();
    return;
  }
  const lastDown = state.downHistory[state.downHistory.length - 1] || 5.0;
  const nextDown = Math.max(0.8, Math.min(14.5, +(lastDown + (Math.random() - 0.48) * 2.4).toFixed(1)));
  const nextUp = Math.max(0.2, Math.min(3.8, +(nextDown * 0.17 + Math.random() * 0.4).toFixed(1)));
  state.downHistory.push(nextDown);
  state.downHistory.shift();
  state.upHistory.push(nextUp);
  state.upHistory.shift();
  notify();
}
