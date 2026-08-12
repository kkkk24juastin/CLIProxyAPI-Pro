import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PRO_ROOT = ROOT / 'overlay/src/pro/modules'
SHARED_ROOT = ROOT / 'overlay/src/pro/shared'
LOCALES = ROOT / 'monitoring-locales.json'


class PolicyPageConsistencyCustomizationTest(unittest.TestCase):
    def test_optional_features_use_header_activation_actions(self) -> None:
        routing = (PRO_ROOT / 'routing/RoutingPolicyPage.tsx').read_text()
        account = (PRO_ROOT / 'oauthPolicy/OAuthPolicyPage.tsx').read_text()
        proxy_header = (PRO_ROOT / 'proxyPool/features/ProxyPoolHeader.tsx').read_text()

        self.assertIn('<ProFeatureHeader', routing)
        self.assertIn('onToggle={handleEnabledChange}', routing)
        self.assertNotIn("setProtection('enabled'", routing)
        self.assertIn('<ProFeatureHeader', account)
        self.assertIn('onToggle={toggleEnabled}', account)
        self.assertNotIn('className={styles.enabledField}', account)
        self.assertIn('<ProFeatureHeader', proxy_header)
        self.assertIn('onToggle={onTakeover}', proxy_header)

    def test_optional_feature_headers_share_copy_buttons_and_breakpoints(self) -> None:
        header = (SHARED_ROOT / 'ProFeatureHeader.tsx').read_text()
        styles = (SHARED_ROOT / 'ProFeatureHeader.module.scss').read_text()

        self.assertIn("variant={active ? 'danger' : 'primary'}", header)
        self.assertGreaterEqual(header.count('size="sm"'), 2)
        for key in ('active', 'inactive', 'start_takeover', 'stop_takeover'):
            self.assertIn(f'pro_feature_header.{key}', header)
        self.assertIn('@media (max-width: 720px)', styles)
        self.assertIn('grid-template-columns: repeat(2, minmax(0, 1fr));', styles)
        self.assertIn('@media (max-width: 480px)', styles)

    def test_floating_save_bars_only_render_for_modified_drafts(self) -> None:
        routing = (PRO_ROOT / 'routing/RoutingPolicyPage.tsx').read_text()
        account = (PRO_ROOT / 'oauthPolicy/OAuthPolicyPage.tsx').read_text()
        proxy_page = (PRO_ROOT / 'proxyPool/ProxyPoolPage.tsx').read_text()

        self.assertIn('isCurrentLayer && dirty', routing)
        self.assertIn('{dirty &&', account)
        self.assertIn('visible={dirty}', proxy_page)

    def test_status_overviews_keep_two_columns_on_narrow_screens(self) -> None:
        account_styles = (PRO_ROOT / 'oauthPolicy/OAuthPolicyPage.module.scss').read_text()
        proxy_styles = (PRO_ROOT / 'proxyPool/features/ProxyPool.module.scss').read_text()
        account_mobile = account_styles[account_styles.index('@media (max-width: 720px)'):]
        proxy_mobile = proxy_styles[proxy_styles.index('@media (max-width: 780px)'):]

        self.assertIn('grid-template-columns: repeat(2, minmax(0, 1fr));', account_mobile)
        self.assertIn('grid-template-columns: repeat(2, minmax(0, 1fr));', proxy_mobile)
        self.assertNotIn('.statusGrid > div {\n    border-right: 0;', account_mobile)

    def test_refresh_callbacks_preserve_modified_drafts_without_reload_loops(self) -> None:
        for relative in (
            'routing/RoutingPolicyPage.tsx',
            'oauthPolicy/OAuthPolicyPage.tsx',
            'proxyPool/ProxyPoolPage.tsx',
        ):
            source = (PRO_ROOT / relative).read_text()
            self.assertIn('dirtyRef', source)

    def test_routing_disconnect_preserves_modified_draft(self) -> None:
        routing = (PRO_ROOT / 'routing/RoutingPolicyPage.tsx').read_text()
        disconnected = routing[routing.index("if (connectionStatus !== 'connected') {"):]
        disconnected = disconnected[:disconnected.index("    try {")]

        self.assertIn('if (!dirtyRef.current) {', disconnected)
        self.assertIn('setRequestProtection(null);', disconnected)

    def test_header_and_discard_actions_cover_all_locales(self) -> None:
        locales = json.loads(LOCALES.read_text())
        routing = (PRO_ROOT / 'routing/RoutingPolicyPage.tsx').read_text()
        account = (PRO_ROOT / 'oauthPolicy/OAuthPolicyPage.tsx').read_text()
        required = {
            'routing_policy': {'discard_changes'},
            'oauth_policy': {'discard_changes'},
            'proxy_pool': {'discard_changes'},
            'pro_feature_header': {
                'active',
                'inactive',
                'start_takeover',
                'stop_takeover',
            },
        }

        for locale in ('en.json', 'ru.json', 'zh-CN.json', 'zh-TW.json'):
            for section, keys in required.items():
                self.assertTrue(keys.issubset(locales[locale][section]), f'{locale}: {section}')
            discard_labels = {
                locales[locale][section]['discard_changes']
                for section in ('routing_policy', 'oauth_policy', 'proxy_pool')
            }
            self.assertEqual(len(discard_labels), 1, f'{locale}: discard_changes')

        self.assertNotIn("t('common.disable'", routing)
        self.assertNotIn('t("common.disable"', account)
        self.assertNotIn("title={t('config_management.reload')}", routing)
        self.assertIn("routing_policy.discard_changes", routing)
        self.assertIn("oauth_policy.discard_changes", account)


if __name__ == '__main__':
    unittest.main()
