import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("check_patch_surface.py")
SPEC = importlib.util.spec_from_file_location("check_patch_surface", MODULE_PATH)
assert SPEC and SPEC.loader
CHECK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECK)


class PatchSurfaceTest(unittest.TestCase):
    def test_exact_modified_file_set_is_required(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            subprocess.run(["git", "-C", str(root), "config", "user.name", "test"], check=True)
            subprocess.run(
                ["git", "-C", str(root), "config", "user.email", "test@example.com"],
                check=True,
            )
            (root / "a.txt").write_text("a\n", encoding="utf-8")
            (root / "b.txt").write_text("b\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(root), "add", "a.txt", "b.txt"], check=True)
            subprocess.run(["git", "-C", str(root), "commit", "-qm", "base"], check=True)

            (root / "a.txt").write_text("changed\n", encoding="utf-8")
            manifest = root / "surface.txt"
            manifest.write_text("a.txt\n", encoding="utf-8")
            CHECK.verify_surface(root, manifest, set())

            manifest.write_text("b.txt\n", encoding="utf-8")
            with self.assertRaises(SystemExit) as raised:
                CHECK.verify_surface(root, manifest, set())
            self.assertIn("unexpected modified upstream files", str(raised.exception))
            self.assertIn("expected upstream files not modified", str(raised.exception))

    def test_manifest_must_be_sorted_and_unique(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            manifest = Path(temp_dir) / "surface.txt"
            manifest.write_text("b.txt\na.txt\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "must be sorted"):
                CHECK.load_manifest(manifest)

            manifest.write_text("a.txt\na.txt\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "duplicate path"):
                CHECK.load_manifest(manifest)


if __name__ == "__main__":
    unittest.main()
