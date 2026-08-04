import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


class QuotaCardCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_adds_cached_time_to_quota_renderers(self) -> None:
        source = MODULE_PATH.read_text()
        self.assertIn('<QuotaCachedTime quotaStatus={status} cachedAt={quota.cachedAt} />', source)
        self.assertIn('<QuotaCachedTime quotaStatus={quotaStatus} cachedAt={quota.cachedAt} />', source)
        self.assertIn("className={styles.selectCheckbox}", source)
        self.assertIn("selection.deleteOne(entry.file.name)", source)


if __name__ == '__main__':
    unittest.main()
