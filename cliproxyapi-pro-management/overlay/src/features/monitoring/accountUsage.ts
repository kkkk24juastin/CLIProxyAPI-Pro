export function maskAccountUsageAPIKeyHash(hash: string, unattributedLabel: string): string {
  const normalized = hash.trim();
  if (!normalized) return unattributedLabel;
  if (normalized.length <= 10) return `${normalized.slice(0, 2)}...${normalized.slice(-2)}`;
  return `${normalized.slice(0, 6)}...${normalized.slice(-4)}`;
}

export function buildAccountUsageLogPath(authIndex: string, fromMs: number, toMs: number): string {
  const params = new URLSearchParams({
    auth_index: authIndex,
    from_ms: String(Math.max(0, Math.round(fromMs))),
    to_ms: String(Math.max(0, Math.round(toMs))),
  });
  return `/monitoring?${params.toString()}#request-events`;
}

export function ratio(numerator: number, denominator: number): number {
  if (!Number.isFinite(numerator) || !Number.isFinite(denominator) || denominator <= 0) return 0;
  return Math.min(Math.max(numerator / denominator, 0), 1);
}
