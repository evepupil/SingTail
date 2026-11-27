import { type ChangeEvent, type FormEvent, useEffect, useState } from "react";
import type {
  AddOrgPayload,
  AddProviderPayload,
  AddRulePayload,
} from "../services/daemonClient";
import type { ModalKind } from "../state/selectors";
import type {
  RuleAction,
  RuleType,
  RunningApp,
  SanitizedExport,
} from "../types/contract";

const SAMPLE_IMPORT_RULES_TEXT = `DOMAIN-SUFFIX,anthropic.com,EXIT
DOMAIN-SUFFIX,steamcontent.com,DIRECT
PROCESS-NAME,steam.exe,DIRECT
IP-CIDR,47.96.0.0/11,DIRECT
DOMAIN-KEYWORD,adservice,REJECT`;

interface ModalsProps {
  activeModal: ModalKind;
  runningApps: readonly RunningApp[];
  onClose: () => void;
  onAddRule: (payload: AddRulePayload) => Promise<boolean>;
  onImportRules: (rawText: string) => Promise<boolean>;
  onAddOrg: (payload: AddOrgPayload) => Promise<boolean>;
  onAddProvider: (payload: AddProviderPayload) => Promise<boolean>;
  onFetchExport: () => Promise<SanitizedExport>;
  onToast: (message: string, isError?: boolean) => void;
}

