import { useTranslation } from 'react-i18next';
import type { XaiQuotaState } from '@/types';
import { XaiQuotaBody } from '@/features/quota/providers/xai/XaiQuotaBody';
import { QuotaMeter } from '@/features/quota/components/QuotaMeter';
import type { QuotaBodyProps } from '@/features/quota/types';
import { XAI_PAID_HEALTH_MODEL } from '@/utils/quota';
import {
  isXaiMonthlyBillingKnown,
  resolveXaiPlanType,
  xaiFreeQuotaRemainingPercent,
} from './xaiQuota';

export function ProXaiQuotaBody(props: QuotaBodyProps<XaiQuotaState>) {
  const { quota, classes } = props;
  const { t } = useTranslation();
  const billing = quota.billing;
  const planType = billing
    ? billing.planType ?? resolveXaiPlanType(
        billing.monthlyLimitCents,
        isXaiMonthlyBillingKnown(billing),
        billing.onDemandCapCents,
        billing.onDemandUsedCents
      )
    : undefined;
  const freeQuota = planType === 'free' ? billing?.freeQuota : undefined;
  const remaining = freeQuota ? xaiFreeQuotaRemainingPercent(billing) : null;

  return (
    <>
      {planType === 'free' && (
        <div className={classes.codexPlan}>
          <span className={classes.codexPlanLabel}>{t('xai_quota.plan_label')}</span>
          <span className={classes.codexPlanValue}>{t('xai_quota.plan_free')}</span>
        </div>
      )}
      {planType === 'free' && (
        <div className={classes.quotaRow}>
          <div className={classes.quotaRowHeader}>
            <span className={classes.quotaModel}>
              {`${t('xai_quota.free_quota')} · ${freeQuota?.model || XAI_PAID_HEALTH_MODEL}`}
            </span>
            <div className={classes.quotaMeta}>
              <span className={classes.quotaPercent}>
                {!freeQuota
                  ? t('xai_quota.free_quota_pending')
                  : freeQuota.exhausted
                  ? t('xai_quota.free_quota_exhausted')
                  : remaining === null
                    ? '--'
                    : `${Math.round(remaining)}%`}
              </span>
              <span className={classes.quotaReset}>{t('xai_quota.free_quota_window')}</span>
            </div>
          </div>
          <QuotaMeter percent={remaining} classes={classes} index={0} />
        </div>
      )}
      <XaiQuotaBody {...props} />
    </>
  );
}
