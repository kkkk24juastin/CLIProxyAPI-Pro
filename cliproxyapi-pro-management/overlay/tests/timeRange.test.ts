import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  createCustomTimeRange,
  createPresetTimeRange,
  formatDateTimeLocalValue,
  getTimeRangeDurationMinutes,
  getTimeRangeKey,
  normalizeCustomTimeRange,
  parseDateTimeLocalValue,
  resolveTimeRange,
  timeRangeCoversElapsedLocalToday,
} from '../src/pro/modules/monitoring/features/timeRange/timeRange';

describe('shared monitoring time range', () => {
  test('uses the shared modal stack for the mobile custom editor', () => {
    const selectorSource = readFileSync(resolve(
      import.meta.dir,
      '../src/pro/modules/monitoring/features/timeRange/TimeRangeSelector.tsx'
    ), 'utf8');
    const styleSource = readFileSync(resolve(
      import.meta.dir,
      '../src/pro/modules/monitoring/features/timeRange/TimeRangeSelector.module.scss'
    ), 'utf8');

    expect(selectorSource).toContain("window.matchMedia('(max-width: 620px)')");
    expect(selectorSource).toContain("import { ProFormDialog } from '@/pro/shared/ProSurface'");
    expect(selectorSource).toContain('open={editingCustom && useMobileDialog}');
    expect(selectorSource).toContain('className={styles.mobileDialog}');
    expect(selectorSource).toContain('onClick={() => editingCustom ? closeCustom() : openCustom()}');
    expect(selectorSource).toContain('if (!editingCustom || useMobileDialog) return');
    expect(selectorSource).toContain("document.addEventListener('keydown', closeOnEscape, true)");
    expect(selectorSource).not.toContain('createPortal(');
    expect(styleSource).not.toContain('z-index: 2100');
    expect(styleSource).toContain('width: min(440px, calc(100vw - 24px));');
    expect(styleSource).toContain(':global(.modal-overlay):has(.mobileDialog)');
    expect(styleSource).toContain('grid-template-columns: 1fr 1fr;');
    expect(styleSource).toContain('padding: 12px 16px calc(12px + env(safe-area-inset-bottom));');
    expect(styleSource).toContain('animation: mobile-panel-exit 0.35s cubic-bezier(0.4, 0, 0.2, 1) forwards;');
  });

  test('resolves presets from local calendar boundaries', () => {
    const now = new Date(2026, 7, 31, 12, 34, 56, 789);
    const today = resolveTimeRange(createPresetTimeRange('today'), now.getTime());
    const sevenDays = resolveTimeRange(createPresetTimeRange('7d'), now.getTime());

    expect(new Date(today.fromMs).getHours()).toBe(0);
    expect(new Date(today.fromMs).getMinutes()).toBe(0);
    expect(today.toMs).toBe(now.getTime());
    expect(today.interval).toBe('hour');
    expect(new Date(sevenDays.fromMs).getDate()).toBe(25);
    expect(sevenDays.interval).toBe('day');
  });

  test('normalizes visible minute precision to an inclusive end minute', () => {
    expect(normalizeCustomTimeRange(10_456, 20_123)).toEqual({
      fromMs: 0,
      toMs: 59_999,
    });
    expect(normalizeCustomTimeRange(61_000, 20_000)).toBeNull();
  });

  test('round trips datetime-local values at minute precision', () => {
    const timestamp = new Date(2026, 7, 31, 8, 9, 10, 987).getTime();
    const value = formatDateTimeLocalValue(timestamp);

    expect(value).toBe('2026-08-31T08:09');
    expect(parseDateTimeLocalValue('2026-08-31T08:09')).toBe(new Date(2026, 7, 31, 8, 9, 0, 0).getTime());
    expect(parseDateTimeLocalValue('2026-08-31T08:09:10')).toBeNull();
    expect(parseDateTimeLocalValue('2026-02-31T08:09')).toBeNull();
  });

  test('keeps custom windows fixed and gives them a stable query key', () => {
    const selection = createCustomTimeRange(10_000, 20_000);
    expect(selection).not.toBeNull();
    if (!selection) return;

    expect(resolveTimeRange(selection, 99_999)).toEqual({ fromMs: 0, toMs: 59_999, interval: 'hour' });
    expect(getTimeRangeKey(selection)).toBe('custom:0:59999');
  });

  test('computes rates from elapsed range time instead of bucket count', () => {
    const now = new Date(2026, 7, 31, 12, 0, 0, 0).getTime();
    const sevenDays = createPresetTimeRange('7d');
    const sevenDayStart = resolveTimeRange(sevenDays, now).fromMs;

    expect(getTimeRangeDurationMinutes(sevenDays, now)).toBeCloseTo(
      (now - sevenDayStart + 1) / 60_000,
      8
    );
    expect(getTimeRangeDurationMinutes(createPresetTimeRange('all'), now, now - 2 * 60 * 60 * 1000))
      .toBeCloseTo(120, 4);
  });

  test('only treats ranges covering the elapsed local day as complete today data', () => {
    const now = new Date(2026, 7, 31, 12, 0, 0, 0).getTime();
    const historical = createCustomTimeRange(
      new Date(2026, 7, 1, 0, 0, 0, 0).getTime(),
      new Date(2026, 7, 2, 23, 59, 59, 0).getTime()
    );
    const currentPartial = createCustomTimeRange(
      new Date(2026, 7, 31, 8, 0, 0, 0).getTime(),
      new Date(2026, 7, 31, 9, 0, 0, 0).getTime()
    );
    const currentComplete = createCustomTimeRange(
      new Date(2026, 7, 31, 0, 0, 0, 0).getTime(),
      now
    );

    expect(historical && timeRangeCoversElapsedLocalToday(historical, now)).toBe(false);
    expect(currentPartial && timeRangeCoversElapsedLocalToday(currentPartial, now)).toBe(false);
    expect(currentComplete && timeRangeCoversElapsedLocalToday(currentComplete, now)).toBe(true);
    expect(timeRangeCoversElapsedLocalToday(createPresetTimeRange('today'), now)).toBe(true);
  });
});
