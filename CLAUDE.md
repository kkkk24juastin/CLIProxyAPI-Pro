# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository is

This is **not** a fork of the upstream projects. It is a set of **re-appliable customization layers** (Python patch scripts + file overlays) that get applied on top of freshly downloaded upstream releases at build/release time. Source for the actual application lives upstream; this repo only holds the deltas.

Two upstream projects are customized:

- `cliproxyapi-pro-core/` — backend layer for `router-for-me/CLIProxyAPI` (Go). Produces the Pro Docker image and Pro binary release assets.
- `cliproxyapi-pro-management/` — frontend layer for `router-for-me/Cli-Proxy-API-Management-Center` (React/TS/Vite). Produces a single-file `management.html`.

`upstreamcode/CLIProxyAPI/` and `upstreamcode/Cli-Proxy-API-Management-Center/` are checked-out copies of the upstream repos, kept **for reference only** (reading APIs, finding patch anchors). They are not the build inputs — the release workflows download fresh upstream tarballs.

## Critical working model: edit the patches, not the patched output

The single most important thing to understand: **you almost never hand-edit upstream files.** To change backend or frontend behavior you edit the patch scripts so the change re-applies cleanly the next time upstream is pulled.

- Backend changes → edit `cliproxyapi-pro-core/patches/apply_upstream_patches.py` (and the `.go` files it copies in: `account_inspection_scheduler.go`, `redisqueue_plugin.go`, `redisqueue_usage_toggle.go`) or the self-contained `cliproxyapi-pro-core/embeddedusage/` package.
- Frontend changes → edit `cliproxyapi-pro-management/apply_customizations.py`, add/modify files under `cliproxyapi-pro-management/overlay/`, or update `monitoring-locales.json`.

Both patch scripts are **idempotent**: every mutation is guarded by a `present`/`new in text` check that skips if the change is already applied, and raises `SystemExit`/`RuntimeError` if the anchor text it expects is missing. This means: **when upstream changes its code and an anchor string no longer matches, the patch script hard-fails** — that is the intended signal to update the anchor. Patches are line/substring-exact, so they are fragile against upstream refactors by design.

When adding a new patch, follow the existing helpers rather than inventing new mechanics:
- Core (`apply_upstream_patches.py`): `replace_once`, `insert_before`, `insert_before_nth`, `add_go_import`, `replace_go_function`, `replace_go_call_block`, `ensure_go_require`, `write_text` (for whole new files). All edits are buffered in `_writes` and committed by `flush_writes()`, then `gofmt` + `go mod tidy` run.
- Management (`apply_customizations.py`): `replace_once`, `replace_all`, `insert_once`, `replace_once_in_quota_config`, `copy_overlay`. Buffered the same way via `_writes`/`flush_writes()`.

Module import paths upstream use a versioned path (`github.com/router-for-me/CLIProxyAPI/v6`). The core script reads the real module path from `go.mod` at apply time and rewrites copied-in Go files to match, so don't hardcode the `/v6`.

## Commands

### Apply + build the management layer (frontend)
Target must be an upstream management-center checkout (has `src/` and `package.json`):
```bash
./cliproxyapi-pro-management/apply.sh /path/to/Cli-Proxy-API-Management-Center
# or: python3 ./cliproxyapi-pro-management/apply_customizations.py /path/to/...
cd /path/to/Cli-Proxy-API-Management-Center
npm install
npm run type-check        # tsc --noEmit
npm run build             # tsc && vite build  → dist/index.html (released as management.html)
npm run lint              # eslint . --ext ts,tsx
```

### Apply + build the core layer (backend)
The patch script expects to run inside an upstream checkout, defaulting to `/src/CLIProxyAPI` (override with `SRC_ROOT`). It copies `embeddedusage/` into `internal/embeddedusage`, applies all Go patches, then runs `gofmt` and `go mod tidy`:
```bash
SRC_ROOT=/path/to/CLIProxyAPI python3 cliproxyapi-pro-core/patches/apply_upstream_patches.py
```
Upstream Go commands (run inside the checkout after patching):
```bash
gofmt -w .                                   # required after Go changes
go build -o cli-proxy-api ./cmd/server
go test ./...
go test -v -run TestName ./path/to/pkg       # single test
```
The Pro patch ships its own tests; relevant suites include `./internal/embeddedusage/...`, `./internal/pluginstore/...`, `./internal/pluginhost/...`, and `./internal/api/handlers/management/...`.

### Build the core Docker image
```bash
docker build -t cliproxyapi-pro ./cliproxyapi-pro-core
# pin upstream release + Pro build version:
docker build \
  --build-arg CLIPROXY_VERSION=v7.1.18 \
  --build-arg CLIPROXY_BUILD_VERSION=v7.1.18-pro \
  -t cliproxyapi-pro:v7.1.18-pro ./cliproxyapi-pro-core
```
`CLIPROXY_VERSION` selects which upstream source tag to download; `CLIPROXY_BUILD_VERSION` is the version string compiled into the binary (`-pro` suffix). If `CLIPROXY_VERSION` is empty the Dockerfile resolves upstream's latest release.

## Backend customizations (what the patches inject)

The core patch turns stock CLIProxyAPI into the Pro backend. Major injections, all in `apply_upstream_patches.py`:

