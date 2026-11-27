import type { ModalKind } from "../state/selectors";
import type { RuleProvider } from "../types/contract";

interface ProvidersViewProps {
  providers: readonly RuleProvider[];
  refreshing: boolean;
  onRefreshAll: () => void;
  onOpenModal: (modal: Exclude<ModalKind, null>) => void;
}

export function ProvidersView({
  providers,
  refreshing,
  onRefreshAll,
  onOpenModal,
}: ProvidersViewProps) {
  return (
    <>
      <div className="view-topbar">
        <h1 className="view-title">规则集</h1>
        <div className="view-actions">
          <button
            type="button"
            className="btn-hairline"
            disabled={refreshing}
            data-testid="btn-refresh-providers"
            onClick={onRefreshAll}
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              aria-hidden="true"
            >
              <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9M13.5 2.5v3h-3" />
            </svg>
            <span>{refreshing ? "更新中…" : "更新全部"}</span>
          </button>
          <button
            type="button"
            className="btn-lime"
            data-testid="btn-add-provider"
            onClick={() => onOpenModal("add-provider")}
          >
            + 添加订阅
          </button>
        </div>
      </div>

      <div className="view-scroll">
        <div className="cards-2col">
          {providers.map((p) => (
            <article
              key={p.id}
              className="provider-card"
              data-testid={`provider-card-${p.tag}`}
            >
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 8,
                }}
              >
                <span
                  className="font-mono-ui truncate"
                  style={{
                    fontSize: 14,
                    fontWeight: 600,
                    color: "var(--text-primary)",
                  }}
                >
                  {p.tag}
                </span>
                <span className="tag-sharp">{p.sourceName}</span>
              </div>

              <div
                className="font-mono-ui truncate selectable-text"
                title={p.url}
                style={{
                  fontSize: 11,
                  color: "var(--text-subtle)",
                  padding: "6px 8px",
                  background: "var(--bg-code)",
                  border: "1px solid var(--border-hairline)",
                }}
              >
                {p.url}
              </div>

              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 8,
                  fontFamily: "var(--font-mono)",
                  fontSize: "11.5px",
                  color: "var(--text-muted)",
                }}
              >
                <span>{p.entryCount.toLocaleString()} 条</span>
                <span>{p.updatedAt}</span>
              </div>
            </article>
          ))}
        </div>
      </div>
    </>
  );
}
