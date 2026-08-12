import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PRO_ROOT = ROOT / 'overlay/src/pro/modules'


class PolicyPageConsistencyCustomizationTest(unittest.TestCase):
    def test_optional_features_use_header_activation_actions(self) -> None:
        routing = (PRO_ROOT / 'routing/RoutingPolicyPage.tsx').read_text()
        account = (PRO_ROOT / 'oauthPolicy/OAuthPolicyPage.tsx').read_text()
        proxy_header = (PRO_ROOT / 'proxyPool/features/ProxyPoolHeader.tsx').read_text()

        self.assertIn('className={styles.headerActions}', routing)
        self.assertIn('onClick={handleEnabledChange}', routing)
        self.assertNotIn("setProtection('enabled'", routing)
        self.assertIn('className={styles.headerActions}', account)
        self.assertIn('onClick={toggleEnabled}', account)
        self.assertNotIn('className={styles.enabledField}', account)
        self.assertIn('className={styles.headerActions}', proxy_header)
        self.assertIn('onClick={onTakeover}', proxy_header)

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


if __name__ == '__main__':
    unittest.main()
