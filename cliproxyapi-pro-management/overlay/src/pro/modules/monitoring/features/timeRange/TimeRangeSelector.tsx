import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  TIME_RANGE_PRESETS,
  createCustomTimeRange,
  createPresetTimeRange,
  formatDateTimeLocalValue,
  parseDateTimeLocalValue,
  type TimeRangeSelection,
} from './timeRange';
import styles from './TimeRangeSelector.module.scss';

type TimeRangeSelectorProps = {
  value: TimeRangeSelection;
  onChange: (value: TimeRangeSelection) => void;
  disabled?: boolean;
  className?: string;
  panelAlign?: 'start' | 'end';
};

const presetTranslationKey = (preset: string) => `time_range.${preset === '7d' ? 'days_7' : preset === '30d' ? 'days_30' : preset}`;

export function TimeRangeSelector({
  value,
  onChange,
  disabled = false,
  className = '',
  panelAlign = 'start',
}: TimeRangeSelectorProps) {
  const { t } = useTranslation();
  const [editingCustom, setEditingCustom] = useState(false);
  const [fromValue, setFromValue] = useState('');
  const [toValue, setToValue] = useState('');
  const [errorKey, setErrorKey] = useState('');
  const rootRef = useRef<HTMLDivElement | null>(null);

  const openCustom = () => {
    const nowMs = Date.now();
    const todayStart = new Date(nowMs);
    todayStart.setHours(0, 0, 0, 0);
    setFromValue(formatDateTimeLocalValue(value.type === 'custom' ? value.range.fromMs : todayStart.getTime()));
    setToValue(formatDateTimeLocalValue(value.type === 'custom' ? value.range.toMs : nowMs));
    setErrorKey('');
    setEditingCustom(true);
  };

  useEffect(() => {
    if (!editingCustom) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setEditingCustom(false);
    };
    window.addEventListener('keydown', closeOnEscape);
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (event.target instanceof Node && !rootRef.current?.contains(event.target)) {
        setEditingCustom(false);
      }
    };
    document.addEventListener('mousedown', closeOnOutsideClick);
    return () => {
      window.removeEventListener('keydown', closeOnEscape);
      document.removeEventListener('mousedown', closeOnOutsideClick);
    };
  }, [editingCustom]);

  const applyCustom = () => {
    const fromMs = parseDateTimeLocalValue(fromValue);
    const toMs = parseDateTimeLocalValue(toValue);
    if (fromMs === null || toMs === null) {
      setErrorKey('time_range.required_error');
      return;
    }
    const selection = createCustomTimeRange(fromMs, toMs);
    if (!selection) {
      setErrorKey('time_range.order_error');
      return;
    }
    onChange(selection);
    setErrorKey('');
    setEditingCustom(false);
  };

  return (
    <div ref={rootRef} className={`${styles.root} ${className}`.trim()}>
      <div className={styles.presets} role="group" aria-label={t('time_range.label')}>
        {TIME_RANGE_PRESETS.map((preset) => (
          <button
            key={preset}
            type="button"
            className={value.type === 'preset' && value.preset === preset ? styles.active : undefined}
            onClick={() => {
              setEditingCustom(false);
              onChange(createPresetTimeRange(preset));
            }}
            disabled={disabled}
          >
            {t(presetTranslationKey(preset))}
          </button>
        ))}
        <button
          type="button"
          className={value.type === 'custom' || editingCustom ? styles.active : undefined}
          onClick={openCustom}
          disabled={disabled}
          aria-expanded={editingCustom}
        >
          {t('time_range.custom')}
        </button>
      </div>

      {editingCustom ? (
        <div
          className={`${styles.customPanel} ${panelAlign === 'end' ? styles.panelEnd : styles.panelStart}`}
          role="dialog"
          aria-label={t('time_range.custom')}
        >
          <div className={styles.fields}>
            <label>
              <span>{t('time_range.start')}</span>
              <input
                type="datetime-local"
                step="1"
                value={fromValue}
                onChange={(event) => {
                  setFromValue(event.target.value);
                  setErrorKey('');
                }}
              />
            </label>
            <label>
              <span>{t('time_range.end')}</span>
              <input
                type="datetime-local"
                step="1"
                value={toValue}
                onChange={(event) => {
                  setToValue(event.target.value);
                  setErrorKey('');
                }}
              />
            </label>
          </div>
          {errorKey ? <p className={styles.error}>{t(errorKey)}</p> : null}
          <div className={styles.actions}>
            <button type="button" onClick={() => setEditingCustom(false)}>{t('time_range.cancel')}</button>
            <button type="button" className={styles.apply} onClick={applyCustom}>{t('time_range.apply')}</button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
