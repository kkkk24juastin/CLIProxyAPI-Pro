# CLIProxyAPI Pro Core

Customized Docker build layer for upstream `router-for-me/CLIProxyAPI`.

This directory does not maintain a full fork of upstream. During Docker build it downloads an upstream release, copies in the local `embeddedusage/` package, applies the patch script in `patches/`, and builds a multi-arch image for the Pro deployment.

## What this customization adds

### Embedded usage service

`embeddedusage/` is copied into upstream as:

```text
internal/embeddedusage
```

The patch layer starts the service with the main API process, enables upstream usage statistics, and exposes the service under the management API prefix:

```text
/v0/management/usage
```

By default it stores SQLite data at:

```text
/CLIProxyAPI/usage/usage.sqlite
```

The image declares `/CLIProxyAPI/usage` as a Docker volume so usage data, quota cache, model prices, and account-inspection schedule state can survive container replacement.

At service startup the patch layer forces the upstream config values required by Pro:

- `usage-statistics-enabled: true`
- `remote-management.panel-github-repository: https://github.com/kkkk24juastin/CLIProxyAPI-Pro`

The loaded in-memory config is always corrected. `config.yaml` is updated only when the loaded values differ, preserving normal startup behavior when the file is already correct.

### Usage API

The embedded service exposes these management routes:

- `GET /v0/management/usage` — aggregated usage payload for the management UI.
- `GET /v0/management/usage/events` — incremental usage events after a cursor.
- `GET /v0/management/usage/aggregates` — aggregate usage by time bucket and provider/model/endpoint/API key.
- `GET /v0/management/usage/stream` — SSE stream for live usage updates.
- `GET /v0/management/usage/export` — JSONL/NDJSON export.
- `POST /v0/management/usage/import` — JSONL/NDJSON import.
- `POST /v0/management/usage/reset` — atomically clear request events and derived statistics while preserving monitoring settings, model prices, quota cache, and backups.
- `GET /v0/management/usage/status` — service status and record counts.
- `GET /v0/management/usage/quota-cache` — read quota cache entries or stats.
- `PUT /v0/management/usage/quota-cache` — write a quota cache entry.
- `DELETE /v0/management/usage/quota-cache` — delete quota cache entries.
- `GET /v0/management/usage/model-prices` — read model price settings.
- `PUT /v0/management/usage/model-prices` — write model price settings.
- `GET|PUT|DELETE /v0/management/usage/model-price-rules` — manage provider/model rules and context tiers.
- `POST /v0/management/usage/model-prices/sync` — synchronize observed models from models.dev.
- `GET /v0/management/usage/model-prices/sync-status` — read synchronization status.
- `POST /v0/management/usage/model-prices/recalculate` — explicitly recalculate historical costs.
- `GET /v0/management/usage/settings` — read retention, WebDAV, and model-price synchronization settings.
- `PUT /v0/management/usage/settings` — write retention, WebDAV, and model-price synchronization settings.

Details returned by `/usage/events` and `/usage/stream` include a stable event `id`, which the management UI uses for incremental deduplication and cursor catch-up. Usage responses also include a persistent `generation`; manual resets and retention cleanup advance it, and SSE emits a `reset` event so open pages replace their complete snapshot. SSE connections are awakened by an in-process notification after SQLite commits, with only a low-frequency keepalive instead of one database poll per connection per second.

`/usage/aggregates` supports `from_ms`, `to_ms`, `interval=minute|hour|day|all`, `group_by=provider,model,endpoint,api_key_hash`, `api_key_hash`, and `timezone_offset_minutes`. Responses include `latest_id`, `snapshot_at_ms`, and event-level `estimatedCost` sums so context tiers are never selected from aggregated token totals.

### JSONL usage backup and restore

`/usage/export` returns `application/x-ndjson`, one JSON object per line.

The export contains usage events and may also include metadata records:

- `model_prices` — legacy base prices plus complete provider/model pricing rules.
- `quota_cache` — SQLite-backed quota snapshots used by quota cards and account-scoped refresh.
- `monitoring_settings` — retention, WebDAV backup, and scheduled models.dev synchronization settings.
- `account_inspection_schedule` — persisted backend account-inspection schedule.
- `account_inspection_snapshot` — the latest finished inspection result, including run settings, summary, health counts, complete results, and raw error details, but excluding inspection logs.

`/usage/import` accepts the same JSONL format. It reads each line's `record_type` once, imports usage events, and restores model prices, quota cache entries, monitoring settings, the account-inspection schedule, and the latest inspection-result snapshot when present. A restored result snapshot is read-only until a new full inspection runs. Older event-only JSONL files remain compatible.

