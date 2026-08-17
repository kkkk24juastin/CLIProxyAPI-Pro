import os
from pathlib import Path
import re
import signal
import subprocess
import tempfile
import time
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
ENTRYPOINT = REPO_ROOT / "cliproxyapi-pro-core" / "entrypoint.sh"


class EntrypointLifecycleTests(unittest.TestCase):
    def test_term_is_forwarded_to_main_process(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            ready_path = root / "ready"
            term_path = root / "term"
            main_path = root / "main.sh"
            main_path.write_text(
                "#!/bin/sh\n"
                f"trap 'printf received > {term_path}; exit 0' TERM\n"
                f"printf ready > {ready_path}\n"
                "while :; do sleep 1; done\n"
            )
            main_path.chmod(0o755)

            entrypoint_path = root / "entrypoint.sh"
            entrypoint_path.write_text(
                ENTRYPOINT.read_text().replace("/CLIProxyAPI/CLIProxyAPI", str(main_path))
            )
            entrypoint_path.chmod(0o755)

            environment = os.environ.copy()
            for name in (
                "KOMARI_SERVER",
                "KOMARI_SECRET",
                "WEBDAV_URL",
                "WEBDAV_USERNAME",
                "WEBDAV_PASSWORD",
                "CLIPROXY_BACKUP_WEBDAV_URL",
                "CLIPROXY_BACKUP_WEBDAV_USERNAME",
                "CLIPROXY_BACKUP_WEBDAV_PASSWORD",
                "MANAGEMENT_PASSWORD",
            ):
                environment.pop(name, None)

            process = subprocess.Popen(
                ["/bin/sh", str(entrypoint_path)],
                env=environment,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                start_new_session=True,
            )
            try:
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and not ready_path.exists():
                    if process.poll() is not None:
                        self.fail(f"entrypoint exited before main became ready: {process.stdout.read()}")
                    time.sleep(0.02)
                self.assertTrue(ready_path.exists(), "fake main process did not start")

                process.send_signal(signal.SIGTERM)
                output, _ = process.communicate(timeout=5)
                self.assertEqual(process.returncode, 0, output)
                self.assertEqual(term_path.read_text(), "received")
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)

    def test_docker_restore_uses_the_data_management_pipeline(self) -> None:
        entrypoint = ENTRYPOINT.read_text()

        self.assertIn(
            "/v0/management/data/backups/restore", entrypoint
        )
        self.assertIn("/v0/management/data/overview", entrypoint)
        self.assertNotIn("/v0/management/usage/data/", entrypoint)
        self.assertIn("cliproxy-pro-backup-[0-9_]+", entrypoint)
        self.assertIn("usage-export-[0-9_]+", entrypoint)
        legacy_pattern = next(
            pattern
            for pattern in re.findall(r"grep -oE '([^']+)'", entrypoint)
            if pattern.startswith("usage-export-")
        )
        for file_name in (
            "usage-export-20260813_120000.jsonl",
            "usage-export-20260813_120000.json",
        ):
            result = subprocess.run(
                ["grep", "-oE", legacy_pattern],
                input=f"<d:href>/backups/{file_name}</d:href>",
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.strip(), file_name)
        self.assertNotIn("/v0/management/usage/import", entrypoint)
        self.assertIn("CLIPROXY_BACKUP_WEBDAV_URL", entrypoint)

        for dockerfile_name in ("Dockerfile", "Dockerfile.runtime"):
            dockerfile = (
                REPO_ROOT / "cliproxyapi-pro-core" / dockerfile_name
            ).read_text()
            self.assertIn(
                "COPY cliproxyapi-pro-core/entrypoint.sh /CLIProxyAPI/entrypoint.sh"
                if dockerfile_name == "Dockerfile"
                else "COPY entrypoint.sh /CLIProxyAPI/entrypoint.sh",
                dockerfile,
            )


if __name__ == "__main__":
    unittest.main()
