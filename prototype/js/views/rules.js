// View 2: Visual Routing Rules Matrix (Sequential Evaluation, Reorder, Add, Pick App, Import File)
import { getActiveExit } from "../core/store.js";
import { esc, renderActionBadge } from "../core/utils.js";

export function renderRulesView(state) {
  const activeExit = getActiveExit();
  const q = state.ruleSearch.trim().toLowerCase();

  const filtered = state.rules.filter((r) => {
    if (state.ruleFilterAction !== "ALL" && r.action !== state.ruleFilterAction) {
      return false;
    }
    if (!q) return true;
    return (
      r.payload.toLowerCase().includes(q) ||
      r.type.toLowerCase().includes(q) ||
      r.note.toLowerCase().includes(q)
    );
  });

  const actionTabs = [
    { id: "ALL", label: `全部 (${state.rules.length})` },
    { id: "EXIT", label: `走出口 (${state.rules.filter((r) => r.action === "EXIT").length})` },
    { id: "DIRECT", label: `直连 (${state.rules.filter((r) => r.action === "DIRECT").length})` },
    { id: "FALLBACK_PROXY", label: `兜底代理 (${state.rules.filter((r) => r.action === "FALLBACK_PROXY").length})` },
    { id: "REJECT", label: `拦截 (${state.rules.filter((r) => r.action === "REJECT").length})` }
  ];

  const rowsHtml = filtered.length
    ? filtered
        .map((rule) => {
          const fullIdx = state.rules.findIndex((x) => x.id === rule.id);
          const canMoveUp = !rule.locked && fullIdx > 2;
          const canMoveDown = !rule.locked && fullIdx < state.rules.length - 3;
          return `
            <tr class="${rule.enabled ? "" : "is-disabled"}" data-testid="rule-row-${esc(rule.id)}">
              <td class="font-mono-ui" style="width:52px;color:var(--text-subtle);">#${String(fullIdx + 1).padStart(2, "0")}</td>
              <td style="width:136px;"><span class="tag-sharp">${esc(rule.type)}</span></td>
              <td class="font-mono-ui selectable-text" title="${esc(rule.payload)}" style="color:var(--text-primary);">${esc(rule.payload)}</td>
              <td style="width:146px;">${renderActionBadge(rule.action, activeExit.hostname)}</td>
              <td title="${esc(rule.note)}" style="width:210px;color:var(--text-muted);">${esc(rule.note)}</td>
              <td class="font-mono-ui" style="width:86px;text-align:right;color:var(--text-subtle);">${rule.hits.toLocaleString()}</td>
              <td style="width:142px;text-align:right;">
                ${
                  rule.locked
                    ? `<span class="tag-sharp" title="系统底层保护规则">系统锁定</span>`
                    : `<div style="display:inline-flex;align-items:center;gap:4px;">
                        <button type="button" class="btn-hairline btn-sm" title="上移优先级" data-rule-up="${esc(rule.id)}" ${canMoveUp ? "" : "disabled"}>↑</button>
                        <button type="button" class="btn-hairline btn-sm" title="下移优先级" data-rule-down="${esc(rule.id)}" ${canMoveDown ? "" : "disabled"}>↓</button>
                        <button type="button" class="sharp-switch" role="switch" aria-label="启用规则" aria-checked="${rule.enabled}" data-rule-toggle="${esc(rule.id)}">
                          <span class="sharp-switch-thumb"></span>
                        </button>
                        <button type="button" class="btn-hairline btn-sm" title="删除规则" data-rule-del="${esc(rule.id)}" style="color:var(--status-red);">✕</button>
                      </div>`
                }
              </td>
            </tr>
          `;
        })
        .join("")
    : "";

  return `
    <div class="view-topbar">
      <div class="view-title-group">
        <h1 class="view-title">分流规则</h1>
        <span class="tag-sharp">自上而下首条命中生效</span>
      </div>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-open-modal="pick-app" data-testid="btn-pick-app">⊞ 点选运行中程序</button>
        <button type="button" class="btn-hairline" data-open-modal="import-rules" data-testid="btn-import-rules">⇪ 导入规则文件</button>
        <button type="button" class="btn-lime" data-open-modal="add-rule" data-testid="btn-add-rule">+ 新建规则</button>
      </div>
    </div>

    <div class="section-bar">
      <div class="seg-tabs" role="tablist" aria-label="按分流动作筛选">
        ${actionTabs
          .map(
            (t) =>
              `<button type="button" class="seg-tab ${state.ruleFilterAction === t.id ? "is-active" : ""}" data-rule-filter="${t.id}">${esc(t.label)}</button>`
          )
          .join("")}
      </div>
      <input
        id="rule-search-input"
        type="search"
        class="input-sharp"
        style="width:230px;"
        placeholder="搜索域名 / 进程名 / IP 段…"
        value="${esc(state.ruleSearch)}"
      />
    </div>

    <div class="view-scroll">
      ${
        filtered.length
          ? `<table class="data-table" aria-label="分流规则列表">
              <thead>
                <tr>
                  <th style="width:52px;">顺序</th>
                  <th style="width:136px;">匹配类型</th>
                  <th>匹配内容</th>
                  <th style="width:146px;">走向策略</th>
                  <th style="width:210px;">备注</th>
                  <th style="width:86px;text-align:right;">命中次数</th>
                  <th style="width:142px;text-align:right;">操作</th>
                </tr>
              </thead>
              <tbody>${rowsHtml}</tbody>
            </table>`
          : `<div class="empty-box">
              <div>未找到匹配当前筛选条件的分流规则</div>
              <div style="display:flex;gap:8px;">
                <button type="button" class="btn-hairline" data-action="reset-rule-filter">清空筛选</button>
                <button type="button" class="btn-lime" data-open-modal="add-rule">+ 新建规则</button>
              </div>
            </div>`
      }
    </div>
  `;
}
