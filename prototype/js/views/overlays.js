// Floating Desktop Overlays: System Tray Popover, Ctrl+K Command Palette & Action Modals
import { getActiveExit } from "../core/store.js";
import { SAMPLE_IMPORT_RULES_TEXT } from "../data/mock-data.js";
import { esc, renderLatencyTag } from "../core/utils.js";

export function renderTrayFlyout(state) {
  const activeExit = getActiveExit();
  const downNow = state.tunEnabled ? state.downHistory[state.downHistory.length - 1] : 0;
  const upNow = state.tunEnabled ? state.upHistory[state.upHistory.length - 1] : 0;

  return `
    <div class="tray-head">
      <div style="display:flex;align-items:center;gap:7px;">
        <span class="status-dot ${state.tunEnabled ? "is-on" : ""}"></span>
        <span style="font-weight:700;font-size:12.5px;">SingTail</span>
      </div>
      <button
        type="button"
        class="sharp-switch"
        role="switch"
        aria-label="总开关"
        aria-checked="${state.tunEnabled}"
        data-action="toggle-tun"
      >
        <span class="sharp-switch-thumb"></span>
      </button>
    </div>

    <div style="padding:10px 12px;border-bottom:1px solid var(--border-hairline);background:var(--bg-surface);">
      <div style="display:flex;align-items:center;justify-content:space-between;font-family:var(--font-mono);font-size:11.5px;">
        <span style="color:var(--text-muted);">${esc(activeExit.publicIp)}</span>
        <span><span style="color:var(--accent-text);">↓${downNow.toFixed(1)}</span> / <span style="color:var(--status-cyan);">↑${upNow.toFixed(1)}</span> MB/s</span>
      </div>
      <div class="mode-seg-group" style="margin-top:8px;">
        <button type="button" class="mode-seg-btn ${state.routingMode === "rule" ? "is-active" : ""}" data-set-mode="rule">规则</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "global" ? "is-active" : ""}" data-set-mode="global">全局</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "direct" ? "is-active" : ""}" data-set-mode="direct">直连</button>
      </div>
    </div>

    <div style="max-height:172px;overflow-y:auto;">
      ${state.exitNodes
        .filter((n) => n.healthy)
        .map(
          (n) => `
          <button type="button" class="tray-node-item ${n.id === state.activeExitId ? "is-selected" : ""}" data-switch-exit="${esc(n.id)}">
            <span class="truncate">${n.id === state.activeExitId ? "● " : ""}${esc(n.label)}</span>
            ${renderLatencyTag(n.latencyMs, n.healthy)}
          </button>
        `
        )
        .join("")}
    </div>
  `;
}

