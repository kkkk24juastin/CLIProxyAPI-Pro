#!/usr/bin/env python3
import argparse
import json
import time
import urllib.error
import urllib.request


def request(base: str, management_key: str, method: str, path: str, body=None):
    payload = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=payload, method=method)
    req.add_header("Authorization", f"Bearer {management_key}")
    if payload is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            data = response.read()
            return response.status, json.loads(data) if data else None
    except urllib.error.HTTPError as error:
        data = error.read()
        return error.code, json.loads(data) if data else None


def wait_for_bindings(base: str, management_key: str, predicate, label: str):
    deadline = time.monotonic() + 10
    latest = None
    while time.monotonic() < deadline:
        status, latest = request(base, management_key, "GET", "/v0/management/api-key-policy-bindings")
        if status == 200 and predicate(latest):
            return latest
        time.sleep(0.05)
    raise AssertionError((label, latest))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base", required=True)
    parser.add_argument("--management-key", required=True)
    args = parser.parse_args()
    base = args.base.rstrip("/")

    status, capabilities = request(base, args.management_key, "GET", "/v0/management/api-key-policy-capabilities")
    assert status == 200, (status, capabilities)
    assert "policy_delete_preview" in capabilities["features"], capabilities
    assert "orphaned_purge_guard" in capabilities["features"], capabilities

    status, api_keys = request(base, args.management_key, "GET", "/v0/management/api-keys")
    assert status == 200 and api_keys["api-keys"], (status, api_keys)
    raw_key = api_keys["api-keys"][0]

    status, bindings = request(base, args.management_key, "GET", "/v0/management/api-key-policy-bindings")
    assert status == 200, (status, bindings)
    assert len(bindings["items"]) == 1, bindings
    assert bindings["configGeneration"] >= 1, bindings
    key_ref = bindings["items"][0]["keyRef"]

    status, catalog = request(base, args.management_key, "GET", "/v0/management/api-key-policy-catalog")
    assert status == 200 and catalog["providers"] and catalog["models"], (status, catalog)
    profile = {
        "name": "Smoke",
        "providers": [catalog["providers"][0], catalog["providers"][0]],
        "models": [catalog["models"][0]],
        "mappings": [],
    }
    status, policy = request(base, args.management_key, "POST", "/v0/management/api-key-policies", {
        "keyRef": key_ref,
        "displayName": "Runtime smoke",
        "initialProfile": profile,
    })
    assert status == 201, (status, policy)
    assert policy["state"] == "configured", policy
    assert policy["profiles"][0]["providers"] == [catalog["providers"][0]], policy

    policy_path = "/v0/management/api-key-policies/" + policy["id"]
    purge_path = "/v0/management/orphaned-api-key-policies/" + policy["id"]
    status, preview = request(base, args.management_key, "GET", policy_path + "/delete-preview")
    assert status == 200, (status, preview)
    assert preview["version"] == policy["version"], preview
    assert preview["targetPolicyMode"] == "passthrough", preview
    assert preview["requiresConfirmation"] == "RESTORE_UNRESTRICTED_PASSTHROUGH", preview

    configured_generation = bindings["configGeneration"]
    status, response = request(base, args.management_key, "PUT", "/v0/management/api-keys", [])
    assert status == 200, (status, response)
    orphaned = wait_for_bindings(
        base,
        args.management_key,
        lambda value: len(value["items"]) == 0 and len(value["orphaned"]) == 1,
        "policy did not become orphaned after removing the upstream key",
    )
    assert orphaned["configGeneration"] > configured_generation, orphaned

    status, error = request(base, args.management_key, "DELETE", purge_path, {
        "version": policy["version"], "configGeneration": configured_generation,
    })
    assert status == 409 and error["error"]["code"] == "api_key_policy_config_changed", (status, error)

    status, response = request(base, args.management_key, "PUT", "/v0/management/api-keys", [raw_key])
    assert status == 200, (status, response)
    restored = wait_for_bindings(
        base,
        args.management_key,
        lambda value: len(value["items"]) == 1 and value["items"][0]["state"] == "configured",
        "policy did not become configured after restoring the upstream key",
    )
    status, error = request(base, args.management_key, "DELETE", purge_path, {
        "version": policy["version"], "configGeneration": restored["configGeneration"],
    })
    assert status == 409 and error["error"]["code"] == "api_key_policy_not_orphaned", (status, error)
    status, retained = request(base, args.management_key, "GET", policy_path)
    assert status == 200 and retained["state"] == "configured", (status, retained)

    status, response = request(base, args.management_key, "PUT", "/v0/management/api-keys", [])
    assert status == 200, (status, response)
    fresh_orphaned = wait_for_bindings(
        base,
        args.management_key,
        lambda value: len(value["items"]) == 0 and len(value["orphaned"]) == 1,
        "policy did not become orphaned after the second upstream key removal",
    )
    status, response = request(base, args.management_key, "DELETE", purge_path, {
        "version": policy["version"], "configGeneration": fresh_orphaned["configGeneration"],
    })
    assert status == 204 and response is None, (status, response)

    status, response = request(base, args.management_key, "PUT", "/v0/management/api-keys", [raw_key])
    assert status == 200, (status, response)
    unconfigured = wait_for_bindings(
        base,
        args.management_key,
        lambda value: len(value["items"]) == 1 and value["items"][0]["state"] == "unconfigured",
        "purged policy returned after restoring the upstream key",
    )

    status, policy = request(base, args.management_key, "POST", "/v0/management/api-key-policies", {
        "keyRef": unconfigured["items"][0]["keyRef"],
        "displayName": "Runtime smoke delete",
        "initialProfile": profile,
    })
    assert status == 201, (status, policy)
    policy_path = "/v0/management/api-key-policies/" + policy["id"]

    status, error = request(base, args.management_key, "DELETE", policy_path, {
        "version": policy["version"], "confirmPassthrough": "WRONG",
    })
    assert status == 409 and error["error"]["code"] == "policy_delete_requires_passthrough_confirmation", (status, error)
    status, response = request(base, args.management_key, "DELETE", policy_path, {
        "version": policy["version"], "confirmPassthrough": "RESTORE_UNRESTRICTED_PASSTHROUGH",
    })
    assert status == 204 and response is None, (status, response)

    status, final_bindings = request(base, args.management_key, "GET", "/v0/management/api-key-policy-bindings")
    assert status == 200 and len(final_bindings["items"]) == 1, (status, final_bindings)
    assert final_bindings["items"][0]["state"] == "unconfigured", final_bindings
    print(json.dumps({
        "apiVersion": capabilities["apiVersion"],
        "configGeneration": final_bindings["configGeneration"],
        "bindingCount": len(final_bindings["items"]),
        "deletePreview": True,
        "orphanPurgeGuard": True,
        "restoredState": final_bindings["items"][0]["state"],
    }, sort_keys=True))


if __name__ == "__main__":
    main()
