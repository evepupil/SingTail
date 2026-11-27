// Shared formatting & escaping helpers for SingTail views

export function esc(str) {
  return String(str ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

export function renderActionBadge(action, exitLabel = "默认出口") {
  switch (action) {
    case "EXIT":
      return `<span class="tag-sharp tag-lime-outline">↗ ${esc(exitLabel)}</span>`;
    case "DIRECT":
      return `<span class="tag-sharp tag-green">→ 直连宽带</span>`;
    case "FALLBACK_PROXY":
      return `<span class="tag-sharp tag-cyan">⇄ 兜底代理</span>`;
    case "REJECT":
      return `<span class="tag-sharp tag-red">✕ 拦截阻断</span>`;
    default:
      return `<span class="tag-sharp">${esc(action)}</span>`;
  }
}

export function renderLatencyTag(ms, healthy = true) {
  if (!healthy) {
    return `<span class="tag-sharp tag-red">超时</span>`;
  }
  if (ms <= 70) {
    return `<span class="tag-sharp tag-green">${ms} ms</span>`;
  }
  if (ms <= 165) {
    return `<span class="tag-sharp tag-lime-outline">${ms} ms</span>`;
  }
  return `<span class="tag-sharp tag-amber">${ms} ms</span>`;
}
