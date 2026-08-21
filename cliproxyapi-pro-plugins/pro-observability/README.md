# Pro Observability Plugin

This is the dynamic-plugin home for Pro request-monitoring persistence. It has
no frontend resource and does not replace the current Management UI.

The current migration slice implements the native plugin ABI, receives the
enriched `usage.handle` contract, and can write the existing `usage_events` and
`usage_summary` SQLite schema. It remains opt-in until the current Management
query, import/export, backup, pricing, and reset contracts have parity tests.

Example plugin configuration:

```yaml
plugins:
  enabled: true
  configs:
    pro-observability:
      enabled: true
      priority: 100
      write-enabled: false
      database-path: /CLIProxyAPI/usage/usage.sqlite
```

`write-enabled` defaults to `false`, preventing duplicate writes during the
transition. No host route or persistence patch should be removed until this
plugin becomes the single authoritative writer.
