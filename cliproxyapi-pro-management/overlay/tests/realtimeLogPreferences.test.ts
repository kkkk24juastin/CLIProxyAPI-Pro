import { describe, expect, test } from 'bun:test';
import {
  clampRealtimeLogColumnWidth,
  createDefaultRealtimeLogColumns,
  normalizeRealtimeLogColumns,
} from '../src/pro/modules/monitoring/features/realtimeLogPreferences';

describe('realtime log column preferences', () => {
  test('migrates legacy usage and inserts new columns beside the model column', () => {
    const columns = normalizeRealtimeLogColumns([
      { key: 'model', visible: true },
      { key: 'usage', visible: false },
      { key: 'time', visible: true },
    ]);

    expect(columns.slice(0, 4).map(({ key }) => key)).toEqual([
      'model',
      'reasoningEffort',
      'stream',
      'tokens',
    ]);
    expect(columns.find(({ key }) => key === 'tokens')?.visible).toBe(false);
    expect(columns.find(({ key }) => key === 'cacheRead')?.visible).toBe(false);
    expect(columns.at(-1)?.key).toBe('time');
  });

  test('merges legacy TTFT and total-latency preferences into one duration column', () => {
    const columns = normalizeRealtimeLogColumns([
      { key: 'model', visible: true },
      { key: 'ttft', visible: true, width: 92 },
      { key: 'latency', visible: false, width: 148 },
      { key: 'time', visible: true },
    ]);
    const durationColumns = columns.filter(({ key }) => key === 'latency');

    expect(durationColumns).toHaveLength(1);
    expect(durationColumns[0]).toMatchObject({ visible: true, width: 148 });
    expect(createDefaultRealtimeLogColumns().map(({ key }) => key)).not.toContain('ttft');
  });

  test('clamps persisted widths and restores defaults when every column is hidden', () => {
    expect(clampRealtimeLogColumnWidth('type', 999)).toBe(320);
    expect(clampRealtimeLogColumnWidth('model', 1)).toBe(132);

    const normalized = normalizeRealtimeLogColumns(
      createDefaultRealtimeLogColumns().map((column) => ({ ...column, visible: false }))
    );
    expect(normalized.every(({ visible }) => visible)).toBe(true);
  });

  test('drops the retired account-plan column from saved preferences', () => {
    const columns = normalizeRealtimeLogColumns([
      { key: 'type', visible: true, width: 96 },
      { key: 'accountPlan', visible: true, width: 132 },
      { key: 'model', visible: true },
    ]);

    expect(columns.map(({ key }) => key)).not.toContain('accountPlan');
    expect(columns.find(({ key }) => key === 'type')?.width).toBe(160);
  });
});
