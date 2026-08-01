import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


AUTH_FILES_PAGE_SOURCE = """import { buildQuotaSearchValues, matchesQuotaSearch } from '@/pro/modules/quota';

export function AuthFilesPage() {
  const normalizedFilter = normalizeProviderKey(String(filter));
  const sorted = useMemo(() => sortAuthFiles(filtered, sortMode), [filtered, sortMode]);

  return (
    <Select
                value={sortMode}
                options={sortOptions}
                onChange={handleSortModeChange}
          sortMode={sortMode}
    />
  );
}
"""


UI_STATE_SOURCE = """export const AUTH_FILES_SORT_MODES = ['default', 'az', 'priority'] as const;
export type AuthFilesSortMode = (typeof AUTH_FILES_SORT_MODES)[number];
"""


class AuthFilesSortingCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_adds_provider_scoped_sorting_and_state_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            feature_dir = target / 'src/features/authFiles'
            feature_dir.mkdir(parents=True)
            page_path = feature_dir / 'AuthFilesPage.tsx'
            ui_state_path = feature_dir / 'uiState.ts'
            page_path.write_text(AUTH_FILES_PAGE_SOURCE)
            ui_state_path.write_text(UI_STATE_SOURCE)

            CUSTOMIZATIONS.patch_auth_files_page_sorting_latest(target)
            CUSTOMIZATIONS.flush_writes()

            page = page_path.read_text()
            ui_state = ui_state_path.read_text()

            self.assertIn("['default', 'az', 'priority', 'plan', 'quota']", ui_state)
            self.assertEqual(page.count("from '@/pro/modules/quota'"), 1)
            self.assertIn('compareAuthFilesByPlanDescending', page)
            self.assertIn('compareAuthFilesByAvailableQuotaDescending', page)
            self.assertIn("sortMode === 'plan' && !isAuthFilePlanSortProvider(normalizedFilter)", page)
            self.assertIn("sortMode === 'quota' && !isAuthFileQuotaSortProvider(normalizedFilter)", page)
            self.assertIn('compareAuthFilesByPlanDescending(a, b, quotaSearchStore)', page)
            self.assertIn('compareAuthFilesByAvailableQuotaDescending(a, b, quotaSearchStore)', page)
            self.assertIn('sortMode={effectiveSortMode}', page)

            CUSTOMIZATIONS.patch_auth_files_page_sorting_latest(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(page, page_path.read_text())
            self.assertEqual(ui_state, ui_state_path.read_text())

    def test_adds_sort_locale_labels(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            locales_dir = target / 'src/i18n/locales'
            locales_dir.mkdir(parents=True)
            for name in ('en.json', 'ru.json', 'zh-CN.json', 'zh-TW.json'):
                (locales_dir / name).write_text('{}')

            CUSTOMIZATIONS.patch_locales(target)
            CUSTOMIZATIONS.flush_writes()

            expected = {
                'en.json': ('Plan: High to Low', 'Available Quota: High to Low'),
                'ru.json': ('Тариф: по убыванию', 'Доступная квота: по убыванию'),
                'zh-CN.json': ('套餐从高到低', '可用额度从高到低'),
                'zh-TW.json': ('套餐由高到低', '可用額度由高到低'),
            }
            for name, labels in expected.items():
                data = json.loads((locales_dir / name).read_text())
                self.assertEqual(labels[0], data['auth_files']['sort_plan_desc'])
                self.assertEqual(labels[1], data['auth_files']['sort_quota_desc'])
                self.assertEqual('X Premium+', data['xai_quota']['plan_x_premium_plus'])
                self.assertIn('plan_free', data['xai_quota'])
                self.assertIn('free_quota_window', data['xai_quota'])


if __name__ == '__main__':
    unittest.main()
