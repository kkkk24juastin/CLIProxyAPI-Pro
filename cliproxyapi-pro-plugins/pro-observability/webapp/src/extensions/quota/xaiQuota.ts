export const resolveXaiPlanType = (monthlyLimitCents: number | null, hasBilling: boolean): string => {
  if (!hasBilling) return '';
  if ((monthlyLimitCents ?? 0) <= 0) return 'free';
  if (monthlyLimitCents === 15_000) return 'supergrok';
  if (monthlyLimitCents === 20_000) return 'x-premium-plus';
  if (monthlyLimitCents === 150_000) return 'supergrok-heavy';
  return 'paid-unknown';
};
