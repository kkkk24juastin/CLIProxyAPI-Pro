import { describe, expect, test } from 'bun:test';
import { collectUsageDetailsWithEndpoint } from '../src/pro/modules/monitoring/features/usage';

describe('client request metadata', () => {
  test('normalizes snake case metadata from embedded usage details', () => {
    const details = collectUsageDetailsWithEndpoint({
      apis: {
        'POST /v1/responses': {
          models: {
            'gpt-test': {
              details: [{
                timestamp: '2026-07-28T00:00:00Z',
                source: 'owner@example.com',
                auth_index: '1',
                client_ip: '192.0.2.10',
                x_forwarded_for: '203.0.113.5, 198.51.100.8',
                user_agent: 'test-client/1.0',
                tokens: { total_tokens: 1 },
                failed: false,
              }],
            },
          },
        },
      },
    });

    expect(details).toHaveLength(1);
    expect(details[0]).toMatchObject({
      client_ip: '192.0.2.10',
      x_forwarded_for: '203.0.113.5, 198.51.100.8',
      user_agent: 'test-client/1.0',
    });
  });

  test('accepts camel case metadata from compatible backup payloads', () => {
    const details = collectUsageDetailsWithEndpoint({
      apis: {
        'POST /v1/responses': {
          models: {
            'gpt-test': {
              details: [{
                timestamp: '2026-07-28T00:00:00Z',
                source: '',
                authIndex: '1',
                clientIp: '192.0.2.11',
                xForwardedFor: '203.0.113.6',
                userAgent: 'backup-client/1.0',
                tokens: {},
                failed: false,
              }],
            },
          },
        },
      },
    });

    expect(details[0]).toMatchObject({
      client_ip: '192.0.2.11',
      x_forwarded_for: '203.0.113.6',
      user_agent: 'backup-client/1.0',
    });
  });

  test('preserves snake and camel case pricing modes from cost breakdowns', () => {
    const details = collectUsageDetailsWithEndpoint({
      apis: {
        'POST /v1/responses': {
          models: {
            'gpt-test': {
              details: [
                {
                  timestamp: '2026-07-28T00:00:01Z',
                  source: '',
                  tokens: {},
                  failed: false,
                  cost_breakdown: {
                    pricing_mode: 'service_tier',
                    service_tier: 'fast',
                    requested_service_tier: 'fast',
                    effective_service_tier: 'priority',
                    matched_service_tier: 'fast',
                    service_tier_source: 'response',
                    total_cost: 0.01,
                  },
                },
                {
                  timestamp: '2026-07-28T00:00:00Z',
                  source: '',
                  tokens: {},
                  failed: false,
                  costBreakdown: {
                    pricingMode: 'context',
                    contextTierSize: 272000,
                    totalCost: 0.02,
                  },
                },
              ],
            },
          },
        },
      },
    });

    expect(details[0].cost_breakdown).toMatchObject({
      pricingMode: 'service_tier',
      serviceTier: 'fast',
      requestedServiceTier: 'fast',
      effectiveServiceTier: 'priority',
      matchedServiceTier: 'fast',
      serviceTierSource: 'response',
    });
    expect(details[1].cost_breakdown).toMatchObject({ pricingMode: 'context', contextTierSize: 272000 });
  });

  test('preserves retry and canonical accounting diagnostics', () => {
    const details = collectUsageDetailsWithEndpoint({
      apis: {
        'POST /v1/responses': {
          models: {
            'gpt-test': {
              details: [{
                timestamp: '2026-07-28T00:00:00Z',
                source: '',
                attempt_index: 2,
                accounting_version: 2,
                accounting_quality: 'complete',
                token_breakdown: {
                  schema_version: 2,
                  quality: 'complete',
                  total_tokens: 12,
                  input: {
                    total_tokens: 10,
                    uncached_tokens: 7,
                    cache_read_tokens: 2,
                    cache_write_tokens: 1,
                  },
                  output: {
                    total_tokens: 2,
                    non_reasoning_tokens: 1,
                    reasoning_tokens: 1,
                  },
                  unclassified_tokens: 0,
                },
                tokens: { total_tokens: 12 },
                failed: false,
              }],
            },
          },
        },
      },
    });

    expect(details[0]).toMatchObject({
      attempt_index: 2,
      accounting_version: 2,
      accounting_quality: 'complete',
      token_breakdown: {
        schema_version: 2,
        total_tokens: 12,
        input: { uncached_tokens: 7, cache_read_tokens: 2 },
        output: { reasoning_tokens: 1 },
      },
    });
  });
});
