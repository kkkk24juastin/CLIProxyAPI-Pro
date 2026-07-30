# CLIProxyAPI Pro Management customizations

Customization layer for upstream `router-for-me/Cli-Proxy-API-Management-Center`.

This directory does not vendor the upstream application. It keeps overlay files plus a patch script that can be applied to a clean upstream checkout during local development or GitHub Actions release builds.

## What this customization adds

### Plugin resource pages

`proxy-pool` registers the sole proxy-pool management page through the upstream plugin resource protocol. It configures HTTP/HTTPS/SOCKS5/SOCKS5H nodes, selection strategy, weights, health checks, isolation, and failover, with batch import, draft tests, manual recovery, and runtime diagnostics. Management no longer compiles or registers a second `/proxy-pool` page.

`oauth-model-policy` likewise registers its sole plugin resource page. Provider tabs edit plan-specific exclusions for xAI, Codex, Claude, Gemini CLI, Antigravity, and Kimi, including custom plan keys and distinct `_unknown` and `_default` fallbacks. Cache TTL, provider resolve timeout, and plugin priority are written directly to plugin configuration; Management no longer owns that business page, service, or styling.

`pro-observability` registers the sole complete monitoring-center resource and owns the UI implementation, usage/aggregate contracts, model prices, backups, quota cache, and routing runtime. Management no longer compiles a `/monitoring` route, static menu item, or second set of monitoring components; the dynamic entry is `#/plugin-pages/pro-observability/0`. Core retains only host startup, Management authentication, and the SSE transport bridge.

After the listener is ready, the page can point Core's global `proxy-url` at the fixed loopback SOCKS5 endpoint; stopping takeover restores the previous global proxy. Credentials with their own `proxy-url` are listed explicitly as bypasses. Plugin configuration persists with the normal Core configuration, while health state and connection counters remain process-local runtime data and are not mixed into usage backups.

Rotation is per SOCKS5 TCP tunnel, not necessarily per multiplexed HTTP request. `fail-open=false` is the default to prevent silent direct traffic leakage.

### Plugin monitoring center

The page is built from `pro-observability/webapp`, embedded in the plugin, and loaded through the upstream plugin resource protocol:

```text
#/plugin-pages/pro-observability/0
```

The page consumes the plugin-owned `/usage*` API. It loads an initial usage snapshot, follows incremental event polling or the SSE usage stream, and provides:

- request totals and success/failure metrics
- success rate and latency summaries
- input, output, cached, reasoning, and total token summaries
- estimated cost based on configurable model prices
- time-range filtering for today, 7d, 14d, 30d, and all data
- search plus account/provider/model/channel/status filters
- auto refresh interval selection and manual refresh
- sortable account overview table
- expandable account rows with model spend details
- account-scoped quota refresh and quota display
- realtime request table with recent success/failure pattern bars
- masking for sensitive token-like text in request metadata

Large account and realtime tables scroll inside their panels, so long histories do not stretch the whole page.

Management supplies only a controlled Bridge v2 for the active Core connection: JSON requests, SSE open/chunk/close, binary imports, host downloads, toast/confirm UI, and clipboard operations. The sanitized iframe bootstrap contains no management key, raw API key, auth token, or unmasked credential material, and remote-Core connections do not require a second iframe login.

### Model price persistence

Model price settings are persisted through the `pro-observability` SQLite API instead of normal browser-only state:

- `GET /usage/model-prices`
- `PUT /usage/model-prices`
- `GET|PUT|DELETE /usage/model-price-rules`
- `POST /usage/model-prices/sync`
- `GET /usage/model-prices/sync-status`
- `POST /usage/model-prices/recalculate`

If the backend has no saved prices, the UI can migrate old `localStorage` price settings once. Normal reads and writes then use SQLite.

Rules apply globally by model ID, so the same model shares one rule across providers. They support input, output, cache-read, cache-write, multiple context-size tiers, and service-tier overrides. Prices can be synchronized manually or periodically from models.dev; only models observed in request history are persisted, and locked manual rules are not overwritten.

