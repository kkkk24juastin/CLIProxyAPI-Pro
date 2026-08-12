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
        ):
            self.assertIn(component, source)
        self.assertIn('useProSurfaceState', state_source)
        self.assertIn("@media (max-width: 720px)", styles)
        self.assertIn('height: 100dvh;', styles)

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
        self.assertIn('onAfterClose?: () => void;', surface)
        self.assertIn('onAfterClose={onAfterClose}', surface)
        self.assertIn('onAfterClose={() => setSelectedRealtimeErrorRowState(null)}', monitoring)
        self.assertIn('onAfterClose={() => setSelectedDetailResultState(null)}', inspection)
        self.assertIn('onAfterClose={() => setSelectedRuntimeDetailState(null)}', routing)

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
