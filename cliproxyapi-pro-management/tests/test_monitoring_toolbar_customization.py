import json
import unittest
from pathlib import Path


PAGE_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/monitoring/MonitoringCenterPage.tsx'
)
STYLE_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/monitoring/features/monitoring.module.scss'
)
STYLE_DIR = STYLE_PATH.parent / 'styles'
REALTIME_HOOK_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/monitoring/features/hooks/useRealtimeLogData.ts'
)
REALTIME_PREFERENCES_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/monitoring/features/realtimeLogPreferences.ts'
)
WEBDAV_DIALOG_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/monitoring/features/components/WebDAVRestoreDialog.tsx'
)
ACCOUNT_PLAN_PATH = (
    Path(__file__).resolve().parents[1]
    / 'overlay/src/pro/modules/quota/accountPlan.ts'
)
LOCALES_PATH = Path(__file__).resolve().parents[1] / 'monitoring-locales.json'


def read_monitoring_styles() -> str:
    return '\n'.join(path.read_text() for path in sorted(STYLE_DIR.glob('*.scss')))


class MonitoringToolbarCustomizationTest(unittest.TestCase):
    def test_import_actions_share_one_dropdown_entry(self) -> None:
        source = PAGE_PATH.read_text()

        self.assertIn('aria-haspopup="menu"', source)
        self.assertIn("usage_stats.import_from_file", source)
        self.assertIn("usage_stats.import_from_webdav", source)
        self.assertIn('onClick={handleImportFromFileClick}', source)
        self.assertNotIn("t('usage_stats.webdav_restore_action')", source)

    def test_webdav_restore_uses_compact_global_pro_copy(self) -> None:
        source = PAGE_PATH.read_text()
        dialog = WEBDAV_DIALOG_PATH.read_text()
        styles = read_monitoring_styles()

        self.assertIn('className={styles.webdavRestoreSurface}', dialog)
        self.assertIn('width: min(720px, calc(100vw - 32px)) !important;', styles)
        self.assertIn("usage_stats.import_restore_scope", source)
        self.assertIn("usage_stats.import_restore_scope", dialog)
        self.assertNotIn("webdavRestoreWarning", dialog)
        self.assertIn(".webdavRestoreScope::after", styles)
        self.assertIn("inset 4px 0 0 var(--monitor-accent-strong)", styles)

    def test_realtime_duration_combines_ttft_and_total_latency(self) -> None:
        source = PAGE_PATH.read_text()

        self.assertNotIn("    ttft: {", source)
        self.assertIn("t('monitoring.realtime_duration_ttft')", source)
        self.assertIn("t('monitoring.realtime_duration_total')", source)
        self.assertIn("className={styles.realtimeDurationCell}", source)

    def test_simplified_chinese_inspection_duration_uses_concise_minute_unit(self) -> None:
        locales = json.loads(LOCALES_PATH.read_text())
        self.assertEqual(
            '{{count}}分',
            locales['zh-CN.json']['monitoring']['account_inspection_duration_minute'],
        )

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

    def test_async_surface_openers_use_synchronous_promise_guards(self) -> None:
        source = PAGE_PATH.read_text()
        self.assertIn('monitoringSettingsRequestRef.current', source)
        self.assertIn('priceManagementRequestRef.current', source)
        self.assertIn('if (monitoringSettingsRequestRef.current) return', source)
        self.assertIn('if (priceManagementRequestRef.current) return', source)
        self.assertIn('disabled={isPriceLoading}', source)
        self.assertIn("openSurface('monitoring-settings')", source)
        self.assertIn("openSurface('price-management')", source)

    def test_realtime_logs_pause_auto_refresh_during_browsing(self) -> None:
        source = PAGE_PATH.read_text()
        hook_source = REALTIME_HOOK_PATH.read_text()

        self.assertIn("const autoRefreshPaused = page !== 1", hook_source)
        self.assertIn("|| !followEnabled", hook_source)
        self.assertIn("|| !atTop", hook_source)
        self.assertIn("|| detailsOpen", hook_source)
        self.assertIn("&& page === 1", hook_source)
        self.assertIn("&& !autoRefreshPaused", hook_source)
        self.assertIn("void refresh('top');", hook_source)
        self.assertIn("onScroll={handleRealtimeLogScroll}", source)

    def test_realtime_logs_restore_the_internal_scroll_anchor(self) -> None:
        source = PAGE_PATH.read_text()
        hook_source = REALTIME_HOOK_PATH.read_text()
        styles = read_monitoring_styles()

        self.assertIn("data-realtime-row-id={row.id}", source)
        self.assertIn("pendingScrollSnapshotRef", hook_source)
        self.assertIn("anchor.getBoundingClientRect().top - wrapperRect.top - snapshot.anchorOffset", hook_source)
        self.assertIn("overflow-anchor: none;", styles)

    def test_realtime_follow_control_and_pending_update_action_are_present(self) -> None:
        source = PAGE_PATH.read_text()

        self.assertIn('role="switch"', source)
        self.assertIn("monitoring.request_events_live_follow", source)
        self.assertIn("monitoring.request_events_paused_hint", source)
        self.assertIn("monitoring.request_events_view_latest", source)

    def test_realtime_follow_refresh_does_not_change_outer_layout_height(self) -> None:
        source = PAGE_PATH.read_text()
        styles = read_monitoring_styles()

        self.assertIn("pendingRealtimeEventCount > 0 && realtimeLogAutoRefreshPaused", source)
        self.assertIn("className={styles.realtimeTableShell}", source)
        self.assertIn("height: min(620px, 68vh);", styles)
        self.assertIn(".realtimeUpdateBar {\n  position: absolute;", styles)
        self.assertIn("flex-wrap: nowrap;", styles)

    def test_realtime_logs_show_account_plan_from_shared_quota_sources(self) -> None:
        source = PAGE_PATH.read_text()
        preferences = REALTIME_PREFERENCES_PATH.read_text()
        account_plan = ACCOUNT_PLAN_PATH.read_text()
        locales = LOCALES_PATH.read_text()

        self.assertIn("'accountPlan'", preferences)
        self.assertIn("shouldMigrateAccountPlan", preferences)
        self.assertIn("label: t('monitoring.column_account_plan')", source)
        self.assertIn("authFileByAuthIndex.get(row.authIndex)", source)
        self.assertIn("accountPlan: resolveAccountPlanLabel({", source)
        self.assertIn("quotaStore.antigravityQuota[fileName]", account_plan)
        self.assertIn("quotaStore.claudeQuota[fileName]", account_plan)
        self.assertIn("quotaStore.codexQuota[fileName]", account_plan)
        self.assertIn("quotaStore.geminiCliQuota[fileName]", account_plan)
        self.assertIn("quotaStore.kimiQuota[fileName]", account_plan)
        self.assertIn("quotaStore.xaiQuota[fileName]", account_plan)
        self.assertIn('"column_account_plan": "Account Plan"', locales)
        self.assertIn('"column_account_plan": "账号套餐"', locales)


if __name__ == '__main__':
    unittest.main()
