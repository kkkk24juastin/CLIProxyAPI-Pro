# Pro Observability Plugin

This is the dynamic-plugin home for Pro request-monitoring persistence. It has
no frontend resource and does not replace the current Management UI.

The current migration slice implements the native plugin ABI, receives the
enriched `usage.handle` contract, and can write the existing `usage_events` and
`usage_summary` SQLite schema. It remains opt-in until the current Management
query, import/export, backup, pricing, and reset contracts have parity tests.

The next read-only migration slice exposes authenticated shadow endpoints at
`/v0/management/plugins/pro-observability/usage` and
`/v0/management/plugins/pro-observability/usage/events`. They preserve the
current snapshot, forward-incremental, history cursor, and structured-filter
payload shapes without replacing the canonical `/v0/management/usage*` routes.
Aggregates, account analytics, and SSE remain on the host until their contracts
and generic plugin hooks are complete.

Example plugin configuration:

```yaml
plugins:
  enabled: true
  configs:
    pro-observability:
      enabled: true
      priority: 100
      read-enabled: false
      write-enabled: false
      database-path: /CLIProxyAPI/usage/usage.sqlite
```

`read-enabled` and `write-enabled` both default to `false`. Enabling writes also
enables reads; enabling reads alone opens the existing SQLite database in
read-only mode. No host route or persistence patch should be removed until this
plugin becomes the single authoritative writer.