Example import response fields:

```json
{
  "added": 100,
  "skipped": 5,
  "total": 105,
  "failed": 0,
  "modelPrices": 12,
  "modelPriceRecords": 1,
  "quotaCache": 8,
  "quotaCacheRecords": 1,
  "accountInspectionSchedule": true,
  "accountInspectionScheduleRecords": 1,
  "accountInspectionSnapshot": true,
  "accountInspectionSnapshotRecords": 1,
  "monitoringSettings": true,
  "monitoringSettingsRecords": 1
}
```

### SQLite-backed quota cache

The embedded service stores quota snapshots in SQLite for these providers:

- Antigravity
- Claude
- Codex
- Kimi

The management UI reads and writes this cache through `/usage/quota-cache`, so quota cards can be restored after page refreshes, browser changes, and backend restarts.

### Backend account inspection scheduler

The patch layer adds backend account-inspection routes under the management API:

Request monitoring also stores TTFT, HTTP status code, structured error, reasoning effort, and service tier. `/usage/status` returns recent dead-letter samples with sensitive fields redacted. Account-inspection automatic actions support consecutive-confirmation gating, and quota cache entries include parser version plus response-shape hashes.

- `GET /v0/management/account-inspection/schedule`
- `GET /v0/management/account-inspection/status`
- `GET /v0/management/account-inspection/logs` (WebSocket/WSS log and status stream)
- `PUT /v0/management/account-inspection/schedule`
- `POST /v0/management/account-inspection/run`
- `POST /v0/management/account-inspection/pause`
- `POST /v0/management/account-inspection/resume`
- `POST /v0/management/account-inspection/stop`
- `POST /v0/management/account-inspection/actions`

The scheduler can inspect accounts for:

- Antigravity
- Claude
- Codex
- Kimi

It supports provider filtering, worker limits, retry/timeout settings, sampling, usage-threshold decisions, progress/status/log/result snapshots, pause/resume/stop controls, manual actions, and optional automatic actions for quota exhaustion, quota recovery, and account errors.

Before probing an account, the scheduler can refresh its auth record when it is already in the normal upstream refresh window. This inspection refresh path reuses upstream provider refresh logic and persistence, allows disabled accounts, skips API-key accounts, skips accounts not yet due, and respects `NextRefreshAfter`. If refresh succeeds, probing uses the refreshed auth; if refresh fails, the scheduler keeps the account and skips probing it for that run.

The schedule file defaults to:

```text
/CLIProxyAPI/usage/account-inspection-schedule.json
```

Override it with `ACCOUNT_INSPECTION_SCHEDULE_PATH` if needed.

The latest finished inspection result is persisted separately at `/CLIProxyAPI/usage/account-inspection-snapshot.json` with mode `0600`. A snapshot restored after process restart or usage import is read-only and is replaced when the next full inspection finishes. Override its path with `ACCOUNT_INSPECTION_SNAPSHOT_PATH` if needed.

### Root redirect and health response

The patch layer also changes upstream API behavior:

- `/` redirects to `/management.html`.
- `/healthz` returns a richer CLIProxyAPI status payload while preserving `HEAD /healthz`.

### Management panel defaults

The patch layer changes upstream's default remote management panel repository to:

```text
https://github.com/kkkk24juastin/CLIProxyAPI-Pro
```

This affects the built-in default config, `config.example.yaml`, and the management asset updater's default latest-release API URL.

### Runtime helper process

`entrypoint.sh` can start the bundled Komari agent before the main API process when both variables are configured:

- `KOMARI_SERVER`
- `KOMARI_SECRET`

It then starts `CLIProxyAPI` and optionally restores the latest usage backup from WebDAV.

## Repository layout

- `Dockerfile` — downloads upstream CLIProxyAPI, applies this customization layer, and builds the final image.
- `entrypoint.sh` — starts Komari, starts the main API, and restores WebDAV usage backups.
- `embeddedusage/` — embedded SQLite usage service and management routes.
- `patches/apply_upstream_patches.py` — patches upstream source during Docker build.
- `patches/account_inspection_scheduler.go` — backend account-inspection scheduler injected into upstream management handlers.
- `.github/workflows/release-core.yml` — multi-architecture image and `management.html` publishing, test gates, and run cleanup.

## Docker build

Published image:

```bash
docker pull sfun/cliproxyapi-pro:latest
```

Build latest upstream release:

