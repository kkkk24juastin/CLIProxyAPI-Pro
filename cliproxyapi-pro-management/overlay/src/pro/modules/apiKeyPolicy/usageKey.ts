const maskAPIKey = (value: string): string => {
  const visibleChars = value.length < 4 ? 1 : 2;
  const maskedLength = Math.max(10 - visibleChars * 2, 1);
  return `${value.slice(0, visibleChars)}${'*'.repeat(maskedLength)}${value.slice(-visibleChars)}`;
};

const sha256Hex = async (value: string): Promise<string> => {
  const digest = await globalThis.crypto.subtle.digest('SHA-256', new TextEncoder().encode(value));
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
};

export const resolveAPIKeyUsageHash = async ({
  configuredKeys,
  bindingIndex,
  maskedKey,
}: {
  configuredKeys: readonly string[];
  bindingIndex: number;
  maskedKey: string;
}): Promise<string> => {
  const candidates = await Promise.all(configuredKeys
    .map((value) => value.trim())
    .filter(Boolean)
    .map(async (value) => ({
      hash: await sha256Hex(value),
      masked: maskAPIKey(value),
    })));
  const seen = new Set<string>();
  const uniqueCandidates = candidates.filter((candidate) => {
    if (seen.has(candidate.hash)) return false;
    seen.add(candidate.hash);
    return true;
  });
  const indexed = uniqueCandidates[bindingIndex];
  if (indexed?.masked === maskedKey) return indexed.hash;
  const matching = uniqueCandidates.filter((candidate) => candidate.masked === maskedKey);
  if (matching.length === 1) return matching[0].hash;
  throw new Error('Unable to resolve the selected API Key for usage filtering');
};
