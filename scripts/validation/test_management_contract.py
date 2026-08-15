import importlib.util
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = ROOT / "cliproxyapi-pro-core/patches/management_contract.py"
SPEC = importlib.util.spec_from_file_location("management_contract", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ManagementContractTests(unittest.TestCase):
    def write_constants(self, source: str) -> Path:
        temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(temp_dir.cleanup)
        temp_root = Path(temp_dir.name)
        path = temp_root / "src/utils/quota/constants.ts"
        path.parent.mkdir(parents=True)
        path.write_text(source, encoding="utf-8")
        return temp_root

    def test_resolves_the_auth_card_user_agent(self):
        root = self.write_constants(
            """
export const ANTIGRAVITY_CLI_VERSION = '9.8.7';
export const ANTIGRAVITY_CLIENT_NAME = 'next_client';
export const ANTIGRAVITY_CLIENT_PLATFORM = {
  osType: 'linux',
  arch: 'amd64',
} as const;
export const buildAntigravityUserAgent = ({
  version = ANTIGRAVITY_CLI_VERSION,
  clientName = ANTIGRAVITY_CLIENT_NAME,
  osType = ANTIGRAVITY_CLIENT_PLATFORM.osType,
  arch = ANTIGRAVITY_CLIENT_PLATFORM.arch,
} = {}) =>
  `antigravity/cli/${version} (${clientName}; os_type=${osType}; arch=${arch})`;
export const ANTIGRAVITY_USER_AGENT = buildAntigravityUserAgent();
export const ANTIGRAVITY_REQUEST_HEADERS = {
  'User-Agent': ANTIGRAVITY_USER_AGENT,
};
"""
        )
        self.assertEqual(
            "antigravity/cli/9.8.7 (next_client; os_type=linux; arch=amd64)",
            MODULE.antigravity_user_agent(root),
        )

    def test_accepts_a_future_direct_user_agent_constant(self):
        root = self.write_constants(
            """
export const ANTIGRAVITY_USER_AGENT = 'antigravity/next/3.0';
export const ANTIGRAVITY_REQUEST_HEADERS = {
  'User-Agent': ANTIGRAVITY_USER_AGENT,
};
"""
        )
        self.assertEqual("antigravity/next/3.0", MODULE.antigravity_user_agent(root))

    def test_rejects_a_header_that_no_longer_uses_the_shared_constant(self):
        root = self.write_constants(
            """
export const ANTIGRAVITY_USER_AGENT = 'antigravity/next/3.0';
export const ANTIGRAVITY_REQUEST_HEADERS = {
  'User-Agent': 'different-agent',
};
"""
        )
        with self.assertRaisesRegex(ValueError, "request headers"):
            MODULE.antigravity_user_agent(root)


if __name__ == "__main__":
    unittest.main()
