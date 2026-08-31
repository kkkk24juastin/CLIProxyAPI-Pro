import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProFormDialog } from '@/pro/shared/ProSurface';
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
  const [useMobileDialog, setUseMobileDialog] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const customButtonRef = useRef<HTMLButtonElement | null>(null);

  const closeCustom = useCallback((restoreFocus = false) => {
    setEditingCustom(false);
    if (restoreFocus && !useMobileDialog) {
      window.setTimeout(() => customButtonRef.current?.focus({ preventScroll: true }), 0);
    }
  }, [useMobileDialog]);

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
    const mediaQuery = window.matchMedia('(max-width: 620px)');
    const syncDialogMode = () => setUseMobileDialog(mediaQuery.matches);
    syncDialogMode();
    mediaQuery.addEventListener('change', syncDialogMode);
    return () => mediaQuery.removeEventListener('change', syncDialogMode);
  }, []);

  useEffect(() => {
    if (!editingCustom || useMobileDialog) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      closeCustom(true);
    };
    document.addEventListener('keydown', closeOnEscape, true);
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (event.target instanceof Node && !rootRef.current?.contains(event.target)) {
        closeCustom();
      }
    };
    document.addEventListener('mousedown', closeOnOutsideClick);
    return () => {
      document.removeEventListener('keydown', closeOnEscape, true);
      document.removeEventListener('mousedown', closeOnOutsideClick);
    };
  }, [closeCustom, editingCustom, useMobileDialog]);

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
    closeCustom(true);
  };

  const editorFields = (
    <div className={styles.fields}>
      <label>
        <span>{t('time_range.start')}</span>
        <input
          type="datetime-local"
          step="60"
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
          step="60"
          value={toValue}
          onChange={(event) => {
            setToValue(event.target.value);
            setErrorKey('');
          }}
        />
      </label>
    </div>
  );

  const editorActions = (className: string) => (
    <div className={className}>
      <button type="button" onClick={() => closeCustom(true)}>{t('time_range.cancel')}</button>
      <button type="button" className={styles.apply} onClick={applyCustom}>{t('time_range.apply')}</button>
    </div>
  );

  const desktopPanel = editingCustom && !useMobileDialog ? (
    <div
      className={`${styles.customPanel} ${panelAlign === 'end' ? styles.panelEnd : styles.panelStart}`}
      role="dialog"
      aria-label={t('time_range.custom')}
    >
      {editorFields}
      {errorKey ? <p className={styles.error}>{t(errorKey)}</p> : null}
      {editorActions(styles.actions)}
    </div>
  ) : null;

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
          ref={customButtonRef}
          type="button"
          className={value.type === 'custom' || editingCustom ? styles.active : undefined}
          onClick={openCustom}
          disabled={disabled}
          aria-haspopup="dialog"
          aria-expanded={editingCustom}
        >
          {t('time_range.custom')}
        </button>
      </div>

      {desktopPanel}
      <ProFormDialog
        open={editingCustom && useMobileDialog}
        title={t('time_range.custom')}
        onClose={() => closeCustom()}
        footer={editorActions(styles.mobileActions)}
      >
        {editorFields}
        {errorKey ? <p className={styles.error}>{t(errorKey)}</p> : null}
      </ProFormDialog>
    </div>
  );
}
