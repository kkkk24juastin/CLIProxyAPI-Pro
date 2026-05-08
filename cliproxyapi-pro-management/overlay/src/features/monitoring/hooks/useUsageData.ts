import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/services/api/client';
import {
  loadLegacyModelPrices,
  loadModelPricesFromSqlite,
  saveModelPricesToSqlite,
  type ModelPrice,
} from '@/utils/usage';

export interface UsagePayload {
  total_requests?: number;
  success_count?: number;
  failure_count?: number;
  total_tokens?: number;
  apis?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface UseUsageDataReturn {
  usage: UsagePayload | null;
  loading: boolean;
  error: string;
  lastRefreshedAt: Date | null;
  modelPrices: Record<string, ModelPrice>;
  setModelPrices: (prices: Record<string, ModelPrice>) => void;
  loadUsage: () => Promise<void>;
}

export function useUsageData(): UseUsageDataReturn {
  const [usage, setUsage] = useState<UsagePayload | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [lastRefreshedAt, setLastRefreshedAt] = useState<Date | null>(null);
  const [modelPrices, setModelPricesState] = useState<Record<string, ModelPrice>>({});
  const requestIdRef = useRef(0);

  const loadUsage = useCallback(async () => {
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;
    setLoading(true);
    setError('');

    try {
      const payload = await apiClient.get<UsagePayload>('/usage');
      if (requestIdRef.current !== requestId) return;
      setUsage(payload ?? null);
      setLastRefreshedAt(new Date());
    } catch (err) {
      if (requestIdRef.current !== requestId) return;
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (requestIdRef.current === requestId) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    const legacyPrices = loadLegacyModelPrices();
    setModelPricesState(legacyPrices);

    const syncModelPrices = async () => {
      try {
        const sqlitePrices = await loadModelPricesFromSqlite();
        if (cancelled) return;
        if (Object.keys(sqlitePrices).length > 0) {
          setModelPricesState(sqlitePrices);
          return;
        }
        if (Object.keys(legacyPrices).length > 0) {
          await saveModelPricesToSqlite(legacyPrices);
        }
      } catch (err) {
        console.error('Failed to sync model prices with sqlite:', err);
      }
    };

    void syncModelPrices();
    void loadUsage();

    return () => {
      cancelled = true;
    };
  }, [loadUsage]);

  const setModelPrices = useCallback((prices: Record<string, ModelPrice>) => {
    setModelPricesState(prices);
    void saveModelPricesToSqlite(prices).catch((err) => {
      console.error('Failed to save model prices to sqlite:', err);
    });
  }, []);

  return {
    usage,
    loading,
    error,
    lastRefreshedAt,
    modelPrices,
    setModelPrices,
    loadUsage,
  };
}
