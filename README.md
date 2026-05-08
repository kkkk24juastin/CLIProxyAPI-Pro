# CLIProxyAPI Pro

CLIProxyAPI Pro is a minimal customization-layer collection for two upstream projects:

- `cliproxyapi-pro-core/` — backend Docker build customization for `router-for-me/CLIProxyAPI`.
- `cliproxyapi-pro-management/` — frontend management-center customization for `router-for-me/Cli-Proxy-API-Management-Center`.

This project does not maintain a full fork of either upstream project. Instead, it keeps repeatable patches, overlays, and build workflows. Release workflows fetch the latest upstream release, apply the Pro customization layer, and publish the resulting artifacts.

## Repository layout

```text
.
├── cliproxyapi-pro-core/
│   ├── Dockerfile
│   ├── entrypoint.sh
│   ├── embeddedusage/
│   └── patches/
│
├── cliproxyapi-pro-management/
│   ├── apply.sh
│   ├── apply_customizations.py
│   ├── monitoring-locales.json
│   └── overlay/
│
└── .github/workflows/
    ├── release-core.yml
    └── release-mangement.yml
```

## Subprojects

### cliproxyapi-pro-core

Backend customization layer for building the Pro Docker image.

Main capabilities:

- Builds a multi-arch Docker image from an upstream CLIProxyAPI release.
- Embeds a SQLite usage service.
- Exposes `/v0/management/usage` API routes.
- Supports usage JSONL/NDJSON import and export.
- Supports WebDAV usage backup restore.
- Supports SQLite-backed quota cache.
- Supports model price persistence.
- Adds a backend account-inspection scheduler.
- Optionally starts the Komari agent.
- Redirects `/` to `/management.html`.
- Enhances the `/healthz` response.

See:

- `cliproxyapi-pro-core/README.md`
- `cliproxyapi-pro-core/README_CN.md`

### cliproxyapi-pro-management

Frontend management-center customization layer for generating the single-file `management.html` artifact.

Main capabilities:

- Adds the `/monitoring` request monitoring page.
- Adds the `/account-inspection` account inspection page.
- Shows request count, success rate, latency, token, and cost metrics.
- Persists model prices through SQLite.
- Persists quota cache through SQLite.
- Shows quota-card cache timestamps and supports single-card refresh.
- Integrates frontend inspection with backend scheduled inspection.
- Supports suggested account disable, enable, and delete actions.
- Adds locale patches.
- Uses a minimal overlay + patch application flow.

See:

- `cliproxyapi-pro-management/README.md`
- `cliproxyapi-pro-management/README_CN.md`

## Backend and frontend relationship

Some `cliproxyapi-pro-management` features depend on enhanced management APIs provided by `cliproxyapi-pro-core`.

Core dependent routes:

```text
/v0/management/usage
/v0/management/usage/export
/v0/management/usage/import
/v0/management/usage/quota-cache
/v0/management/usage/model-prices
/v0/management/account-inspection/schedule
/v0/management/account-inspection/run
```

If the management UI is used with the unmodified upstream backend, request monitoring, SQLite persistence, model prices, and backend account inspection will show errors or empty data.

## Release workflows

### Core image release

Workflow:

```text
.github/workflows/release-core.yml
```

Overview:

1. Checks the latest upstream `router-for-me/CLIProxyAPI` release.
2. Compares it with the current Docker Hub image tag.
3. Builds and pushes the Docker image when upstream is newer.
4. Backs up usage statistics to WebDAV.
5. Triggers Render deployments.
6. Sends a Telegram notification.
7. Deletes old workflow runs.

Image tags follow upstream release tags.

### Management release

Workflow:

```text
.github/workflows/release-mangement.yml
```

Overview:

1. Checks the latest upstream `router-for-me/Cli-Proxy-API-Management-Center` release.
2. Compares it with this repository's latest release, normalizing the `-pro` suffix.
3. Checks out the latest upstream release tag when upstream is newer.
4. Applies the `cliproxyapi-pro-management` customization layer.
5. Runs `npm ci` and `npm run build`.
6. Renames `dist/index.html` to `management.html`.
7. Creates a GitHub Release and uploads `management.html`.
8. Deletes old workflow runs.

Management release tag format:

```text
<upstream-tag>-pro
```

Example:

```text
v1.7.41-pro
```

## Local build

### Build the core Docker image

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

### Apply the management customization layer

```bash
./cliproxyapi-pro-management/apply.sh /path/to/Cli-Proxy-API-Management-Center
```

Or:

```bash
python3 ./cliproxyapi-pro-management/apply_customizations.py /path/to/Cli-Proxy-API-Management-Center
```

The target must be an upstream management-center checkout containing:

- `src/`
- `package.json`

After applying customizations, run in the target directory:

```bash
npm install
npm run type-check
npm run build
```

## Runtime data directory

The core image uses this directory by default:

```text
/CLIProxyAPI/usage
```

It stores:

- usage SQLite database: `usage.sqlite`
- account-inspection schedule file: `account-inspection-schedule.json`
- quota cache
- model prices

Configure a persistent volume for this directory in production.

## Key environment variables

### Usage service

```text
USAGE_SERVICE_ENABLED
USAGE_DATA_DIR
USAGE_DB_PATH
USAGE_BATCH_SIZE
USAGE_POLL_INTERVAL_MS
USAGE_QUERY_LIMIT
```

### WebDAV restore

```text
WEBDAV_URL
WEBDAV_USERNAME
WEBDAV_PASSWORD
MANAGEMENT_PASSWORD
```

### Account inspection

```text
ACCOUNT_INSPECTION_SCHEDULE_PATH
```

### Komari agent

```text
KOMARI_SERVER
KOMARI_SECRET
```

For full details, see `cliproxyapi-pro-core/README.md`.

## Design principles

This project follows a minimal customization approach:

- Do not vendor full upstream source code.
- Prefer overlays and patches for customization.
- Reapply the customization layer when upstream updates.
- Keep documentation, scripts, and workflows verifiable and repeatable.

## Documentation

- Core English README: `cliproxyapi-pro-core/README.md`
- Core Chinese README: `cliproxyapi-pro-core/README_CN.md`
- Management English README: `cliproxyapi-pro-management/README.md`
- Management Chinese README: `cliproxyapi-pro-management/README_CN.md`
- Chinese project overview: `README_CN.md`
