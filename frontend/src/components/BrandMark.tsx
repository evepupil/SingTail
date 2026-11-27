/** Pixel-snapped 20×20 SingTail exit-node matrix brand mark, adapting to dark and light themes via CSS tokens. */
export function BrandMark({ size = 20 }: { size?: number }) {
  return (
    <span className="brand-mark" aria-hidden="true">
      <svg
        width={size}
        height={size}
        viewBox="0 0 20 20"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <rect
          x="0.5"
          y="0.5"
          width="19"
          height="19"
          rx="2"
          fill="var(--bg-code)"
          stroke="var(--border-strong)"
          strokeWidth="1"
        />
        {/* Inactive peer nodes in 3x3 mesh */}
        <rect x="2.5" y="2.5" width="3.5" height="3.5" rx="0.5" fill="var(--gray-400)" opacity="0.55" />
        <rect x="8.25" y="2.5" width="3.5" height="3.5" rx="0.5" fill="var(--gray-400)" opacity="0.55" />
        <rect x="2.5" y="8.25" width="3.5" height="3.5" rx="0.5" fill="var(--gray-400)" opacity="0.55" />
        <rect x="14" y="8.25" width="3.5" height="3.5" rx="0.5" fill="var(--gray-400)" opacity="0.55" />
        <rect x="14" y="14" width="3.5" height="3.5" rx="0.5" fill="var(--gray-400)" opacity="0.55" />
        {/* Routed exit trajectory */}
        <path d="M6 15.75H8.25M10 14V11.75" stroke="var(--text-primary)" strokeWidth="1.5" />
        <path d="M11.5 8.5L14.25 5.75" stroke="var(--accent-text)" strokeWidth="1.6" strokeLinecap="square" />
        {/* Active tail path & highlighted exit node */}
        <rect x="2.5" y="14" width="3.5" height="3.5" rx="0.5" fill="var(--text-primary)" />
        <rect x="8.25" y="14" width="3.5" height="3.5" rx="0.5" fill="var(--text-primary)" />
        <rect x="8.25" y="8.25" width="3.5" height="3.5" rx="0.5" fill="var(--text-primary)" />
        <rect
          x="14"
          y="2.5"
          width="3.5"
          height="3.5"
          rx="0.5"
          fill="var(--accent-fill)"
          stroke="var(--accent-text)"
          strokeWidth="0.6"
        />
      </svg>
    </span>
  );
}
