import { useCallback, useEffect, useState } from 'react';
import {
  getAccountUsage,
  type AccountUsageRangeDays,
  type AccountUsageResponse,
} from '@/pro/modules/monitoring/api';

const isCanceledRequest = (error: unknown) => {
  if (!(error instanceof Error)) return false;
  return error.name === 'AbortError' || error.name === 'CanceledError';
};

export function useAccountUsage(
  authIndex: string | null,
  days: AccountUsageRangeDays,
  enabled: boolean
) {
  const [data, setData] = useState<AccountUsageResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [refreshToken, setRefreshToken] = useState(0);

  const refresh = useCallback(() => setRefreshToken((current) => current + 1), []);

  useEffect(() => {
    if (!enabled || !authIndex) {
      setData(null);
      setLoading(false);
      setError('');
      return;
    }

    const controller = new AbortController();
    setLoading(true);
    setError('');
    void getAccountUsage(authIndex, days, -new Date().getTimezoneOffset(), {
      signal: controller.signal,
    })
      .then((response) => setData(response))
      .catch((requestError: unknown) => {
        if (!isCanceledRequest(requestError)) {
          setError(requestError instanceof Error ? requestError.message : String(requestError));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [authIndex, days, enabled, refreshToken]);

  return { data, loading, error, refresh };
}
