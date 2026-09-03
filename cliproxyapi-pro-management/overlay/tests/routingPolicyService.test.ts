import { describe, expect, test } from 'bun:test';
import { normalizeRoutingPolicyInteger } from '../src/pro/modules/routing/routingPolicy';

describe('routing policy service model', () => {
  test('normalizes integer policy fields to backend bounds', () => {
    expect(normalizeRoutingPolicyInteger('1.9', 1, 5)).toBe(1);
    expect(normalizeRoutingPolicyInteger('-1', 0, 10080)).toBe(0);
    expect(normalizeRoutingPolicyInteger('90000', 1, 86400)).toBe(86400);
    expect(normalizeRoutingPolicyInteger('invalid', 1, 5)).toBe(1);
    expect(normalizeRoutingPolicyInteger('', 1, 86400, 600)).toBe(600);
    expect(normalizeRoutingPolicyInteger('invalid', 1, 86400, 600)).toBe(600);
  });
});
