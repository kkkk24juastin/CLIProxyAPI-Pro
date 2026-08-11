import { describe, expect, test } from 'bun:test';
import {
  buildModelPriceRule,
  collectServiceTierChanges,
  createServiceTierDraft,
  createPriceDraft,
  formatDeltaPercent,
  parsePriceValue,
  resolvePricingMode,
  validatePriceDraft,
} from '../src/pro/modules/monitoring/features/modelPricePresentation';

describe('model price presentation model', () => {
  test('creates editable strings without mutating the source rule', () => {
    const draft = createPriceDraft({
      id: 1,
      version: 2,
      model: 'gpt-test',
      source: 'manual',
      base: { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 0.75, reasoning: 3 },
      tiers: [{ contextSize: 1000, input: 3, output: 4, cacheRead: 1, cacheWrite: 1.5, reasoning: 5 }],
      serviceTiers: {
        priority: { input: 6, output: 7, cacheRead: 0.6, cacheWrite: 0.7, reasoning: 8 },
      },
    });

    expect(draft.input).toBe('1');
    expect(draft.reasoning).toBe('3');
    expect(draft.tiers[0]).toMatchObject({ contextSize: '1000', output: '4', reasoning: '5' });
    expect(draft.serviceTiers[0]).toMatchObject({ name: 'priority', output: '7', reasoning: '8' });

    const rebuilt = buildModelPriceRule('gpt-test', draft);
    expect(rebuilt.base.reasoning).toBe(3);
    expect(rebuilt.tiers?.[0].reasoning).toBe(5);
    expect(rebuilt.serviceTiers?.priority).toMatchObject({ output: 7, reasoning: 8 });
  });

  test('copies base rates for a new service tier and clears removed overrides', () => {
    const draft = createPriceDraft({
      model: 'gpt-test',
      base: { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 0.75, reasoning: 3 },
      serviceTiers: { priority: { input: 4, output: 5, cacheRead: 0.4, cacheWrite: 0.5 } },
    });
    expect(createServiceTierDraft(draft)).toMatchObject({ name: '', input: '1', reasoning: '3' });
    draft.serviceTiers = [];
    expect(buildModelPriceRule('gpt-test', draft).serviceTiers).toEqual({});
  });

  test('keeps missing base rates empty when creating a service tier', () => {
    expect(createServiceTierDraft(createPriceDraft())).toEqual({
      name: '',
      input: '',
      output: '',
      cacheRead: '',
      cacheWrite: '',
      reasoning: '',
    });
  });

  test('validates normalized duplicate service tiers and invalid rates', () => {
    const draft = createPriceDraft({
      model: 'gpt-test',
      base: { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 0.75 },
      serviceTiers: {
        priority: { input: 4, output: 5, cacheRead: 0.4, cacheWrite: 0.5 },
      },
    });
    draft.serviceTiers.push({ ...draft.serviceTiers[0], name: ' PRIORITY ' });
    expect(validatePriceDraft(draft)).toBe('service_tier_name_duplicate');
    draft.serviceTiers[1].name = 'flex';
    draft.serviceTiers[1].output = '';
    expect(validatePriceDraft(draft)).toBe('rate_required');
  });

  test('describes service-tier sync changes', () => {
    const rate = { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 0.75 };
    expect(collectServiceTierChanges(
      { priority: rate, removed: rate },
      { priority: { ...rate, output: 3 }, flex: rate }
    )).toEqual([
      { name: 'flex', action: 'added' },
      { name: 'priority', action: 'updated' },
      { name: 'removed', action: 'removed' },
    ]);
  });

  test('resolves explicit and legacy pricing modes without claiming a tier match', () => {
    expect(resolvePricingMode({ pricingMode: 'service_tier', contextTierSize: 0, serviceTier: 'priority' })).toBe('service_tier');
    expect(resolvePricingMode({ pricingMode: 'context', contextTierSize: 1000, serviceTier: 'flex' })).toBe('context');
    expect(resolvePricingMode({ contextTierSize: 0, serviceTier: 'priority' })).toBe('legacy_unknown');
    expect(resolvePricingMode({ contextTierSize: 1000, serviceTier: 'priority' })).toBe('legacy_unknown');
    expect(resolvePricingMode({ contextTierSize: 1000, serviceTier: '' })).toBe('context');
    expect(resolvePricingMode({ contextTierSize: 0, serviceTier: '' })).toBe('base');
  });

  test('normalizes invalid rates and reports rounded deltas', () => {
    expect(parsePriceValue('-1')).toBe(0);
    expect(parsePriceValue('not-a-number')).toBe(0);
    expect(formatDeltaPercent(1.5, 1)).toBe('+50.0%');
    expect(formatDeltaPercent(0, 0)).toBe('0.0%');
  });
});
