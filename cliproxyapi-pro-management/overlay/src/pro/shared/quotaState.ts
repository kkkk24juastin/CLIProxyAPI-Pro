export type TimestampedQuotaState = {
  cachedAt: number;
};

export const withQuotaCachedAt = <T extends object>(
  state: T,
  cachedAt = Date.now()
): T & TimestampedQuotaState => ({ ...state, cachedAt });
