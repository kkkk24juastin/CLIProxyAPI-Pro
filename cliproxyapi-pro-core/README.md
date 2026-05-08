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

The image declares `/CLIProxyAPI/usage` as a Docker volume so usage data and account-inspection schedule state can survive container replacement.

### Usage API

The embedded service exposes these management routes:

- `GET /v0/management/usage` — aggregated usage payload for the management UI.
- `GET /v0/management/usage/export` — JSONL/NDJSON export.
- `POST /v0/management/usage/import` — JSONL/NDJSON import.
- `GET /v0/management/usage/status` — service status and record counts.
- `GET /v0/management/usage/quota-cache` — read quota cache entries or stats.
- `PUT /v0/management/usage/quota-cache` — write a quota cache entry.
- `DELETE /v0/management/usage/quota-cache` — delete quota cache entries.
- `GET /v0/management/usage/model-prices` — read model price settings.
- `PUT /v0/management/usage/model-prices` — write model price settings.

### JSONL usage backup and restore

`/usage/export` returns `application/x-ndjson`, one JSON object per line.

The export contains usage events and may also include metadata records:

- `model_prices` — persisted model price settings used by the management UI cost view.
- `account_inspection_schedule` — persisted backend account-inspection schedule.

`/usage/import` accepts the same JSONL format. It imports usage events, restores model prices, and restores the account-inspection schedule when that metadata record is present. Older event-only JSONL files remain compatible.

Example import response fields:

```json
{
  "added": 100,
  "skipped": 5,
  "total": 105,
  "failed": 0,
  "modelPrices": 12,
  "modelPriceRecords": 1,
  "accountInspectionSchedule": true,
  "accountInspectionScheduleRecords": 1
}
```

### SQLite-backed quota cache

The embedded service stores quota snapshots in SQLite for these providers:

- Antigravity
- Claude
- Codex
- Gemini CLI
- Kimi

The management UI reads and writes this cache through `/usage/quota-cache`, so quota cards can be restored after page refreshes, browser changes, and backend restarts.

### Backend account inspection scheduler

The patch layer adds backend account-inspection routes under the management API:

- `GET /v0/management/account-inspection/schedule`
- `PUT /v0/management/account-inspection/schedule`
- `POST /v0/management/account-inspection/run`

The scheduler can inspect accounts for:

- Antigravity
- Claude
- Codex
- Gemini CLI
- Kimi

It supports provider filtering, worker limits, retry/timeout settings, sampling, usage-threshold decisions, and optional automatic actions for quota exhaustion, quota recovery, and account errors.

The schedule file defaults to:

```text
/CLIProxyAPI/usage/account-inspection-schedule.json
```

Override it with `ACCOUNT_INSPECTION_SCHEDULE_PATH` if needed.

### Root redirect and health response

The patch layer also changes upstream API behavior:

- `/` redirects to `/management.html`.
- `/healthz` returns a richer CLIProxyAPI status payload while preserving `HEAD /healthz`.

### Management panel defaults

The patch layer changes upstream's default remote management panel repository to:

```text
https://github.com/ssfun/CLIProxyAPI-Pro
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
- `.github/workflows/release-core.yml` — image publish, usage backup, Render deployment trigger, Telegram notification, and run cleanup.

## Docker build

Build latest upstream release:

```bash
docker build -t cliproxyapi-pro ./cliproxyapi-pro-core
```

Build a specific upstream release:

```bash
docker build \
  --build-arg CLIPROXY_VERSION=v6.10.1 \
  -t cliproxyapi-pro:v6.10.1 \
  ./cliproxyapi-pro-core
```

Build args:

- `CLIPROXY_REPO` — upstream repository, default `router-for-me/CLIProxyAPI`.
- `CLIPROXY_VERSION` — upstream release tag. If empty, the Dockerfile resolves the latest release.
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

The workflow:

1. Checks the latest upstream CLIProxyAPI release.
2. Compares it with the latest Docker Hub image tag.
3. Builds and pushes a `linux/amd64` and `linux/arm64` Docker image when upstream is newer.
4. Exports usage statistics from one or more running CPA instances to WebDAV.
5. Triggers one or more Render deployments.
6. Sends a Telegram notification.
7. Deletes old workflow runs.

### Required Docker secrets

- `DOCKER_USERNAME`
- `DOCKER_PASSWORD`

### Multi-instance usage backup

Preferred configuration uses one JSON secret:

```text
CLIPROXY_USAGE_BACKUP_TARGETS
```

Example:

```json
[
  {
    "name": "cpa-main",
    "api_url": "https://cpa-main.example.com",
    "management_password": "management-password-1",
    "webdav_url": "https://webdav.example.com/cpa-main",
    "webdav_username": "webdav-user-1",
    "webdav_password": "webdav-password-1"
  }
]
```

Each target is exported from its own CPA API and uploaded to its own WebDAV directory as:

```text
usage-export-YYYYMMDD_HHMMSS.jsonl
```

The workflow keeps the latest 7 backups per WebDAV directory and cleans both `.jsonl` and legacy `.json` files.

Legacy single-instance fallback is still supported when `CLIPROXY_USAGE_BACKUP_TARGETS` is not set:

- `CLIPROXY_API_URL`
- `CLIPROXY_MANAGEMENT_PASSWORD`
- `WEBDAV_URL`
- `WEBDAV_USERNAME`
- `WEBDAV_PASSWORD`

### Multi-target Render deploy hooks

Preferred configuration uses one JSON secret:

```text
CLIPROXY_RENDER_DEPLOY_HOOKS
```

Example:

```json
[
  {
    "name": "cpa-main",
    "hook_url": "https://api.render.com/deploy/srv-xxx?key=xxx"
  }
]
```

`url` is also accepted as an alias for `hook_url`.

Legacy single-target fallback is still supported when `CLIPROXY_RENDER_DEPLOY_HOOKS` is not set:

- `CLIPROXY_RENDER_DEPLOY_HOOK`

### Telegram notification secrets

- `TELEGRAM_CHAT_ID`
- `TELEGRAM_BOT_TOKEN`

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
