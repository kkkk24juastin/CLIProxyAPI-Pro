import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PATCHES = ROOT / 'cliproxyapi-pro-core/patches'
PRO = PATCHES / 'sources/internal/pro'
PRO_IMPORT_PATTERN = re.compile(
    r'github\.com/router-for-me/CLIProxyAPI/v\d+/internal/pro/([^/"\s]+)'
)


class CoreModuleBoundaryTests(unittest.TestCase):
    def test_foundation_modules_remain_leaf_layers(self):
        allowed_dependencies = {
            'settings': set(),
            'state': set(),
            'storage': set(),
            'quota': set(),
            'routing': set(),
            'inspection': set(),
            'backup': {'state'},
        }
        violations = []
        for owner, allowed in allowed_dependencies.items():
            for path in sorted((PRO / owner).rglob('*.go')):
                if path.name.endswith('_test.go'):
                    continue
                source = path.read_text(encoding='utf-8')
                for dependency in PRO_IMPORT_PATTERN.findall(source):
                    if dependency != owner and dependency not in allowed:
                        violations.append(
                            f'{path.relative_to(ROOT)} imports internal/pro/{dependency}'
                        )
        self.assertEqual([], violations, '\n'.join(violations))

    def test_management_feature_files_use_explicit_host_adapters(self):
        inspection = (PATCHES / 'account_inspection_scheduler.go').read_text(encoding='utf-8')
        routing = (PATCHES / 'routing_policy.go').read_text(encoding='utf-8')
        plugin_quota = (PATCHES / 'plugin_quota_management.go').read_text(encoding='utf-8')
        auth_adapter = (PATCHES / 'pro_auth_mutation.go').read_text(encoding='utf-8')
        runtime = (PATCHES / 'pro_management_runtime.go').read_text(encoding='utf-8')

        self.assertNotIn('startRoutingPolicyController(', inspection)
        self.assertNotIn('stopRoutingPolicyController(', inspection)
        self.assertNotIn('accountInspectionQuotaAdapter', inspection)
        self.assertNotIn('internal/pro/inspection', plugin_quota)
        self.assertNotIn('accountInspectionQuotaAdapter', plugin_quota)

        for declaration in (
            'func setProAuthDisabledState(',
            'func (h *Handler) updateProAuth(',
            'func stringFromAny(',
            'func clearRoutingProtectionOwnership(',
        ):
            self.assertIn(declaration, auth_adapter)
            self.assertNotIn(declaration, inspection)
            self.assertNotIn(declaration, routing)

        self.assertIn('h.startAccountInspectionScheduler(', runtime)
        self.assertIn('startRoutingPolicyController(h)', runtime)
        self.assertIn('stopRoutingPolicyController(h)', runtime)

        for alias in (
            'type accountInspectionResult = proinspection.Result',
            'type accountInspectionSummary = proinspection.Summary',
            'type accountInspectionHealthCounts = proinspection.HealthCounts',
            'type accountInspectionPageInfo = proinspection.PageInfo',
            'type accountInspectionResultSnapshot = proinspection.ResultSnapshot',
            'type accountInspectionRunState = proinspection.RunState',
            'type accountInspectionLogEntry = proinspection.LogEntry',
            'type accountInspectionProgress = proinspection.Progress',
            'type accountInspectionStatus = proinspection.Status',
            'type accountInspectionLogStreamMessage = proinspection.LogStreamMessage',
            'type accountInspectionActionItem = proinspection.ActionItem',
            'type accountInspectionActionOutcome = proinspection.ActionOutcome',
        ):
            self.assertIn(alias, inspection)

        for declaration in (
            'type accountInspectionHTTPResult struct {',
            'Header     http.Header',
            'func (r accountInspectionHTTPResult) probeResponse() proinspection.ProbeResponse {',
        ):
            self.assertIn(declaration, inspection)

        results_module = (PRO / 'inspection/results.go').read_text(encoding='utf-8')
        for declaration in (
            'func HealthBucketOf(',
            'func PaginateResults(',
            'func AutoActionForResult(',
            'func SummarizeResults(',
        ):
            self.assertIn(declaration, results_module)

        providers_module = (PRO / 'inspection/providers.go').read_text(encoding='utf-8')
        for declaration in (
            'func BuildAntigravityGroups(',
            'func BuildClaudeWindows(',
            'func BuildCodexWindows(',
            'func BuildKimiRows(',
        ):
            self.assertIn(declaration, providers_module)

        probes_module = (PRO / 'inspection/probes.go').read_text(encoding='utf-8')
        self.assertNotIn('http.Header', probes_module)
        for declaration in (
            'type ProbeResponse struct',
            'func ShouldDeepProbe(',
            'func BuildAntigravityDeepProbeBody(',
            'func ClassifyAntigravityDeepProbeResponse(',
            'func BuildXAIDeepProbeBody(',
            'func ClassifyXAIDeepProbeResponse(',
            'func RunXAIDeepProbeWithRetry(',
            'func SummarizeHTTPBody(',
        ):
            self.assertIn(declaration, probes_module)

        for delegation in (
            'return proinspection.BuildAntigravityDeepProbeBody(',
            'return proinspection.ClassifyAntigravityDeepProbeResponse(',
            'return proinspection.BuildXAIDeepProbeBody(',
            'return proinspection.ClassifyXAIDeepProbeResponse(',
        ):
            self.assertIn(delegation, inspection)
        self.assertIn('proinspection.RunXAIDeepProbeWithRetry(', inspection)

        actions_module = (PRO / 'inspection/actions.go').read_text(encoding='utf-8')
        for declaration in (
            'type ActionItem struct',
            'type ActionOutcome struct',
            'func AccountKey(',
            'func ActionItemFromResult(',
            'func DedupeActionItems(',
            'func SummarizeActionOutcomes(',
            'func MergeManualActionResult(',
        ):
            self.assertIn(declaration, actions_module)
        for delegation in (
            'return proinspection.AccountKey(',
            'return proinspection.ActionItemFromResult(',
            'return proinspection.DedupeActionItems(',
            'return proinspection.SummarizeActionOutcomes(',
            'return proinspection.MergeManualActionResult(',
        ):
            self.assertIn(delegation, inspection)

        selection_module = (PRO / 'inspection/selection.go').read_text(encoding='utf-8')
        for declaration in (
            'func ShouldInspectCandidate(',
            'func Sample[',
            'func ProviderLimiters(',
        ):
            self.assertIn(declaration, selection_module)

        policy_module = (PRO / 'inspection/policy.go').read_text(encoding='utf-8')
        for declaration in (
            'func CodexDecision(',
            'func ErrorCode(',
            'func DecisionErrorCode(',
        ):
            self.assertIn(declaration, policy_module)

        for delegation in (
            'return proinspection.ShouldInspectCandidate(',
            'return proinspection.Sample(',
            'return proinspection.ProviderLimiters(',
            'return proinspection.CodexDecision(',
            'return proinspection.ErrorCode(',
            'return proinspection.DecisionErrorCode(',
        ):
            self.assertIn(delegation, inspection)

        snapshot_module = (PRO / 'inspection/snapshot.go').read_text(encoding='utf-8')
        for declaration in (
            'type RunState string',
            'type ResultSnapshot struct',
            'func NormalizeSnapshotState(',
            'func DecodeResultSnapshot(',
        ):
            self.assertIn(declaration, snapshot_module)
        self.assertIn('return proinspection.DecodeResultSnapshot(', inspection)

        status_module = (PRO / 'inspection/status.go').read_text(encoding='utf-8')
        for declaration in (
            'type LogEntry struct',
            'type Status struct',
            'type LogStreamMessage struct',
            'func PaginateLogs(',
            'func ProjectStatus(',
            'func MergeTokenRefreshResult(',
            'func MergeReinspectionResult(',
        ):
            self.assertIn(declaration, status_module)
        for delegation in (
            'return proinspection.PaginateLogs(',
            'return proinspection.ProjectStatus(',
            'return proinspection.MergeTokenRefreshResult(',
            'return proinspection.MergeReinspectionResult(',
        ):
            self.assertIn(delegation, inspection)

        xai_billing = (PRO / 'quota/xai_billing.go').read_text(encoding='utf-8')
        for declaration in (
            'func BuildXAIBillingSummary(',
            'func MergeXAIBillingSummaries(',
            'func XAISummaryUsedPercent(',
            'func XAIPlanTypeFromBillingBody(',
            'func XAIPaidHealthSummary(',
        ):
            self.assertIn(declaration, xai_billing)

        quota_snapshot = (PRO / 'quota/snapshot.go').read_text(encoding='utf-8')
        self.assertIn('func SnapshotMaxUsedPercent(', quota_snapshot)

        quota_cache = (PRO / 'quota/cache.go').read_text(encoding='utf-8')
        for declaration in (
            'func SuccessCacheState(',
            'func JSONShapeHash(',
            'func JSONShapeHashForBodies(',
        ):
            self.assertIn(declaration, quota_cache)

        for delegation in (
            'return proquota.SnapshotMaxUsedPercent(',
            'return proquota.SuccessCacheState(',
            'return proquota.JSONShapeHash(',
            'return proquota.JSONShapeHashForBodies(',
            'return proinspection.XAIOfficialAPIQuotaDecision(',
        ):
            self.assertIn(delegation, inspection)


if __name__ == '__main__':
    unittest.main()
