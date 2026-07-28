import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PAGE = ROOT / 'overlay/src/pages/ProxyPoolPage.tsx'
SERVICE = ROOT / 'overlay/src/services/api/proxyPool.ts'
CUSTOMIZER = ROOT / 'apply_customizations.py'


class ProxyPoolCustomizationTest(unittest.TestCase):
    def test_page_exposes_takeover_nodes_health_and_bypass_surfaces(self) -> None:
        source = PAGE.read_text()
        self.assertIn('proxyPoolApi.activate', source)
        self.assertIn('proxyPoolApi.deactivate', source)
        self.assertIn('snapshot?.bypassCredentials', source)
        self.assertIn('proxyPoolApi.testAll', source)
        self.assertIn('Rotation occurs per TCP tunnel', source)

    def test_service_enables_plugin_before_global_proxy_takeover(self) -> None:
        source = SERVICE.read_text()
        save_start = source.index('async save(config: ProxyPoolConfig)')
        activate_start = source.index('async activate(config: ProxyPoolConfig)')
        activate_end = source.index('async deactivate(config: ProxyPoolConfig)')
        save_block = source[save_start:activate_start]
        activate_block = source[activate_start:activate_end]

        self.assertIn("document.setIn(['plugins', 'enabled'], true)", source)
        self.assertIn('pluginsApi.updateEnabled(PROXY_POOL_PLUGIN_ID, true)', save_block)
        self.assertLess(activate_block.index('this.save(nextConfig)'), activate_block.index("apiClient.put('/proxy-url'"))
        self.assertIn("apiClient.put('/proxy-url', { value: config.restoreProxyUrl.trim() })", source)

    def test_routes_and_navigation_are_durable_customizer_edits(self) -> None:
        source = CUSTOMIZER.read_text()
        self.assertIn("import { ProxyPoolPage } from '@/pages/ProxyPoolPage'", source)
        self.assertIn("path: '/proxy-pool'", source)
        self.assertIn('IconSidebarProxyPool', source)
        self.assertIn('PROXY_POOL_NAV_LOCALE_KEYS', source)


if __name__ == '__main__':
    unittest.main()
