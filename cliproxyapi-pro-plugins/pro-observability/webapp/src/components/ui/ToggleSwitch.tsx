type ToggleSwitchProps = {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label?: string;
  ariaLabel?: string;
  disabled?: boolean;
	labelPosition?: string;
};

export function ToggleSwitch({ checked, onChange, label, ariaLabel, disabled }: ToggleSwitchProps) {
  return (
    <label className="toggle-switch" aria-disabled={disabled || undefined}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        aria-label={ariaLabel || label}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span aria-hidden="true" />
      {label ? <em>{label}</em> : null}
    </label>
  );
}
