import { useEffect, useRef, useState } from "react";
import { buildCmdkItems, type ModalKind, type ViewId } from "../state/selectors";
import type { ExitNode } from "../types/contract";

interface CmdkPaletteProps {
  open: boolean;
  exitNodes: readonly ExitNode[];
  onClose: () => void;
  onSelectExit: (exitId: string) => void;
  onSelectView: (view: ViewId) => void;
  onOpenModal: (modal: Exclude<ModalKind, null>) => void;
}

export function CmdkPalette({
  open,
  exitNodes,
  onClose,
  onSelectExit,
  onSelectView,
  onOpenModal,
}: CmdkPaletteProps) {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (open) {
      setQuery("");
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [open]);

  const items = buildCmdkItems(exitNodes, query);

  const executeCmd = (cmd: string) => {
    const colonIdx = cmd.indexOf(":");
    const kind = colonIdx >= 0 ? cmd.slice(0, colonIdx) : "";
    const val = colonIdx >= 0 ? cmd.slice(colonIdx + 1) : "";
    onClose();
    if (kind === "exit" && val) {
      onSelectExit(val);
    } else if (kind === "view" && val) {
      onSelectView(val as ViewId);
    } else if (kind === "modal" && val) {
      onOpenModal(val as Exclude<ModalKind, null>);
    }
  };

  return (
    <div
      id="cmdk-backdrop"
      className="modal-backdrop"
      hidden={!open}
      onClick={(e) => {
        if (e.target === e.currentTarget) {
          onClose();
        }
      }}
    >
      {open && (
        <div
          className="modal-panel"
          style={{ width: "min(480px, 100%)" }}
          role="dialog"
          aria-modal="true"
          aria-label="快速检索"
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "10px 14px",
              borderBottom: "1px solid var(--border-hairline)",
              background: "var(--bg-code)",
            }}
          >
            <span className="font-mono-ui" style={{ color: "var(--accent-text)" }}>
              ›
            </span>
            <input
              ref={inputRef}
              id="cmdk-search-input"
              type="search"
              placeholder="搜索出口、视图或动作…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && items[0]) {
                  e.preventDefault();
                  executeCmd(items[0].cmd);
                }
              }}
              style={{
                width: "100%",
                border: 0,
                background: "transparent",
                outline: "none",
                fontSize: 13,
              }}
            />
            <button
              type="button"
              className="tag-sharp"
              style={{ cursor: "pointer" }}
              onClick={onClose}
            >
              ESC
            </button>
          </div>
          <div style={{ maxHeight: 280, overflowY: "auto" }}>
            {items.map((it) => (
              <button
                key={it.cmd}
                type="button"
                onClick={() => executeCmd(it.cmd)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  width: "100%",
                  height: 36,
                  padding: "0 14px",
                  background: "transparent",
                  border: 0,
                  borderBottom: "1px solid var(--border-hairline)",
                  cursor: "pointer",
                  textAlign: "left",
                }}
              >
                <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
                  <span className="tag-sharp">{it.kind}</span>
                  <span className="truncate" style={{ color: "var(--text-primary)" }}>
                    {it.label}
                  </span>
                </div>
                <span
                  className="font-mono-ui"
                  style={{ fontSize: 11, color: "var(--text-subtle)" }}
                >
                  {it.sub}
                </span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
