import {
  type ChangeEvent,
  type FormEvent,
  useEffect,
  useRef,
  useState,
} from "react";
import type {
  AddProviderPayload,
  AddRulePayload,
  ImportOrgStateDirPayload,
  StartOrgLoginPayload,
} from "../services/daemonClient";
import type { ModalKind } from "../state/selectors";
import type {
  OrgLoginSession,
  RuleAction,
  RuleType,
  RunningApp,
  SanitizedExport,
} from "../types/contract";

type AddOrgTab = "login" | "import-dir";

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
  onStartOrgLogin: (payload: StartOrgLoginPayload) => Promise<OrgLoginSession>;
  onPollOrgLogin: (sessionId: string) => Promise<OrgLoginSession>;
  onImportOrgDir: (payload: ImportOrgStateDirPayload) => Promise<boolean>;
  onOrgAuthenticated: () => Promise<void>;
  onOpenExternalUrl: (url: string) => void;
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
  onStartOrgLogin,
  onPollOrgLogin,
  onImportOrgDir,
  onOrgAuthenticated,
  onOpenExternalUrl,
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

  // Add Org state (zero-input default; advanced options collapsed)
  const [orgAdvancedOpen, setOrgAdvancedOpen] = useState(false);
  const [orgTab, setOrgTab] = useState<AddOrgTab>("login");
  const [orgAlias, setOrgAlias] = useState("");
  const [orgControlUrl, setOrgControlUrl] = useState("");
  const [loginSession, setLoginSession] = useState<OrgLoginSession | null>(null);
  const [loginError, setLoginError] = useState<string | null>(null);
  const [importSourceDir, setImportSourceDir] = useState("");
  const [importAlias, setImportAlias] = useState("");
  const [importDirError, setImportDirError] = useState<string | null>(null);

  const openedAuthUrlRef = useRef("");
  const isAddOrgOpenRef = useRef(false);
  isAddOrgOpenRef.current = activeModal === "add-org";

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
    } else if (activeModal === "add-org") {
      setOrgAdvancedOpen(false);
      setOrgTab("login");
      setOrgAlias("");
      setOrgControlUrl("");
      setLoginSession(null);
      setLoginError(null);
      setImportSourceDir("");
      setImportAlias("");
      setImportDirError(null);
      openedAuthUrlRef.current = "";
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

  // Poll Tailscale web authorization session every 1.5s while modal & login tab remain open
  useEffect(() => {
    if (
      activeModal !== "add-org" ||
      orgTab !== "login" ||
      !loginSession ||
      (loginSession.state !== "starting" && loginSession.state !== "auth_url")
    ) {
      return undefined;
    }

    let cancelled = false;
    let inFlight = false;
    const sessionId = loginSession.sessionId;

    const timer = window.setInterval(() => {
      if (cancelled || inFlight) {
        return;
      }
      inFlight = true;
      void onPollOrgLogin(sessionId)
        .then(async (res) => {
          if (cancelled) {
            return;
          }
          if (res.authUrl && openedAuthUrlRef.current !== res.authUrl) {
            openedAuthUrlRef.current = res.authUrl;
            onOpenExternalUrl(res.authUrl);
          }
          setLoginSession(res);
          if (res.state === "authenticated") {
            window.clearInterval(timer);
            await onOrgAuthenticated();
            if (cancelled) {
              return;
            }
            onToast(
              `已挂载 ${res.orgName || "Tailscale 组织"}（发现 ${res.nodeCount ?? 0} 个出口节点）`,
            );
            onClose();
          } else if (res.state === "error") {
            window.clearInterval(timer);
            setLoginError(res.error || "授权失败，请重试");
          }
        })
        .catch((err: unknown) => {
          if (cancelled) {
            return;
          }
          window.clearInterval(timer);
          const msg = err instanceof Error ? err.message : "轮询授权状态失败";
          setLoginError(msg);
          setLoginSession((prev) =>
            prev ? { ...prev, state: "error", error: msg } : null,
          );
        })
        .finally(() => {
          inFlight = false;
        });
    }, 1500);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [
    activeModal,
    orgTab,
    loginSession,
    onPollOrgLogin,
    onOrgAuthenticated,
    onOpenExternalUrl,
    onToast,
    onClose,
  ]);

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

  const handleStartOrgLogin = async (e: FormEvent) => {
    e.preventDefault();
    if (submitting) {
      return;
    }
    setSubmitting(true);
    setLoginError(null);
    setLoginSession(null);
    openedAuthUrlRef.current = "";
    try {
      const cleanAlias = orgAlias.trim();
      const cleanControlUrl = orgControlUrl.trim();
      const res = await onStartOrgLogin({
        alias: cleanAlias,
        ...(cleanControlUrl ? { controlUrl: cleanControlUrl } : {}),
      });
      if (!isAddOrgOpenRef.current) {
        return;
      }
      if (res.authUrl && openedAuthUrlRef.current !== res.authUrl) {
        openedAuthUrlRef.current = res.authUrl;
        onOpenExternalUrl(res.authUrl);
      }
      if (res.state === "authenticated") {
        await onOrgAuthenticated();
        if (!isAddOrgOpenRef.current) {
          return;
        }
        onToast(
          `已挂载 ${res.orgName || "Tailscale 组织"}（发现 ${res.nodeCount ?? 0} 个出口节点）`,
        );
        onClose();
        return;
      }
      if (res.state === "error") {
        setLoginError(res.error || "获取授权链接失败");
      }
      setLoginSession(res);
    } catch (err: unknown) {
      if (!isAddOrgOpenRef.current) {
        return;
      }
      setLoginError(err instanceof Error ? err.message : "获取授权链接失败");
    } finally {
      setSubmitting(false);
    }
  };

  const handleCopyAuthUrl = async () => {
    const url = loginSession?.authUrl;
    if (!url) {
      return;
    }
    try {
      await navigator.clipboard?.writeText(url);
      onToast("已复制授权链接");
    } catch {
      onToast("复制失败", true);
    }
  };

  const handleImportOrgDirSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const sourceDir = importSourceDir.trim();
    if (!sourceDir || submitting) {
      return;
    }
    setSubmitting(true);
    setImportDirError(null);
    try {
      const cleanAlias = importAlias.trim();
      const ok = await onImportOrgDir({
        sourceDir,
        ...(cleanAlias ? { alias: cleanAlias } : {}),
      });
      if (ok) {
        onClose();
      }
    } catch (err: unknown) {
      setImportDirError(err instanceof Error ? err.message : "导入失败");
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

          {orgAdvancedOpen && (
            <div
              style={{
                padding: "10px 16px 0",
                borderBottom: "1px solid var(--border-hairline)",
                background: "var(--bg-surface)",
              }}
            >
              <div
                className="seg-tabs"
                role="tablist"
                aria-label="挂载方式"
                style={{ marginBottom: 10 }}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={orgTab === "login"}
                  className={`seg-tab ${orgTab === "login" ? "is-active" : ""}`}
                  data-testid="tab-org-login"
                  onClick={() => {
                    setOrgTab("login");
                    setImportDirError(null);
                  }}
                >
                  网页授权登录
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={orgTab === "import-dir"}
                  className={`seg-tab ${orgTab === "import-dir" ? "is-active" : ""}`}
                  data-testid="tab-org-import-dir"
                  onClick={() => {
                    setOrgTab("import-dir");
                    setLoginSession(null);
                    setLoginError(null);
                  }}
                >
                  导入已有凭据目录
                </button>
              </div>
            </div>
          )}

          {!orgAdvancedOpen || orgTab === "login" ? (
            <form className="modal-body" onSubmit={handleStartOrgLogin}>
              <div
                style={{
                  padding: "10px 12px",
                  border: "1px solid var(--border-hairline)",
                  background: "var(--bg-code)",
                  fontSize: 12.5,
                  color: "var(--text-body)",
                  lineHeight: 1.55,
                }}
              >
                在系统浏览器点击确认授权后，自动拉取组织名称、账号、全部出口节点并自动探测出口公网 IP。
              </div>

              {orgAdvancedOpen && (
                <>
                  <div className="form-field">
                    <label className="form-label" htmlFor="org-alias-inp">
                      组织备注名（选填）
                    </label>
                    <input
                      id="org-alias-inp"
                      name="alias"
                      type="text"
                      className="input-sharp"
                      placeholder="留空自动读取 Tailscale 组织名"
                      value={orgAlias}
                      onChange={(e) => setOrgAlias(e.target.value)}
                    />
                  </div>

                  <div className="form-field">
                    <label className="form-label" htmlFor="org-control-url-inp">
                      Headscale 控制地址（选填）
                    </label>
                    <input
                      id="org-control-url-inp"
                      name="controlUrl"
                      type="text"
                      className="input-sharp font-mono-ui"
                      placeholder="留空使用官方 login.tailscale.com"
                      value={orgControlUrl}
                      onChange={(e) => setOrgControlUrl(e.target.value)}
                    />
                  </div>
                </>
              )}

              {loginError && (
                <div
                  className="inline-banner is-rollback"
                  role="alert"
                  data-testid="org-login-error"
                  style={{ border: "1px solid var(--status-red-border)" }}
                >
                  <span>{loginError}</span>
                </div>
              )}

              {loginSession &&
                (loginSession.state === "starting" ||
                  loginSession.state === "auth_url") && (
                  <div
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 8,
                      padding: 10,
                      border: "1px solid var(--border-hairline)",
                      background: "var(--bg-code)",
                    }}
                    data-testid="org-login-polling-box"
                  >
                    {loginSession.authUrl ? (
                      <>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "space-between",
                            gap: 8,
                          }}
                        >
                          <span className="form-label">授权链接</span>
                          <div style={{ display: "flex", gap: 6 }}>
                            <button
                              type="button"
                              className="btn-hairline btn-sm"
                              onClick={() =>
                                onOpenExternalUrl(loginSession.authUrl ?? "")
                              }
                            >
                              打开浏览器
                            </button>
                            <button
                              type="button"
                              className="btn-hairline btn-sm"
                              data-testid="btn-copy-auth-url"
                              onClick={() => void handleCopyAuthUrl()}
                            >
                              复制链接
                            </button>
                          </div>
                        </div>
                        <div
                          className="font-mono-ui selectable-text"
                          data-testid="org-auth-url"
                          style={{
                            padding: "8px 10px",
                            border: "1px solid var(--border-hairline)",
                            background: "var(--bg-surface)",
                            color: "var(--accent-text)",
                            fontSize: 11.5,
                            wordBreak: "break-all",
                          }}
                        >
                          {loginSession.authUrl}
                        </div>
                        <div style={{ fontSize: 12, color: "var(--text-muted)" }}>
                          请在浏览器完成确认，正在自动同步节点与公网 IP…
                        </div>
                      </>
                    ) : (
                      <div style={{ fontSize: 12, color: "var(--text-muted)" }}>
                        正在拉起 Tailscale 授权页…
                      </div>
                    )}
                  </div>
                )}

              <div
                className="modal-foot"
                style={{
                  margin: "4px -16px -16px",
                  justifyContent: "space-between",
                }}
              >
                <button
                  type="button"
                  className="btn-hairline btn-sm"
                  data-testid="btn-toggle-org-advanced"
                  onClick={() => setOrgAdvancedOpen((prev) => !prev)}
                >
                  {orgAdvancedOpen ? "收起高级选项" : "高级选项"}
                </button>
                <div style={{ display: "flex", gap: 8 }}>
                  <button type="button" className="btn-hairline" onClick={onClose}>
                    取消
                  </button>
                  <button
                    type="submit"
                    className="btn-lime"
                    disabled={
                      submitting ||
                      loginSession?.state === "starting" ||
                      loginSession?.state === "auth_url"
                    }
                    data-testid="btn-confirm-add-org"
                  >
                    {submitting
                      ? "正在连接…"
                      : loginSession?.state === "starting" ||
                          loginSession?.state === "auth_url"
                        ? "等待浏览器授权…"
                        : "打开浏览器登录 Tailscale 授权"}
                  </button>
                </div>
              </div>
            </form>
          ) : (
            <form className="modal-body" onSubmit={handleImportOrgDirSubmit}>
              <div className="form-field">
                <label className="form-label" htmlFor="org-source-dir-inp">
                  凭据目录路径
                </label>
                <input
                  id="org-source-dir-inp"
                  name="sourceDir"
                  type="text"
                  required
                  className="input-sharp font-mono-ui"
                  placeholder="C:\sing-box\data\ts-state-desktop 或 ~/sing-box/data/ts-state"
                  value={importSourceDir}
                  onChange={(e) => setImportSourceDir(e.target.value)}
                />
              </div>

              <div className="form-field">
                <label className="form-label" htmlFor="org-import-alias-inp">
                  组织备注名（选填）
                </label>
                <input
                  id="org-import-alias-inp"
                  name="importAlias"
                  type="text"
                  className="input-sharp"
                  placeholder="留空自动读取 Tailscale 组织名"
                  value={importAlias}
                  onChange={(e) => setImportAlias(e.target.value)}
                />
              </div>

              {importDirError && (
                <div
                  className="inline-banner is-rollback"
                  role="alert"
                  style={{ border: "1px solid var(--status-red-border)" }}
                >
                  <span>{importDirError}</span>
                </div>
              )}

              <div
                className="modal-foot"
                style={{
                  margin: "4px -16px -16px",
                  justifyContent: "space-between",
                }}
              >
                <button
                  type="button"
                  className="btn-hairline btn-sm"
                  onClick={() => {
                    setOrgAdvancedOpen(false);
                    setOrgTab("login");
                  }}
                >
                  收起高级选项
                </button>
                <div style={{ display: "flex", gap: 8 }}>
                  <button type="button" className="btn-hairline" onClick={onClose}>
                    取消
                  </button>
                  <button
                    type="submit"
                    className="btn-lime"
                    disabled={submitting || !importSourceDir.trim()}
                    data-testid="btn-confirm-import-org-dir"
                  >
                    {submitting ? "导入中…" : "导入并同步节点"}
                  </button>
                </div>
              </div>
            </form>
          )}
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
