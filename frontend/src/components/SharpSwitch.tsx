interface SharpSwitchProps {
  checked: boolean;
  onToggle: () => void;
  ariaLabel: string;
  disabled?: boolean;
  testId?: string;
}

export function SharpSwitch({
  checked,
  onToggle,
  ariaLabel,
  disabled = false,
  testId,
}: SharpSwitchProps) {
  return (
    <button
      type="button"
      className="sharp-switch"
      role="switch"
      aria-label={ariaLabel}
      aria-checked={checked}
      disabled={disabled}
      data-testid={testId}
      onClick={onToggle}
    >
      <span className="sharp-switch-thumb" />
    </button>
  );
}
