import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
REPO_ROOT = ROOT.parent
CUSTOMIZER = ROOT / 'apply_customizations.py'
PLUGIN_MAIN = REPO_ROOT / 'cliproxyapi-pro-plugins/oauth-model-policy/main.go'
PLUGIN_PAGE = REPO_ROOT / 'cliproxyapi-pro-plugins/oauth-model-policy/web/index.html'


class OAuthModelPolicyCustomizationTest(unittest.TestCase):
    def test_oauth_model_policy_ui_is_plugin_owned(self) -> None:
        self.assertFalse((ROOT / 'overlay/src/pages/OAuthModelPolicyPage.tsx').exists())
        self.assertFalse((ROOT / 'overlay/src/pages/OAuthModelPolicyPage.module.scss').exists())
        self.assertFalse((ROOT / 'overlay/src/services/api/oauthModelPolicy.ts').exists())

        customizer = CUSTOMIZER.read_text()
        self.assertNotIn('OAuthModelPolicyPage', customizer)
        self.assertNotIn("path: '/oauth-model-policy'", customizer)
        self.assertNotIn('oauthModelPolicy:', customizer)

        plugin = PLUGIN_MAIN.read_text()
        self.assertIn('ManagementAPI: true', plugin)
        self.assertIn('pluginabi.MethodManagementRegister', plugin)
        self.assertIn('oauthModelPolicyManagementPage', plugin)

    def test_plugin_page_preserves_complete_policy_editor_contract(self) -> None:
        page = PLUGIN_PAGE.read_text()
        for provider in ('xai', 'codex', 'claude', 'gemini-cli', 'antigravity', 'kimi'):
            self.assertIn(provider, page)
        for marker in (
            "'_unknown'",
            "'_default'",
            "'excluded-models'",
            "'cache-ttl'",
            "'resolve-timeout'",
            'cliproxy-plugin-resource',
            "request('PATCH','/plugins/'+PLUGIN_ID+'/config',config)",
            '添加自定义套餐',
            '空列表表示明确不排除',
        ):
            self.assertIn(marker, page)
        self.assertIn('@media(max-width:520px)', page)


if __name__ == '__main__':
    unittest.main()
