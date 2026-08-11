import { describe, expect, test } from 'bun:test';
import type { TFunction } from 'i18next';
import {
  buildQuotaSearchValues,
  matchesQuotaSearch,
  type QuotaSearchStore,
} from '../src/pro/modules/quota/extensions/quotaSearch';

const emptyStore: QuotaSearchStore = {
  antigravityQuota: {},
  claudeQuota: {},
  codexQuota: {},
  geminiCliQuota: {},
  xaiQuota: {},
};

const translate = ((key: string) => key) as TFunction;

describe('quota search', () => {
  test('matches an auth file by email', () => {
    const values = buildQuotaSearchValues(
      {
        name: 'kimi-1712345678901.json',
        type: 'kimi',
        email: 'user@example.com',
      },
      emptyStore,
      translate
    );

    expect(matchesQuotaSearch(values, 'USER@EXAMPLE')).toBe(true);
    expect(matchesQuotaSearch(values, 'user@*com')).toBe(true);
  });
});
