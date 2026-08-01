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
});
