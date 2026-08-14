#!/usr/bin/env python3
import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


def wait_ready(base: str, management_key: str, process: subprocess.Popen) -> None:
    request = urllib.request.Request(base + "/v0/management/api-key-policy-capabilities")
    request.add_header("Authorization", f"Bearer {management_key}")
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"server exited with status {process.returncode}")
        try:
            with urllib.request.urlopen(request, timeout=1) as response:
                if response.status == 200:
                    return
        except (urllib.error.URLError, TimeoutError):
            time.sleep(0.1)
    raise RuntimeError("server did not become ready")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    parser.add_argument("--port", required=True, type=int)
    parser.add_argument("--label", required=True)
    args = parser.parse_args()
    management_key = "akp-review-management-secret"
    raw_key = "akp-review-runtime-key"
    base = f"http://127.0.0.1:{args.port}"

    with tempfile.TemporaryDirectory(prefix=f"akp-{args.label}-") as directory:
        root = Path(directory)
        config = root / "config.yaml"
        config.write_text(
            f'''host: "127.0.0.1"
port: {args.port}
auth-dir: "{root / 'auth'}"
api-keys:
  - "{raw_key}"
  - "{raw_key}"
remote-management:
  allow-remote: false
  secret-key: "{management_key}"
  disable-control-panel: true
  disable-auto-update-panel: true
plugins:
  enabled: false
usage-statistics-enabled: false
'''
        )
        environment = os.environ.copy()
        environment.update({
            "USAGE_DB_PATH": str(root / "usage.sqlite"),
            "USAGE_SERVICE_ENABLED": "false",
        })
        process = subprocess.Popen(
            [args.binary, "-config", str(config), "-local-model"],
            env=environment,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        try:
            wait_ready(base, management_key, process)
            smoke = Path(__file__).with_name("api_key_policy_runtime_smoke.py")
            subprocess.run(
                [sys.executable, str(smoke), "--base", base, "--management-key", management_key],
                check=True,
            )
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
            output = process.stdout.read() if process.stdout else ""
            if process.returncode not in (0, -15):
                print(output, file=sys.stderr)
                raise SystemExit(process.returncode)
        print(f"{args.label}: ok")


if __name__ == "__main__":
    main()
