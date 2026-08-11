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

  test('keeps the JWT-derived xAI plan when monthly billing is zero', () => {
    const values = buildQuotaSearchValues(
      { name: 'premium.json', type: 'xai' },
      {
        ...emptyStore,
        xaiQuota: {
          'premium.json': { billing: { planType: 'x-premium', monthlyLimitCents: 0 } },
        },
      },
      translate
    );

    expect(values).toContain('x-premium');
    expect(values).toContain('xai_quota.plan_x_premium');
    expect(values).not.toContain('free');
  });
});
