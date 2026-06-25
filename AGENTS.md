# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Project Nature

This repo is a **customization layer**, NOT a standalone application. It does not contain complete upstream source. It maintains patches, overlays, and build flows that are re-applied onto fresh upstream checkouts at build/release time.

- `cliproxyapi-pro-core/` — patches Go backend (`router-for-me/CLIProxyAPI`) via Docker build
- `cliproxyapi-pro-management/` — overlays React frontend (`router-for-me/Cli-Proxy-API-Management-Center`) via Python script
- `upstreamcode/` — vendored upstream checkouts for reference only; do NOT edit these to change Pro behavior

## Build & Apply (non-standard)

Backend (must run against upstream checkout, requires `SRC_ROOT` env pointing to extracted upstream Go source):
```bash
SRC_ROOT=/tmp/cliproxy-check python3 cliproxyapi-pro-core/patches/apply_upstream_patches.py
go -C /tmp/cliproxy-check mod tidy
go -C /tmp/cliproxy-check test ./internal/embeddedusage/...
# Docker: docker build --build-arg CLIPROXY_VERSION=v7.1.18 -t cliproxyapi-pro ./cliproxyapi-pro-core
```
Frontend (target dir must contain `src/` and `package.json`):
```bash
./cliproxyapi-pro-management/apply.sh /path/to/upstream-checkout
# then in that dir: npm install && npm run type-check && npm run build
```

## Critical Patch Mechanics

- Both patch scripts use **idempotent string-replace helpers** (`replace_once`, `insert_before`, `insert_once`) that silently no-op if the target text already contains the replacement. Patches are order-dependent and fragile to upstream drift.
- `apply_upstream_patches.py` rewrites module import paths (`github.com/router-for-me/CLIProxyAPI/vN` → actual module path from upstream `go.mod`) in every generated/copied Go file, and runs `gofmt` + `go mod tidy` at the end.
- Backend patches hardcode `modernc.org/sqlite v1.51.0` and force `usage-statistics-enabled=true` + `panel-github-repository=ssfun/CLIProxyAPI-Pro` on startup (only writes config.yaml when value differs).
- Frontend `apply_customizations.py` does NOT run a bundler; it only copies overlay files and mutates upstream source in-memory, flushing writes at the end. Locale merges use `monitoring-locales.json` keyed by filename.

## Test Commands

Go (run inside patched upstream checkout):
```bash
go test ./internal/embeddedusage/...
go test ./internal/pluginstore/...
go test ./internal/pluginhost/...
```
Single Go test: `go test -run TestFunctionName ./internal/embeddedusage/`

Frontend has no test runner; validation is `npm run type-check` (strict TS) and `npm run build`. Lint: `npm run lint`.

## Code Style (non-obvious)

- Frontend: Prettier `singleQuote`, `printWidth 100`, `trailingComma: es5`; ESLint warns (not errors) on `@typescript-eslint/no-explicit-any` and unused vars with `argsIgnorePattern: '^_'`. TS `strict` + `noUnusedLocals`/`noUnusedParameters` are errors.
- Go generated files use `gofmt`; the patch script enforces this on specific files only.
- Frontend path alias `@` → `./src`. CSS modules use `camelCase` locals convention. SCSS auto-injects `@/styles/variables.scss`.
- New quota providers must be added in parallel across: `QuotaType` union, quota store, quota configs, locale constants, and the `auth_files` search field keys in `apply_customizations.py`.

## Release

Versions derive from upstream core tag + `-pro` suffix (e.g. `v7.1.18-pro`). Source still comes from upstream `v7.1.18`. Binary asset names keep `CLIProxyAPI` prefix. `_no-plugin` builds are CGO-free static.