export function Modals({
  activeModal,
  runningApps,
  onClose,
  onAddRule,
  onImportRules,
  onAddOrg,
  onAddProvider,
  onFetchExport,
  onToast,
}: ModalsProps) {
  const [submitting, setSubmitting] = useState(false);

  // Add Rule state
  const [ruleType, setRuleType] = useState<RuleType>("DOMAIN-SUFFIX");
  const [ruleAction, setRuleAction] = useState<RuleAction>("EXIT");
  const [rulePayload, setRulePayload] = useState("");

  // Import Rules state
  const [importText, setImportText] = useState(SAMPLE_IMPORT_RULES_TEXT);

  // Add Org state
  const [orgName, setOrgName] = useState("EU Lab Tailnet");
  const [orgAccount, setOrgAccount] = useState("infra@eulab.dev");
  const [orgNode, setOrgNode] = useState("eu-fra-exit-01");
  const [orgIp, setOrgIp] = useState("88.198.44.102");
  const [orgRegion, setOrgRegion] = useState("DE · 法兰克福");

  // Add Provider state
  const [provTag, setProvTag] = useState("dustinwin-ai");
  const [provSource, setProvSource] = useState("DustinWin");
  const [provUrl, setProvUrl] = useState(
    "https://raw.githubusercontent.com/DustinWin/ruleset_geodata/sing-box-ruleset/ai.srs",
  );

  // Export state
  const [exportJson, setExportJson] = useState("");
  const [exportLoading, setExportLoading] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  useEffect(() => {
    setSubmitting(false);
    if (activeModal === "add-rule") {
      setRuleType("DOMAIN-SUFFIX");
      setRuleAction("EXIT");
      setRulePayload("");
    } else if (activeModal === "import-rules") {
      setImportText(SAMPLE_IMPORT_RULES_TEXT);
    } else if (activeModal === "export-config") {
      let cancelled = false;
      setExportLoading(true);
      setExportError(null);
      setExportJson("");
      onFetchExport()
        .then((data) => {
          if (cancelled) {
            return;
          }
          setExportJson(JSON.stringify(data, null, 2));
        })
        .catch((err: unknown) => {
          if (cancelled) {
            return;
          }
          const msg = err instanceof Error ? err.message : "导出失败";
          setExportError(msg);
          onToast(msg, true);
        })
        .finally(() => {
          if (!cancelled) {
            setExportLoading(false);
          }
        });
      return () => {
        cancelled = true;
      };
    }
    return undefined;
  }, [activeModal, onFetchExport, onToast]);

  const handleRuleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const clean = rulePayload.trim();
    if (!clean || submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onAddRule({
        type: ruleType,
        payload: clean,
        action: ruleAction,
      });
      if (ok) {
        onClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handlePickApp = async (processName: string, action: RuleAction) => {
    if (submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onAddRule({
        type: "PROCESS-NAME",
        payload: processName,
        action,
      });
      if (ok) {
        onClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleFileChange = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) {
      return;
    }
    const text = await file.text();
    setImportText(text);
  };

  const handleImportSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!importText.trim() || submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onImportRules(importText);
      if (ok) {
        onClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleOrgSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!orgName.trim() || !orgAccount.trim() || submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onAddOrg({
        name: orgName.trim(),
        account: orgAccount.trim(),
        nodeHostname: orgNode.trim(),
        publicIp: orgIp.trim(),
        region: orgRegion.trim(),
      });
      if (ok) {
        onClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleProviderSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!provTag.trim() || !provUrl.trim() || submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const ok = await onAddProvider({
        tag: provTag.trim(),
        sourceName: provSource.trim() || "Custom",
        url: provUrl.trim(),
      });
      if (ok) {
        onClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleCopyExport = async () => {
    if (exportLoading || exportError || !exportJson) {
      return;
    }
    try {
      await navigator.clipboard?.writeText(exportJson);
      onToast("已复制");
      onClose();
    } catch {
      onToast("复制失败", true);
    }
  };

  return (
    <div
      id="modal-backdrop"
      className="modal-backdrop"
      hidden={activeModal === null}
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          onClose();
        }
      }}
    >
      {activeModal === "add-rule" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="新建规则">
          <div className="modal-head">
            <span>新建规则</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <form className="modal-body" onSubmit={handleRuleSubmit}>
            <div style={{ display: "grid", gridTemplateColumns: "170px minmax(0,1fr)", gap: 10 }}>
              <div className="form-field">
                <label className="form-label" htmlFor="rule-type-sel">
                  类型
                </label>
                <select
                  id="rule-type-sel"
                  name="type"
                  className="select-sharp"
                  value={ruleType}
                  onChange={(e) => setRuleType(e.target.value as RuleType)}
                >
                  <option value="DOMAIN-SUFFIX">DOMAIN-SUFFIX</option>
                  <option value="PROCESS-NAME">PROCESS-NAME</option>
                  <option value="IP-CIDR">IP-CIDR</option>
                  <option value="DOMAIN-KEYWORD">DOMAIN-KEYWORD</option>
                  <option value="RULE-SET">RULE-SET</option>
                </select>
              </div>
              <div className="form-field">
                <label className="form-label" htmlFor="rule-action-sel">
                  走向
                </label>
                <select
                  id="rule-action-sel"
                  name="action"
                  className="select-sharp"
                  value={ruleAction}
                  onChange={(e) => setRuleAction(e.target.value as RuleAction)}
                >
                  <option value="EXIT">↗ 走出口</option>
                  <option value="DIRECT">→ 直连</option>
                  <option value="FALLBACK_PROXY">⇄ 本地代理</option>
                  <option value="REJECT">✕ 拦截</option>
                </select>
              </div>
            </div>

            <div className="form-field">
              <label className="form-label" htmlFor="rule-payload-inp">
                匹配内容
              </label>
              <input
                id="rule-payload-inp"
                name="payload"
                type="text"
                required
                className="input-sharp font-mono-ui"
                placeholder="cursor.sh / WeChat.exe / 8.128.0.0/10"
                value={rulePayload}
                onChange={(e) => setRulePayload(e.target.value)}
              />
            </div>

            <div className="modal-foot" style={{ margin: "4px -16px -16px" }}>
              <button type="button" className="btn-hairline" onClick={onClose}>
                取消
              </button>
              <button
                type="submit"
                className="btn-lime"
                disabled={submitting}
                data-testid="btn-submit-rule"
              >
                保存
              </button>
            </div>
          </form>
        </div>
      )}

      {activeModal === "pick-app" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="点选程序">
          <div className="modal-head">
            <span>选择运行中程序</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <div className="modal-body" style={{ padding: 0 }}>
            <table className="data-table">
              <thead>
                <tr>
                  <th style={{ width: 130 }}>进程</th>
                  <th>路径</th>
                  <th style={{ width: 148, textAlign: "right" }}>动作</th>
                </tr>
              </thead>
              <tbody>
                {runningApps.map((app) => (
                  <tr key={app.process}>
                    <td
                      className="font-mono-ui"
                      style={{ color: "var(--text-primary)", fontWeight: 600 }}
                    >
                      {app.process}
                    </td>
                    <td
                      className="font-mono-ui"
                      title={app.path}
                      style={{ fontSize: 11, color: "var(--text-subtle)" }}
                    >
                      {app.path}
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <button
                        type="button"
                        className="btn-hairline btn-sm"
                        disabled={submitting}
                        style={{ marginRight: 4 }}
                        onClick={() => void handlePickApp(app.process, "DIRECT")}
                      >
                        直连
                      </button>
                      <button
                        type="button"
                        className="btn-lime btn-sm"
                        disabled={submitting}
                        onClick={() => void handlePickApp(app.process, "EXIT")}
                      >
                        出口
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {activeModal === "import-rules" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="导入规则">
          <div className="modal-head">
            <span>导入规则文件</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <form className="modal-body" onSubmit={handleImportSubmit}>
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
              <span className="form-label">格式：类型,匹配值,走向</span>
              <label className="btn-hairline btn-sm" style={{ cursor: "pointer" }}>
                选择文件
                <input
                  id="rule-file-picker"
                  type="file"
                  accept=".txt,.conf,.yaml,.yml,.json"
                  hidden
                  onChange={(e) => void handleFileChange(e)}
                />
              </label>
            </div>
            <textarea
              id="import-rules-textarea"
              name="rawRules"
              rows={6}
              className="textarea-sharp"
              value={importText}
              onChange={(e) => setImportText(e.target.value)}
            />
            <div className="modal-foot" style={{ margin: "4px -16px -16px" }}>
              <button type="button" className="btn-hairline" onClick={onClose}>
                取消
              </button>
              <button
                type="submit"
                className="btn-lime"
                disabled={submitting}
                data-testid="btn-confirm-import"
              >
                导入
              </button>
            </div>
          </form>
        </div>
      )}

      {activeModal === "add-org" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="挂载组织">
          <div className="modal-head">
            <span>挂载 Tailscale 组织</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <form className="modal-body" onSubmit={handleOrgSubmit}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(2,minmax(0,1fr))", gap: 10 }}>
              <div className="form-field">
                <label className="form-label" htmlFor="org-name-inp">
                  组织名称
                </label>
                <input
                  id="org-name-inp"
                  name="name"
                  type="text"
                  required
                  className="input-sharp"
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                />
              </div>
              <div className="form-field">
                <label className="form-label" htmlFor="org-acc-inp">
                  账号
                </label>
                <input
                  id="org-acc-inp"
                  name="account"
                  type="text"
                  required
                  className="input-sharp font-mono-ui"
                  value={orgAccount}
                  onChange={(e) => setOrgAccount(e.target.value)}
                />
              </div>
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3,minmax(0,1fr))", gap: 10 }}>
              <div className="form-field">
                <label className="form-label" htmlFor="org-node-inp">
                  出口主机名
                </label>
                <input
                  id="org-node-inp"
                  name="nodeHostname"
                  type="text"
                  className="input-sharp font-mono-ui"
                  value={orgNode}
                  onChange={(e) => setOrgNode(e.target.value)}
                />
              </div>
              <div className="form-field">
                <label className="form-label" htmlFor="org-ip-inp">
                  公网 IP
                </label>
                <input
                  id="org-ip-inp"
                  name="publicIp"
                  type="text"
                  className="input-sharp font-mono-ui"
                  value={orgIp}
                  onChange={(e) => setOrgIp(e.target.value)}
                />
              </div>
              <div className="form-field">
                <label className="form-label" htmlFor="org-reg-inp">
                  区域
                </label>
                <input
                  id="org-reg-inp"
                  name="region"
                  type="text"
                  className="input-sharp"
                  value={orgRegion}
                  onChange={(e) => setOrgRegion(e.target.value)}
                />
              </div>
            </div>
            <div className="modal-foot" style={{ margin: "4px -16px -16px" }}>
              <button type="button" className="btn-hairline" onClick={onClose}>
                取消
              </button>
              <button
                type="submit"
                className="btn-lime"
                disabled={submitting}
                data-testid="btn-confirm-add-org"
              >
                登录授权
              </button>
            </div>
          </form>
        </div>
      )}

      {activeModal === "add-provider" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="添加订阅">
          <div className="modal-head">
            <span>添加规则集订阅</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <form className="modal-body" onSubmit={handleProviderSubmit}>
            <div className="form-field">
              <label className="form-label" htmlFor="prov-tag-inp">
                名称
              </label>
              <input
                id="prov-tag-inp"
                name="tag"
                type="text"
                required
                className="input-sharp font-mono-ui"
                value={provTag}
                onChange={(e) => setProvTag(e.target.value)}
              />
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="prov-src-inp">
                来源
              </label>
              <input
                id="prov-src-inp"
                name="sourceName"
                type="text"
                className="input-sharp"
                value={provSource}
                onChange={(e) => setProvSource(e.target.value)}
              />
            </div>
            <div className="form-field">
              <label className="form-label" htmlFor="prov-url-inp">
                订阅链接
              </label>
              <input
                id="prov-url-inp"
                name="url"
                type="url"
                required
                className="input-sharp font-mono-ui"
                value={provUrl}
                onChange={(e) => setProvUrl(e.target.value)}
              />
            </div>
            <div className="modal-foot" style={{ margin: "4px -16px -16px" }}>
              <button type="button" className="btn-hairline" onClick={onClose}>
                取消
              </button>
              <button type="submit" className="btn-lime" disabled={submitting}>
                保存
              </button>
            </div>
          </form>
        </div>
      )}

      {activeModal === "export-config" && (
        <div className="modal-panel" role="dialog" aria-modal="true" aria-label="导出配置">
          <div className="modal-head">
            <span>导出脱敏配置</span>
            <button type="button" className="btn-hairline btn-sm" onClick={onClose}>
              ESC
            </button>
          </div>
          <div className="modal-body">
            {exportError ? (
              <div
                className="inline-banner is-rollback"
                role="alert"
                style={{ border: "1px solid var(--status-red-border)" }}
              >
                <span>{exportError}</span>
              </div>
            ) : (
              <textarea
                readOnly
                rows={9}
                className="textarea-sharp"
                value={exportLoading ? "正在生成脱敏配置…" : exportJson}
              />
            )}
          </div>
          <div className="modal-foot">
            <button type="button" className="btn-hairline" onClick={onClose}>
              关闭
            </button>
            <button
              type="button"
              className="btn-lime"
              disabled={exportLoading || Boolean(exportError) || !exportJson}
              onClick={() => void handleCopyExport()}
            >
              复制 JSON
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
