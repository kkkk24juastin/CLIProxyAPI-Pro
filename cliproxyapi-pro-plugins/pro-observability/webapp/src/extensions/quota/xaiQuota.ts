export const resolveXaiPlanType = (monthlyLimitCents: number | null, hasBilling: boolean): string => {
  if (!hasBilling) return '';
  if ((monthlyLimitCents ?? 0) <= 0) return 'free';
  return 'paid-unknown';
};
