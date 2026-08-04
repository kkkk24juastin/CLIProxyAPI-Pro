import importlib.util
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


QUOTA_PAGE_SOURCE = """import { readQuotaUiState, writeQuotaUiState } from './uiState';

export function QuotaPage() {
  useEffect(() => {
    void loadFiles();
  }, [loadFiles]);
}
"""


class QuotaPageCacheRefreshCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_refreshes_sqlite_quota_cache_when_page_mounts(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            quota_dir = target / 'src/features/quota'
            quota_dir.mkdir(parents=True)
            page_path = quota_dir / 'QuotaPage.tsx'
            page_path.write_text(QUOTA_PAGE_SOURCE)

            CUSTOMIZATIONS.patch_quota_page_cache_refresh(target)
            CUSTOMIZATIONS.flush_writes()

            page = page_path.read_text()
            self.assertIn(
                "import { quotaPersistenceMiddleware } from '@/pro/modules/quota';",
                page,
            )
            self.assertIn('void quotaPersistenceMiddleware.ensureFresh();', page)

            CUSTOMIZATIONS.patch_quota_page_cache_refresh(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(page, page_path.read_text())


if __name__ == '__main__':
    unittest.main()
