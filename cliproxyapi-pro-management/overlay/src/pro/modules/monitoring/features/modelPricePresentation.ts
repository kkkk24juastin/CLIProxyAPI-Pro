import type {
  ModelPriceRate,
  ModelPriceRule,
  ModelPriceSyncChangeAction,
  UsageCostBreakdown,
} from '@/pro/modules/monitoring/features/usage';

export type PriceRateDraft = {
  input: string;
  output: string;
  cacheRead: string;
  cacheWrite: string;
  reasoning: string;
};

export type PriceTierDraft = PriceRateDraft & {
  contextSize: string;
};

export type ServiceTierDraft = PriceRateDraft & {
  name: string;
};

export type PriceDraft = PriceRateDraft & {
  tiers: PriceTierDraft[];
  serviceTiers: ServiceTierDraft[];
};

export type PriceDraftValidationError =
  | 'rate_required'
  | 'context_size_invalid'
  | 'context_size_duplicate'
  | 'service_tier_name_required'
  | 'service_tier_name_duplicate';

export type ServiceTierChange = {
  name: string;
  action: 'added' | 'removed' | 'updated';
};

export type ResolvedPricingMode = 'base' | 'context' | 'service_tier' | 'legacy_unknown';

export type PriceManagementView = 'rules' | 'sync';
export type PriceSyncChangeFilter = 'all' | ModelPriceSyncChangeAction;

export type PriceRuleTarget = {
  key: string;
  model: string;
  requests: number;
  lastSeenAtMs: number;
  rule?: ModelPriceRule;
};

const roundCurrency = (value: number) => Math.round(value * 100) / 100;

const PRICE_RATE_FIELDS = ['input', 'output', 'cacheRead', 'cacheWrite', 'reasoning'] as const;

const createPriceRateDraft = (rate?: ModelPriceRate): PriceRateDraft => ({
  input: rate ? String(rate.input) : '',
  output: rate ? String(rate.output) : '',
  cacheRead: rate ? String(rate.cacheRead) : '',
  cacheWrite: rate ? String(rate.cacheWrite) : '',
  reasoning: rate ? String(rate.reasoning ?? 0) : '',
});

const parsePriceRateDraft = (rate: PriceRateDraft): ModelPriceRate => ({
  input: parsePriceValue(rate.input),
  output: parsePriceValue(rate.output),
  cacheRead: parsePriceValue(rate.cacheRead),
  cacheWrite: parsePriceValue(rate.cacheWrite),
  reasoning: parsePriceValue(rate.reasoning),
});

const isValidPriceRateDraft = (rate: PriceRateDraft) => PRICE_RATE_FIELDS.every((field) => {
  if (rate[field].trim() === '') return false;
  const parsed = Number(rate[field]);
  return Number.isFinite(parsed) && parsed >= 0;
});

export const formatDeltaPercent = (current: number, previous: number) => {
  const roundedCurrent = roundCurrency(current);
  const roundedPrevious = roundCurrency(previous);
  if (roundedPrevious <= 0) return roundedCurrent > 0 ? '+100.0%' : '0.0%';
  const delta = (roundedCurrent - roundedPrevious) / roundedPrevious;
  return `${delta >= 0 ? '+' : ''}${(delta * 100).toFixed(1)}%`;
};

export const createPriceDraft = (rule?: ModelPriceRule): PriceDraft => ({
  ...createPriceRateDraft(rule?.base),
  tiers: rule?.tiers?.map((tier) => ({
    contextSize: String(tier.contextSize),
    ...createPriceRateDraft(tier),
  })) ?? [],
  serviceTiers: Object.entries(rule?.serviceTiers ?? {})
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, rate]) => ({ name, ...createPriceRateDraft(rate) })),
});

export const createServiceTierDraft = (base: PriceRateDraft): ServiceTierDraft => ({
  name: '',
  input: base.input,
  output: base.output,
  cacheRead: base.cacheRead,
  cacheWrite: base.cacheWrite,
  reasoning: base.reasoning,
});

export const validatePriceDraft = (draft: PriceDraft): PriceDraftValidationError | null => {
  if (!isValidPriceRateDraft(draft)) return 'rate_required';

  const contextSizes = new Set<number>();
  for (const tier of draft.tiers) {
    const contextSize = Number(tier.contextSize);
    if (!Number.isInteger(contextSize) || contextSize <= 0) return 'context_size_invalid';
    if (contextSizes.has(contextSize)) return 'context_size_duplicate';
    contextSizes.add(contextSize);
    if (!isValidPriceRateDraft(tier)) return 'rate_required';
  }

  const serviceTierNames = new Set<string>();
  for (const tier of draft.serviceTiers) {
    const name = tier.name.trim().toLowerCase();
    if (!name) return 'service_tier_name_required';
    if (serviceTierNames.has(name)) return 'service_tier_name_duplicate';
    serviceTierNames.add(name);
    if (!isValidPriceRateDraft(tier)) return 'rate_required';
  }
  return null;
};

export const buildModelPriceRule = (model: string, draft: PriceDraft): ModelPriceRule => {
  const serviceTiers = Object.fromEntries(draft.serviceTiers.map((tier) => (
    [tier.name.trim().toLowerCase(), parsePriceRateDraft(tier)]
  )));
  return {
    model,
    base: parsePriceRateDraft(draft),
    tiers: draft.tiers.map((tier) => ({
      contextSize: parsePriceContextSize(tier.contextSize),
      ...parsePriceRateDraft(tier),
    })),
    serviceTiers,
  };
};

export const collectServiceTierChanges = (
  before: ModelPriceRule['serviceTiers'],
  after: ModelPriceRule['serviceTiers']
): ServiceTierChange[] => {
  const previous = before ?? {};
  const next = after ?? {};
  return Array.from(new Set([...Object.keys(previous), ...Object.keys(next)]))
    .sort((left, right) => left.localeCompare(right))
    .flatMap((name): ServiceTierChange[] => {
      if (!(name in previous)) return [{ name, action: 'added' }];
      if (!(name in next)) return [{ name, action: 'removed' }];
      return PRICE_RATE_FIELDS.some((field) => (previous[name][field] ?? 0) !== (next[name][field] ?? 0))
        ? [{ name, action: 'updated' }]
        : [];
    });
};

export const resolvePricingMode = (
  breakdown: Pick<UsageCostBreakdown, 'pricingMode' | 'contextTierSize' | 'serviceTier'>
): ResolvedPricingMode => {
  if (breakdown.pricingMode === 'base' || breakdown.pricingMode === 'context' || breakdown.pricingMode === 'service_tier') {
    return breakdown.pricingMode;
  }
  if (breakdown.serviceTier) return 'legacy_unknown';
  if (breakdown.contextTierSize > 0) return 'context';
  return 'base';
};

export const parsePriceValue = (value: string) => {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
};

export const parsePriceContextSize = (value: string) => {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
};

export const formatModelPriceRate = (value: number | undefined) => {
  const normalized = Number(value) || 0;
  return `$${normalized.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
};

export const MODEL_PRICE_SYNC_RATE_FIELDS = [
  ['input', 'usage_stats.model_price_input'],
  ['output', 'usage_stats.model_price_output'],
  ['cacheRead', 'usage_stats.model_price_cache_read'],
  ['cacheWrite', 'usage_stats.model_price_cache_write'],
  ['reasoning', 'usage_stats.model_price_reasoning'],
] as const;
