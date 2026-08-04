import type { ProxyPoolHealthState, ProxyPoolNodeConfig } from '@/pro/modules/proxyPool/proxyPool';
import { parsePositiveGoDuration, serializeGoDuration } from '@/pro/shared/duration';

export type ProxyPoolView = 'nodes' | 'diagnostics' | 'settings';
export type ProxyPoolStatusFilter = 'all' | ProxyPoolHealthState;
export type ProxyPoolDurationUnit = 's' | 'm';

export const proxyNodeKey = (node: ProxyPoolNodeConfig, index: number): string =>
  node.id.trim() || `draft-${index + 1}`;

export const maskProxyCredentials = (value: string): string => {
  const schemeEnd = value.indexOf('//');
  const credentialsEnd = value.lastIndexOf('@');
  if (schemeEnd < 0 || credentialsEnd <= schemeEnd + 2) return value;
  return `${value.slice(0, schemeEnd + 2)}***@${value.slice(credentialsEnd + 1)}`;
};

export const formatProxyPoolTime = (value: string, language: string): string => {
  if (!value || value.startsWith('0001-')) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(language, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
};

export const formatProxyPoolSuccessRate = (success: number, total: number): string => {
  if (!Number.isFinite(total) || total <= 0) return '-';
  const normalizedSuccess = Math.min(Math.max(Number.isFinite(success) ? success : 0, 0), total);
  return `${Math.round((normalizedSuccess / total) * 1000) / 10}%`;
};

export const proxyPoolDurationValue = (
  value: string,
  targetUnit: ProxyPoolDurationUnit
): number | null => parsePositiveGoDuration(value, targetUnit);

export const serializeProxyPoolDuration = (value: number, unit: ProxyPoolDurationUnit): string =>
  serializeGoDuration(value, unit);

export const proxyPoolStateLabel = (state: ProxyPoolHealthState): string => {
  if (state === 'healthy') return 'Healthy';
  if (state === 'degraded') return 'Degraded';
  if (state === 'isolated') return 'Isolated';
  if (state === 'disabled') return 'Disabled';
  return 'Unknown';
};

export const createProxyPoolNode = (index: number): ProxyPoolNodeConfig => ({
  id:
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? `proxy-${crypto.randomUUID().slice(0, 8)}`
      : `proxy-${Date.now().toString(36)}-${index + 1}`,
  label: '',
  url: '',
  enabled: true,
  weight: 1,
  order: (index + 1) * 10,
});
