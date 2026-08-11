import importlib.util
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


BUILDERS_SOURCE = """
function getAntigravityWindowOrder(bucket: AntigravityQuotaBucket): number {
  return 0;
}

export function buildAntigravityGroups() {
      const label = normalizeStringValue(group.name) ?? `Quota Group ${groupIndex + 1}`;
      const groupId = toStableId(label, `quota-group-${groupIndex + 1}`);
      const buckets = Array.isArray(group.buckets) ? group.buckets : [];
      return {
        description: normalizeStringValue(group.description) ?? undefined,
      };
}

export function mergeXaiBillingSummaries(primary, fallback) {
  return {
    productUsage: primary.productUsage.length > 0 ? primary.productUsage : fallback.productUsage,
    monthlyLimitCents: primary.monthlyLimitCents ?? fallback.monthlyLimitCents,
    usedCents: primary.usedCents ?? fallback.usedCents,
    includedUsedCents: primary.includedUsedCents ?? fallback.includedUsedCents,
    onDemandCapCents: primary.onDemandCapCents ?? fallback.onDemandCapCents,
    onDemandUsedCents: primary.onDemandUsedCents ?? fallback.onDemandUsedCents,
    onDemandUsedPercent: primary.onDemandUsedPercent ?? fallback.onDemandUsedPercent,
    billingPeriodStart: primary.billingPeriodStart ?? fallback.billingPeriodStart,
    billingPeriodEnd: primary.billingPeriodEnd ?? fallback.billingPeriodEnd,
    usedPercent: primary.usedPercent ?? fallback.usedPercent,
  };
}
"""


class XAIBillingMergeCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_monthly_endpoint_owns_monthly_billing_fields(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            path = target / 'src/utils/quota/builders.ts'
            path.parent.mkdir(parents=True)
            path.write_text(BUILDERS_SOURCE)

            CUSTOMIZATIONS.patch_antigravity_quota_builders(target)
            CUSTOMIZATIONS.flush_writes()

            source = path.read_text()
            self.assertIn(
                'monthlyLimitCents: fallback.monthlyLimitCents ?? primary.monthlyLimitCents',
                source,
            )
            self.assertIn('usedCents: fallback.usedCents ?? primary.usedCents', source)
            self.assertIn(
                'onDemandCapCents: fallback.onDemandCapCents ?? primary.onDemandCapCents',
                source,
            )
            self.assertIn(
                'billingPeriodEnd: fallback.billingPeriodEnd ?? primary.billingPeriodEnd',
                source,
            )

            CUSTOMIZATIONS.patch_antigravity_quota_builders(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(source, path.read_text())


if __name__ == '__main__':
    unittest.main()
