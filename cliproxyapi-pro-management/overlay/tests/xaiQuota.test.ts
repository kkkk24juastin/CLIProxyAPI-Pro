import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import type { TFunction } from 'i18next';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import type { QuotaClassMap } from '@/features/quota/types';
import { PRO_XAI_CONFIG } from '@/pro/modules/quota/extensions/xaiQuotaAdapter';
import { ProXaiQuotaBody } from '@/pro/modules/quota/extensions/ProXaiQuotaBody';
import {
  XAI_FREE_QUOTA_PROBE_URL,
  mergeXaiBillingRuntimeState,
  parseXaiFreeQuotaProbe,
  resolveXaiPlanType,
  xaiFreeQuotaRemainingPercent,
} from '@/pro/modules/quota/extensions/xaiQuota';
import { apiCallApi, type ApiCallRequest, type ApiCallResult } from '@/services/api';
import {
  XAI_BILLING_MONTHLY_URL,
  XAI_BILLING_WEEKLY_URL,
  XAI_PAID_HEALTH_MODEL,
} from '@/utils/quota';

const t = ((key: string) => key) as unknown as TFunction;
const originalApiCallRequest = apiCallApi.request;
const quotaClasses = new Proxy({}, { get: (_target, key) => String(key) }) as QuotaClassMap;

const result = (
  statusCode: number,
  body: unknown = null,
  header: Record<string, string[]> = {}
): ApiCallResult => ({
  statusCode,
  hasStatusCode: true,
  header,
  bodyText: body === null ? '' : typeof body === 'string' ? body : JSON.stringify(body),
  body,
});

describe('xAI quota normalization', () => {
  test('recognizes plans only from a known monthly billing response', () => {
    expect(resolveXaiPlanType(null, false)).toBeUndefined();
    expect(resolveXaiPlanType(null, true)).toBe('free');
    expect(resolveXaiPlanType(0, true)).toBe('free');
    expect(resolveXaiPlanType(15_000, true)).toBe('supergrok');
    expect(resolveXaiPlanType(20_000, true)).toBe('x-premium-plus');
    expect(resolveXaiPlanType(150_000, true)).toBe('supergrok-heavy');
    expect(resolveXaiPlanType(99_000, true)).toBe('paid-unknown');
  });

  test('preserves the newest runtime free-quota observation', () => {
    const incoming = {
      mode: 'billing' as const,
      periodType: 'monthly' as const,
      usagePercent: null,
      productUsage: [],
      monthlyLimitCents: 0,
      usedCents: null,
      includedUsedCents: null,
      onDemandCapCents: null,
      onDemandUsedCents: null,
      onDemandUsedPercent: null,
      usedPercent: null,
      planType: 'free' as const,
      freeQuota: { observedAt: 100, remainingTokens: 80 },
    };
    const previous = {
      ...incoming,
      planType: 'free' as const,
      freeQuota: { observedAt: 200, remainingTokens: 10 },
    };

    const merged = mergeXaiBillingRuntimeState(incoming, previous);
    expect(merged.planType).toBe('free');
    expect(merged.freeQuota?.remainingTokens).toBe(10);
  });

  test('does not carry a previous free quota into a paid plan', () => {
    const billing = {
      mode: 'billing' as const,
      periodType: 'monthly' as const,
      usagePercent: null,
      productUsage: [],
      monthlyLimitCents: 20_000,
      usedCents: null,
      includedUsedCents: null,
      onDemandCapCents: null,
      onDemandUsedCents: null,
      onDemandUsedPercent: null,
      usedPercent: null,
      planType: 'x-premium-plus' as const,
    };
    const previous = {
      ...billing,
      planType: 'free' as const,
      freeQuota: { observedAt: 200, remainingTokens: 10 },
    };

    expect(mergeXaiBillingRuntimeState(billing, previous).freeQuota).toBeUndefined();
  });

  test('derives free-token availability and handles exhaustion', () => {
    expect(xaiFreeQuotaRemainingPercent({
      freeQuota: { usedTokens: 25, limitTokens: 100 },
    })).toBe(75);
    expect(xaiFreeQuotaRemainingPercent({ freeQuota: { exhausted: true } })).toBe(0);
  });

  test('parses a fresh free quota from rate-limit headers', () => {
    expect(
      parseXaiFreeQuotaProbe(
        result(200, { id: 'response-1' }, {
          'X-RateLimit-Limit-Tokens': ['1000'],
          'x-ratelimit-remaining-tokens': ['760'],
          'x-ratelimit-limit-requests': ['20'],
          'x-ratelimit-remaining-requests': ['19'],
        }),
        XAI_PAID_HEALTH_MODEL,
        1234
      )
    ).toEqual({
      source: 'rate_limit_headers',
      windowKind: 'rolling_24h',
      usedTokens: 240,
      limitTokens: 1000,
      remainingTokens: 760,
      limitRequests: 20,
      remainingRequests: 19,
      observedAt: 1234,
      exhausted: false,
      model: XAI_PAID_HEALTH_MODEL,
    });
  });

  test('parses a free-usage-exhausted response', () => {
    const body =
      'subscription:free-usage-exhausted used all the included free usage for model grok-4.5, tokens (actual/limit): 1000/1000';
    expect(parseXaiFreeQuotaProbe(result(429, body), 'fallback', 5678)).toMatchObject({
      source: 'free_usage_exhausted',
      usedTokens: 1000,
      limitTokens: 1000,
      remainingTokens: 0,
      observedAt: 5678,
      exhausted: true,
      model: 'grok-4.5',
    });
  });

  test('renders the free-token row only for the Free plan', () => {
    const billing = {
      mode: 'billing' as const,
      periodType: 'monthly' as const,
      usagePercent: null,
      productUsage: [],
      monthlyLimitCents: 0,
      usedCents: null,
      includedUsedCents: null,
      onDemandCapCents: null,
      onDemandUsedCents: null,
      onDemandUsedPercent: null,
      usedPercent: null,
      freeQuota: { model: 'grok-4.5', usedTokens: 25, limitTokens: 100 },
    };
    const render = (planType: 'free' | 'x-premium-plus') =>
      renderToStaticMarkup(createElement(ProXaiQuotaBody, {
        quota: { status: 'success', billing: { ...billing, planType } },
        classes: quotaClasses,
      }));

    expect(render('free')).toContain('grok-4.5');
    expect(render('x-premium-plus')).not.toContain('grok-4.5');
  });
});

