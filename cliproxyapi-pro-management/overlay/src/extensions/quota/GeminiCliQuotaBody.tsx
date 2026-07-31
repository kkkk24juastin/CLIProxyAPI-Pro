import { useTranslation } from 'react-i18next';
import type { GeminiCliQuotaState } from '@/types';
import { formatQuotaResetTime } from '@/utils/quota';
import { QuotaMeter } from '@/features/quota/components/QuotaMeter';
import type { QuotaBodyProps } from '@/features/quota/types';
import {
  PREMIUM_GEMINI_CLI_TIER_IDS,
  resolveGeminiCliTierDisplay,
} from './geminiCliQuotaConfig';

export function GeminiCliQuotaBody({ quota, classes }: QuotaBodyProps<GeminiCliQuotaState>) {
  const { t } = useTranslation();
  const tierId = quota.tierId ?? null;
  const tierLabel = resolveGeminiCliTierDisplay(tierId, quota.tierLabel, t);
  const creditBalance = quota.creditBalance ?? null;
  const buckets = quota.buckets ?? [];

  return (
    <>
      {(tierLabel || creditBalance !== null) && (
        <div className={classes.codexPlan}>
          {tierLabel && (
            <span className={classes.codexPlanItem}>
              <span className={classes.codexPlanLabel}>{t('gemini_cli_quota.tier_label')}</span>
              <span
                className={
                  tierId && PREMIUM_GEMINI_CLI_TIER_IDS.has(tierId)
                    ? classes.premiumPlanValue
                    : classes.codexPlanValue
                }
              >
                {tierLabel}
              </span>
            </span>
          )}
          {creditBalance !== null && (
            <span className={classes.codexPlanItem}>
              <span className={classes.codexPlanLabel}>{t('gemini_cli_quota.credit_label')}</span>
              <span className={classes.codexPlanValue}>
                {t('gemini_cli_quota.credit_amount', { count: creditBalance })}
              </span>
            </span>
          )}
        </div>
      )}
      {buckets.length === 0 ? (
        <div className={classes.quotaMessage}>{t('gemini_cli_quota.empty_buckets')}</div>
      ) : (
        buckets.map((bucket, index) => {
          const remaining =
            bucket.remainingFraction === null
              ? null
              : Math.max(0, Math.min(100, bucket.remainingFraction * 100));
          const amountLabel =
            bucket.remainingAmount === null
              ? null
              : t('gemini_cli_quota.remaining_amount', { count: bucket.remainingAmount });
          return (
            <div key={bucket.id} className={classes.quotaRow}>
              <div className={classes.quotaRowHeader}>
                <span className={classes.quotaModel} title={bucket.modelIds?.join(', ')}>
                  {bucket.label}
                </span>
                <div className={classes.quotaMeta}>
                  <span className={classes.quotaPercent}>
                    {remaining === null ? '--' : `${Math.round(remaining)}%`}
                  </span>
                  {amountLabel && <span className={classes.quotaAmount}>{amountLabel}</span>}
                  <span className={classes.quotaReset}>{formatQuotaResetTime(bucket.resetTime)}</span>
                </div>
              </div>
              <QuotaMeter percent={remaining} classes={classes} index={index} />
            </div>
          );
        })
      )}
    </>
  );
}
