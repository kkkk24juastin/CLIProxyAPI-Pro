import type { XaiBillingSummary, XaiFreeQuotaSummary } from '@/types';

export const XAI_SUPERGROK_LIMIT_CENTS = 15_000;
export const XAI_SUPERGROK_HEAVY_LIMIT_CENTS = 150_000;
export const XAI_FREE_QUOTA_PROBE_URL = 'https://cli-chat-proxy.grok.com/v1/responses';

export type XaiNormalizedPlanType =
  | 'free'
  | 'supergrok'
  | 'x-basic'
  | 'x-premium'
  | 'x-premium-plus'
  | 'supergrok-heavy'
  | 'supergrok-lite'
  | 'paid'
  | 'paid-unknown';

const XAI_PLAN_TYPES: readonly XaiNormalizedPlanType[] = [
  'free',
  'supergrok',
  'x-basic',
  'x-premium',
  'x-premium-plus',
  'supergrok-heavy',
  'supergrok-lite',
  'paid',
  'paid-unknown',
];

export const normalizeXaiPlanType = (value: unknown): XaiNormalizedPlanType | undefined => {
  if (typeof value !== 'string') return undefined;
  const normalized = value.trim().toLowerCase().replace(/_/g, '-');
  return XAI_PLAN_TYPES.includes(normalized as XaiNormalizedPlanType)
    ? (normalized as XaiNormalizedPlanType)
    : undefined;
};

export const resolveXaiPlanType = (
  monthlyLimitCents: number | null,
  monthlyBillingKnown: boolean
): XaiNormalizedPlanType | undefined => {
  if (!monthlyBillingKnown) return undefined;
  if (monthlyLimitCents === null || monthlyLimitCents === 0) return 'free';
  if (monthlyLimitCents === XAI_SUPERGROK_LIMIT_CENTS) return 'supergrok';
  if (monthlyLimitCents === XAI_SUPERGROK_HEAVY_LIMIT_CENTS) return 'supergrok-heavy';
  return monthlyLimitCents > 0 ? 'paid-unknown' : undefined;
};

export const isXaiMonthlyBillingKnown = (billing: XaiBillingSummary): boolean =>
  billing.monthlyLimitCents !== null ||
  billing.usedCents !== null ||
  Boolean(billing.billingPeriodStart || billing.billingPeriodEnd);

const observedAt = (billing: XaiBillingSummary | null | undefined): number => {
  const value = billing?.freeQuota?.observedAt;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return 0;
};

export const mergeXaiBillingRuntimeState = (
  billing: XaiBillingSummary,
  previous: XaiBillingSummary | null | undefined
): XaiBillingSummary => {
  let merged = billing;
  if (billing.planType === undefined && previous?.planType !== undefined) {
    merged = { ...merged, planType: previous.planType };
  }
  if (
    previous?.freeQuota &&
    (merged.planType === undefined || merged.planType === 'free') &&
    (!billing.freeQuota || observedAt(previous) > observedAt(billing))
  ) {
    merged = { ...merged, freeQuota: previous.freeQuota };
  }
  return merged;
};

const xaiFreeQuotaUsedPercent = (
  billing: unknown
): number | null => {
  if (!billing || typeof billing !== 'object' || Array.isArray(billing)) return null;
  const record = billing as Record<string, unknown>;
  const candidate = record.freeQuota ?? record.free_quota;
  if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) return null;
  const freeQuota = candidate as Record<string, unknown>;
  if (freeQuota.exhausted === true) return 100;
  const used = Number(freeQuota.usedTokens ?? freeQuota.used_tokens);
  const limit = Number(freeQuota.limitTokens ?? freeQuota.limit_tokens);
  if (!Number.isFinite(used) || !Number.isFinite(limit) || limit <= 0) return null;
  return Math.max(0, Math.min(100, (used / limit) * 100));
};

