import importlib.util
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


AUTH_FILES_PAGE_SOURCE = """import { useAuthStore, useNotificationStore, useThemeStore } from '@/stores';

export function AuthFilesPage() {
  const statusBarCache = useAuthFilesStatusBarCache(files);
  const filtered = useMemo(
    () => filesMatchingStatusFilters.filter((item) => {
        const matchType = !normalizedFilter || normalizeProviderKey(item.type) === normalizedFilter;
        return matchType && matchesAuthFileSearch(item, normalizedSearch, wildcardSearch);
      }),
    [filesMatchingStatusFilters, normalizedFilter, normalizedSearch, wildcardSearch]
  );
}
"""

QUOTA_PAGE_SOURCE = """import { EmptyState } from '@/components/ui/EmptyState';
import { readQuotaUiState, writeQuotaUiState } from './uiState';

export function QuotaPage() {
  const { t } = useTranslation();
  useEffect(() => {
    void loadFiles();
  }, [loadFiles]);

  const antigravityQuota = useQuotaStore((state) => state.antigravityQuota);
  const claudeQuota = useQuotaStore((state) => state.claudeQuota);
  const codexQuota = useQuotaStore((state) => state.codexQuota);
  const kimiQuota = useQuotaStore((state) => state.kimiQuota);
  const xaiQuota = useQuotaStore((state) => state.xaiQuota);

  const quotaByType = useMemo(
    () => ({
        antigravity: antigravityQuota,
        claude: claudeQuota,
        codex: codexQuota,
        kimi: kimiQuota,
        xai: xaiQuota,
      }),
    [antigravityQuota, claudeQuota, codexQuota, kimiQuota, xaiQuota]
  );

  const getQuota = useCallback(
    (entry) => quotaByType[entry.type][entry.file.name],
    [quotaByType]
  );

  const entries = useMemo(() => classifyQuotaFiles(files), [files]);
  const tabCounts = useMemo(() => buildTabCounts(entries), [entries]);
  const filteredEntries = useMemo(() => filterEntriesByTab(entries, tab), [entries, tab]);
  const sortedEntries = useMemo(
    () => sortQuotaEntries(filteredEntries, sortMode, resolveNextRecovery),
    [filteredEntries, sortMode, resolveNextRecovery]
  );
  const { pageItems, currentPage, totalPages } = useMemo(
    () => paginate(sortedEntries, page, QUOTA_PAGE_SIZE),
    [sortedEntries, page]
  );

  return (
    <>
        {error && (
          <div>{error}</div>
        )}
    </>
  );
}
"""


QUOTA_SECTION_REFRESH_SOURCE = """import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { triggerHeaderRefresh } from '@/hooks/useHeaderRefresh';

export function QuotaSection() {
  const {
    goToNext,
    loading: sectionLoading,
    setLoading,
  } = useQuotaPagination(filteredFiles);

  const { quota, loadQuota } = useQuotaLoader(config);

  const pendingQuotaRefreshRef = useRef(false);
  const prevFilesLoadingRef = useRef(loading);

  const handleRefresh = useCallback(() => {
    pendingQuotaRefreshRef.current = true;
    void triggerHeaderRefresh();
  }, []);

  useEffect(() => {
    const wasLoading = prevFilesLoadingRef.current;
    prevFilesLoadingRef.current = loading;

    if (!pendingQuotaRefreshRef.current) return;
    if (loading) return;
    if (!wasLoading) return;

    pendingQuotaRefreshRef.current = false;
    const targets = effectiveViewMode === 'all' ? filteredFiles : pageItems;
    if (targets.length === 0) return;
    loadQuota(targets, setLoading);
  }, [loading, effectiveViewMode, filteredFiles, pageItems, loadQuota, setLoading]);

  const isRefreshing = sectionLoading || loading;

  return (
    <Button
            onClick={handleRefresh}
    >
            {t('quota_management.refresh_all_credentials')}
    </Button>
  );
}
"""


class QuotaSearchCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_adds_search_without_pruning_hidden_quota_state(self) -> None:
        hook = (
            MODULE_PATH.parent / 'overlay/src/pro/authFiles/useAuthFilesQuotaExtensions.ts'
        ).read_text()
        customizer = MODULE_PATH.read_text()

        self.assertIn('buildQuotaSearchValues', hook)
        self.assertIn('matchesQuotaSearch', hook)
        self.assertIn('const quotaSearchStore = useMemo(', hook)
        self.assertIn('state.geminiCliQuota', hook)
        self.assertIn('matchesQuotaSearch(buildQuotaSearchValues(file, quotaSearchStore, t)', hook)
        self.assertIn('matchesQuotaMetadata(item)', customizer)
        self.assertIn('matchesQuotaMetadata, normalizedFilter', customizer)

    def test_quota_page_search_preserves_upstream_sort_pipeline(self) -> None:
        source = MODULE_PATH.read_text()
        self.assertIn('buildTabCounts(searchedEntries)', source)
        self.assertIn('filterEntriesByTab(searchedEntries, tab)', source)
        self.assertIn('sortQuotaEntries(filteredEntries, sortMode, resolveNextRecovery)', QUOTA_PAGE_SOURCE)
        self.assertIn('paginate(sortedEntries, page, QUOTA_PAGE_SIZE)', QUOTA_PAGE_SOURCE)

    def test_search_placeholders_are_concise_and_match_each_page(self) -> None:
        self.assertEqual(
            {
                'en.json': 'Search name, email, note, or plan',
                'ru.json': 'Поиск по имени, почте, заметке или тарифу',
                'zh-CN.json': '搜索名称、邮箱、备注或套餐',
                'zh-TW.json': '搜尋名稱、電子郵件、備註或套餐',
            },
            CUSTOMIZATIONS.AUTH_FILES_SEARCH_PLACEHOLDER_KEYS,
        )
        quota_placeholders = {
            locale: values['search_placeholder']
            for locale, values in CUSTOMIZATIONS.QUOTA_LOCALE_KEYS.items()
        }
        placeholders = [
            *CUSTOMIZATIONS.AUTH_FILES_SEARCH_PLACEHOLDER_KEYS.values(),
            *quota_placeholders.values(),
        ]
        self.assertTrue(all('auth_index' not in placeholder for placeholder in placeholders))


if __name__ == '__main__':
    unittest.main()
