import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', SCRIPT_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


PAGE_SOURCE = """import { useCallback, useEffect, useState } from 'react';
import { configApi, versionApi } from '@/services/api';

export function SystemPage() {
  const [checkingVersion, setCheckingVersion] = useState(false);

  useEffect(() => {
    fetchConfig().catch(() => {
      // ignore
    });
  }, [fetchConfig]);

  return (
            <button
              type="button"
              className={`${styles.infoTile} ${styles.tapTile}`}
              onClick={handleInfoVersionTap}
            >
              <div className={styles.tileHeader}>
                <div className={styles.tileLabel}>{t('footer.version')}</div>
              </div>
              <div className={styles.tileValue}>{appVersion}</div>
            </button>
  );
}
"""


class ManagementUpdateCustomizationTest(unittest.TestCase):
    def setUp(self):
        CUSTOMIZATIONS._writes.clear()

    def tearDown(self):
        CUSTOMIZATIONS._writes.clear()

    def test_adds_hash_aware_management_update_check(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp)
            pages_dir = target / 'src/pages'
            pages_dir.mkdir(parents=True)
            page_path = pages_dir / 'SystemPage.tsx'
            page_path.write_text(PAGE_SOURCE)

            for _ in range(2):
                CUSTOMIZATIONS.patch_management_update_check(target)
                CUSTOMIZATIONS.flush_writes()

            page = page_path.read_text()
            service = (
                SCRIPT_PATH.parent / 'overlay/src/pro/system/managementUpdate.ts'
            ).read_text()
            self.assertIn("apiClient.post<ManagementUpdateResult>('/management-panel/check-update')", service)
            extension = (
                SCRIPT_PATH.parent / 'overlay/src/pro/system/ManagementVersionTileExtension.tsx'
            ).read_text()
            self.assertEqual(
                page.count("from '@/pro/system/ManagementVersionTileExtension'"),
                1,
            )
            self.assertIn('<ManagementVersionTileExtension', page)
            self.assertIn('appVersion={appVersion}', page)
            self.assertIn('onVersionTap={handleInfoVersionTap}', page)
            self.assertIn('const result = await checkManagementPanelUpdate();', extension)
            self.assertIn('if (result.updated)', extension)
            self.assertIn("nextUrl.searchParams.set('_management_updated'", extension)
            self.assertNotIn('<button\n              type="button"\n              className={`${styles.infoTile}', page)


if __name__ == '__main__':
    unittest.main()
