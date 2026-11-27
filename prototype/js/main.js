// SingTail Desktop Prototype — Main Orchestrator & Event Delegation
import {
  state,
  subscribe,
  notify,
  pushToast,
  getActiveExit,
  setView,
  toggleTheme,
  toggleTun,
  setRoutingMode,
  switchExitNode,
  pingAllExitNodes,
  toggleClashHandover,
  addCustomRule,
  importRulesFromText,
  toggleRuleEnabled,
  moveRule,
  deleteRule,
  quickRouteFromConnection,
  refreshAllProviders,
  addProvider,
  addOrganization
} from "./core/store.js";
import { esc } from "./core/utils.js";
import { renderOverviewView, drawSparklineCanvas } from "./views/overview.js";
import { renderRulesView } from "./views/rules.js";
import { renderProvidersView } from "./views/providers.js";
import { renderConnectionsView } from "./views/connections.js";
import { renderSettingsView } from "./views/settings.js";
import { renderTrayFlyout, renderModalContent, renderCmdkModal } from "./views/overlays.js";

const NAV_ITEMS = [
  {
    id: "overview",
    label: "总览与出口",
    countFn: (s) => s.exitNodes.length,
    icon: `<svg class="rail-nav-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2" y="2" width="5" height="5"/><rect x="9" y="2" width="5" height="5"/><rect x="2" y="9" width="5" height="5"/><rect x="9" y="9" width="5" height="5"/></svg>`
  },
  {
    id: "rules",
    label: "分流规则",
    countFn: (s) => s.rules.length,
    icon: `<svg class="rail-nav-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 4h12M2 8h8M2 12h10"/><circle cx="13" cy="8" r="1.5"/></svg>`
  },
  {
    id: "providers",
    label: "规则集与订阅",
    countFn: (s) => s.providers.length,
    icon: `<svg class="rail-nav-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2.5 3.5h11v9h-11z"/><path d="M2.5 6.5h11M6 6.5v6"/></svg>`
  },
  {
    id: "connections",
    label: "实时连接",
    countFn: (s) => s.connections.length,
    icon: `<svg class="rail-nav-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 10l3-4 3 5 3-6 3 5"/></svg>`
  },
  {
    id: "settings",
    label: "组织与设置",
    countFn: (s) => s.orgs.length,
    icon: `<svg class="rail-nav-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="2.5"/><path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4"/></svg>`
  }
];

function renderSidebar() {
  const rail = document.getElementById("sidebar-rail");
  if (!rail) return;
  const activeExit = getActiveExit();

  rail.innerHTML = `
    <div class="rail-engine-box">
      <div class="engine-top-row">
        <div class="engine-status-label">
          <span class="status-dot ${state.tunEnabled ? "is-on" : state.clashHandover ? "is-amber" : ""}"></span>
          <span>${state.clashHandover ? "已让路 Clash" : state.tunEnabled ? "TUN 虚拟网卡" : "引擎已停用"}</span>
        </div>
        <button
          type="button"
          class="sharp-switch"
          role="switch"
          aria-label="TUN 总开关"
          aria-checked="${state.tunEnabled}"
          data-action="toggle-tun"
          data-testid="switch-master-tun"
        >
          <span class="sharp-switch-thumb"></span>
        </button>
      </div>

      <div class="mode-seg-group" role="group" aria-label="全局分流模式">
        <button type="button" class="mode-seg-btn ${state.routingMode === "rule" ? "is-active" : ""}" data-set-mode="rule">规则</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "global" ? "is-active" : ""}" data-set-mode="global">全局</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "direct" ? "is-active" : ""}" data-set-mode="direct">直连</button>
      </div>

      <div class="engine-exit-mini">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:4px;">
          <span class="eyebrow-mono" style="font-size:9.5px;">ACTIVE EXIT</span>
          <span class="font-mono-ui" style="font-size:10.5px;color:var(--accent-text);">${activeExit.latencyMs} ms</span>
        </div>
        <div class="truncate" style="font-size:12.5px;font-weight:600;color:var(--text-primary);">${esc(activeExit.label)}</div>
        <div class="font-mono-ui truncate" style="font-size:11px;color:var(--text-muted);">${esc(activeExit.publicIp)} · ${esc(activeExit.orgName)}</div>
      </div>
    </div>

    <nav class="rail-nav" aria-label="主功能导航">
      <div class="rail-group-label">工作台 (WORKBENCH)</div>
      ${NAV_ITEMS.map(
        (item) => `
        <button
          type="button"
          class="rail-nav-btn ${state.activeView === item.id ? "is-active" : ""}"
          data-nav-view="${item.id}"
          data-testid="nav-${item.id}"
        >
          <span class="rail-nav-left">
            ${item.icon}
            <span class="truncate">${esc(item.label)}</span>
          </span>
          <span class="rail-count">${item.countFn(state)}</span>
        </button>
      `
      ).join("")}
    </nav>

    <div class="rail-footer">
      <button type="button" class="btn-hairline" style="width:100%;justify-content:space-between;" data-action="toggle-clash" data-testid="btn-rail-clash">
        <span>${state.clashHandover ? "⇄ 接管回 SingTail" : "⇄ 让路至 Clash Verge"}</span>
        <span class="font-mono-ui" style="font-size:10.5px;color:var(--text-subtle);">互斥</span>
      </button>
    </div>
  `;
}

