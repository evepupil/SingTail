// View 3: Rule Sets & Remote Subscriptions (.srs Binary Rule Providers)
import { esc } from "../core/utils.js";

export function renderProvidersView(state) {
  const totalEntries = state.providers.reduce((acc, p) => acc + p.entryCount, 0);

  const cardsHtml = state.providers
    .map(
      (p) => `
      <article class="provider-card" data-testid="provider-card-${esc(p.tag)}">
        <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:8px;">
          <div style="min-width:0;">
            <div class="font-mono-ui truncate" style="font-size:14px;font-weight:600;color:var(--text-primary);">${esc(p.tag)}</div>
            <div class="truncate" style="font-size:12px;color:var(--text-muted);margin-top:2px;">${esc(p.sourceName)}</div>
          </div>
          <span class="tag-sharp ${p.tag === "geoip-cn" ? "tag-lime-outline" : "tag-green"}">${esc(p.badge)}</span>
        </div>

        <div class="font-mono-ui truncate selectable-text" title="${esc(p.url)}" style="font-size:11px;color:var(--text-subtle);padding:6px 8px;background:var(--bg-code);border:1px solid var(--border-hairline);">
          ${esc(p.url)}
        </div>

        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;padding-top:6px;border-top:1px solid var(--border-hairline);font-family:var(--font-mono);font-size:11.5px;color:var(--text-muted);">
          <span>条目 ${p.entryCount.toLocaleString()} · 反编译校验通过</span>
          <span>${esc(p.updatedAt)}</span>
        </div>
      </article>
    `
    )
    .join("");

  return `
    <div class="view-topbar">
      <div class="view-title-group">
        <h1 class="view-title">规则集与订阅</h1>
        <span class="tag-sharp">${state.providers.length} 个规则包 · 共 ${totalEntries.toLocaleString()} 条</span>
      </div>
      <div class="view-actions">
        <button type="button" class="btn-hairline" data-action="refresh-providers" data-testid="btn-refresh-providers">↻ 全部更新并验包</button>
        <button type="button" class="btn-lime" data-open-modal="add-provider" data-testid="btn-add-provider">+ 添加规则订阅</button>
      </div>
    </div>

    <div class="section-bar">
      <span class="font-mono-ui" style="font-size:12px;color:var(--text-muted);">
        自动更新周期：每 ${state.settings.autoUpdateRulesDays} 天 · 下载后经 sing-box 反编译校验无误才覆盖本地文件
      </span>
      <span class="tag-sharp tag-green">本地离线缓存就绪</span>
    </div>

    <div class="view-scroll">
      <div class="cards-2col">
        ${cardsHtml}
      </div>
    </div>
  `;
}
