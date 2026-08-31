export type TimeRangePreset = 'today' | '7d' | '30d' | 'all';

export type CustomTimeRange = {
  fromMs: number;
  toMs: number;
};

export type TimeRangeSelection =
  | { type: 'preset'; preset: TimeRangePreset }
  | { type: 'custom'; range: CustomTimeRange };

export type ResolvedTimeRange = CustomTimeRange & {
  interval: 'hour' | 'day';
};

export const TIME_RANGE_PRESETS: TimeRangePreset[] = ['today', '7d', '30d', 'all'];
export const DEFAULT_TIME_RANGE: TimeRangeSelection = { type: 'preset', preset: 'today' };

const SECOND_MS = 1000;
const MINUTE_MS = 60 * SECOND_MS;
const TWO_DAYS_MS = 2 * 24 * 60 * 60 * 1000;

export function createPresetTimeRange(preset: TimeRangePreset): TimeRangeSelection {
  return { type: 'preset', preset };
}

export function normalizeCustomTimeRange(fromMs: number, toMs: number): CustomTimeRange | null {
  if (!Number.isFinite(fromMs) || !Number.isFinite(toMs)) return null;
  const normalizedFromMs = Math.floor(fromMs / MINUTE_MS) * MINUTE_MS;
  const normalizedToMs = Math.floor(toMs / MINUTE_MS) * MINUTE_MS + MINUTE_MS - 1;
  if (normalizedFromMs < 0 || normalizedToMs < normalizedFromMs) return null;
  return { fromMs: normalizedFromMs, toMs: normalizedToMs };
}

export function createCustomTimeRange(fromMs: number, toMs: number): TimeRangeSelection | null {
  const range = normalizeCustomTimeRange(fromMs, toMs);
  return range ? { type: 'custom', range } : null;
}

export function resolveTimeRange(selection: TimeRangeSelection, nowMs = Date.now()): ResolvedTimeRange {
  if (selection.type === 'custom') {
    const normalized = normalizeCustomTimeRange(selection.range.fromMs, selection.range.toMs)
      ?? { fromMs: 0, toMs: Math.max(0, nowMs) };
    return {
      ...normalized,
      interval: normalized.toMs - normalized.fromMs <= TWO_DAYS_MS ? 'hour' : 'day',
    };
  }

  const toMs = Math.max(0, nowMs);
  if (selection.preset === 'all') return { fromMs: 0, toMs, interval: 'day' };

  const start = new Date(toMs);
  start.setHours(0, 0, 0, 0);
  if (selection.preset === '7d') start.setDate(start.getDate() - 6);
  if (selection.preset === '30d') start.setDate(start.getDate() - 29);
  return {
    fromMs: start.getTime(),
    toMs,
    interval: selection.preset === 'today' ? 'hour' : 'day',
  };
}

export function getTimeRangeDurationMinutes(
  selection: TimeRangeSelection,
  nowMs = Date.now(),
  observedFromMs?: number
): number {
  const resolved = resolveTimeRange(selection, nowMs);
  const useObservedStart = selection.type === 'preset'
    && selection.preset === 'all'
    && Number.isFinite(observedFromMs)
    && Number(observedFromMs) >= 0
    && Number(observedFromMs) <= resolved.toMs;
  const fromMs = useObservedStart ? Number(observedFromMs) : resolved.fromMs;
  return Math.max((resolved.toMs - fromMs + 1) / MINUTE_MS, 1);
}

export function timeRangeCoversElapsedLocalToday(selection: TimeRangeSelection, nowMs = Date.now()): boolean {
  const resolved = resolveTimeRange(selection, nowMs);
  const todayStart = new Date(nowMs);
  todayStart.setHours(0, 0, 0, 0);
  return resolved.fromMs <= todayStart.getTime() && resolved.toMs >= nowMs;
}

export function getTimeRangeKey(selection: TimeRangeSelection): string {
  if (selection.type === 'preset') return selection.preset;
  const range = normalizeCustomTimeRange(selection.range.fromMs, selection.range.toMs);
  return range ? `custom:${range.fromMs}:${range.toMs}` : 'custom:invalid';
}

export function getLocalTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone?.trim() || '';
  } catch {
    return '';
  }
}

const pad = (value: number) => String(value).padStart(2, '0');

export function formatDateTimeLocalValue(timestampMs: number): string {
  const date = new Date(timestampMs);
  if (!Number.isFinite(date.getTime())) return '';
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
    + `T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function parseDateTimeLocalValue(value: string): number | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null;
  const timestampMs = new Date(value).getTime();
  if (!Number.isFinite(timestampMs)) return null;
  const normalizedTimestampMs = Math.floor(timestampMs / MINUTE_MS) * MINUTE_MS;
  return formatDateTimeLocalValue(normalizedTimestampMs) === value ? normalizedTimestampMs : null;
}

export function formatCustomTimeRange(selection: TimeRangeSelection, locale?: string): string {
  if (selection.type !== 'custom') return '';
  const formatter = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
  return `${formatter.format(selection.range.fromMs)} – ${formatter.format(selection.range.toMs)}`;
}
