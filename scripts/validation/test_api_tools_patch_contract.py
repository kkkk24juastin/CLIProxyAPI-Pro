import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PATCHER = ROOT / 'cliproxyapi-pro-core/patches/apply_upstream_patches.py'


class APIToolsPatchContractTests(unittest.TestCase):
    def test_executor_patch_preserves_upstream_request_proxy_transport(self) -> None:
        source = PATCHER.read_text(encoding='utf-8')

        self.assertIn("api_call_transport_args = (", source)
        self.assertIn("'auth, requestProxyURL'", source)
        self.assertIn("'h.apiCallTransport(auth, requestProxyURL)' in read(api_tools)", source)
        self.assertIn(
            "h.apiCallTransport(__API_CALL_TRANSPORT_ARGS__)",
            source,
        )
        self.assertNotIn('h.apiCallTransport(auth)\n', source)

    def test_account_inspection_transport_adapts_to_proxy_aware_helpers(self) -> None:
        source = PATCHER.read_text(encoding='utf-8')

        self.assertIn(
            "'func (h *Handler) apiCallTransport(auth *coreauth.Auth, requestProxyURL string)' in read(api_tools)",
            source,
        )
        self.assertIn("'s.h.resolveTokenForAuth(reqCtx, auth, \"\")'", source)
        self.assertIn("'s.h.apiCallTransport(auth, \"\")'", source)

    def test_executor_request_proxy_uses_request_scoped_auth_clone(self) -> None:
        source = PATCHER.read_text(encoding='utf-8')

        self.assertIn(
            'func requestScopedExecutorAuth(auth *coreauth.Auth, requestProxyURL string)',
            source,
        )
        self.assertIn('requestAuth := auth.Clone()', source)
        self.assertIn('requestAuth.ProxyURL = strings.TrimSpace(requestProxyURL)', source)
        self.assertIn(
            "executor_auth_args = 'requestScopedExecutorAuth(auth, requestProxyURL)'",
            source,
        )
        self.assertIn('api_tools_executor_proxy_test', source)


if __name__ == '__main__':
    unittest.main()
