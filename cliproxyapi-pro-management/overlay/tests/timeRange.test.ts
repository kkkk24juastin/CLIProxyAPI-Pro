import { describe, expect, test } from 'bun:test';
import {
  createCustomTimeRange,
  createPresetTimeRange,
  formatDateTimeLocalValue,
  getTimeRangeDurationMinutes,
  getTimeRangeKey,
  normalizeCustomTimeRange,
  parseDateTimeLocalValue,
  resolveTimeRange,
  timeRangeIncludesLocalToday,
} from '../src/pro/modules/monitoring/features/timeRange/timeRange';

describe('shared monitoring time range', () => {
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

  test('normalizes visible second precision to an inclusive end second', () => {
    expect(normalizeCustomTimeRange(10_456, 20_123)).toEqual({
      fromMs: 10_000,
      toMs: 20_999,
    });
    expect(normalizeCustomTimeRange(21_000, 20_000)).toBeNull();
  });

  test('round trips datetime-local values without exposing milliseconds', () => {
    const timestamp = new Date(2026, 7, 31, 8, 9, 10, 987).getTime();
    const value = formatDateTimeLocalValue(timestamp);

    expect(value).toBe('2026-08-31T08:09:10');
    expect(parseDateTimeLocalValue(value)).toBe(new Date(2026, 7, 31, 8, 9, 10, 0).getTime());
    expect(parseDateTimeLocalValue('2026-08-31T08:09')).toBe(new Date(2026, 7, 31, 8, 9, 0, 0).getTime());
    expect(parseDateTimeLocalValue('2026-02-31T08:09:10')).toBeNull();
  });

  test('keeps custom windows fixed and gives them a stable query key', () => {
    const selection = createCustomTimeRange(10_000, 20_000);
    expect(selection).not.toBeNull();
    if (!selection) return;

    expect(resolveTimeRange(selection, 99_999)).toEqual({ fromMs: 10_000, toMs: 20_999, interval: 'hour' });
    expect(getTimeRangeKey(selection)).toBe('custom:10000:20999');
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

  test('detects whether a custom range includes the current local day', () => {
    const now = new Date(2026, 7, 31, 12, 0, 0, 0).getTime();
    const historical = createCustomTimeRange(
      new Date(2026, 7, 1, 0, 0, 0, 0).getTime(),
      new Date(2026, 7, 2, 23, 59, 59, 0).getTime()
    );
    const current = createCustomTimeRange(
      new Date(2026, 7, 31, 8, 0, 0, 0).getTime(),
      new Date(2026, 7, 31, 9, 0, 0, 0).getTime()
    );

    expect(historical && timeRangeIncludesLocalToday(historical, now)).toBe(false);
    expect(current && timeRangeIncludesLocalToday(current, now)).toBe(true);
  });
});
