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


class QuotaSearchCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_adds_search_without_pruning_hidden_quota_state(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            feature_dir = target / 'src/features/authFiles'
            feature_dir.mkdir(parents=True)
            page_path = feature_dir / 'AuthFilesPage.tsx'
            page_path.write_text(AUTH_FILES_PAGE_SOURCE)

            CUSTOMIZATIONS.patch_auth_files_page_search_latest(target)
            CUSTOMIZATIONS.flush_writes()

            page = page_path.read_text()
            self.assertIn('buildQuotaSearchValues', page)
            self.assertIn('matchesQuotaSearch', page)
            self.assertIn('const quotaSearchStore = useMemo(', page)
            self.assertIn('state.geminiCliQuota', page)
            self.assertIn('matchesQuotaSearch(buildQuotaSearchValues(item, quotaSearchStore, t)', page)
            self.assertIn('quotaSearchStore, t, wildcardSearch', page)

            CUSTOMIZATIONS.patch_auth_files_page_search_latest(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(page, page_path.read_text())


if __name__ == '__main__':
    unittest.main()
