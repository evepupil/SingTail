import { useEffect, useState } from "react";
import { SharpSwitch } from "../components/SharpSwitch";
import type { ModalKind } from "../state/selectors";
import type { ExitNode, Organization, SystemSettings } from "../types/contract";

interface SettingsViewProps {
  orgs: readonly Organization[];
  exitNodes: readonly ExitNode[];
  settings: SystemSettings;
  onSyncOrg: (orgId: string) => void;
  onUpdateSettings: (next: SystemSettings) => void;
  onOpenModal: (modal: Exclude<ModalKind, null>) => void;
}

export function SettingsView({
  orgs,
  exitNodes,
  settings,
  onSyncOrg,
  onUpdateSettings,
  onOpenModal,
}: SettingsViewProps) {
  const [proxyAddrDraft, setProxyAddrDraft] = useState(
    settings.fallbackProxyAddr,
  );

  useEffect(() => {
    setProxyAddrDraft(settings.fallbackProxyAddr);
  }, [settings.fallbackProxyAddr]);

  const commitProxyAddr = () => {
    const trimmed = proxyAddrDraft.trim();
    if (!trimmed || trimmed === settings.fallbackProxyAddr) {
      setProxyAddrDraft(settings.fallbackProxyAddr);
      return;
    }
    onUpdateSettings({
      ...settings,
      fallbackProxyAddr: trimmed,
    });
  };

  return (
    <>
      <div className="view-topbar">
        <h1 className="view-title">设置</h1>
        <div className="view-actions">
          <button
            type="button"
            className="btn-hairline"
            data-testid="btn-export-config"
            onClick={() => onOpenModal("export-config")}
          >
            ⇩ 导出配置
          </button>
          <button
            type="button"
            className="btn-lime"
            onClick={() => onOpenModal("add-org")}
          >
            + 挂载组织
          </button>
        </div>
      </div>

      <div className="view-scroll">
        <div className="settings-stack">
          <section className="settings-block">
            <div className="settings-block-head">
              <span className="eyebrow-mono">
                Tailscale 组织 ({orgs.length})
              </span>
            </div>
            {orgs.map((org) => {
              const nodeCount = exitNodes.filter(
                (n) => n.orgId === org.id,
              ).length;
              return (
                <div key={org.id} className="settings-row">
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 10,
                      minWidth: 0,
                    }}
                  >
                    <span className="status-dot is-green" />
                    <span
                      style={{ fontWeight: 600, color: "var(--text-primary)" }}
                    >
                      {org.name}
                    </span>
                    <span
                      className="font-mono-ui truncate"
                      style={{ fontSize: 12, color: "var(--text-muted)" }}
                    >
                      {org.account}
                    </span>
                    <span className="tag-sharp">{nodeCount} 节点</span>
                  </div>
                  <div
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      flexShrink: 0,
                    }}
                  >
                    <span
                      className="font-mono-ui"
                      style={{ fontSize: 11, color: "var(--text-subtle)" }}
                    >
                      {org.lastSync}
                    </span>
                    <button
                      type="button"
                      className="btn-hairline btn-sm"
                      onClick={() => onSyncOrg(org.id)}
                    >
                      同步
                    </button>
                  </div>
                </div>
              );
            })}
          </section>

          <section className="settings-block">
            <div className="settings-block-head">
              <span className="eyebrow-mono">常规与转发</span>
            </div>

            <div className="settings-row">
              <span style={{ fontWeight: 500, color: "var(--text-primary)" }}>
                本地代理转发地址（供「本地代理」规则调用）
              </span>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  type="text"
                  className="input-sharp font-mono-ui"
                  style={{ width: 148 }}
                  aria-label="本地代理地址"
                  value={proxyAddrDraft}
                  onChange={(e) => setProxyAddrDraft(e.target.value)}
                  onBlur={commitProxyAddr}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.currentTarget.blur();
                    }
                  }}
                />
                <SharpSwitch
                  checked={settings.fallbackProxyEnabled}
                  ariaLabel="启用本地代理转发"
                  onToggle={() =>
                    onUpdateSettings({
                      ...settings,
                      fallbackProxyEnabled: !settings.fallbackProxyEnabled,
                    })
                  }
                />
              </div>
            </div>

            <div className="settings-row">
              <span style={{ fontWeight: 500, color: "var(--text-primary)" }}>
                开机自启
              </span>
              <SharpSwitch
                checked={settings.autoStartService}
                ariaLabel="开机自启"
                onToggle={() =>
                  onUpdateSettings({
                    ...settings,
                    autoStartService: !settings.autoStartService,
                  })
                }
              />
            </div>

            <div className="settings-row">
              <span style={{ fontWeight: 500, color: "var(--text-primary)" }}>
                切换出口核验公网 IP
              </span>
              <SharpSwitch
                checked={settings.autoVerifyIp}
                ariaLabel="切换出口核验公网 IP"
                onToggle={() =>
                  onUpdateSettings({
                    ...settings,
                    autoVerifyIp: !settings.autoVerifyIp,
                  })
                }
              />
            </div>
          </section>
        </div>
      </div>
    </>
  );
}