describe('xAI free quota forced refresh', () => {
  let requests: ApiCallRequest[];

  beforeEach(() => {
    requests = [];
  });

  afterEach(() => {
    apiCallApi.request = originalApiCallRequest;
  });

  const installFreeBillingMock = (probeResult: ApiCallResult) => {
    apiCallApi.request = async (payload) => {
      requests.push(payload);
      if (payload.url === XAI_BILLING_WEEKLY_URL) {
        return result(200, { config: { currentPeriod: { type: 'weekly' } } });
      }
      if (payload.url === XAI_BILLING_MONTHLY_URL) {
        return result(200, { config: { monthlyLimit: { val: 0 }, used: { val: 0 } } });
      }
      if (payload.url === XAI_FREE_QUOTA_PROBE_URL) return probeResult;
      throw new Error(`Unexpected URL: ${payload.url}`);
    };
  };

  test('forces an executor request and returns its fresh quota snapshot', async () => {
    installFreeBillingMock(
      result(200, { id: 'response-1' }, {
        'x-ratelimit-limit-tokens': ['1000'],
        'x-ratelimit-remaining-tokens': ['700'],
      })
    );

    const summary = await PRO_XAI_CONFIG.fetchQuota(
      { name: 'free.json', type: 'xai', auth_index: 'xai:free' },
      t
    );

    expect(requests.map((request) => request.url)).toContain(XAI_FREE_QUOTA_PROBE_URL);
    const probe = requests.find((request) => request.url === XAI_FREE_QUOTA_PROBE_URL);
    expect(probe).toMatchObject({ method: 'POST', useExecutor: true });
    expect(probe?.url).toBe('https://cli-chat-proxy.grok.com/v1/responses');
    expect(probe?.url).not.toContain('api.x.ai');
    expect(probe?.header).toMatchObject({
      accept: 'text/event-stream',
      'x-xai-token-auth': 'xai-grok-cli',
    });
    expect(JSON.parse(probe?.data ?? '{}')).toMatchObject({
      model: XAI_PAID_HEALTH_MODEL,
      input: [
        {
          role: 'user',
          content: [{ type: 'input_text', text: 'ping' }],
        },
      ],
      instructions: 'You are a helpful assistant. Reply briefly.',
      max_output_tokens: 1,
      stream: true,
      store: false,
    });
    expect(summary).toMatchObject({
      planType: 'free',
      freeQuota: {
        source: 'rate_limit_headers',
        usedTokens: 300,
        limitTokens: 1000,
        remainingTokens: 700,
      },
    });
  });

  test('accepts a 429 exhaustion response as the current quota snapshot', async () => {
    installFreeBillingMock(
      result(429, 'subscription:free-usage-exhausted tokens (actual/limit): 1000/1000')
    );

    const summary = await PRO_XAI_CONFIG.fetchQuota(
      { name: 'exhausted.json', type: 'xai', auth_index: 'xai:exhausted' },
      t
    );

    expect(summary.freeQuota).toMatchObject({ exhausted: true, remainingTokens: 0 });
  });

  test('fails instead of returning stale quota when the forced probe has no quota data', async () => {
    installFreeBillingMock(result(200, { id: 'response-without-rate-limit-headers' }));

    await expect(
      PRO_XAI_CONFIG.fetchQuota({ name: 'stale.json', type: 'xai', auth_index: 'xai:stale' }, t)
    ).rejects.toThrow('xai_quota.empty_data');
  });
});
