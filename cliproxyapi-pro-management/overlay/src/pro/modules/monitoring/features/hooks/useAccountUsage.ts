import { useCallback, useEffect, useState } from 'react';
import {
  getAccountUsage,
  type AccountUsageResponse,
} from '@/pro/modules/monitoring/api';
import { getLocalTimeZone, getTimeRangeKey, type TimeRangeSelection } from '../timeRange';

const isCanceledRequest = (error: unknown) => {
  if (!(error instanceof Error)) return false;
  return error.name === 'AbortError' || error.name === 'CanceledError';
};

export function useAccountUsage(
  authIndex: string | null,
  timeRange: TimeRangeSelection,
  enabled: boolean
) {
  const [dataState, setDataState] = useState<{
    scopeKey: string;
    response: AccountUsageResponse;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [errorState, setErrorState] = useState<{
    scopeKey: string;
    message: string;
  } | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const timeRangeKey = getTimeRangeKey(timeRange);
  const scopeKey = `${authIndex ?? ''}:${timeRangeKey}`;
  const data = dataState?.scopeKey === scopeKey ? dataState.response : null;
  const error = errorState?.scopeKey === scopeKey ? errorState.message : '';

  const refresh = useCallback(() => setRefreshToken((current) => current + 1), []);

  useEffect(() => {
    if (!enabled || !authIndex) {
      setDataState(null);
      setLoading(false);
      setErrorState(null);
      return;
    }

    const controller = new AbortController();
    setLoading(true);
    setErrorState(null);
    void getAccountUsage(authIndex, timeRange, -new Date().getTimezoneOffset(), getLocalTimeZone(), {
      signal: controller.signal,
    })
      .then((response) => setDataState({ scopeKey, response }))
      .catch((requestError: unknown) => {
        if (!isCanceledRequest(requestError)) {
          setErrorState({
            scopeKey,
            message: requestError instanceof Error ? requestError.message : String(requestError),
          });
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [authIndex, enabled, refreshToken, scopeKey, timeRange, timeRangeKey]);

  return { data, loading, error, refresh };
}
