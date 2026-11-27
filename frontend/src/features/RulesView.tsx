import { useState } from "react";
import { SharpSwitch } from "../components/SharpSwitch";
import {
  canMoveRule,
  countRulesByAction,
  filterRules,
  getActionMeta,
  type ModalKind,
  type RuleFilterAction,
} from "../state/selectors";
import type { Rule } from "../types/contract";

interface RulesViewProps {
  rules: readonly Rule[];
  activeExitLabel: string;
  onToggleRule: (ruleId: string) => void;
  onMoveRule: (ruleId: string, direction: -1 | 1) => void;
  onDeleteRule: (ruleId: string) => void;
  onOpenModal: (modal: Exclude<ModalKind, null>) => void;
}

export function RulesView({
  rules,
  activeExitLabel,
  onToggleRule,
  onMoveRule,
  onDeleteRule,
  onOpenModal,
}: RulesViewProps) {
  const [filterAction, setFilterAction] = useState<RuleFilterAction>("ALL");
  const [search, setSearch] = useState("");

  const counts = countRulesByAction(rules);
  const filtered = filterRules(rules, filterAction, search);

  const actionTabs: ReadonlyArray<{ id: RuleFilterAction; label: string }> = [
    { id: "ALL", label: `全部 (${counts.ALL})` },
    { id: "EXIT", label: `出口 (${counts.EXIT})` },
    { id: "DIRECT", label: `直连 (${counts.DIRECT})` },
    { id: "FALLBACK_PROXY", label: `本地代理 (${counts.FALLBACK_PROXY})` },
    { id: "REJECT", label: `拦截 (${counts.REJECT})` },
  ];

  return (
    <>
      <div className="view-topbar">
        <h1 className="view-title">分流规则</h1>
        <div className="view-actions">
          <button
            type="button"
            className="btn-hairline"
            data-testid="btn-pick-app"
            onClick={() => onOpenModal("pick-app")}
          >
            ⊞ 点选程序
          </button>
          <button
            type="button"
            className="btn-hairline"
            data-testid="btn-import-rules"
            onClick={() => onOpenModal("import-rules")}
          >
            ⇪ 导入文件
          </button>
          <button
            type="button"
            className="btn-lime"
            data-testid="btn-add-rule"
            onClick={() => onOpenModal("add-rule")}
          >
            + 新建规则
          </button>
        </div>
      </div>

      <div className="section-bar">
        <div className="seg-tabs" role="tablist">
          {actionTabs.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`seg-tab ${filterAction === t.id ? "is-active" : ""}`}
              onClick={() => setFilterAction(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <input
          id="rule-search-input"
          type="search"
          className="input-sharp"
          style={{ width: 220 }}
          placeholder="过滤域名 / 进程 / IP…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <div className="view-scroll">
        {filtered.length > 0 ? (
          <table className="data-table" aria-label="分流规则">
            <thead>
              <tr>
                <th style={{ width: 54 }}>序号</th>
                <th style={{ width: 146 }}>类型</th>
                <th>匹配内容</th>
                <th style={{ width: 162 }}>走向</th>
                <th style={{ width: 94, textAlign: "right" }}>命中</th>
                <th style={{ width: 168, textAlign: "right" }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((rule) => {
                const fullIdx = rules.findIndex((x) => x.id === rule.id);
                const actionMeta = getActionMeta(rule.action, activeExitLabel);
                const moveUpAllowed = canMoveRule(rules, rule.id, -1);
                const moveDownAllowed = canMoveRule(rules, rule.id, 1);

                return (
                  <tr
                    key={rule.id}
                    className={rule.enabled ? undefined : "is-disabled"}
                    data-testid={`rule-row-${rule.id}`}
                  >
                    <td
                      className="font-mono-ui"
                      style={{ width: 54, color: "var(--text-subtle)" }}
                    >
                      #{String(fullIdx + 1).padStart(2, "0")}
                    </td>
                    <td style={{ width: 146 }}>
                      <span className="tag-sharp">{rule.type}</span>
                    </td>
                    <td
                      className="font-mono-ui selectable-text"
                      title={rule.payload}
                      style={{ color: "var(--text-primary)" }}
                    >
                      {rule.payload}
                    </td>
                    <td style={{ width: 162 }}>
                      <span className={actionMeta.tagClass}>{actionMeta.text}</span>
                    </td>
                    <td
                      className="font-mono-ui"
                      style={{
                        width: 94,
                        textAlign: "right",
                        color: "var(--text-subtle)",
                      }}
                    >
                      {rule.hits.toLocaleString()}
                    </td>
                    <td
                      style={{
                        width: 168,
                        textAlign: "right",
                        overflow: "visible",
                      }}
                    >
                      {rule.locked ? (
                        <span className="tag-sharp">内置</span>
                      ) : (
                        <div
                          style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 4,
                          }}
                        >
                          <button
                            type="button"
                            className="btn-hairline btn-sm"
                            disabled={!moveUpAllowed}
                            onClick={() => onMoveRule(rule.id, -1)}
                          >
                            ↑
                          </button>
                          <button
                            type="button"
                            className="btn-hairline btn-sm"
                            disabled={!moveDownAllowed}
                            onClick={() => onMoveRule(rule.id, 1)}
                          >
                            ↓
                          </button>
                          <SharpSwitch
                            checked={rule.enabled}
                            onToggle={() => onToggleRule(rule.id)}
                            ariaLabel="开关规则"
                          />
                          <button
                            type="button"
                            className="btn-hairline btn-sm"
                            style={{ color: "var(--status-red)" }}
                            onClick={() => onDeleteRule(rule.id)}
                          >
                            ✕
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div className="empty-box">
            <div>无匹配规则</div>
            <button
              type="button"
              className="btn-hairline"
              onClick={() => {
                setFilterAction("ALL");
                setSearch("");
              }}
            >
              清空筛选
            </button>
          </div>
        )}
      </div>
    </>
  );
}
