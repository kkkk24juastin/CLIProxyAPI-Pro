import json
import unittest
from pathlib import Path


PAGE_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pages/MonitoringCenterPage.tsx'
)
ACCOUNT_INSPECTION_PAGE_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pages/AccountInspectionPage.tsx'
)
LOCALES_PATH = Path(__file__).resolve().parents[1] / 'monitoring-locales.json'


class MonitoringToolbarCustomizationTest(unittest.TestCase):
    def test_monitoring_settings_button_keeps_a_stable_label_while_loading(self) -> None:
        source = PAGE_PATH.read_text()
        handler = 'onClick={() => void loadMonitoringSettings()}'
        start = source.index(handler)
        end = source.index('</button>', start)
        button = source[start:end]

        self.assertIn('disabled={isMonitoringSettingsLoading}', button)
        self.assertIn('aria-busy={isMonitoringSettingsLoading}', button)
        self.assertIn("{t('usage_stats.monitoring_settings')}", button)
        self.assertNotIn("isMonitoringSettingsLoading ? t('common.loading')", button)

    def test_pagination_uses_the_localized_usage_stats_key(self) -> None:
        sources = [PAGE_PATH.read_text(), ACCOUNT_INSPECTION_PAGE_PATH.read_text()]
        self.assertTrue(any("t('usage_stats.pagination_info'" in source for source in sources))
        for source in sources:
            self.assertNotIn("t('monitoring.pagination_info'", source)

        locales = json.loads(LOCALES_PATH.read_text())
        self.assertEqual({'en.json', 'ru.json', 'zh-CN.json', 'zh-TW.json'}, set(locales))
        for locale, additions in locales.items():
            with self.subTest(locale=locale):
                self.assertTrue(additions['usage_stats']['pagination_info'])


if __name__ == '__main__':
    unittest.main()
