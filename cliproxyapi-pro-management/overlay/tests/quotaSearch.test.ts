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

  test('keeps explicit paid plans and recognizes zero-monthly on-demand plans', () => {
    const store: QuotaSearchStore = {
      ...emptyStore,
      xaiQuota: {
        'paid-health.json': { billing: { planType: 'paid', monthlyLimitCents: null } },
        'on-demand.json': {
          billing: {
            planType: 'free',
            monthlyLimitCents: 0,
            onDemandCapCents: 50_000,
            onDemandUsedCents: 0,
          },
        },
      },
    };

    const paidHealth = buildQuotaSearchValues(
      { name: 'paid-health.json', type: 'xai' },
      store,
      translate
    );
    const onDemand = buildQuotaSearchValues(
      { name: 'on-demand.json', type: 'xai' },
      store,
      translate
    );

    expect(paidHealth).toContain('paid');
    expect(paidHealth).not.toContain('free');
    expect(onDemand).toContain('paid-unknown');
    expect(onDemand).not.toContain('free');
  });
});
