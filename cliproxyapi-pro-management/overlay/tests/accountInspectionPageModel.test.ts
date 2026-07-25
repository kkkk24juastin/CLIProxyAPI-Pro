import { describe, expect, test } from 'bun:test';
import {
  buildInspectionResultsViewState,
  getPaginationRange,
  isXaiQuotaLow,
  resolveAccountInspectionPlanLabel,
  toSettingsDraft,
} from '../src/features/monitoring/accountInspectionPageModel';
import type { TFunction } from 'i18next';
import {
  DEFAULT_ACCOUNT_INSPECTION_SETTINGS,
  type AccountInspectionResultItem,
} from '../src/features/monitoring/accountInspection';

const result = (overrides: Partial<AccountInspectionResultItem> = {}): AccountInspectionResultItem => ({
  key: 'auth-1',
  fileName: 'account.json',
  displayAccount: 'owner@example.com',
  authIndex: 'auth-1',
  accountId: null,
  provider: 'codex',
  disabled: false,
  status: 'active',
  state: 'active',
  raw: { name: 'account.json', provider: 'codex' },
  action: 'keep',
  actionReason: '',
  statusCode: 200,
  usedPercent: 20,
  isQuota: false,
  error: '',
  ...overrides,
});

describe('account inspection page model', () => {
  test('classifies result rows and pending actions in one pass', () => {
    const view = buildInspectionResultsViewState([
      result(),
      result({ key: 'auth-2', fileName: 'invalid.json', statusCode: 401, action: 'delete' }),
      result({ key: 'auth-3', fileName: 'quota.json', isQuota: true, action: 'disable' }),
    ]);

    expect(view.healthCounts.total).toBe(3);
    expect(view.healthCounts.healthy).toBe(1);
    expect(view.healthCounts.authInvalid).toBe(1);
    expect(view.healthCounts.quotaExhausted).toBe(1);
    expect(view.filterRowCounts.pending).toBe(2);
    expect(view.actionableActionCounts).toMatchObject({ delete: 1, disable: 1 });
  });

  test('keeps pagination and settings draft conversion deterministic', () => {
    expect(getPaginationRange({
      page: 2,
      pageSize: 100,
      total: 250,
      totalPages: 3,
      hasMore: true,
    }, 100)).toMatchObject({ from: 101, to: 200, hasPrevious: true, hasNext: true });

    expect(toSettingsDraft(DEFAULT_ACCOUNT_INSPECTION_SETTINGS)).toMatchObject({
      workers: String(DEFAULT_ACCOUNT_INSPECTION_SETTINGS.workers),
      targetType: DEFAULT_ACCOUNT_INSPECTION_SETTINGS.targetType,
    });
  });

  test('uses free-token exhaustion only for free xAI plans', () => {
    expect(isXaiQuotaLow({
      status: 'success',
      billing: { planType: 'free', freeQuota: { exhausted: true } },
    }, 90)).toBe(true);
    expect(isXaiQuotaLow({
      status: 'success',
      billing: {
        planType: 'x-premium-plus',
        usagePercent: 10,
        freeQuota: { exhausted: true },
      },
    }, 90)).toBe(false);
  });

  test('resolves account plans from quota state with auth-file fallback', () => {
    const t = ((key: string) => ({
      'codex_quota.plan_pro': '专业版',
      'xai_quota.plan_x_premium_plus': 'X Premium+',
    }[key] ?? key)) as TFunction;
    const quotaStore = {
      antigravityQuota: {},
      claudeQuota: {},
      codexQuota: { 'account.json': { status: 'success', planType: 'pro' } },
      geminiCliQuota: {},
      kimiQuota: {},
      xaiQuota: {},
    } as Parameters<typeof resolveAccountInspectionPlanLabel>[2];

    expect(resolveAccountInspectionPlanLabel(result(), undefined, quotaStore, t)).toBe('专业版');
    expect(resolveAccountInspectionPlanLabel(
      result({ provider: 'gemini-cli' }),
      { name: 'account.json', type: 'gemini-cli', tier_id: 'standard-tier' },
      quotaStore,
      t
    )).toBe('Standard Tier');
    expect(resolveAccountInspectionPlanLabel(
      result({ provider: 'xai' }),
      undefined,
      {
        ...quotaStore,
        xaiQuota: {
          'account.json': { status: 'success', billing: { monthlyLimitCents: 20_000 } },
        },
      } as Parameters<typeof resolveAccountInspectionPlanLabel>[2],
      t
    )).toBe('X Premium+');
  });
});
