import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', SCRIPT_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


VERSION_SOURCE = """import { apiClient } from './client';

export const versionApi = {
  checkLatest: () => apiClient.get<Record<string, unknown>>('/latest-version'),
};
"""

PAGE_SOURCE = """import { useCallback, useEffect, useState } from 'react';

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
            api_dir = target / 'src/services/api'
            pages_dir = target / 'src/pages'
            api_dir.mkdir(parents=True)
            pages_dir.mkdir(parents=True)
            version_path = api_dir / 'version.ts'
            page_path = pages_dir / 'SystemPage.tsx'
            version_path.write_text(VERSION_SOURCE)
            page_path.write_text(PAGE_SOURCE)

            for _ in range(2):
                CUSTOMIZATIONS.patch_management_update_check(target)
                CUSTOMIZATIONS.flush_writes()

            version = version_path.read_text()
            page = page_path.read_text()
            self.assertEqual(version.count('checkManagementPanelUpdate:'), 1)
            self.assertIn("'/management-panel/check-update'", version)
            self.assertEqual(page.count('handleManagementUpdateCheck'), 2)
            self.assertIn('if (result.updated)', page)
            self.assertIn("nextUrl.searchParams.set('_management_updated'", page)
            self.assertNotIn('<button\n              type="button"\n              className={`${styles.infoTile}', page)


if __name__ == '__main__':
    unittest.main()
