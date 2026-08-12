import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github" / "workflows"


class ReleaseWorkflowArtifactTests(unittest.TestCase):
    def test_core_release_validates_the_pinned_release_models_data(self) -> None:
        workflow = (WORKFLOWS / "release-core.yml").read_text()
        core_validation = (ROOT / "scripts" / "validation" / "core.sh").read_text()

        validate_job_start = workflow.index("  validate-core:\n")
        validate_job_end = workflow.index("  validate-management:\n")
        validate_job = workflow[validate_job_start:validate_job_end]
        build_job_start = workflow.index("  build-core-binaries:\n")
        build_job_end = workflow.index("  assemble-core-assets:\n")
        build_job = workflow[build_job_start:build_job_end]

        self.assertIn("repository: router-for-me/models", validate_job)
        self.assertIn(
            "ref: ${{ needs.check-version.outputs.models_sha }}", validate_job
        )
        self.assertIn("path: upstream-models", validate_job)
        self.assertIn(
            'test "$(git -C upstream-models rev-parse HEAD)" = "${MODELS_SHA}"',
            validate_job,
        )
        self.assertIn("upstream-models/models.json", validate_job)

        validation_models_install = core_validation.index(
            'cp "${release_models_file}" '
            '"${upstream_root}/internal/registry/models/models.json"'
        )
        validation_customization_apply = core_validation.index(
            'python3 "${repo_root}/cliproxyapi-pro-core/patches/'
            'apply_upstream_patches.py"'
        )
        self.assertLess(validation_models_install, validation_customization_apply)

        models_install = build_job.index(
            "git -C upstream-core show FETCH_HEAD:models.json > "
            "upstream-core/internal/registry/models/models.json"
        )
        customization_apply = build_job.index(
            "python customizations-repo/cliproxyapi-pro-core/patches/"
            "apply_upstream_patches.py"
        )
        self.assertLess(models_install, customization_apply)

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
