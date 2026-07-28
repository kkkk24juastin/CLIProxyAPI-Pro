# CLIProxyAPI Pro Proxy Pool

`proxy-pool` is a prebundled CLIProxyAPI dynamic plugin. It exposes a loopback-only SOCKS5 listener and selects one configured upstream proxy for each TCP `CONNECT` tunnel. Core keeps using a single fixed `proxy-url`; node rotation and failover remain inside the plugin.

## Runtime model

- Local endpoint: `socks5://127.0.0.1:8318` by default.
- Upstream nodes: HTTP, HTTPS, SOCKS5, or SOCKS5H URLs, including authenticated URLs.
- Strategies: round robin, smooth weighted round robin, or least active connections.
- Failure handling: try additional eligible nodes, isolate repeatedly failing nodes, and run direct per-node health probes that do not use the host HTTP client.
- Leak protection: `fail-open` defaults to `false`; direct fallback must be explicitly enabled.
- Scope: rotation happens per SOCKS5 TCP tunnel. HTTP keep-alive or multiplexed connections can carry multiple HTTP requests through one selected node.
- Overrides: credential-level `proxy-url` values still take precedence over Core's global proxy and therefore bypass this pool.

The plugin and Pro management page never rewrite Core's global `proxy-url`. Configure Core manually to use the fixed endpoint shown by the page, for example `socks5://127.0.0.1:8318`.

## Plugin configuration

```yaml
plugins:
  enabled: true
  configs:
    proxy-pool:
      enabled: true
      priority: 100
      listen: 127.0.0.1:8318
      strategy: round-robin
      dial-timeout: 8s
      max-failover-attempts: 3
      fail-open: false
      health-check:
        enabled: true
        interval: 30s
        timeout: 8s
        isolation-threshold: 3
        isolation-duration: 5m
        probe-address: www.gstatic.com:443
        test-url: https://ipwho.is/
      nodes:
        - id: proxy-a
          label: Proxy A
          url: socks5://user:password@proxy-a.example:1080
          enabled: true
          weight: 1
          order: 10
```

Only numeric loopback listener addresses are accepted. A node URL pointing back to the listener is rejected to prevent recursive proxy loops.

## Packaging

Standard Pro macOS, Windows, and Linux archives bundle the library under `plugins/<goos>/<goarch>/proxy-pool.<extension>`. Docker images bundle Linux amd64/arm64 libraries. FreeBSD and `_no-plugin` archives do not currently bundle this feature.
