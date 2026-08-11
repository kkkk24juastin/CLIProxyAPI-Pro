import type { TFunction } from 'i18next';
import type { AuthFileItem } from '@/types';
import { resolveXaiPlanType } from './xaiQuota';

export type QuotaSearchStore = {
  antigravityQuota: Record<
    string,
    { subscription?: { plan?: unknown; tierName?: unknown; tierId?: unknown } | null }
  >;
  claudeQuota: Record<string, { planType?: unknown }>;
  codexQuota: Record<string, { planType?: unknown }>;
  geminiCliQuota: Record<string, { tierLabel?: unknown; tierId?: unknown; creditBalance?: unknown }>;
  xaiQuota: Record<
    string,
    {
      billing?: {
        planType?: unknown;
        monthlyLimitCents?: number | null;
        onDemandCapCents?: number | null;
        onDemandUsedCents?: number | null;
      } | null;
    }
  >;
};

const FILE_KEYS = [
  'name', 'email', 'type', 'provider', 'note', 'notes', 'remark', 'remarks', 'description',
  'plan', 'plan_type', 'planType', 'package', 'package_name', 'tier', 'tier_id', 'tierId',
] as const;

const append = (values: string[], value: unknown) => {
  if (typeof value === 'string' && value.trim()) values.push(value.trim());
  else if (typeof value === 'number' || typeof value === 'boolean') values.push(String(value));
};

export function buildQuotaSearchValues(
  file: AuthFileItem,
  store: QuotaSearchStore,
  t: TFunction
): string[] {
  const values: string[] = [];
  FILE_KEYS.forEach((key) => append(values, file[key]));
  const name = file.name;

  const codex = store.codexQuota[name];
  append(values, codex?.planType);
  if (codex?.planType) append(values, t(`codex_quota.plan_${String(codex.planType).toLowerCase()}`));

  const claude = store.claudeQuota[name];
  append(values, claude?.planType);

  const antigravity = store.antigravityQuota[name]?.subscription;
  append(values, antigravity?.plan);
  append(values, antigravity?.tierName);
  append(values, antigravity?.tierId);

  const gemini = store.geminiCliQuota[name];
  append(values, gemini?.tierLabel);
  append(values, gemini?.tierId);
  append(values, gemini?.creditBalance);

  const xai = store.xaiQuota[name]?.billing;
  const xaiMonthlyBillingKnown =
    xai?.monthlyLimitCents != null ||
    xai?.onDemandCapCents != null ||
    xai?.onDemandUsedCents != null;
  const xaiPlan = resolveXaiPlanType(
    xai?.monthlyLimitCents ?? null,
    xaiMonthlyBillingKnown,
    xai?.onDemandCapCents ?? null,
    xai?.onDemandUsedCents ?? null
  ) ?? xai?.planType;
  append(values, xaiPlan);
  if (xaiPlan) append(values, t(`xai_quota.plan_${String(xaiPlan).replace(/-/g, '_')}`));
  return values;
}

export function matchesQuotaSearch(values: string[], query: string): boolean {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  const escaped = normalized.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*');
  const matcher = normalized.includes('*') ? new RegExp(escaped, 'i') : null;
  return values.some((value) => matcher ? matcher.test(value) : value.toLowerCase().includes(normalized));
}