function renderStatusBar() {
  const bar = document.getElementById("window-statusbar");
  if (!bar) return;
  const totalProviderRules = state.providers.reduce((acc, p) => acc + p.entryCount, 0);

  bar.innerHTML = `
    <div class="statusbar-seg">
      <span class="status-kv">
        <span class="status-dot ${state.tunEnabled ? "is-green" : ""}"></span>
        <span>网卡 singbox (MTU ${state.settings.mtu})</span>
      </span>
      <span class="status-divider"></span>
      <span class="status-kv">混合端口 127.0.0.1:${state.settings.localMixedPort}</span>
      <span class="status-divider"></span>
      <span class="status-kv">DNS 223.5.5.5 / 1.1.1.1 DoH</span>
    </div>
    <div class="statusbar-seg">
      <span class="status-kv">规则库 ${totalProviderRules.toLocaleString()} 条</span>
      <span class="status-divider"></span>
      <span class="status-kv">后台内存 26.8 MB</span>
    </div>
  `;
}

function renderTitlebarControls() {
  const themeBtn = document.getElementById("btn-toggle-theme");
  if (themeBtn) {
    themeBtn.textContent = state.theme === "dark" ? "◐ 暗色" : "◑ 亮色";
  }
  const trayBtn = document.getElementById("btn-toggle-tray");
  if (trayBtn) {
    trayBtn.classList.toggle("is-active", state.trayOpen);
  }
}

function renderWorkspace() {
  const main = document.getElementById("workspace-main");
  if (!main) return;

  // Preserve focused input & caret across reactive renders
  const activeEl = document.activeElement;
  const focusedId = activeEl && activeEl.id ? activeEl.id : null;
  const selStart = activeEl && typeof activeEl.selectionStart === "number" ? activeEl.selectionStart : null;
  const selEnd = activeEl && typeof activeEl.selectionEnd === "number" ? activeEl.selectionEnd : null;

  switch (state.activeView) {
    case "overview":
      main.innerHTML = renderOverviewView(state);
      requestAnimationFrame(() => drawSparklineCanvas(state));
      break;
    case "rules":
      main.innerHTML = renderRulesView(state);
      break;
    case "providers":
      main.innerHTML = renderProvidersView(state);
      break;
    case "connections":
      main.innerHTML = renderConnectionsView(state);
      break;
    case "settings":
      main.innerHTML = renderSettingsView(state);
      break;
    default:
      main.innerHTML = renderOverviewView(state);
  }

  if (focusedId) {
    const nextEl = document.getElementById(focusedId);
    if (nextEl) {
      nextEl.focus();
      if (selStart !== null && typeof nextEl.setSelectionRange === "function") {
        nextEl.setSelectionRange(selStart, selEnd);
      }
    }
  }
}

function renderOverlays() {
  const trayEl = document.getElementById("tray-flyout");
  if (trayEl) {
    trayEl.hidden = !state.trayOpen;
    if (state.trayOpen) {
      trayEl.innerHTML = renderTrayFlyout(state);
    }
  }

  const modalBackdrop = document.getElementById("modal-backdrop");
  if (modalBackdrop) {
    const hasModal = Boolean(state.activeModal);
    modalBackdrop.hidden = !hasModal;
    modalBackdrop.innerHTML = hasModal ? renderModalContent(state) : "";
  }

  const cmdkBackdrop = document.getElementById("cmdk-backdrop");
  if (cmdkBackdrop) {
    const wasHidden = cmdkBackdrop.hidden;
    cmdkBackdrop.hidden = !state.cmdkOpen;
    if (state.cmdkOpen) {
      const prevCaret = document.getElementById("cmdk-search-input")?.selectionStart ?? null;
      cmdkBackdrop.innerHTML = renderCmdkModal(state);
      const inp = document.getElementById("cmdk-search-input");
      if (inp) {
        inp.focus();
        const pos = prevCaret ?? inp.value.length;
        inp.setSelectionRange(pos, pos);
      }
    } else if (!wasHidden) {
      cmdkBackdrop.innerHTML = "";
    }
  }
}

