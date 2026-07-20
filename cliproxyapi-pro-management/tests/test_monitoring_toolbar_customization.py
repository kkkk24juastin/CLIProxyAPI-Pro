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
STYLE_PATH = PAGE_PATH.with_suffix('.module.scss')


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

    def test_realtime_logs_pause_auto_refresh_during_browsing(self) -> None:
        source = PAGE_PATH.read_text()

        self.assertIn("const realtimeLogAutoRefreshPaused = realtimeLogPage !== 1", source)
        self.assertIn("|| !realtimeLogFollowEnabled", source)
        self.assertIn("|| !realtimeLogAtTop", source)
        self.assertIn("|| Boolean(selectedRealtimeErrorRow)", source)
        self.assertIn("&& realtimeLogPage === 1", source)
        self.assertIn("&& !realtimeLogAutoRefreshPaused", source)
        self.assertIn("void refreshRealtimeLogs('top');", source)
        self.assertIn("onScroll={handleRealtimeLogScroll}", source)

    def test_realtime_logs_restore_the_internal_scroll_anchor(self) -> None:
        source = PAGE_PATH.read_text()
        styles = STYLE_PATH.read_text()

        self.assertIn("data-realtime-row-id={row.id}", source)
        self.assertIn("pendingRealtimeLogScrollSnapshotRef", source)
        self.assertIn("anchor.getBoundingClientRect().top - wrapperRect.top - snapshot.anchorOffset", source)
        self.assertIn("overflow-anchor: none;", styles)

    def test_realtime_follow_control_and_pending_update_action_are_present(self) -> None:
        source = PAGE_PATH.read_text()

        self.assertIn('role="switch"', source)
        self.assertIn("monitoring.request_events_live_follow", source)
        self.assertIn("monitoring.request_events_paused_hint", source)
        self.assertIn("monitoring.request_events_view_latest", source)

    def test_realtime_follow_refresh_does_not_change_outer_layout_height(self) -> None:
        source = PAGE_PATH.read_text()
        styles = STYLE_PATH.read_text()

        self.assertIn("pendingRealtimeEventCount > 0 && realtimeLogAutoRefreshPaused", source)
        self.assertIn("className={styles.realtimeTableShell}", source)
        self.assertIn("height: min(620px, 68vh);", styles)
        self.assertIn(".realtimeUpdateBar {\n  position: absolute;", styles)
        self.assertIn("flex-wrap: nowrap;", styles)


if __name__ == '__main__':
    unittest.main()
