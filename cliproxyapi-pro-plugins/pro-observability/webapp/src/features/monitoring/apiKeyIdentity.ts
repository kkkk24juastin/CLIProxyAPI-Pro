import { sha256Hex } from '@/utils/hash';

export type MonitoringApiKeyIdentity = {
  id: string;
  hash: string;
  masked: string;
};

export const maskConfiguredApiKey = (value: string): string => {
  const trimmed = value.trim();
  if (!trimmed || trimmed === '-') return '-';
  const visibleChars = trimmed.length < 4 ? 1 : 2;
  return `${trimmed.slice(0, visibleChars)}${'*'.repeat(Math.max(10 - visibleChars * 2, 1))}${trimmed.slice(-visibleChars)}`;
};

export const buildConfiguredApiKeyMap = (
  apiKeys: readonly (string | MonitoringApiKeyIdentity | { hash: string; masked: string })[] | undefined
) => {
  const keys = (apiKeys || [])
    .map((key) => typeof key === 'string' ? key.trim() : key)
    .filter(Boolean)
    .map((key, index): MonitoringApiKeyIdentity => {
      if (typeof key !== 'string') {
        return {
          id: `clientApiKey:${key.hash || index}`,
          hash: key.hash,
          masked: key.masked,
        };
      }
      const hash = sha256Hex(key);
      return {
        id: `clientApiKey:${hash || index}`,
        hash,
        masked: maskConfiguredApiKey(key),
      };
    });

  return {
    keys,
    byHash: new Map(keys.map((key) => [key.hash, key])),
  };
};

export function resolveConfiguredApiKeyLabel(
  apiKeyHash: string,
  configuredApiKeys: ReturnType<typeof buildConfiguredApiKeyMap>,
  unattributedLabel: string,
  unknownLabel: string
): string {
  const normalizedHash = apiKeyHash.trim();
  if (!normalizedHash) return unattributedLabel;
  return configuredApiKeys.byHash.get(normalizedHash)?.masked ?? unknownLabel;
}
