import { describe, expect, test } from 'bun:test';
import { loadInspectionDetailsWithRetry } from '../src/pro/modules/inspection/hooks/useInspectionDetailsLoader';

describe('account inspection details loader', () => {
  test('retries transient detail failures with bounded exponential delays', async () => {
    let calls = 0;
    const delays: number[] = [];
    const value = await loadInspectionDetailsWithRetry(async () => {
      calls += 1;
      if (calls < 3) throw new Error(`transient-${calls}`);
      return 'loaded';
    }, async (milliseconds) => { delays.push(milliseconds); });

    expect(value).toBe('loaded');
    expect(calls).toBe(3);
    expect(delays).toEqual([500, 1000]);
  });

  test('surfaces the final detail error after the retry budget', async () => {
    let calls = 0;
    await expect(loadInspectionDetailsWithRetry(async () => {
      calls += 1;
      throw new Error('still unavailable');
    }, async () => {}, 3)).rejects.toThrow('still unavailable');
    expect(calls).toBe(3);
  });

  test('stops retrying and rejects when the current detail request is canceled', async () => {
    const controller = new AbortController();
    let calls = 0;
    let delays = 0;
    const pending = loadInspectionDetailsWithRetry(async () => {
      calls += 1;
      throw new Error('transient');
    }, async (_milliseconds, signal) => {
      delays += 1;
      controller.abort(new Error('superseded'));
      if (signal.aborted) throw signal.reason;
    }, 3, controller.signal);

    await expect(pending).rejects.toThrow('superseded');
    expect(calls).toBe(1);
    expect(delays).toBe(1);
  });

  test('does not accept a response that resolves after cancellation', async () => {
    const controller = new AbortController();
    let resolveLoad: ((value: string) => void) | undefined;
    const pending = loadInspectionDetailsWithRetry(
      () => new Promise<string>((resolve) => { resolveLoad = resolve; }),
      async () => {},
      1,
      controller.signal
    );
    while (!resolveLoad) await Promise.resolve();
    controller.abort(new Error('stale response'));
    resolveLoad('old page');

    await expect(pending).rejects.toThrow('stale response');
  });
});