- **Embedded usage service** (`internal/embeddedusage`, copied wholesale from `cliproxyapi-pro-core/embeddedusage/`): a SQLite-backed (`modernc.org/sqlite`) usage store + HTTP API mounted under `/v0/management/usage`. Provides status, incremental event polling, SSE stream, NDJSON import/export, quota cache, model prices, monitoring settings, and WebDAV backup. Started from `internal/cmd/run.go` via `embeddedusage.Start(...)` and routed in `internal/api/server.go`.
- **Account inspection scheduler** (`internal/api/handlers/management/account_inspection_scheduler.go`, ~5k lines, copied from `patches/`): backend-driven scheduler/executor exposed under `/v0/management/account-inspection/*`. Inspects Codex/Claude/Antigravity/Kimi/xAI accounts, can refresh tokens before probing, and can auto enable/disable/delete/refresh based on confirmation thresholds.
- **Inspection-aware token refresh** (`sdk/cliproxy/auth/inspection_refresh.go`): `RefreshIfDueForInspection` / `ForceRefreshForInspection` on the auth `Manager`, skipping API-key accounts and accounts still inside their `NextRefreshAfter` backoff.
- **Pro-required startup config** (`applyProRequiredStartupConfig` in `run.go`): forces `usage-statistics-enabled=true` and `remote-management.panel-github-repository=https://github.com/ssfun/CLIProxyAPI-Pro`, persisting back to `config.yaml` **only** when the loaded config differs. Note the panel repository constant is changed via `DefaultPanelGitHubRepository` in `internal/config/config.go` and `internal/managementasset/updater.go`.
- **Plugin auto-install** (`internal/pluginstore/autoinstall.go`): downloads enabled-but-missing plugins from configured registries before the plugin host scans local binaries.
- **Gemini-CLI storage normalization** + **plugin auth `disabled` round-trip** (`internal/pluginhost/`): normalizes string/object token shapes and preserves the `disabled` flag through plugin metadata.
- **`requestmeta` package** + logging shims: request-id/endpoint/response-status/headers carried on context for monitoring diagnostics (TTFT, HTTP status, structured errors).
- **Auth-file `last_error` exposure** and Codex id_token claims in `internal/api/handlers/management/auth_files.go`; `use_executor` option on management `api-call` in `api_tools.go`.
- **Root redirect**: `GET /` → `/management.html`.

## Frontend customizations (what the patches inject)

The management patch adds two pages and persistence, all driven by `apply_customizations.py`:

- **Overlay** (`overlay/src/...`) is copied verbatim into the target: new pages `MonitoringCenterPage` and `AccountInspectionPage`, the `extensions/quota/*` persistence layer (IndexedDB + SQLite-backed quota cache, persistence middleware), `features/monitoring/*` hooks, and new API clients (`services/api/accountInspection.ts`, `apiCall.ts`).
- **In-place patches** wire the overlay into upstream: routes (`MainRoutes.tsx`), sidebar/layout (`MainLayout.tsx`, `icons.tsx`), quota types/store/configs/constants/styles, auth-file multi-field search (name/type/provider/note/plan/tier/etc. with `*` wildcard), and `last_error` surfacing on the auth-files page. The layout patch tolerates **both** the flat and grouped upstream sidebar shapes — preserve that branching when touching it.
- **Locales**: `monitoring-locales.json` plus inline dicts in the script are merged into every `src/i18n/locales/*.json`. New user-facing strings need entries for en/ru/zh-CN/zh-TW.

Frontend features that depend on the Pro backend (monitoring, SQLite persistence, model prices, account inspection) show errors or empty data against a stock upstream backend.

## Release pipeline (`.github/workflows/`)

- `release-core.yml` — the unified Pro release. Resolves upstream core latest release, computes the Pro tag (`<upstream>-pro`, e.g. `v7.1.18-pro`), applies core patches + builds/pushes multi-arch Docker images, builds Pro binaries (default packages: CGO-enabled with dynamic-library plugin support; `_no-plugin` packages: CGO-free static), applies the management layer to build `management.html`, then creates/updates the GitHub Release with binaries, `checksums.txt`, and `management.html`. Asset name prefix stays `CLIProxyAPI` to match upstream packaging.
- `release-management.yml` — does **not** create its own release. When management upstream updates (or `management.html` is missing from the latest release), it rebuilds and overwrites `management.html` on the current latest release, keeping the `panel-github-repository` updater able to fetch it from `/releases/latest`.

## Runtime (Docker)

- Data dir: `/CLIProxyAPI/usage` (declared `VOLUME`) holding `usage.sqlite`, `account-inspection-schedule.json`, quota cache, model prices, monitoring settings. Persist this in production.
- `entrypoint.sh` (POSIX `sh`): optionally starts the Komari agent (`KOMARI_SERVER`/`KOMARI_SECRET`), starts the main app, then — if WebDAV vars are set — waits for `/v0/management/usage` readiness and restores the latest `usage-export-*.jsonl` backup via the import API.
- Key env vars: `USAGE_SERVICE_ENABLED`, `USAGE_DATA_DIR`, `USAGE_DB_PATH`, `USAGE_BATCH_SIZE`, `USAGE_POLL_INTERVAL_MS`, `USAGE_QUERY_LIMIT`; `WEBDAV_URL`/`WEBDAV_USERNAME`/`WEBDAV_PASSWORD`/`MANAGEMENT_PASSWORD`; `ACCOUNT_INSPECTION_SCHEDULE_PATH`; `KOMARI_SERVER`/`KOMARI_SECRET`.

## Upstream code conventions (apply to patched Go code)

When writing Go that lands in the upstream tree, follow upstream's `AGENTS.md`:
- `gofmt` after every change; English-only comments; KISS.
- Do not use `log.Fatal`/`log.Fatalf`; return errors and log via logrus.
- After an upstream connection is established, do not set network timeouts (credential-acquisition timeouts are the documented exception, plus the management APICall timeout in `api_tools.go`).
- Avoid standalone changes to `internal/translator/`.