The backend selects pricing per request and snapshots the estimated cost on each usage event. Aggregate APIs sum those event costs. JSONL export/import preserves both complete rules and cost snapshots.

### SQLite-backed quota persistence

Quota snapshots are persisted through the `pro-observability` usage service:

- `GET /usage/quota-cache`
- `PUT /usage/quota-cache`
- `DELETE /usage/quota-cache`

The UI starts `QuotaPersistenceBootstrap` from the main layout. It preloads saved quota snapshots into the Zustand quota store and syncs successful quota checks back to SQLite. Quota cache entries are also included in usage JSONL export/import as a `quota_cache` metadata record.

Supported quota providers:

- Antigravity
- Claude
- Codex
- Gemini CLI
- Kimi
- xAI

Quota cards also show cache timestamps and support single-card refresh when the feature flags in `src/config/features.ts` are enabled.

### Account inspection page

Adds a top-level account inspection route:

```text
/account-inspection
```

The page controls and displays backend-run inspections. The browser does not execute probes directly. The auth files page also shows inspection-written `last_error` health messages when no explicit status message exists. The backend can inspect:

- Antigravity
- Claude
- Codex
- Gemini CLI
- Kimi
- xAI

Features include:

- target provider selection
- configurable workers, delete workers, timeout, retries, used-percent threshold, and sample size
- backend run, pause, resume, and stop controls
- backend schedule enablement and interval configuration
- progress, summary cards, and result table from backend status polling
- logs and live status from the backend WebSocket/WSS stream
- suggested actions: keep, delete, disable, enable
- manual execution for a single planned action or all planned actions through the backend
- business-result toast messages for single-account rechecks, such as account errors, quota exhaustion, or healthy state
- optional backend auto-execution policies for quota-limit disable, quota-recovery enable, and account-error disable/delete
- quota snapshot refresh from backend inspection results

Backend schedule/status/control routes expected by the page:

- `GET /account-inspection/schedule`
- `GET /account-inspection/status`
- `GET /account-inspection/logs` (WebSocket/WSS log and status stream)
- `PUT|PATCH /account-inspection/schedule`
- `POST /account-inspection/run`
- `POST /account-inspection/inspect-one`
- `POST /account-inspection/pause`
- `POST /account-inspection/resume`
- `POST /account-inspection/stop`
- `POST /account-inspection/actions`

Under the full management API prefix these are exposed by the backend as `/v0/management/account-inspection/...`.

### Routing policy page

Adds a top-level routing-policy route:

```text
/routing
```

The page combines upstream routing configuration with Pro request-state protection in three views:

- Global routing: round-robin/fill-first mode, session stickiness and TTL, retries and account switching, cooldown and cooldown persistence, transient-error cooldown, quota fallback, and Codex identity cloaking.
- Provider protection: only supported providers with current API configuration or credentials, with per-provider enablement, HTTP statuses, confirmation thresholds and windows, 429 quota evidence, automatic release, and fallback disable duration.
- Runtime status: accounts currently disabled by request protection plus recent events, detailed reason/context dialogs, and manual release for one account.

The page uses:

- `GET /routing-policy`
- `PUT|PATCH /routing-policy`
- `POST /routing-policy/release`

Protection is disabled by default. `observe` records matches; only `enforce` disables accounts. Automatic and manual release affect only accounts carrying request-protection ownership metadata.

### Supporting API and type patches

`apply_customizations.py` also patches upstream files to add:

- the `/routing` route; both monitoring and account inspection are discovered from the plugin resource registry.
- Plugin Resource Bridge v2 plus account-inspection/routing sidebar labels and icons.
- locale entries from `monitoring-locales.json`.
- `usageStatisticsEnabled` and `clean` config types used by the plugin bootstrap.
- `authFilesApi.patchFile` and `setStatusWithFallback` helpers.
- `Select` `triggerClassName` and `dropdownClassName` props.
- `maskSensitiveText` utility.
- `cachedAt` fields for quota state types and success states.
- a “Check for updates” action on the Management Center version tile; it calls `POST /management-panel/check-update`, replaces the panel only when the latest-release asset hash changes, and reloads only after an actual update.