export function renderModalContent(state) {
  switch (state.activeModal) {
    case "add-rule":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="新建规则">
          <div class="modal-head">
            <span>新建规则</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-rule" class="modal-body">
            <div style="display:grid;grid-template-columns:170px minmax(0,1fr);gap:10px;">
              <div class="form-field">
                <label class="form-label" for="rule-type-sel">类型</label>
                <select id="rule-type-sel" name="type" class="select-sharp">
                  <option value="DOMAIN-SUFFIX">DOMAIN-SUFFIX</option>
                  <option value="PROCESS-NAME">PROCESS-NAME</option>
                  <option value="IP-CIDR">IP-CIDR</option>
                  <option value="DOMAIN-KEYWORD">DOMAIN-KEYWORD</option>
                  <option value="RULE-SET">RULE-SET</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label" for="rule-action-sel">走向</label>
                <select id="rule-action-sel" name="action" class="select-sharp">
                  <option value="EXIT">↗ 走出口</option>
                  <option value="DIRECT">→ 直连</option>
                  <option value="FALLBACK_PROXY">⇄ 本地代理</option>
                  <option value="REJECT">✕ 拦截</option>
                </select>
              </div>
            </div>

            <div class="form-field">
              <label class="form-label" for="rule-payload-inp">匹配内容</label>
              <input id="rule-payload-inp" name="payload" type="text" required class="input-sharp font-mono-ui" placeholder="cursor.sh / WeChat.exe / 8.128.0.0/10" />
            </div>

            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-submit-rule">保存</button>
            </div>
          </form>
        </div>
      `;

    case "pick-app":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="点选程序">
          <div class="modal-head">
            <span>选择运行中程序</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <div class="modal-body" style="padding:0;">
            <table class="data-table">
              <thead>
                <tr>
                  <th style="width:130px;">进程</th>
                  <th>路径</th>
                  <th style="width:148px;text-align:right;">动作</th>
                </tr>
              </thead>
              <tbody>
                ${state.runningApps
                  .map(
                    (app) => `
                    <tr>
                      <td class="font-mono-ui" style="color:var(--text-primary);font-weight:600;">${esc(app.process)}</td>
                      <td class="font-mono-ui" title="${esc(app.path)}" style="font-size:11px;color:var(--text-subtle);">${esc(app.path)}</td>
                      <td style="text-align:right;">
                        <button type="button" class="btn-hairline btn-sm" data-pick-app-rule="${esc(app.process)}:DIRECT">直连</button>
                        <button type="button" class="btn-lime btn-sm" data-pick-app-rule="${esc(app.process)}:EXIT">出口</button>
                      </td>
                    </tr>
                  `
                  )
                  .join("")}
              </tbody>
            </table>
          </div>
        </div>
      `;

    case "import-rules":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="导入规则">
          <div class="modal-head">
            <span>导入规则文件</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-import-rules" class="modal-body">
            <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
              <span class="form-label">格式：类型,匹配值,走向</span>
              <label class="btn-hairline btn-sm" style="cursor:pointer;">
                选择文件
                <input id="rule-file-picker" type="file" accept=".txt,.conf,.yaml,.yml,.json" hidden />
              </label>
            </div>
            <textarea id="import-rules-textarea" name="rawRules" rows="6" class="textarea-sharp">${esc(SAMPLE_IMPORT_RULES_TEXT)}</textarea>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-confirm-import">导入</button>
            </div>
          </form>
        </div>
      `;

    case "add-org":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="挂载组织">
          <div class="modal-head">
            <span>挂载 Tailscale 组织</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-org" class="modal-body">
            <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px;">
              <div class="form-field">
                <label class="form-label" for="org-name-inp">组织名称</label>
                <input id="org-name-inp" name="name" type="text" required class="input-sharp" value="EU Lab Tailnet" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-acc-inp">账号</label>
                <input id="org-acc-inp" name="account" type="text" required class="input-sharp font-mono-ui" value="infra@eulab.dev" />
              </div>
            </div>
            <div style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;">
              <div class="form-field">
                <label class="form-label" for="org-node-inp">出口主机名</label>
                <input id="org-node-inp" name="nodeHostname" type="text" class="input-sharp font-mono-ui" value="eu-fra-exit-01" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-ip-inp">公网 IP</label>
                <input id="org-ip-inp" name="publicIp" type="text" class="input-sharp font-mono-ui" value="88.198.44.102" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-reg-inp">区域</label>
                <input id="org-reg-inp" name="region" type="text" class="input-sharp" value="DE · 法兰克福" />
              </div>
            </div>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-confirm-add-org">登录授权</button>
            </div>
          </form>
        </div>
      `;

    case "add-provider":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="添加订阅">
          <div class="modal-head">
            <span>添加规则集订阅</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-provider" class="modal-body">
            <div class="form-field">
              <label class="form-label" for="prov-tag-inp">名称</label>
              <input id="prov-tag-inp" name="tag" type="text" required class="input-sharp font-mono-ui" value="dustinwin-ai" />
            </div>
            <div class="form-field">
              <label class="form-label" for="prov-src-inp">来源</label>
              <input id="prov-src-inp" name="sourceName" type="text" class="input-sharp" value="DustinWin" />
            </div>
            <div class="form-field">
              <label class="form-label" for="prov-url-inp">订阅链接</label>
              <input id="prov-url-inp" name="url" type="url" required class="input-sharp font-mono-ui" value="https://raw.githubusercontent.com/DustinWin/ruleset_geodata/sing-box-ruleset/ai.srs" />
            </div>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime">保存</button>
            </div>
          </form>
        </div>
      `;

    case "export-config": {
      const sanitized = {
        version: "1.0.0",
        customRules: state.rules
          .filter((r) => !r.locked)
          .map(({ type, payload, action, enabled }) => ({ type, payload, action, enabled })),
        providers: state.providers.map(({ tag, url }) => ({ tag, url }))
      };
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="导出配置">
          <div class="modal-head">
            <span>导出脱敏配置</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <div class="modal-body">
            <textarea readonly rows="9" class="textarea-sharp">${esc(JSON.stringify(sanitized, null, 2))}</textarea>
          </div>
          <div class="modal-foot">
            <button type="button" class="btn-hairline" data-close-modal>关闭</button>
            <button type="button" class="btn-lime" data-action="copy-export-json">复制 JSON</button>
          </div>
        </div>
      `;
    }

    default:
      return "";
  }
}

