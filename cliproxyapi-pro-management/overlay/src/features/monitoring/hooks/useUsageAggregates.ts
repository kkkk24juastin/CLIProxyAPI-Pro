import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/services/api/client';
import { getRangeStartMs, type MonitoringTimeRange } from './useMonitoringData';

export type UsageAggregateBucket = {
  bucketStartMs: number;
  bucketStart: string;
  provider?: string;
  model?: string;
  endpoint?: string;
  apiKeyHash?: string;
  totalRequests: number;
  successCount: number;
  failureCount: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number;
  cacheTokens: number;
  avgLatencyMs?: number;
  avgTtftMs?: number;
};

type UsageAggregateResponse = {
  items?: UsageAggregateBucket[];
  latest_id?: number;
  snapshot_at_ms?: number;
};

export type UsageAggregates = {
  trend: UsageAggregateBucket[];
  models: UsageAggregateBucket[];
  apiKeys: UsageAggregateBucket[];
  latestId: number;
  snapshotAtMs: number;
};

type UseUsageAggregatesParams = {
  latestId: number;
  timeRange: MonitoringTimeRange;
  apiKeyHash: string;
  enabled?: boolean;
};

type UseUsageAggregatesReturn = {
  data: UsageAggregates | null;
  loading: boolean;
  refreshing: boolean;
  error: string;
  refresh: () => Promise<void>;
};

const getAggregateRefreshIntervalMs = (timeRange: MonitoringTimeRange) => timeRange === 'all' ? 10000 : 3000;

const normalizeItems = (payload: UsageAggregateResponse | null | undefined) =>
  Array.isArray(payload?.items) ? payload.items : [];

export function useUsageAggregates({
  latestId,
  timeRange,
  apiKeyHash,
  enabled = true,
}: UseUsageAggregatesParams): UseUsageAggregatesReturn {
  const [data, setData] = useState<UsageAggregates | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [refreshNonce, setRefreshNonce] = useState(0);
  const requestIdRef = useRef(0);
  const queryGenerationRef = useRef(0);
  const lastFetchedAtRef = useRef(0);
  const refreshInFlightRef = useRef(false);
  const refreshPendingRef = useRef(false);
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const load = useCallback(async () => {
    if (!enabled) return;
    if (refreshInFlightRef.current) {
      refreshPendingRef.current = true;
      return;
    }
    refreshInFlightRef.current = true;
    const queryGeneration = queryGenerationRef.current;
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;
    setRefreshing(true);
    setError('');

    const nowMs = Date.now();
    const rangeStartMs = Number.isFinite(getRangeStartMs(timeRange, nowMs))
      ? getRangeStartMs(timeRange, nowMs)
      : 0;
    const allTrendStart = new Date(nowMs);
    allTrendStart.setHours(0, 0, 0, 0);
    allTrendStart.setDate(allTrendStart.getDate() - 23);
    const trendFromMs = timeRange === 'all' ? allTrendStart.getTime() : rangeStartMs;
    const interval = timeRange === 'today' ? 'hour' : 'day';
    const timezoneOffsetMinutes = -new Date().getTimezoneOffset();
    const trendParams: Record<string, string | number> = {
      from_ms: Math.max(trendFromMs, 0),
      to_ms: nowMs,
      interval,
      group_by: 'model',
      limit: 10000,
      timezone_offset_minutes: timezoneOffsetMinutes,
    };
    if (apiKeyHash !== 'all') {
      trendParams.api_key_hash = apiKeyHash;
    }
    const rankingParams = {
      from_ms: Math.max(rangeStartMs, 0),
      to_ms: nowMs,
      interval: 'all',
      limit: 10000,
      timezone_offset_minutes: timezoneOffsetMinutes,
    };

    try {
      const [trendPayload, modelPayload, apiKeyPayload] = await Promise.all([
        apiClient.get<UsageAggregateResponse>('/usage/aggregates', {
          params: trendParams,
        }),
        apiClient.get<UsageAggregateResponse>('/usage/aggregates', {
          params: { ...rankingParams, group_by: 'model', ...(apiKeyHash !== 'all' ? { api_key_hash: apiKeyHash } : {}) },
        }),
        apiClient.get<UsageAggregateResponse>('/usage/aggregates', {
          params: { ...rankingParams, group_by: 'api_key_hash,model' },
        }),
      ]);
      if (requestIdRef.current !== requestId || queryGenerationRef.current !== queryGeneration) return;
      const snapshotAtMs = Math.max(
        Number(trendPayload?.snapshot_at_ms) || 0,
        Number(modelPayload?.snapshot_at_ms) || 0,
        Number(apiKeyPayload?.snapshot_at_ms) || 0
      );
      setData({
        trend: normalizeItems(trendPayload),
        models: normalizeItems(modelPayload),
        apiKeys: normalizeItems(apiKeyPayload),
        latestId: Math.min(
          Number(trendPayload?.latest_id) || 0,
          Number(modelPayload?.latest_id) || 0,
          Number(apiKeyPayload?.latest_id) || 0
        ),
        snapshotAtMs,
      });
      lastFetchedAtRef.current = Date.now();
      setLoading(false);
    } catch (err) {
      if (requestIdRef.current !== requestId || queryGenerationRef.current !== queryGeneration) return;
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    } finally {
      if (requestIdRef.current === requestId) {
        refreshInFlightRef.current = false;
        setRefreshing(false);
        if (refreshPendingRef.current) {
          refreshPendingRef.current = false;
          setRefreshNonce((value) => value + 1);
        }
      }
    }
  }, [apiKeyHash, enabled, timeRange]);

  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    queryGenerationRef.current += 1;
    lastFetchedAtRef.current = 0;
    refreshPendingRef.current = refreshInFlightRef.current;
    setData(null);
    setError('');
    setLoading(enabled);
    if (refreshTimerRef.current) {
      window.clearTimeout(refreshTimerRef.current);
      refreshTimerRef.current = null;
    }
    setRefreshNonce((value) => value + 1);
  }, [apiKeyHash, enabled, timeRange]);

  useEffect(() => {
    if (!enabled) {
      if (refreshTimerRef.current) {
        window.clearTimeout(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
      setLoading(false);
      return;
    }
    if (refreshTimerRef.current) return;
    const refreshIntervalMs = getAggregateRefreshIntervalMs(timeRange);
    const elapsed = Date.now() - lastFetchedAtRef.current;
    refreshTimerRef.current = window.setTimeout(() => {
      refreshTimerRef.current = null;
      void loadRef.current();
    }, Math.max(0, refreshIntervalMs - elapsed));
  }, [enabled, latestId, refreshNonce, timeRange]);

  useEffect(() => () => {
    if (refreshTimerRef.current) {
      window.clearTimeout(refreshTimerRef.current);
    }
  }, []);

  return { data, loading, refreshing, error, refresh: load };
}
