#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -ne 1 ]]; then
  echo "Usage: $0 /path/to/Cli-Proxy-API-Management-Center" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
upstream_root="$(cd "$1" && pwd)"

if [[ ! -f "${upstream_root}/package.json" || ! -d "${upstream_root}/src" ]]; then
  echo "Management upstream checkout is invalid: ${upstream_root}" >&2
  exit 1
fi

if [[ -d "${upstream_root}/.git" ]] && [[ -n "$(git -C "${upstream_root}" status --porcelain)" ]]; then
  echo "Management upstream checkout must be clean before validation: ${upstream_root}" >&2
  exit 1
fi

observability_webapp="${repo_root}/cliproxyapi-pro-plugins/pro-observability/webapp"
observability_page="${repo_root}/cliproxyapi-pro-plugins/pro-observability/web/index.html"
proxy_pool_page="${repo_root}/cliproxyapi-pro-plugins/proxy-pool/web/index.html"
oauth_model_policy_page="${repo_root}/cliproxyapi-pro-plugins/oauth-model-policy/web/index.html"
observability_page_hash="$(git -C "${repo_root}" hash-object "${observability_page}")"
proxy_pool_page_hash="$(git -C "${repo_root}" hash-object "${proxy_pool_page}")"
oauth_model_policy_page_hash="$(git -C "${repo_root}" hash-object "${oauth_model_policy_page}")"
(
  cd "${observability_webapp}"
  bun install --frozen-lockfile
  bun run test
  bun run build
)
rebuilt_observability_page_hash="$(git -C "${repo_root}" hash-object "${observability_page}")"
rebuilt_proxy_pool_page_hash="$(git -C "${repo_root}" hash-object "${proxy_pool_page}")"
rebuilt_oauth_model_policy_page_hash="$(git -C "${repo_root}" hash-object "${oauth_model_policy_page}")"
if [[ "${observability_page_hash}" != "${rebuilt_observability_page_hash}" ]] ||
   [[ "${proxy_pool_page_hash}" != "${rebuilt_proxy_pool_page_hash}" ]] ||
   [[ "${oauth_model_policy_page_hash}" != "${rebuilt_oauth_model_policy_page_hash}" ]]; then
  echo "plugin management web/index.html resources are stale; run bun run build in ${observability_webapp}" >&2
  exit 1
fi

bash "${repo_root}/cliproxyapi-pro-management/apply.sh" "${upstream_root}"
git -C "${upstream_root}" diff --check

git -C "${upstream_root}" add -N .
patched_diff_hash="$(git -C "${upstream_root}" diff --binary | git hash-object --stdin)"
bash "${repo_root}/cliproxyapi-pro-management/apply.sh" "${upstream_root}"
reapplied_diff_hash="$(git -C "${upstream_root}" diff --binary | git hash-object --stdin)"
if [[ "${patched_diff_hash}" != "${reapplied_diff_hash}" ]]; then
  echo "Management customization is not idempotent" >&2
  exit 1
fi

(
  cd "${upstream_root}"
  bun install --frozen-lockfile
  bun run test
  bun run lint
  bun run type-check
  VERSION="${VERSION:-review}" bun run build
)