export function renderCmdkModal(state) {
  const q = state.cmdkQuery.trim().toLowerCase();
  const items = [
    ...state.exitNodes
      .filter((n) => n.healthy)
      .map((n) => ({
        kind: "出口",
        label: `${n.label} (${n.publicIp})`,
        sub: n.orgName,
        cmd: `exit:${n.id}`
      })),
    { kind: "视图", label: "分流规则", sub: "", cmd: "view:rules" },
    { kind: "视图", label: "规则集", sub: "", cmd: "view:providers" },
    { kind: "视图", label: "实时连接", sub: "", cmd: "view:connections" },
    { kind: "视图", label: "设置", sub: "", cmd: "view:settings" },
    { kind: "动作", label: "新建规则", sub: "", cmd: "modal:add-rule" },
    { kind: "动作", label: "导入规则文件", sub: "", cmd: "modal:import-rules" }
  ].filter((it) => !q || it.label.toLowerCase().includes(q) || it.sub.toLowerCase().includes(q));

  return `
    <div class="modal-panel" style="width:min(480px,100%);" role="dialog" aria-modal="true" aria-label="快速检索">
      <div style="display:flex;align-items:center;gap:8px;padding:10px 14px;border-bottom:1px solid var(--border-hairline);background:var(--bg-code);">
        <span class="font-mono-ui" style="color:var(--accent-text);">›</span>
        <input
          id="cmdk-search-input"
          type="search"
          placeholder="搜索出口、视图或动作…"
          value="${esc(state.cmdkQuery)}"
          style="width:100%;border:0;background:transparent;outline:none;font-size:13px;"
        />
        <button type="button" class="tag-sharp" data-action="close-cmdk" style="cursor:pointer;">ESC</button>
      </div>
      <div style="max-height:280px;overflow-y:auto;">
        ${items
          .map(
            (it) => `
            <button
              type="button"
              data-cmdk-exec="${esc(it.cmd)}"
              style="display:flex;align-items:center;justify-content:space-between;width:100%;height:36px;padding:0 14px;background:transparent;border:0;border-bottom:1px solid var(--border-hairline);cursor:pointer;text-align:left;"
            >
              <div style="display:flex;align-items:center;gap:8px;min-width:0;">
                <span class="tag-sharp">${esc(it.kind)}</span>
                <span class="truncate" style="color:var(--text-primary);">${esc(it.label)}</span>
              </div>
              <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">${esc(it.sub)}</span>
            </button>
          `
          )
          .join("")}
      </div>
    </div>
  `;
}
