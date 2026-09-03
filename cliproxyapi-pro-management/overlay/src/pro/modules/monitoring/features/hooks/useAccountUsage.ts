import { useCallback, useEffect, useState } from 'react';
import {
  getAccountUsage,
  type AccountUsageResponse,
} from '@/pro/modules/monitoring/api';
import { useAuthStore } from '@/stores/useAuthStore';
import { matchesAccountUsageDisplayIdentity } from '../accountUsage';
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
    authIndex: string;
    connectionKey: string;
    scopeKey: string;
    timeRange: TimeRangeSelection;
    response: AccountUsageResponse;
  } | null>(null);
  const [requesting, setRequesting] = useState(false);
  const [errorState, setErrorState] = useState<{
    connectionKey: string;
    scopeKey: string;
    message: string;
  } | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const apiBase = useAuthStore((state) => state.apiBase);
  const managementKey = useAuthStore((state) => state.managementKey);
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const timeRangeKey = getTimeRangeKey(timeRange);
  const scopeKey = `${authIndex ?? ''}:${timeRangeKey}`;
  const effectiveEnabled = enabled
    && connectionStatus === 'connected'
    && Boolean(apiBase)
    && Boolean(managementKey);
  const connectionKey = effectiveEnabled ? `${apiBase}\u0000${managementKey}` : '';
  const displayState = matchesAccountUsageDisplayIdentity(dataState, authIndex, connectionKey)
    ? dataState
    : null;
  const data = displayState?.response ?? null;
  const dataTimeRange = displayState?.timeRange ?? null;
  const scopeMatches = Boolean(displayState?.scopeKey === scopeKey);
  const dataStale = Boolean(displayState && !scopeMatches);
  const error = errorState?.connectionKey === connectionKey && errorState.scopeKey === scopeKey
    ? errorState.message
    : '';
  const loading = effectiveEnabled && Boolean(authIndex) && !data && (requesting || !error);
  const refreshing = requesting && Boolean(data);

  const refresh = useCallback(() => setRefreshToken((current) => current + 1), []);

  useEffect(() => {
    if (!effectiveEnabled || !authIndex) {
      setDataState(null);
      setRequesting(false);
      setErrorState(null);
      return;
    }

    const controller = new AbortController();
    setRequesting(true);
    setErrorState(null);
    void getAccountUsage(authIndex, timeRange, -new Date().getTimezoneOffset(), getLocalTimeZone(), {
      signal: controller.signal,
    })
      .then((response) => {
        if (!controller.signal.aborted) {
          setDataState({ authIndex, connectionKey, scopeKey, timeRange, response });
        }
      })
      .catch((requestError: unknown) => {
        if (!isCanceledRequest(requestError)) {
          setErrorState({
            connectionKey,
            scopeKey,
            message: requestError instanceof Error ? requestError.message : String(requestError),
          });
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setRequesting(false);
      });

    return () => controller.abort();
  }, [authIndex, connectionKey, effectiveEnabled, refreshToken, scopeKey, timeRange, timeRangeKey]);

  return {
    data,
    dataTimeRange,
    dataStale,
    scopeMatches,
    loading,
    refreshing,
    error,
    refresh,
  };
}
