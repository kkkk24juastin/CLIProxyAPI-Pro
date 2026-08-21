import { useCallback, useEffect, useMemo } from 'react';
import type { TFunction } from 'i18next';
import { sortAuthFiles } from '@/features/authFiles/logic';
import type { AuthFilesSortMode } from '@/features/authFiles/uiState';
import { useQuotaStore } from '@/stores';
import type { AuthFileItem } from '@/types';
import {
  buildQuotaSearchValues,
  compareAuthFilesByAvailableQuotaDescending,
  compareAuthFilesByPlanDescending,
  isAuthFilePlanSortProvider,
  isAuthFileQuotaSortProvider,
  matchesQuotaSearch,
  quotaPersistenceMiddleware,
} from '@/pro/modules/quota';

export interface UseAuthFilesQuotaExtensionsOptions {
  files: AuthFileItem[];
  isCurrentLayer: boolean;
  normalizedFilter: string;
  search: string;
  sortMode: AuthFilesSortMode;
  setSortMode: (mode: AuthFilesSortMode) => void;
  resetPage: () => void;
  t: TFunction;
}

export function useAuthFilesQuotaExtensions({
  files,
  isCurrentLayer,
  normalizedFilter,
  search,
  sortMode,
  setSortMode,
  resetPage,
  t,
}: UseAuthFilesQuotaExtensionsOptions) {
  const antigravityQuota = useQuotaStore((state) => state.antigravityQuota);
  const claudeQuota = useQuotaStore((state) => state.claudeQuota);
  const codexQuota = useQuotaStore((state) => state.codexQuota);
  const geminiCliQuota = useQuotaStore((state) => state.geminiCliQuota);
  const kimiQuota = useQuotaStore((state) => state.kimiQuota);
  const xaiQuota = useQuotaStore((state) => state.xaiQuota);
  const quotaSearchStore = useMemo(
    () => ({ antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota }),
    [antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota]
  );

  const planSortAvailable = isAuthFilePlanSortProvider(normalizedFilter);
  const quotaSortAvailable = isAuthFileQuotaSortProvider(normalizedFilter);
  const selectedSortModeAvailable =
    (sortMode !== 'plan' || planSortAvailable) &&
    (sortMode !== 'quota' || quotaSortAvailable);
  const effectiveSortMode: AuthFilesSortMode = selectedSortModeAvailable ? sortMode : 'default';

  useEffect(() => {
    if (selectedSortModeAvailable) return;
    setSortMode('default');
    resetPage();
  }, [resetPage, selectedSortModeAvailable, setSortMode]);

  useEffect(() => {
    if (!isCurrentLayer) return;
    void quotaPersistenceMiddleware.ensureFresh();
  }, [files, isCurrentLayer]);

  const sortOptions = useMemo(() => {
    const options: Array<{ value: AuthFilesSortMode; label: string }> = [
      { value: 'default', label: t('auth_files.sort_default') },
      { value: 'az', label: t('auth_files.sort_az') },
      { value: 'priority', label: t('auth_files.sort_priority') },
    ];
    if (planSortAvailable) {
      options.push({ value: 'plan', label: t('auth_files.sort_plan_desc') });
    }
    if (quotaSortAvailable) {
      options.push({ value: 'quota', label: t('auth_files.sort_quota_desc') });
    }
    return options;
  }, [planSortAvailable, quotaSortAvailable, t]);

  const matchesQuotaMetadata = useCallback(
    (file: AuthFileItem) =>
      matchesQuotaSearch(buildQuotaSearchValues(file, quotaSearchStore, t), search.trim()),
    [quotaSearchStore, search, t]
  );

  const ensureFresh = useCallback(() => quotaPersistenceMiddleware.ensureFresh(), []);

  const sortFiles = useCallback(
    (items: AuthFileItem[]) => {
      if (effectiveSortMode === 'plan') {
        return [...items].sort((a, b) =>
          compareAuthFilesByPlanDescending(a, b, quotaSearchStore)
        );
      }
      if (effectiveSortMode === 'quota') {
        return [...items].sort((a, b) =>
          compareAuthFilesByAvailableQuotaDescending(a, b, quotaSearchStore)
        );
      }
      return sortAuthFiles(items, effectiveSortMode);
    },
    [effectiveSortMode, quotaSearchStore]
  );

  return {
    effectiveSortMode,
    ensureFresh,
    matchesQuotaMetadata,
    sortFiles,
    sortOptions,
  };
}