export const xaiFreeQuotaRemainingPercent = (
  billing: unknown
): number | null => {
  const used = xaiFreeQuotaUsedPercent(billing);
  return used === null ? null : Math.max(0, Math.min(100, 100 - used));
};

interface XaiFreeQuotaProbeResponse {
  statusCode: number;
  header?: Record<string, string[]>;
  bodyText?: string;
  body?: unknown;
}

const probeHeaderNumber = (
  header: Record<string, string[]> | undefined,
  name: string
): number | null => {
  if (!header) return null;
  const entry = Object.entries(header).find(([key]) => key.toLowerCase() === name);
  const raw = entry?.[1]?.[0]?.trim();
  if (!raw) return null;
  const value = Number(raw);
  return Number.isFinite(value) && value >= 0 ? value : null;
};

const probeBodyText = (response: XaiFreeQuotaProbeResponse): string => {
  if (response.bodyText) return response.bodyText;
  if (typeof response.body === 'string') return response.body;
  try {
    return response.body == null ? '' : JSON.stringify(response.body);
  } catch {
    return '';
  }
};

const probeBodyModel = (response: XaiFreeQuotaProbeResponse, text: string): string | undefined => {
  if (response.body && typeof response.body === 'object' && !Array.isArray(response.body)) {
    const model = (response.body as Record<string, unknown>).model;
    if (typeof model === 'string' && model.trim()) return model.trim();
  }
  return text.match(/for\s+model\s+([a-z0-9._-]+)/i)?.[1]?.replace(/[.,;:!?]+$/, '');
};

export const parseXaiFreeQuotaProbe = (
  response: XaiFreeQuotaProbeResponse,
  fallbackModel: string,
  observedAt = Date.now()
): XaiFreeQuotaSummary | null => {
  const text = probeBodyText(response);
  const lower = text.toLowerCase();
  const exhausted =
    lower.includes('subscription:free-usage-exhausted') ||
    lower.includes('used all the included free usage');

  if (exhausted) {
    const snapshot: XaiFreeQuotaSummary = {
      source: 'free_usage_exhausted',
      windowKind: 'rolling_24h',
      observedAt,
      exhausted: true,
      model: probeBodyModel(response, text) ?? fallbackModel,
    };
    const usage = text.match(/tokens\s*\(actual\/limit\)\s*:\s*([0-9]+)\s*\/\s*([0-9]+)/i);
    if (usage) {
      const usedTokens = Number(usage[1]);
      const limitTokens = Number(usage[2]);
      if (Number.isFinite(usedTokens) && Number.isFinite(limitTokens) && limitTokens > 0) {
        snapshot.usedTokens = usedTokens;
        snapshot.limitTokens = limitTokens;
        snapshot.remainingTokens = 0;
      }
    }
    return snapshot;
  }

  if (response.statusCode < 200 || response.statusCode >= 300) return null;
  const limitTokens = probeHeaderNumber(response.header, 'x-ratelimit-limit-tokens');
  const rawRemainingTokens = probeHeaderNumber(response.header, 'x-ratelimit-remaining-tokens');
  if (limitTokens === null || rawRemainingTokens === null || limitTokens <= 0) return null;
  const remainingTokens = Math.min(limitTokens, rawRemainingTokens);
  const snapshot: XaiFreeQuotaSummary = {
    source: 'rate_limit_headers',
    windowKind: 'rolling_24h',
    usedTokens: limitTokens - remainingTokens,
    limitTokens,
    remainingTokens,
    observedAt,
    exhausted: remainingTokens === 0,
    model: fallbackModel,
  };
  const limitRequests = probeHeaderNumber(response.header, 'x-ratelimit-limit-requests');
  const remainingRequests = probeHeaderNumber(response.header, 'x-ratelimit-remaining-requests');
  if (limitRequests !== null) snapshot.limitRequests = limitRequests;
  if (remainingRequests !== null) snapshot.remainingRequests = remainingRequests;
  return snapshot;
};
