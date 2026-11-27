// View 3: Minimalist Rule Sets & Subscriptions
import { esc } from "../core/utils.js";

export function renderProvidersView(state) {
  const cardsHtml = state.providers
    .map(
      (p) => `
      <article class="provider-card" data-testid="provider-card-${esc(p.tag)}">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
          <span class="font-mono-ui truncate" style="font-size:14px;font-weight:600;color:var(--text-primary);">${esc(p.tag)}</span>
          <span class="tag-sharp">${esc(p.sourceName)}</span>
        </div>

        <div class="font-mono-ui truncate selectable-text" title="${esc(p.url)}" style="font-size:11px;color:var(--text-subtle);padding:6px 8px;background:var(--bg-code);border:1px solid var(--border-hairline);">
          ${esc(p.url)}
        </div>

        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;font-family:var(--font-mono);font-size:11.5px;color:var(--text-muted);">
          <span>${p.entryCount.toLocaleString()} 条</span>
          <span>${esc(p.updatedAt)}</span>
        </div>
      </article>
    `
    )
    .join("");

  return `
    <div class="view-topbar">
      <h1 class="view-title">规则集</h1>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="refresh-providers" data-testid="btn-refresh-providers">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9M13.5 2.5v3h-3"/></svg>
          <span>更新全部</span>
        </button>
        <button type="button" class="btn-lime" data-open-modal="add-provider" data-testid="btn-add-provider">+ 添加订阅</button>
      </div>
    </div>

    <div class="view-scroll">
      <div class="cards-2col">
        ${cardsHtml}
      </div>
    </div>
  `;
}