```bash
docker build -t cliproxyapi-pro ./cliproxyapi-pro-core
```

Build a specific upstream release while writing the Pro runtime version:

```bash
docker build \
  --build-arg CLIPROXY_VERSION=v7.1.18 \
  --build-arg CLIPROXY_BUILD_VERSION=v7.1.18-pro \
  -t cliproxyapi-pro:v7.1.18-pro \
  ./cliproxyapi-pro-core
```

`CLIPROXY_VERSION` selects the upstream source tag, while `CLIPROXY_BUILD_VERSION` sets the runtime version.

Build args:

- `CLIPROXY_REPO` — upstream repository, default `router-for-me/CLIProxyAPI`.
- `CLIPROXY_VERSION` — upstream release tag. If empty, the Dockerfile resolves the latest release.
- `CLIPROXY_BUILD_VERSION` — optional runtime version. If empty, it uses the upstream version resolved from `CLIPROXY_VERSION`.
- `GITHUB_TOKEN` — optional token for GitHub API requests.

## Runtime environment variables

### Usage service

- `USAGE_SERVICE_ENABLED` — default `true`; set to `false`/`0`/`no`/`off` to disable the embedded service.
- `USAGE_DATA_DIR` — default `/CLIProxyAPI/usage`.
- `USAGE_DB_PATH` — default `/CLIProxyAPI/usage/usage.sqlite`.
- `USAGE_BATCH_SIZE` — default `100`.
- `USAGE_POLL_INTERVAL_MS` — default `500`.
- `USAGE_QUERY_LIMIT` — default `50000`.

### Account inspection

- `ACCOUNT_INSPECTION_SCHEDULE_PATH` — optional schedule JSON path. Defaults to `USAGE_DATA_DIR/account-inspection-schedule.json`.
- `ACCOUNT_INSPECTION_SNAPSHOT_PATH` — optional latest inspection-result snapshot JSON path. Defaults to `USAGE_DATA_DIR/account-inspection-snapshot.json`.

### WebDAV usage restore

When all variables below are configured, `entrypoint.sh` waits for the local API to become ready, downloads the latest backup from WebDAV, and imports it into `/v0/management/usage/import`:

- `WEBDAV_URL`
- `WEBDAV_USERNAME`
- `WEBDAV_PASSWORD`
- `MANAGEMENT_PASSWORD`

Restore lookup supports both backup names:

```text
usage-export-YYYYMMDD_HHMMSS.json
usage-export-YYYYMMDD_HHMMSS.jsonl
```

The import request uses:

```text
Content-Type: application/x-ndjson
```

### Komari agent

- `KOMARI_SERVER`
- `KOMARI_SECRET`

## GitHub Actions

Workflow:

```text
.github/workflows/release-core.yml
```

The workflow checks upstream on a schedule and supports manual dispatch. A push to `cliproxyapi-pro-core/**` or the workflow itself on `main` forces a new publish even when the upstream version is unchanged.

The workflow:

1. Checks the latest upstream CLIProxyAPI release and computes the Pro release tag, for example `v7.1.18-pro`.
2. Checks the latest upstream management release.
3. Applies the core patches, runs the full Go test suite, and builds and pushes `linux/amd64` and `linux/arm64` Docker images tagged with `latest` and the Pro release tag.
4. Applies the management customization layer, runs customization tests, frontend tests, and lint, then builds `management.html`.
5. Creates or updates the current repository GitHub Release and uploads `management.html`; standalone platform binaries are not published.
6. Writes core upstream, management upstream, and customization revision mappings into the GitHub Release notes.
7. Deletes old workflow runs.

### Required Docker secrets

- `DOCKER_USERNAME`
- `DOCKER_PASSWORD`

## Local validation

Validate the embedded usage package against an upstream checkout:

```bash
cp -R /path/to/CLIProxyAPI /tmp/cliproxy-check
rm -rf /tmp/cliproxy-check/internal/embeddedusage
cp -R cliproxyapi-pro-core/embeddedusage /tmp/cliproxy-check/internal/embeddedusage
cp cliproxyapi-pro-core/patches/account_inspection_scheduler.go /tmp/account_inspection_scheduler.go
SRC_ROOT=/tmp/cliproxy-check python3 cliproxyapi-pro-core/patches/apply_upstream_patches.py
go -C /tmp/cliproxy-check mod tidy
go -C /tmp/cliproxy-check test ./internal/embeddedusage/...
```

Validate entrypoint syntax:

```bash
sh -n cliproxyapi-pro-core/entrypoint.sh
```
