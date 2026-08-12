import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PRO_ROOT = ROOT / 'overlay/src/pro'
CUSTOMIZER = ROOT / 'apply_customizations.py'
SURFACE = PRO_ROOT / 'shared/ProSurface.tsx'
SURFACE_STYLE = PRO_ROOT / 'shared/ProSurface.module.scss'
SURFACE_STATE = PRO_ROOT / 'shared/useProSurfaceState.ts'


class ProSurfaceCustomizationTest(unittest.TestCase):
    def test_business_surfaces_use_semantic_pro_components(self) -> None:
        direct_modal_users = []
        direct_sheet_users = []
        for path in PRO_ROOT.rglob('*.tsx'):
            if path == SURFACE:
                continue
            source = path.read_text()
            if "@/components/ui/Modal" in source or '<Modal' in source:
                direct_modal_users.append(path.relative_to(PRO_ROOT).as_posix())
            if "@/components/ui/Sheet" in source or '<Sheet' in source:
                direct_sheet_users.append(path.relative_to(PRO_ROOT).as_posix())
        self.assertEqual([], direct_modal_users)
        self.assertEqual([], direct_sheet_users)

    def test_surface_layer_defines_all_product_categories(self) -> None:
        source = SURFACE.read_text()
        state_source = SURFACE_STATE.read_text()
        styles = SURFACE_STYLE.read_text()
        for component in (
            'ProDetailDialog',
            'ProFormDialog',
            'ProTaskDialog',
            'ProWorkspaceDialog',
            'ProWorkspaceSheet',
            'ProSettingsSheet',
        ):
            self.assertIn(component, source)
        self.assertIn('useProSurfaceState', state_source)
        self.assertIn("@media (max-width: 720px)", styles)
        self.assertIn('height: 100dvh;', styles)

    def test_workspace_settings_use_sheets_and_shared_footer_contract(self) -> None:
        monitoring_settings = (PRO_ROOT / 'modules/monitoring/features/components/MonitoringSettingsModal.tsx').read_text()
        prices = (PRO_ROOT / 'modules/monitoring/features/components/ModelPriceManagerModal.tsx').read_text()
        inspection = (PRO_ROOT / 'modules/inspection/AccountInspectionPage.tsx').read_text()
        self.assertIn('<ProSettingsSheet', monitoring_settings)
        self.assertIn('<ProSettingsSheet', prices)
        self.assertIn('<ProSettingsSheet', inspection)
        self.assertNotIn('<ProWorkspaceDialog', monitoring_settings)
        self.assertNotIn('<ProWorkspaceDialog', prices)
        self.assertNotIn('<ProWorkspaceDialog', inspection)
        self.assertIn('dirty={monitoringSettingsDirty}', monitoring_settings)
        self.assertIn('dirty={settingsDirty}', inspection)
        self.assertNotIn('size="xl"', monitoring_settings)
        self.assertNotIn('size="xl"', prices)
        self.assertNotIn('size="xl"', inspection)

    def test_settings_sheets_use_native_confirm_close_contract(self) -> None:
        surface = SURFACE.read_text()
        monitoring = (PRO_ROOT / 'modules/monitoring/MonitoringCenterPage.tsx').read_text()
        monitoring_settings = (PRO_ROOT / 'modules/monitoring/features/components/MonitoringSettingsModal.tsx').read_text()
        prices = (PRO_ROOT / 'modules/monitoring/features/components/ModelPriceManagerModal.tsx').read_text()
        inspection = (PRO_ROOT / 'modules/inspection/AccountInspectionPage.tsx').read_text()
        self.assertIn('confirmClose={confirmSettingsClose}', surface)
        self.assertIn('onClick={() => void handleCancelClick()}', surface)
        self.assertIn('onClose={commitClose}', surface)
        self.assertIn('closeDisabled={busy || props.closeDisabled}', surface)
        self.assertIn('confirmClose={confirmMonitoringSettingsClose}', monitoring_settings)
        self.assertIn('onDiscard={discardMonitoringSettingsDraft}', monitoring_settings)
        self.assertIn('onCancel: () => resolve(false)', monitoring)
        self.assertIn('confirmClose={confirmPriceWorkspaceClose}', prices)
        self.assertIn('onDiscard={discardPriceWorkspaceDraft}', prices)
        self.assertIn('confirmClose={confirmSettingsModalClose}', inspection)
        self.assertIn('onDiscard={discardSettingsModalDraft}', inspection)

    def test_workspace_sheet_layout_responds_to_sheet_width(self) -> None:
        surface_styles = SURFACE_STYLE.read_text()
        monitoring_styles = (PRO_ROOT / 'modules/monitoring/features/styles/_responsive.scss').read_text()
        inspection_styles = (PRO_ROOT / 'modules/inspection/features/account-inspection-styles/_responsive.scss').read_text()
        self.assertIn('container: pro-workspace-sheet / inline-size;', surface_styles)
        self.assertIn('@container pro-workspace-sheet (max-width: 1020px)', monitoring_styles)
        self.assertIn('@container pro-workspace-sheet (max-width: 720px)', monitoring_styles)
        self.assertIn('@container pro-workspace-sheet (max-width: 900px)', inspection_styles)

    def test_settings_surfaces_freeze_all_actions_while_busy(self) -> None:
        surface = SURFACE.read_text()
        self.assertGreaterEqual(surface.count('disabled={busy}'), 3)
        self.assertIn(
            '<fieldset className={styles.settingsFooterStart} disabled={busy} aria-busy={busy}>',
            surface,
        )
        self.assertIn(
            '<fieldset className={styles.settingsFields} disabled={busy} aria-busy={busy}>',
            surface,
        )

    def test_price_workspace_preserves_dirty_state_across_tabs_and_model_changes(self) -> None:
        prices = (PRO_ROOT / 'modules/monitoring/features/components/ModelPriceManagerModal.tsx').read_text()
        self.assertIn('const workspaceDirty = priceEditorDirty || scheduleDirty;', prices)
        self.assertIn('const requestPriceTargetChange = (model: string) => {', prices)
        self.assertIn('if (!priceEditorDirty) {', prices)
        self.assertIn('dedupeKey: `model-price:discard:${priceModel}`', prices)
        self.assertIn('onConfirm: () => selectPriceTarget(model)', prices)
        self.assertIn('onClick={() => requestPriceTargetChange(item.model)}', prices)
        self.assertNotIn('onClick={() => selectPriceTarget(item.model)}', prices)

    def test_loading_requests_reopen_the_surface_before_reusing_inflight_work(self) -> None:
        monitoring = (PRO_ROOT / 'modules/monitoring/MonitoringCenterPage.tsx').read_text()
        monitoring_open = monitoring.index('setIsMonitoringSettingsOpen(true);')
        monitoring_reuse = monitoring.index('if (monitoringSettingsRequestRef.current)')
        price_open = monitoring.index('setIsPriceModalOpen(true);')
        price_reuse = monitoring.index('if (priceManagementRequestRef.current)')
        self.assertLess(monitoring_open, monitoring_reuse)
        self.assertLess(price_open, price_reuse)

    def test_pages_coordinate_one_primary_surface(self) -> None:
        monitoring = (PRO_ROOT / 'modules/monitoring/MonitoringCenterPage.tsx').read_text()
        inspection = (PRO_ROOT / 'modules/inspection/AccountInspectionPage.tsx').read_text()
        proxy_pool = (PRO_ROOT / 'modules/proxyPool/ProxyPoolPage.tsx').read_text()
        self.assertIn("useProSurfaceState<'realtime-detail' | 'monitoring-settings' | 'price-management'>", monitoring)
        self.assertIn("useProSurfaceState<'settings' | 'detail'>", inspection)
        self.assertIn("useProSurfaceState<'node' | 'import' | 'takeover'>", proxy_pool)

    def test_detail_data_is_cleared_only_after_the_exit_animation(self) -> None:
        surface = SURFACE.read_text()
        monitoring = (PRO_ROOT / 'modules/monitoring/MonitoringCenterPage.tsx').read_text()
        inspection = (PRO_ROOT / 'modules/inspection/AccountInspectionPage.tsx').read_text()
        routing = (PRO_ROOT / 'modules/routing/RoutingPolicyPage.tsx').read_text()
        auth_test = (PRO_ROOT / 'authFiles/AuthFileConnectionTestModal.tsx').read_text()
        account_usage = (PRO_ROOT / 'modules/monitoring/features/components/AccountUsageModal.tsx').read_text()
        proxy_pool = (PRO_ROOT / 'modules/proxyPool/ProxyPoolPage.tsx').read_text()
        self.assertIn('onAfterClose?: () => void;', surface)
        self.assertIn('onAfterClose={onAfterClose}', surface)
        self.assertIn('onAfterClose={() => setSelectedRealtimeErrorRowState(null)}', monitoring)
        self.assertIn('onAfterClose={() => setSelectedDetailResultState(null)}', inspection)
        self.assertIn('onAfterClose={() => setSelectedRuntimeDetailState(null)}', routing)
        self.assertIn('onAfterClose={() => setDisplayFile(null)}', auth_test)
        self.assertIn('onAfterClose={() => setDisplayFile(null)}', account_usage)
        self.assertIn('onAfterClose={clearClosedNodeSheet}', proxy_pool)

    def test_rich_account_confirmations_use_business_dedupe_keys(self) -> None:
        inspection = (PRO_ROOT / 'modules/inspection/AccountInspectionPage.tsx').read_text()
        self.assertGreaterEqual(
            inspection.count('dedupeKey: `account-inspection:execute:'),
            3,
        )
        self.assertIn('${target.key}:${target.action}', inspection)
        self.assertIn('${item.key}:${item.action}', inspection)

    def test_auth_surface_state_insertion_has_an_exact_idempotency_marker(self) -> None:
        source = CUSTOMIZER.read_text()
        self.assertIn(
            "'const [connectionTestFile, setConnectionTestFileState]'",
            source,
        )
        self.assertNotIn(
            "'const [connectionTestFile, setConnectionTestFile]'",
            source,
        )


if __name__ == '__main__':
    unittest.main()
