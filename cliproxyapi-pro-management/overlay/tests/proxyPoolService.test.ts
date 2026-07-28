import { describe, expect, test } from 'bun:test';
import {
  isProxyPoolListenerUrl,
  normalizeProxyPoolConfig,
  serializeProxyPoolConfig,
} from '../src/services/api/proxyPool';

describe('proxy pool service model', () => {
  test('recognizes credential routes that already point at the pool listener', () => {
    expect(isProxyPoolListenerUrl('socks5://127.0.0.1:8318', '127.0.0.1:8318')).toBe(true);
    expect(isProxyPoolListenerUrl('SOCKS5H://127.0.0.1:8318', '127.0.0.1:8318')).toBe(true);
    expect(isProxyPoolListenerUrl('socks5://127.0.0.1:1080', '127.0.0.1:8318')).toBe(false);
    expect(isProxyPoolListenerUrl('http://127.0.0.1:8318', '127.0.0.1:8318')).toBe(false);
  });

  test('round-trips health timeout, test URL, weight, and node order', () => {
    const normalized = normalizeProxyPoolConfig({
      listen: '127.0.0.1:8318',
      'health-check': { timeout: '11s', 'test-url': 'https://example.com/ip' },
      nodes: [{ id: 'primary', url: 'socks5://127.0.0.1:1080', weight: 4, order: 30 }],
    });
    const serialized = serializeProxyPoolConfig(normalized);

    expect(serialized['health-check']).toMatchObject({
      timeout: '11s',
      'test-url': 'https://example.com/ip',
    });
    expect(serialized.nodes).toEqual([
      {
        id: 'primary',
        label: '',
        url: 'socks5://127.0.0.1:1080',
        enabled: true,
        weight: 4,
        order: 30,
      },
    ]);
  });
});
