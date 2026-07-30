import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ROOT.parent
CUSTOMIZER = ROOT / 'apply_customizations.py'
BRIDGE = ROOT / 'overlay/src/features/plugins/usePluginResourceBridge.ts'
PLUGIN_ROOT = REPO_ROOT / 'cliproxyapi-pro-plugins/pro-observability'
WEBAPP = PLUGIN_ROOT / 'webapp'


class ObservabilityPluginUICustomizationTest(unittest.TestCase):
    def test_monitoring_center_is_owned_only_by_plugin_resource(self) -> None:
        self.assertFalse((ROOT / 'overlay/src/pages/MonitoringCenterPage.tsx').exists())
        self.assertFalse((ROOT / 'overlay/src/features/monitoring/hooks/useUsageData.ts').exists())
        self.assertFalse((ROOT / 'overlay/src/features/monitoring/monitoring.module.scss').exists())

        customizer = CUSTOMIZER.read_text()
        self.assertNotIn("path: '/monitoring', element: <MonitoringCenterPage />", customizer)
        self.assertNotIn("import { MonitoringCenterPage }", customizer)
        self.assertNotIn("sidebarIcons.monitoring },", customizer)

        plugin = (PLUGIN_ROOT / 'main.go').read_text()
        page = (PLUGIN_ROOT / 'web/index.html').read_text()
        source = (WEBAPP / 'src/pages/MonitoringCenterPage.tsx').read_text()
        aggregate_hook = (WEBAPP / 'src/features/monitoring/hooks/useUsageAggregates.ts').read_text()
        self.assertIn('pluginabi.MethodManagementRegister', plugin)
        self.assertIn('observabilityManagementPage', plugin)
        self.assertIn('cliproxy-plugin-resource', page)
        self.assertIn('export function MonitoringCenterPage()', source)
        self.assertIn("'/usage/aggregates'", aggregate_hook)
        self.assertIn("'/usage/ui/settings'", source)

    def test_bridge_v2_keeps_auth_and_stream_transport_in_host(self) -> None:
        bridge = BRIDGE.read_text()
        self.assertIn('BRIDGE_VERSION = 2', bridge)
        self.assertIn("kind === 'host.bootstrap'", bridge)
        self.assertIn("kind === 'stream.open'", bridge)
        self.assertIn("path !== '/usage/stream'", bridge)
        self.assertIn("kind === 'file.upload'", bridge)
        self.assertIn("kind === 'file.download'", bridge)
        self.assertIn('event.source !== frameWindow', bridge)
        self.assertIn("!path.startsWith('//')", bridge)
        self.assertIn('sha256Hex(value)', bridge)
        self.assertNotIn('managementKey:', bridge)

    def test_account_usage_deep_link_targets_plugin_resource(self) -> None:
        source = (ROOT / 'overlay/src/features/monitoring/accountUsage.ts').read_text()
        self.assertIn('/plugin-pages/pro-observability/0?', source)
        self.assertNotIn('return `/monitoring?', source)


if __name__ == '__main__':
    unittest.main()
