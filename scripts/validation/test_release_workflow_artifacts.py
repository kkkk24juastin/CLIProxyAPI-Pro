import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github" / "workflows"


class ReleaseWorkflowArtifactTests(unittest.TestCase):
    def test_core_release_reuses_validated_management_asset(self) -> None:
        workflow = (WORKFLOWS / "release-core.yml").read_text()

        self.assertNotIn("  build-management-html:\n", workflow)
        self.assertNotIn("run: bun run build", workflow)
        self.assertEqual(
            1,
            workflow.count(
                "run: bash customizations-repo/scripts/validation/management.sh "
                "upstream-management"
            ),
        )
        self.assertIn("- name: Upload validated management asset", workflow)
        self.assertIn("path: upstream-management/dist/management.html", workflow)
        self.assertEqual(3, workflow.count("name: management-release-asset"))

    def test_management_release_reuses_validated_management_asset(self) -> None:
        workflow = (WORKFLOWS / "release-management.yml").read_text()

        self.assertNotIn("run: bun run build", workflow)
        self.assertEqual(
            1,
            workflow.count(
                "run: bash customizations-repo/scripts/validation/management.sh upstream"
            ),
        )
        self.assertIn("- name: Upload validated management asset", workflow)
        self.assertIn("- name: Download validated management asset", workflow)
        self.assertIn("  publish-management-asset:\n", workflow)
        self.assertNotIn("  build-and-release:\n", workflow)
        self.assertIn("path: upstream/dist/management.html", workflow)
        self.assertIn(
            "release-assets/management/management.html", workflow
        )
        self.assertEqual(2, workflow.count("name: management-release-asset"))


if __name__ == "__main__":
    unittest.main()