function renderAll() {
  renderTitlebarControls();
  renderSidebar();
  renderWorkspace();
  renderStatusBar();
  renderOverlays();
}

subscribe(renderAll);

// Global Click Delegation
document.addEventListener("click", (e) => {
  const target = e.target.closest("button, [data-select-conn]");
  if (!target) {
    if (e.target.id === "modal-backdrop") {
      state.activeModal = null;
      notify();
    } else if (e.target.id === "cmdk-backdrop") {
      state.cmdkOpen = false;
      notify();
    }
    return;
  }

  if (target.dataset.navView) {
    setView(target.dataset.navView);
    return;
  }

  if (target.dataset.setMode) {
    setRoutingMode(target.dataset.setMode);
    return;
  }

  if (target.dataset.orgFilter) {
    state.selectedOrgFilter = target.dataset.orgFilter;
    notify();
    return;
  }

  if (target.dataset.switchExit) {
    switchExitNode(target.dataset.switchExit);
    return;
  }

  if (target.dataset.ruleFilter) {
    state.ruleFilterAction = target.dataset.ruleFilter;
    notify();
    return;
  }

  if (target.dataset.ruleToggle) {
    toggleRuleEnabled(target.dataset.ruleToggle);
    return;
  }

  if (target.dataset.ruleUp) {
    moveRule(target.dataset.ruleUp, -1);
    return;
  }

  if (target.dataset.ruleDown) {
    moveRule(target.dataset.ruleDown, 1);
    return;
  }

  if (target.dataset.ruleDel) {
    deleteRule(target.dataset.ruleDel);
    return;
  }

  if (target.dataset.connFilter) {
    state.connFilterAction = target.dataset.connFilter;
    notify();
    return;
  }

  if (target.dataset.selectConn) {
    state.selectedConnId = target.dataset.selectConn;
    notify();
    return;
  }

  if (target.dataset.quickRule) {
    const [connId, matchKind, action] = target.dataset.quickRule.split(":");
    quickRouteFromConnection(connId, matchKind, action);
    return;
  }

  if (target.dataset.pickAppRule) {
    const [proc, action] = target.dataset.pickAppRule.split(":");
    addCustomRule({
      type: "PROCESS-NAME",
      payload: proc,
      action,
      note: "点选运行中程序添加"
    });
    state.activeModal = null;
    notify();
    return;
  }

  if (target.dataset.openModal) {
    state.activeModal = target.dataset.openModal;
    notify();
    return;
  }

  if (target.hasAttribute("data-close-modal")) {
    state.activeModal = null;
    notify();
    return;
  }

  if (target.dataset.settingToggle) {
    const key = target.dataset.settingToggle;
    state.settings[key] = !state.settings[key];
    pushToast("系统设置已立即保存生效");
    notify();
    return;
  }

  if (target.dataset.cmdkExec) {
    const [kind, val] = target.dataset.cmdkExec.split(":");
    state.cmdkOpen = false;
    if (kind === "exit") switchExitNode(val);
    else if (kind === "view") setView(val);
    else if (kind === "modal") {
      state.activeModal = val;
      notify();
    } else if (kind === "action" && val === "clash") {
      toggleClashHandover();
    }
    return;
  }

  switch (target.dataset.action) {
    case "toggle-tun":
      toggleTun();
      break;
    case "toggle-theme":
      toggleTheme();
      break;
    case "toggle-tray":
      state.trayOpen = !state.trayOpen;
      notify();
      break;
    case "close-tray":
      state.trayOpen = false;
      notify();
      break;
    case "open-cmdk":
      state.cmdkOpen = true;
      state.cmdkQuery = "";
      notify();
      break;
    case "close-cmdk":
      state.cmdkOpen = false;
      notify();
      break;
    case "ping-nodes":
      pingAllExitNodes();
      break;
    case "toggle-clash":
      toggleClashHandover();
      break;
    case "dismiss-banner":
      state.bannerNotice = null;
      notify();
      break;
    case "reset-rule-filter":
      state.ruleFilterAction = "ALL";
      state.ruleSearch = "";
      notify();
      break;
    case "reset-conn-filter":
      state.connFilterAction = "ALL";
      state.connSearch = "";
      notify();
      break;
    case "toggle-conn-pause":
      state.connPaused = !state.connPaused;
      notify();
      break;
    case "refresh-providers":
      refreshAllProviders();
      break;
    case "sync-org":
      pushToast("已同步组织内最新出口节点状态");
      break;
    case "copy-export-json":
      pushToast("脱敏配置 JSON 已复制到剪贴板");
      state.activeModal = null;
      notify();
      break;
    case "win-minimize":
      state.trayOpen = true;
      pushToast("窗口最小化时驻留至右下角系统托盘");
      notify();
      break;
    default:
      break;
  }
});

