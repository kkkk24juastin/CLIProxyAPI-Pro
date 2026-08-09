import type { TFunction } from 'i18next';
import type { AuthFileItem, XaiBillingSummary, XaiQuotaState } from '@/types';
import { apiCallApi, getApiCallErrorMessage } from '@/services/api/apiCall';
import { useQuotaStore } from '@/stores';
import {
  XAI_PAID_HEALTH_MODEL,
  XAI_REQUEST_HEADERS,
  createStatusError,
  isXaiUsingOfficialAPI,
} from '@/utils/quota';
import { XAI_CONFIG, requestXaiPaidHealth } from '@/features/quota/providers/xai/data';
import type { QuotaProviderData } from '@/features/quota/providers/types';
import { normalizeAuthIndex } from '@/utils/authIndex';
import {
  XAI_FREE_QUOTA_PROBE_URL,
  mergeXaiBillingRuntimeState,
  parseXaiFreeQuotaProbe,
  resolveXaiPlanType,
  isXaiMonthlyBillingKnown,
} from './xaiQuota';

const REQUEST_TIMEOUT_MS = 15_000;

async function requestXaiFreeQuota(authIndex: string, t: TFunction) {
  const result = await apiCallApi.request(
    {
      authIndex,
      method: 'POST',
      url: XAI_FREE_QUOTA_PROBE_URL,
      header: {
        ...XAI_REQUEST_HEADERS,
        accept: 'text/event-stream',
        'Content-Type': 'application/json',
      },
      data: JSON.stringify({
        model: XAI_PAID_HEALTH_MODEL,
        input: [{ role: 'user', content: [{ type: 'input_text', text: 'ping' }] }],
        instructions: 'You are a helpful assistant. Reply briefly.',
        max_output_tokens: 1,
        stream: true,
        store: false,
      }),
      useExecutor: true,
    },
    { timeout: REQUEST_TIMEOUT_MS }
  );
  const quota = parseXaiFreeQuotaProbe(result, XAI_PAID_HEALTH_MODEL);
  if (quota) return quota;
  if (result.statusCode < 200 || result.statusCode >= 300) {
    throw createStatusError(getApiCallErrorMessage(result), result.statusCode);
  }
  throw new Error(t('xai_quota.empty_data'));
}

async function fetchProXaiQuota(file: AuthFileItem, t: TFunction): Promise<XaiBillingSummary> {
  const authIndex = normalizeAuthIndex(file.auth_index ?? file.authIndex);
  if (authIndex && isXaiUsingOfficialAPI(file)) {
    const billing = await requestXaiPaidHealth(authIndex);
    const previous = useQuotaStore.getState().xaiQuota[file.name]?.billing;
    return mergeXaiBillingRuntimeState({ ...billing, planType: 'paid' }, previous);
  }

  const billing = await XAI_CONFIG.fetchQuota(file, t);
  const previous = useQuotaStore.getState().xaiQuota[file.name]?.billing;
  if (billing.mode === 'paid-health') {
    return mergeXaiBillingRuntimeState({ ...billing, planType: 'paid' }, previous);
  }

  const planType = resolveXaiPlanType(billing.monthlyLimitCents, isXaiMonthlyBillingKnown(billing));
  const merged = mergeXaiBillingRuntimeState({ ...billing, planType }, previous);
  if (planType !== 'free') return merged;

  if (!authIndex) return merged;
  const freeQuota = await requestXaiFreeQuota(authIndex, t);
  return { ...merged, freeQuota };
}

export const PRO_XAI_CONFIG: QuotaProviderData<XaiQuotaState, XaiBillingSummary> = {
  ...XAI_CONFIG,
  fetchQuota: fetchProXaiQuota,
  buildSuccessState: (billing) => ({ status: 'success', billing }),
};
