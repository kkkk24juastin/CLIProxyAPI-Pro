import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ROOT.parent
CUSTOMIZER = ROOT / 'apply_customizations.py'
PLUGIN_BRIDGE = ROOT / 'overlay/src/features/plugins/usePluginResourceBridge.ts'
PLUGIN_MAIN = REPO_ROOT / 'cliproxyapi-pro-plugins/proxy-pool/main.go'
PLUGIN_PAGE = REPO_ROOT / 'cliproxyapi-pro-plugins/proxy-pool/web/index.html'


class ProxyPoolCustomizationTest(unittest.TestCase):
    def test_proxy_pool_ui_is_owned_only_by_plugin_resource(self) -> None:
        self.assertFalse((ROOT / 'overlay/src/pages/ProxyPoolPage.tsx').exists())
        self.assertFalse((ROOT / 'overlay/src/services/api/proxyPool.ts').exists())
        self.assertFalse(any((ROOT / 'overlay/src/features/proxyPool').glob('*')))

        customizer = CUSTOMIZER.read_text()
        self.assertNotIn('ProxyPoolPage', customizer)
        self.assertNotIn("path: '/proxy-pool'", customizer)
        self.assertNotIn('IconSidebarProxyPool', customizer)

        plugin = PLUGIN_MAIN.read_text()
        page = PLUGIN_PAGE.read_text()
        self.assertIn('pluginabi.MethodManagementRegister', plugin)
        self.assertIn('proxyPoolManagementPage', plugin)
        self.assertIn('cliproxy-plugin-resource', page)
        self.assertIn("/plugins/'+PLUGIN_ID+'/config", page)
        self.assertIn('/pro/proxy-pool/status', page)

    def test_plugin_resource_bridge_proxies_authenticated_management_requests(self) -> None:
        bridge = PLUGIN_BRIDGE.read_text()
        customizer = CUSTOMIZER.read_text()
        self.assertIn("REQUEST_SOURCE = 'cliproxy-plugin-resource'", bridge)
        self.assertIn('event.source !== frameWindow', bridge)
        self.assertIn("path.startsWith('//')", bridge)
        self.assertIn('apiClient.patch(path, request.body, config)', bridge)
        self.assertIn('patch_plugin_resource_bridge(target)', customizer)
        self.assertIn('ref={frameRef}', customizer)


if __name__ == '__main__':
    unittest.main()
