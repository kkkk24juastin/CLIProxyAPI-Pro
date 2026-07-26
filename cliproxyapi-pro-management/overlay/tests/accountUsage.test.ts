import { describe, expect, test } from 'bun:test';
import {
  buildAccountUsageLogPath,
  maskAccountUsageAPIKeyHash,
  ratio,
} from '../src/features/monitoring/accountUsage';

describe('account usage helpers', () => {
  test('builds an exact, encoded request-log scope', () => {
    expect(buildAccountUsageLogPath('codex:user+one@example.com', 100.4, 200.6)).toBe(
      '/monitoring?auth_index=codex%3Auser%2Bone%40example.com&from_ms=100&to_ms=201#request-events'
    );
  });

  test('masks attributed API key hashes and labels unattributed usage', () => {
    expect(maskAccountUsageAPIKeyHash('', 'Unattributed')).toBe('Unattributed');
    expect(maskAccountUsageAPIKeyHash('1234567890abcdef', 'Unattributed')).not.toBe('1234567890abcdef');
  });

  test('clamps ratios and handles empty denominators', () => {
    expect(ratio(1, 4)).toBe(0.25);
    expect(ratio(3, 2)).toBe(1);
    expect(ratio(1, 0)).toBe(0);
  });
});