// Input & File Change Delegation
document.addEventListener("input", (e) => {
  const el = e.target;
  if (el.id === "rule-search-input") {
    state.ruleSearch = el.value;
    renderWorkspace();
  } else if (el.id === "conn-search-input") {
    state.connSearch = el.value;
    renderWorkspace();
  } else if (el.id === "cmdk-search-input") {
    state.cmdkQuery = el.value;
    renderOverlays();
  } else if (el.dataset.settingInput) {
    state.settings[el.dataset.settingInput] = el.value;
  }
});

document.addEventListener("change", async (e) => {
  const el = e.target;
  if (el.id === "rule-file-picker" && el.files && el.files[0]) {
    const text = await el.files[0].text();
    const area = document.getElementById("import-rules-textarea");
    if (area) area.value = text;
    pushToast(`已读取文件：${el.files[0].name}`);
  }
});

// Modal Form Submissions
document.addEventListener("submit", (e) => {
  e.preventDefault();
  const form = e.target;
  const fd = new FormData(form);

  if (form.id === "form-add-rule") {
    const ok = addCustomRule({
      type: String(fd.get("type") || "DOMAIN-SUFFIX"),
      payload: String(fd.get("payload") || ""),
      action: String(fd.get("action") || "EXIT"),
      note: String(fd.get("note") || "")
    });
    if (ok) {
      state.activeModal = null;
      state.activeView = "rules";
      notify();
    }
  } else if (form.id === "form-import-rules") {
    const count = importRulesFromText(String(fd.get("rawRules") || ""));
    if (count > 0) {
      state.activeModal = null;
      state.activeView = "rules";
      notify();
    }
  } else if (form.id === "form-add-org") {
    addOrganization({
      name: String(fd.get("name") || ""),
      account: String(fd.get("account") || ""),
      nodeHostname: String(fd.get("nodeHostname") || ""),
      publicIp: String(fd.get("publicIp") || ""),
      region: String(fd.get("region") || "")
    });
    state.activeModal = null;
    notify();
  } else if (form.id === "form-add-provider") {
    const ok = addProvider({
      tag: String(fd.get("tag") || ""),
      sourceName: String(fd.get("sourceName") || ""),
      url: String(fd.get("url") || "")
    });
    if (ok) {
      state.activeModal = null;
      state.activeView = "providers";
      notify();
    }
  }
});

// Keyboard Shortcuts (Ctrl+K / Esc)
document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    state.cmdkOpen = !state.cmdkOpen;
    state.cmdkQuery = "";
    notify();
  } else if (e.key === "Escape") {
    if (state.cmdkOpen || state.activeModal || state.trayOpen) {
      state.cmdkOpen = false;
      state.activeModal = null;
      state.trayOpen = false;
      notify();
    }
  }
});

window.addEventListener("resize", () => {
  if (state.activeView === "overview") {
    drawSparklineCanvas(state);
  }
});

// Non-intrusive 1.2s telemetry updater (never steals input focus)
setInterval(() => {
  if (!state.tunEnabled) {
    state.downHistory.push(0);
    state.downHistory.shift();
    state.upHistory.push(0);
    state.upHistory.shift();
  } else {
    const prev = state.downHistory[state.downHistory.length - 1] || 6.0;
    const nextDown = Math.max(0.9, Math.min(14.2, +(prev + (Math.random() - 0.48) * 2.1).toFixed(1)));
    const nextUp = Math.max(0.2, Math.min(3.6, +(nextDown * 0.16 + Math.random() * 0.35).toFixed(1)));
    state.downHistory.push(nextDown);
    state.downHistory.shift();
    state.upHistory.push(nextUp);
    state.upHistory.shift();
  }

  const downEl = document.getElementById("live-down-val");
  const upEl = document.getElementById("live-up-val");
  const dNow = state.downHistory[state.downHistory.length - 1];
  const uNow = state.upHistory[state.upHistory.length - 1];
  if (downEl) downEl.textContent = `↓ ${dNow.toFixed(1)}`;
  if (upEl) upEl.textContent = `↑ ${uNow.toFixed(1)} MB/s`;
  if (state.activeView === "overview") {
    drawSparklineCanvas(state);
  }
}, 1200);

// Initial mount
renderAll();