The plugin monitoring center uses an initial snapshot plus SSE increments and cursor catch-up, with event-ID deduplication. Trends, model rankings, and API-key rankings use server-side `/usage/aggregates` data. Hidden tabs pause SSE and React incremental updates, then catch up by cursor when visible again. The first event received by an empty database also establishes and refreshes the realtime-log snapshot.

## Repository layout

- `overlay/` — files copied directly into the upstream checkout.
- `overlay/src/pages/RoutingPolicyPage.tsx` — routing policy and request-state-protection UI.
- `overlay/src/features/plugins/usePluginResourceBridge.ts` — controlled Management Bridge v2 for plugin pages.
- `overlay/src/features/monitoring/` — account-usage deep links and account-plan display logic; it contains neither the monitoring center nor account inspection.
- `overlay/src/extensions/quota/` — SQLite quota persistence integration.
- `overlay/src/services/api/` — routing-policy and account-usage API clients.
- `overlay-replacements.json` — reviewed upstream SHA-256 values and reasons for full-file replacements that intentionally collide with upstream paths.
- `monitoring-locales.json` — locale additions merged into upstream locale files.
- `apply_customizations.py` — applies all customizations to a target upstream checkout.
- `apply.sh` — shell wrapper around `apply_customizations.py`.
- `quota-persistence.patch` — legacy patch artifact kept for reference; current builds use `apply_customizations.py`.

Overlay collision preflight validates the upstream side of every reviewed replacement. Upstream file changes must update `overlay-replacements.json` explicitly; local replacements are reviewed through normal PR diffs and behavior tests, and new unreviewed path collisions are rejected before any overlay file is copied.

## Applying locally

From this directory:

```bash
./apply.sh /path/to/Cli-Proxy-API-Management-Center
```

Equivalent direct command:

```bash
python3 apply_customizations.py /path/to/Cli-Proxy-API-Management-Center
```

The target directory must be an upstream checkout containing:

- `src/`
- `package.json`

## Local validation

After applying to an upstream checkout:

```bash
bun install --frozen-lockfile
bun run test
bun run lint
bun run type-check
VERSION=review bun run build
```

Use the repository validation script with a disposable clean upstream checkout to verify overlay preflight, reapplication, tests, lint, type checking, and build:

```bash
bash scripts/validation/management.sh /path/to/disposable/clean-management-checkout
```

## GitHub Actions release workflow

Workflow:

```text
.github/workflows/release-management.yml
```

This workflow no longer creates a separate management release. It rebuilds and clobbers `management.html` on the current repository latest release when the management upstream changes, when the latest release is missing `management.html`, or when the workflow is triggered manually.

The workflow:

1. Checks the current repository latest release.
2. Checks the latest upstream `router-for-me/Cli-Proxy-API-Management-Center` release.
3. Reads the management upstream version recorded in the latest release notes.
4. If upstream is newer, the latest release has no `management.html`, or the workflow was triggered manually, checks out the latest upstream release tag.
5. Applies this customization layer from `cliproxyapi-pro-management/apply.sh`.
6. Runs `bun install --frozen-lockfile` and `bun run build`; the Bun version comes from upstream `package.json`.
7. Renames `dist/index.html` to `management.html`.
8. Uploads and clobbers `management.html` on the current latest release.
9. Updates the management version mapping and upstream release notes in the release notes.
10. Deletes old workflow runs.

This keeps `remote-management.panel-github-repository=https://github.com/ssfun/CLIProxyAPI-Pro` able to fetch the latest `management.html` through GitHub `/releases/latest`.

## Backend expectations

These frontend customizations expect the customized `cliproxyapi-pro-core` backend to expose these stable route groups under the management API prefix:

- `/v0/management/usage`
- `/v0/management/usage/*`
- `/v0/management/quota/fetch`
- `/v0/management/account-inspection/*`
- `/v0/management/routing-policy`
- `/v0/management/routing-policy/*`

See the Core README for the complete method/path list. Without the customized backend, monitoring, SQLite-backed persistence, model prices, backend account inspection, and routing protection will show errors or empty data.
