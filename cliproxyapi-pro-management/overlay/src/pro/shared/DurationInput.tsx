import { useEffect, useId, useState } from 'react';

export interface DurationFieldProps<Unit extends string> {
  label: string;
  value: string;
  unit: Unit;
  unitLabel: string;
  fallback: number;
  disabled?: boolean;
  onChange: (value: string) => void;
}

interface DurationInputProps<Unit extends string> extends DurationFieldProps<Unit> {
  className: string;
  min: number;
  step: number;
  inputMode: 'numeric' | 'decimal';
  parse: (value: string, unit: Unit) => number | null;
  normalize: (value: number) => number;
  serialize: (value: number, unit: Unit) => string;
}

const formatDurationNumber = (value: number): string => String(Math.round(value * 1000) / 1000);

export function DurationInput<Unit extends string>({
  label,
  value,
  unit,
  unitLabel,
  fallback,
  className,
  min,
  step,
  inputMode,
  disabled = false,
  parse,
  normalize,
  serialize,
  onChange,
}: DurationInputProps<Unit>) {
  const inputId = useId();
  const numericValue = parse(value, unit) ?? fallback;
  const [text, setText] = useState(() => formatDurationNumber(numericValue));

  useEffect(() => {
    setText(formatDurationNumber(numericValue));
  }, [numericValue]);

  const commit = () => {
    const next = Number(text);
    if (!Number.isFinite(next) || next <= 0) {
      setText(formatDurationNumber(numericValue));
      return;
    }
    const normalized = normalize(next);
    setText(formatDurationNumber(normalized));
    if (Math.abs(normalized - numericValue) < 0.000001) return;
    onChange(serialize(normalized, unit));
  };

  return (
    <div className="form-group">
      <label htmlFor={inputId}>{label}</label>
      <div className={className}>
        <input
          id={inputId}
          className="input"
          type="number"
          min={min}
          step={step}
          inputMode={inputMode}
          value={text}
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === 'Enter') event.currentTarget.blur();
          }}
        />
        <span aria-hidden="true">{unitLabel}</span>
      </div>
    </div>
  );
}
