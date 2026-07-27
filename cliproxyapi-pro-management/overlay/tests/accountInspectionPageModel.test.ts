import { describe, expect, test } from 'bun:test';
import {
  buildInspectionResultsViewState,
  getPaginationRange,
  isAuthFileAccountInvalid,
  isAuthFileRequestError,
  isResultAccountInvalid,
  isResultRequestError,
  isXaiQuotaLow,
  resolveAccountInspectionPlanLabel,
  resolveResultHealthStatus,
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
      result({ key: 'auth-2', fileName: 'invalid.json', statusCode: 401, errorCode: 'inspection_http_error', action: 'delete' }),
      result({ key: 'auth-3', fileName: 'quota.json', isQuota: true, action: 'disable' }),
    ]);

    expect(view.healthCounts.total).toBe(3);
    expect(view.healthCounts.healthy).toBe(1);
    expect(view.healthCounts.authInvalid).toBe(1);
    expect(view.healthCounts.quotaExhausted).toBe(1);
    expect(view.filterRowCounts.pending).toBe(2);
    expect(view.actionableActionCounts).toMatchObject({ delete: 1, disable: 1 });
  });

  test('uses semantic evidence consistently across all providers', () => {
    const providers = ['antigravity', 'claude', 'codex', 'gemini-cli', 'kimi', 'xai'];
    providers.forEach((provider) => {
      expect(resolveResultHealthStatus(result({
        provider,
        statusCode: 401,
        errorCode: 'inspection_http_error',
        action: provider === 'codex' ? 'delete' : 'disable',
      }))).toBe('authInvalid');
      expect(resolveResultHealthStatus(result({
        provider,
        statusCode: provider === 'codex' || provider === 'xai' ? 402 : 200,
        isQuota: true,
        action: 'disable',
      }))).toBe('quotaExhausted');
    });

    const requestErrors: Partial<AccountInspectionResultItem>[] = [
      { provider: 'antigravity', statusCode: 400, errorCode: 'antigravity_deep_probe_error', deepProbeStatus: 'transient_error' },
      { provider: 'claude', statusCode: 502, errorCode: 'inspection_probe_error' },
      { provider: 'codex', statusCode: null, errorCode: 'missing_auth_index' },
      { provider: 'gemini-cli', statusCode: 503, errorCode: 'inspection_probe_error' },
      { provider: 'kimi', statusCode: null, errorCode: 'token_refresh_error', tokenRefreshStatus: 'failed' },
      { provider: 'xai', statusCode: 400, errorCode: 'xai_deep_probe_error', deepProbeStatus: 'transient_error' },
    ];
    requestErrors.forEach((requestError, index) => {
      const item = result({
        key: `request-${index}`,
        action: 'delete',
        error: 'probe failed',
        ...requestError,
      });
      expect(isResultAccountInvalid(item)).toBe(false);
      expect(isResultRequestError(item)).toBe(true);
      expect(resolveResultHealthStatus(item)).toBe('inspectionError');
    });
  });

  test('does not infer account health from suggested actions', () => {
    expect(resolveResultHealthStatus(result({ action: 'delete' }))).toBe('healthy');
    expect(resolveResultHealthStatus(result({ action: 'disable' }))).toBe('healthy');
  });

  test('aligns auth-file and inspection-result error code semantics', () => {
    const invalidFile = {
      name: 'invalid.json',
      type: 'claude',
      last_error: { code: 'inspection_http_error', http_status: 401 },
    };
    expect(isAuthFileAccountInvalid(invalidFile)).toBe(true);
    expect(isAuthFileRequestError(invalidFile)).toBe(false);

    const requestErrorFile = {
      name: 'request-error.json',
      type: 'xai',
      last_error: { code: 'xai_deep_probe_error', http_status: 400 },
    };
    expect(isAuthFileAccountInvalid(requestErrorFile)).toBe(false);
    expect(isAuthFileRequestError(requestErrorFile)).toBe(true);

    expect(isResultAccountInvalid(result({ statusCode: 401, errorCode: 'inspection_http_error' }))).toBe(true);
    expect(isResultRequestError(result({ statusCode: 400, errorCode: 'xai_deep_probe_error', error: 'probe failed' }))).toBe(true);
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
