// Floating Desktop Overlays: Windows System Tray Popover, Ctrl+K Command Palette & Action Modals
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
        <span style="font-weight:700;font-size:12.5px;">SingTail 托盘快捷面板</span>
      </div>
      <button
        type="button"
        class="sharp-switch"
        role="switch"
        aria-label="托盘总开关"
        aria-checked="${state.tunEnabled}"
        data-action="toggle-tun"
      >
        <span class="sharp-switch-thumb"></span>
      </button>
    </div>

    <div style="padding:10px 12px;border-bottom:1px solid var(--border-hairline);background:var(--bg-surface);">
      <div style="display:flex;align-items:center;justify-content:space-between;font-family:var(--font-mono);font-size:11.5px;">
        <span style="color:var(--text-muted);">公网 ${esc(activeExit.publicIp)}</span>
        <span><span style="color:var(--accent-text);">↓${downNow.toFixed(1)}</span> / <span style="color:var(--status-cyan);">↑${upNow.toFixed(1)}</span> MB/s</span>
      </div>
      <div class="mode-seg-group" style="margin-top:8px;">
        <button type="button" class="mode-seg-btn ${state.routingMode === "rule" ? "is-active" : ""}" data-set-mode="rule">规则分流</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "global" ? "is-active" : ""}" data-set-mode="global">全局出口</button>
        <button type="button" class="mode-seg-btn ${state.routingMode === "direct" ? "is-active" : ""}" data-set-mode="direct">全部直连</button>
      </div>
    </div>

    <div class="eyebrow-mono" style="padding:7px 12px 4px;background:var(--bg-code);border-bottom:1px solid var(--border-hairline);">
      快速切换出口节点
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

    <div style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px;background:var(--bg-code);">
      <button type="button" class="btn-hairline btn-sm" data-action="toggle-clash">
        ${state.clashHandover ? "接管回 SingTail" : "⇄ 让路 Clash Verge"}
      </button>
      <button type="button" class="btn-hairline btn-sm" data-action="close-tray">收起</button>
    </div>
  `;
}

export function renderModalContent(state) {
  switch (state.activeModal) {
    case "add-rule":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="新建分流规则">
          <div class="modal-head">
            <span>新建个人分流规则</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-rule" class="modal-body">
            <div style="display:grid;grid-template-columns:160px minmax(0,1fr);gap:10px;">
              <div class="form-field">
                <label class="form-label" for="rule-type-sel">匹配类型</label>
                <select id="rule-type-sel" name="type" class="select-sharp">
                  <option value="DOMAIN-SUFFIX">DOMAIN-SUFFIX (域名后缀)</option>
                  <option value="PROCESS-NAME">PROCESS-NAME (程序进程名)</option>
                  <option value="IP-CIDR">IP-CIDR (IP 网段)</option>
                  <option value="DOMAIN-KEYWORD">DOMAIN-KEYWORD (域名关键字)</option>
                  <option value="RULE-SET">RULE-SET (本地规则集)</option>
                </select>
              </div>
              <div class="form-field">
                <label class="form-label" for="rule-action-sel">分流走向</label>
                <select id="rule-action-sel" name="action" class="select-sharp">
                  <option value="EXIT">↗ 走当前选中出口</option>
                  <option value="DIRECT">→ 直连本机宽带</option>
                  <option value="FALLBACK_PROXY">⇄ 走兜底外部代理</option>
                  <option value="REJECT">✕ 直接拦截阻断</option>
                </select>
              </div>
            </div>

            <div class="form-field">
              <label class="form-label" for="rule-payload-inp">匹配值（多个用英文逗号分隔）</label>
              <input id="rule-payload-inp" name="payload" type="text" required class="input-sharp font-mono-ui" placeholder="例如：cursor.sh 或 WeChat.exe 或 8.128.0.0/10" />
            </div>

            <div class="form-field">
              <label class="form-label" for="rule-note-inp">备注（选填）</label>
              <input id="rule-note-inp" name="note" type="text" class="input-sharp" placeholder="例如：开发工具走出口" />
            </div>

            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-submit-rule">保存并置顶生效</button>
            </div>
          </form>
        </div>
      `;

    case "pick-app":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="从运行中程序点选添加">
          <div class="modal-head">
            <span>从正在运行的程序点选分流</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <div class="modal-body" style="padding:0;">
            <table class="data-table">
              <thead>
                <tr>
                  <th style="width:130px;">进程名</th>
                  <th>安装路径</th>
                  <th style="width:168px;text-align:right;">快捷加入规则</th>
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
                        <button type="button" class="btn-lime btn-sm" data-pick-app-rule="${esc(app.process)}:EXIT">走出口</button>
                      </td>
                    </tr>
                  `
                  )
                  .join("")}
              </tbody>
            </table>
          </div>
          <div class="modal-foot">
            <button type="button" class="btn-hairline" data-close-modal>关闭</button>
          </div>
        </div>
      `;

    case "import-rules":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="导入规则文件">
          <div class="modal-head">
            <span>导入分流规则文件</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-import-rules" class="modal-body">
            <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
              <span class="form-label">选择本地规则文件（.txt / .conf / .yaml / .json）或直接粘贴规则行</span>
              <label class="btn-hairline btn-sm" style="cursor:pointer;">
                选择本地文件
                <input id="rule-file-picker" type="file" accept=".txt,.conf,.yaml,.yml,.json" hidden />
              </label>
            </div>
            <textarea id="import-rules-textarea" name="rawRules" rows="7" class="textarea-sharp">${esc(SAMPLE_IMPORT_RULES_TEXT)}</textarea>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-confirm-import">校验并导入规则</button>
            </div>
          </form>
        </div>
      `;

    case "add-org":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="挂载新 Tailscale 组织">
          <div class="modal-head">
            <span>挂载新 Tailscale 组织（无损独立端口授权）</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-org" class="modal-body">
            <div style="padding:10px 12px;background:var(--bg-code);border:1px solid var(--border-hairline);font-size:12px;color:var(--text-body);">
              授权过程在独立本地端口 <span class="font-mono-ui" style="color:var(--accent-text);">127.0.0.1:17899</span> 进行，不卸载当前虚拟网卡，授权期间现有网络不断流。
            </div>
            <div style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px;">
              <div class="form-field">
                <label class="form-label" for="org-name-inp">组织显示名称</label>
                <input id="org-name-inp" name="name" type="text" required class="input-sharp" value="EU Lab Tailnet" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-acc-inp">组织账号标识</label>
                <input id="org-acc-inp" name="account" type="text" required class="input-sharp font-mono-ui" value="infra@eulab.dev" />
              </div>
            </div>
            <div style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;">
              <div class="form-field">
                <label class="form-label" for="org-node-inp">发现的出口机器名</label>
                <input id="org-node-inp" name="nodeHostname" type="text" class="input-sharp font-mono-ui" value="eu-fra-exit-01" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-ip-inp">出口公网 IP</label>
                <input id="org-ip-inp" name="publicIp" type="text" class="input-sharp font-mono-ui" value="88.198.44.102" />
              </div>
              <div class="form-field">
                <label class="form-label" for="org-reg-inp">所在区域</label>
                <input id="org-reg-inp" name="region" type="text" class="input-sharp" value="DE · 法兰克福" />
              </div>
            </div>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime" data-testid="btn-confirm-add-org">打开浏览器完成授权并同步节点</button>
            </div>
          </form>
        </div>
      `;

    case "add-provider":
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="添加规则集订阅">
          <div class="modal-head">
            <span>添加远程规则集订阅 (.srs)</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <form id="form-add-provider" class="modal-body">
            <div class="form-field">
              <label class="form-label" for="prov-tag-inp">规则集标识名（供分流规则引用）</label>
              <input id="prov-tag-inp" name="tag" type="text" required class="input-sharp font-mono-ui" placeholder="例如：geosite-streaming" value="dustinwin-ai" />
            </div>
            <div class="form-field">
              <label class="form-label" for="prov-src-inp">来源名称</label>
              <input id="prov-src-inp" name="sourceName" type="text" class="input-sharp" value="DustinWin / ruleset_geodata" />
            </div>
            <div class="form-field">
              <label class="form-label" for="prov-url-inp">远程 .srs 下载地址</label>
              <input id="prov-url-inp" name="url" type="url" required class="input-sharp font-mono-ui" value="https://raw.githubusercontent.com/DustinWin/ruleset_geodata/sing-box-ruleset/ai.srs" />
            </div>
            <div class="modal-foot" style="margin:4px -16px -16px;">
              <button type="button" class="btn-hairline" data-close-modal>取消</button>
              <button type="submit" class="btn-lime">拉取验包并保存</button>
            </div>
          </form>
        </div>
      `;

    case "export-config": {
      const sanitized = {
        version: "1.0.0",
        exportedAt: "2026-11-27T11:00:00+08:00",
        credentialsStripped: true,
        excludedPaths: state.orgs.map((o) => o.stateDir),
        network: {
          mtu: state.settings.mtu,
          ipv4Only: state.settings.ipv4Only,
          bypassTailnetCidr: state.settings.bypassTailnetCidr
        },
        customRules: state.rules
          .filter((r) => !r.locked)
          .map(({ type, payload, action, note, enabled }) => ({ type, payload, action, note, enabled })),
        providers: state.providers.map(({ tag, url }) => ({ tag, url }))
      };
      return `
        <div class="modal-panel" role="dialog" aria-modal="true" aria-label="脱敏备份与导出">
          <div class="modal-head">
            <span>脱敏配置导出预览</span>
            <button type="button" class="btn-hairline btn-sm" data-close-modal>ESC</button>
          </div>
          <div class="modal-body">
            <div style="display:flex;align-items:center;justify-content:space-between;">
              <span class="tag-sharp tag-green">已自动剥离全部 ts-state 设备私钥与账号凭据</span>
              <span class="font-mono-ui" style="font-size:11px;color:var(--text-subtle);">singtail-profile.sanitized.json</span>
            </div>
            <textarea readonly rows="10" class="textarea-sharp">${esc(JSON.stringify(sanitized, null, 2))}</textarea>
          </div>
          <div class="modal-foot">
            <button type="button" class="btn-hairline" data-close-modal>关闭</button>
            <button type="button" class="btn-lime" data-action="copy-export-json">复制脱敏配置 JSON</button>
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
        kind: "切换出口",
        label: `${n.label} (${n.publicIp})`,
        sub: n.orgName,
        cmd: `exit:${n.id}`
      })),
    { kind: "跳转视图", label: "打开 分流规则 管理", sub: "Ctrl+2", cmd: "view:rules" },
    { kind: "跳转视图", label: "打开 规则集与订阅", sub: "Ctrl+3", cmd: "view:providers" },
    { kind: "跳转视图", label: "打开 实时连接与检视器", sub: "Ctrl+4", cmd: "view:connections" },
    { kind: "跳转视图", label: "打开 组织与系统设置", sub: "Ctrl+5", cmd: "view:settings" },
    { kind: "快捷动作", label: "新建个人分流规则", sub: "Modal", cmd: "modal:add-rule" },
    { kind: "快捷动作", label: "导入本地规则文件", sub: "Modal", cmd: "modal:import-rules" },
    { kind: "快捷动作", label: "一键让路 / 切回 Clash Verge", sub: "Toggle", cmd: "action:clash" }
  ].filter((it) => !q || it.label.toLowerCase().includes(q) || it.sub.toLowerCase().includes(q));

  return `
    <div class="modal-panel" style="width:min(500px,100%);" role="dialog" aria-modal="true" aria-label="快速命令面板">
      <div style="display:flex;align-items:center;gap:8px;padding:10px 14px;border-bottom:1px solid var(--border-hairline);background:var(--bg-code);">
        <span class="font-mono-ui" style="color:var(--accent-text);">›</span>
        <input
          id="cmdk-search-input"
          type="search"
          placeholder="输入出口名称、视图或动作…"
          value="${esc(state.cmdkQuery)}"
          style="width:100%;border:0;background:transparent;outline:none;font-size:13px;"
        />
        <button type="button" class="tag-sharp" data-action="close-cmdk" style="cursor:pointer;">ESC</button>
      </div>
      <div style="max-height:300px;overflow-y:auto;">
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
