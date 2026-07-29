# Pro Observability Plugin

`pro-observability` is the only persistence owner for Pro usage events, aggregates, model prices, quota cache, JSONL import/export, retention, WebDAV backups, routing cursors, and auth runtime statistics.

The Pro Core patch enables the dynamic plugin system and this plugin by default. Startup is fail-closed: the proxy service does not start unless the plugin loads successfully and completes storage preparation.

## Automatic legacy usage migration

The plugin prepares storage synchronously during plugin registration, before proxy service construction:

1. Core preserves explicit YAML paths, lets `USAGE_DB_PATH` / `PRO_OBSERVABILITY_DB_PATH` override them, fills missing paths from `USAGE_DATA_DIR`, then passes both paths explicitly across the plugin ABI.
2. If target and legacy paths are the same, adopt the existing `usage.sqlite` in place and apply current schema migrations.
3. If a different target is configured and does not exist, checkpoint and integrity-check the legacy SQLite database, copy it atomically, then open the copied target.
4. Write the `observability.storage_owner` marker in `pro_settings` only after the target schema is ready.
5. Start ingestion, retention, price sync, and backup workers.

The operation is idempotent. A target already marked as plugin-owned is reused. If distinct unowned target and legacy databases both exist, startup stops instead of guessing or merging potentially overlapping events.

Migration status is available at:

```text
GET /v0/management/pro/observability/migration/status
```

## Runtime ownership

The plugin always registers the production `/v0/management/usage*` API, owns routing selection and cursor persistence, and provides quota-cache, runtime-state, and `pro_settings` capabilities. Weighted routing persists its complete smooth-weight snapshot. A restricted host callback carries Core-owned account-inspection schedule/snapshot data into backups and immediately reapplies imported runtime and routing-protection state. Core retains only the authenticated `/usage/stream` SSE transport bridge because the current Management plugin ABI buffers responses and cannot stream directly.

Minimal configuration is generated automatically:

```yaml
plugins:
  enabled: true
  configs:
    pro-observability:
      enabled: true
      routing-strategy: round-robin
```

`routing-strategy` follows the existing Core routing strategy at startup. The supported values are `round-robin`, `weighted-round-robin`, and `fill-first`.
