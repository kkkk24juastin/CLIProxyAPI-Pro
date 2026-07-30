#!/usr/bin/env python3
import hashlib
import os
import re
import subprocess
from pathlib import Path

ROOT = Path(os.environ.get('SRC_ROOT', '/src/CLIProxyAPI'))
PATCH_SOURCE_DIR = Path(__file__).resolve().parent / 'sources'
PRO_PANEL_REPOSITORY = 'https://github.com/ssfun/CLIProxyAPI-Pro'
PRO_PANEL_RELEASE_API = 'https://api.github.com/repos/ssfun/CLIProxyAPI-Pro/releases/latest'


_writes = {}


def read_text(path: Path) -> str:
    return path.read_text(encoding='utf-8')


def write_text(path: Path, text: str) -> None:
    path.write_text(text, encoding='utf-8')


def read(path: Path) -> str:
    if path in _writes:
        return _writes[path]
    return read_text(path)


def write(path: Path, text: str) -> None:
    _writes[path] = text


def require_source_hash(path: Path, allowed_hashes: set[str]) -> None:
    digest = hashlib.sha256(read(path).encode('utf-8')).hexdigest()
    if digest not in allowed_hashes:
        raise SystemExit(f'upstream source changed before full-file replacement: {path} ({digest})')


def module_path() -> str:
    match = re.search(r'^module\s+(\S+)', read_text(ROOT / 'go.mod'), re.MULTILINE)
    if not match:
        raise SystemExit(f'module path not found in {ROOT / "go.mod"}')
    return match.group(1)


def import_path(suffix: str) -> str:
    return f'{MODULE_PATH}/{suffix}'


def flush_writes() -> None:
    for path, text in _writes.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        write_text(path, text)
    _writes.clear()


def queue_tree(source: Path, target: Path) -> None:
    for source_path in source.rglob('*'):
        if source_path.is_dir():
            continue
        text = read_text(source_path)
        if source_path.suffix == '.go':
            text = re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, text)
        write(target / source_path.relative_to(source), text)


def queue_go_source(relative_path: str) -> None:
    source = PATCH_SOURCE_DIR / relative_path
    if not source.is_file():
        raise SystemExit(f'Go patch source not found: {source}')
    text = re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(source))
    write(ROOT / relative_path, text)


def replace_once(path: Path, old: str, new: str, present=None) -> None:
    text = read(path)
    if (present or new) and (present or new) in text:
        return
    match_count = text.count(old)
    if match_count != 1:
        raise SystemExit(f'expected one pattern in {path}, found {match_count}: {old[:120]!r}')
    write(path, text.replace(old, new, 1))


def insert_before(path: Path, marker: str, insertion: str, present: str) -> None:
    text = read(path)
    if present in text:
        return
    match_count = text.count(marker)
    if match_count != 1:
        raise SystemExit(f'expected one marker in {path}, found {match_count}: {marker[:120]!r}')
    write(path, text.replace(marker, insertion + marker, 1))


def ensure_go_require(path: Path, module: str, version: str) -> None:
    text = read(path)
    if re.search(rf'^\s*{re.escape(module)}\s+', text, re.MULTILINE):
        return
    line = f'\t{module} {version}\n'
    marker = 'require (\n'
    if marker in text:
        write(path, text.replace(marker, marker + line, 1))
        return
    write(path, text.rstrip() + f'\n\nrequire {module} {version}\n')


def insert_before_nth(path: Path, marker: str, insertion: str, occurrence: int, present: str) -> None:
    text = read(path)
    if present in text:
        return
    start = -1
    for _ in range(occurrence):
        start = text.find(marker, start + 1)
        if start < 0:
            raise SystemExit(f'pattern occurrence {occurrence} not found in {path}: {marker[:120]!r}')
    write(path, text[:start] + insertion + text[start:])


def add_go_import(path: Path, after: str, import_line: str) -> None:
    text = read(path)
    if import_line.strip() in text:
        return
    if after not in text:
        raise SystemExit(f'import anchor not found in {path}: {after[:120]!r}')
    write(path, text.replace(after, after + import_line, 1))


def replace_go_function(path: Path, signature: str, new_function: str, present: str) -> None:
    text = read(path)
    if present in text:
        return
    start = text.find(signature)
    if start < 0:
        raise SystemExit(f'function not found in {path}: {signature!r}')
    brace = text.find('{', start)
    if brace < 0:
        raise SystemExit(f'function body not found in {path}: {signature!r}')
    depth = 0
    for index in range(brace, len(text)):
        char = text[index]
        if char == '{':
            depth += 1
        elif char == '}':
            depth -= 1
            if depth == 0:
                end = index + 1
                if end < len(text) and text[end] == '\n':
                    end += 1
                write(path, text[:start] + new_function + text[end:])
                return
    raise SystemExit(f'function body end not found in {path}: {signature!r}')


def replace_go_call_block(path: Path, call_start: str, new_block: str, present: str) -> None:
    text = read(path)
    if present in text:
        return
    start = text.find(call_start)
    if start < 0:
        raise SystemExit(f'call block not found in {path}: {call_start!r}')
    brace = text.find('{', start)
    if brace < 0:
        raise SystemExit(f'call block body not found in {path}: {call_start!r}')
    depth = 0
    for index in range(brace, len(text)):
        char = text[index]
        if char == '{':
            depth += 1
        elif char == '}':
            depth -= 1
            if depth == 0:
                end = index + 1
                while end < len(text) and text[end] in ')\n':
                    end += 1
                    if text[end - 1] == '\n':
                        break
                write(path, text[:start] + new_block + text[end:])
                return
    raise SystemExit(f'call block end not found in {path}: {call_start!r}')


MODULE_PATH = module_path()
customization_sentinel = ROOT / 'internal/embeddedusage'
if customization_sentinel.exists():
    raise SystemExit(f'target already contains CLIProxyAPI Pro customizations: {customization_sentinel}')

new_customization_paths = (
    'internal/api/handlers/management/pro_lifecycle.go',
    'internal/api/handlers/management/plugin_quota.go',
    'internal/api/handlers/management/plugin_quota_test.go',
    'internal/api/handlers/management/routing_policy.go',
    'internal/api/handlers/management/routing_policy_test.go',
    'internal/api/embeddedusage_cutover_test.go',
    'internal/api/usage_stream_bridge.go',
    'internal/api/usage_stream_bridge_test.go',
    'internal/api/management_stream_bridge.go',
    'internal/api/management_stream_bridge_test.go',
    'internal/cmd/pro_plugin_startup_test.go',
    'internal/config/config_existing_updates.go',
    'internal/config/config_existing_updates_test.go',
    'internal/config/pro_plugin_defaults.go',
    'internal/config/pro_plugin_defaults_test.go',
    'internal/pluginhost/gemini_cli_quota_legacy.go',
    'internal/pluginhost/gemini_cli_quota_legacy_test.go',
    'internal/pluginhost/gemini_cli_storage_compat.go',
    'internal/pluginhost/gemini_cli_storage_compat_test.go',
    'internal/pluginhost/quota_provider.go',
    'internal/pluginhost/quota_provider_test.go',
    'internal/pluginhost/auth_model_filter.go',
    'internal/pluginhost/auth_model_filter_test.go',
    'internal/pluginhost/management_call.go',
    'internal/pluginhost/management_call_test.go',
    'internal/pluginhost/pro_backup_callbacks.go',
    'internal/pluginhost/pro_backup_callbacks_test.go',
    'internal/pluginhost/account_inspection_callbacks.go',
    'internal/pluginhost/account_inspection_callbacks_test.go',
    'internal/pluginhost/pro_observability_integration_test.go',
    'internal/pluginhost/pro_observability_ready.go',
    'internal/pluginhost/pro_observability_ready_test.go',
    'internal/pluginhost/pro_settings_store.go',
    'internal/pluginhost/pro_settings_store_test.go',
    'internal/pluginhost/quota_cache_store.go',
    'internal/pluginhost/quota_cache_store_test.go',
    'internal/pluginhost/runtime_state_store.go',
    'internal/pluginhost/runtime_state_store_test.go',
    'internal/pluginhost/usage_metadata_test.go',
    'internal/pluginstore/autoinstall.go',
    'internal/pluginstore/autoinstall_test.go',
    'internal/requestmeta/client.go',
    'internal/requestmeta/client_test.go',
    'internal/requestmeta/requestid.go',
    'internal/requestmeta/response.go',
    'internal/runtime/executor/xai_quota_observer.go',
    'sdk/cliproxy/auth/auth_runtime_state.go',
    'sdk/cliproxy/auth/auth_runtime_state_test.go',
    'sdk/cliproxy/auth/inspection_refresh.go',
)
for relative_path in new_customization_paths:
    target_path = ROOT / relative_path
    if target_path.exists():
        raise SystemExit(f'upstream path collides with a Pro customization: {target_path}')

write(
    ROOT / 'internal/runtime/executor/xai_quota_observer.go',
    re.sub(
        r'github\.com/router-for-me/CLIProxyAPI/v\d+',
        MODULE_PATH,
        read_text(Path(__file__).resolve().parent / 'xai_quota_observer.go'),
    ),
)

xai_executor_sources = (
    ROOT / 'internal/runtime/executor/xai_executor_execute.go',
    ROOT / 'internal/runtime/executor/xai_executor_stream.go',
    ROOT / 'internal/runtime/executor/xai_executor_media.go',
)
xai_observation_marker = '\thelps.AppendAPIResponseChunk(ctx, e.cfg, data)\n'
xai_executor_texts = {path: read(path) for path in xai_executor_sources}
if not any(
    'observeXAIQuotaResponse(ctx, auth, req.Model' in text
    for text in xai_executor_texts.values()
):
    if sum(text.count(xai_observation_marker) for text in xai_executor_texts.values()) != 6:
        raise SystemExit('unexpected xAI response chunk observation anchors')
    xai_metadata_marker = '\thelps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())\n'
    if sum(text.count(xai_metadata_marker) for text in xai_executor_texts.values()) != 5:
        raise SystemExit('unexpected xAI response metadata observation anchors')
    for path, text in xai_executor_texts.items():
        text = text.replace(
            xai_observation_marker,
            xai_observation_marker + '\tobserveXAIQuotaResponse(ctx, auth, req.Model, httpResp.StatusCode, httpResp.Header.Clone(), data)\n',
        )
        text = text.replace(
            xai_metadata_marker,
            xai_metadata_marker + '\tobserveXAIQuotaResponse(ctx, auth, req.Model, httpResp.StatusCode, httpResp.Header.Clone(), nil)\n',
        )
        write(path, text)

xai_websocket_executor = ROOT / 'internal/runtime/executor/xai_websockets_executor.go'
replace_once(
    xai_websocket_executor,
    '''\t\tif respHS != nil && respHS.StatusCode > 0 {
\t\t\tif sess != nil {
''',
    '''\t\tif respHS != nil && respHS.StatusCode > 0 {
\t\t\tobserveXAIQuotaResponse(ctx, auth, req.Model, respHS.StatusCode, respHS.Header.Clone(), bodyErr)
\t\t\tif sess != nil {
''',
    'observeXAIQuotaResponse(ctx, auth, req.Model, respHS.StatusCode',
)
replace_once(
    xai_websocket_executor,
    '''\t\t\t\tif respHSRetry != nil && respHSRetry.StatusCode > 0 {
\t\t\t\t\treturn nil, xaiStatusErr(respHSRetry.StatusCode, bodyErrRetry)
''',
    '''\t\t\t\tif respHSRetry != nil && respHSRetry.StatusCode > 0 {
\t\t\t\t\tobserveXAIQuotaResponse(ctx, auth, req.Model, respHSRetry.StatusCode, respHSRetry.Header.Clone(), bodyErrRetry)
\t\t\t\t\treturn nil, xaiStatusErr(respHSRetry.StatusCode, bodyErrRetry)
''',
    'observeXAIQuotaResponse(ctx, auth, req.Model, respHSRetry.StatusCode',
)
replace_once(
    xai_websocket_executor,
    '''\t\t\thelps.AppendAPIWebsocketResponse(ctx, e.cfg, payload)

\t\t\tif wsErr, ok := parseXAIWebsocketError(payload); ok {
''',
    '''\t\t\thelps.AppendAPIWebsocketResponse(ctx, e.cfg, payload)
\t\t\tobserveXAIQuotaResponse(ctx, auth, req.Model, 0, nil, payload)

\t\t\tif wsErr, ok := parseXAIWebsocketError(payload); ok {
''',
    'observeXAIQuotaResponse(ctx, auth, req.Model, 0, nil, payload)',
)
replace_once(
    xai_websocket_executor,
    '''\trecordAPIWebsocketHandshake(ctx, e.cfg, respHS)
\treporter.StartResponseTTFT()
''',
    '''\trecordAPIWebsocketHandshake(ctx, e.cfg, respHS)
\tif respHS != nil {
\t\tobserveXAIQuotaResponse(ctx, auth, req.Model, respHS.StatusCode, respHS.Header.Clone(), nil)
\t}
\treporter.StartResponseTTFT()
''',
    'observeXAIQuotaResponse(ctx, auth, req.Model, respHS.StatusCode, respHS.Header.Clone(), nil)',
)
replace_once(
    xai_websocket_executor,
    '''\t\t\trecordAPIWebsocketHandshake(ctx, e.cfg, respHSRetry)
\t\t\treporter.StartResponseTTFT()
''',
    '''\t\t\trecordAPIWebsocketHandshake(ctx, e.cfg, respHSRetry)
\t\t\tif respHSRetry != nil {
\t\t\t\tobserveXAIQuotaResponse(ctx, auth, req.Model, respHSRetry.StatusCode, respHSRetry.Header.Clone(), nil)
\t\t\t}
\t\t\treporter.StartResponseTTFT()
''',
    'observeXAIQuotaResponse(ctx, auth, req.Model, respHSRetry.StatusCode, respHSRetry.Header.Clone(), nil)',
)

require_source_hash(
    ROOT / 'internal/logging/requestid.go',
    {'69d256ca4c4a75759395f6ed6640e9d2673c777e111fcae80faa7b6eea5b15ac'},
)
require_source_hash(
    ROOT / 'internal/logging/requestmeta.go',
    {'276db8481b85d9909073ed823062a2c83fcbf38b68952486fb86712cfb99ce9d'},
)

# Add the optional QuotaProvider capability without changing ABI/schema v1.
pluginapi_types = ROOT / 'sdk/pluginapi/types.go'
insert_before(
    pluginapi_types,
    '// UsageRecord describes request usage and billing metadata.\n',
    '''// UsageRecordContractVersion identifies the extended observability wire contract.
const UsageRecordContractVersion = 2

''',
    'const UsageRecordContractVersion = 2',
)
replace_once(
    pluginapi_types,
    '\t// Provider identifies the upstream provider.\n',
    '''\t// ContractVersion identifies the populated usage wire contract.\n\tContractVersion int\n\t// Provider identifies the upstream provider.\n''',
    'ContractVersion identifies the populated usage wire contract',
)
replace_once(
    pluginapi_types,
    '\t// ReasoningEffort records the requested reasoning effort.\n',
    '''\t// RequestID identifies the downstream request.\n\tRequestID string\n\t// Endpoint is the normalized downstream method and route.\n\tEndpoint string\n\t// ClientIP is the direct downstream client address.\n\tClientIP string\n\t// XForwardedFor is the downstream forwarding chain.\n\tXForwardedFor string\n\t// UserAgent is the downstream client user agent.\n\tUserAgent string\n\t// ReasoningEffort records the requested reasoning effort.\n''',
    'RequestID identifies the downstream request',
)
replace_once(
    pluginapi_types,
    '\t// Generate reports whether the client requested actual generation.\n',
    '''\t// ResponseServiceTier records the tier reported by the upstream response.\n\tResponseServiceTier string\n\t// Stream reports whether the downstream request used streaming semantics.\n\tStream bool\n\t// AttemptIndex is the zero-based upstream attempt number when instrumented.\n\tAttemptIndex *int64\n\t// Generate reports whether the client requested actual generation.\n''',
    'ResponseServiceTier records the tier reported',
)
replace_once(
    pluginapi_types,
    '\t// TotalTokens is the total token count.\n\tTotalTokens int64\n',
    '''\t// TotalTokens is the total token count.\n\tTotalTokens int64\n\t// TokenBreakdown is the canonical non-overlapping accounting contract.\n\tTokenBreakdown UsageTokenBreakdown\n''',
    'TokenBreakdown UsageTokenBreakdown',
)
insert_before(
    pluginapi_types,
    '// UsageDetail contains token accounting counters.\n',
    '''// UsageTokenInputBreakdown contains mutually exclusive input buckets.\ntype UsageTokenInputBreakdown struct {\n\tTotalTokens      int64 `json:"total_tokens"`\n\tUncachedTokens   int64 `json:"uncached_tokens"`\n\tCacheReadTokens  int64 `json:"cache_read_tokens"`\n\tCacheWriteTokens int64 `json:"cache_write_tokens"`\n}\n\n// UsageTokenOutputBreakdown contains mutually exclusive output buckets.\ntype UsageTokenOutputBreakdown struct {\n\tTotalTokens        int64 `json:"total_tokens"`\n\tNonReasoningTokens int64 `json:"non_reasoning_tokens"`\n\tReasoningTokens    int64 `json:"reasoning_tokens"`\n}\n\n// UsageTokenBreakdown is a stable plugin-facing token accounting snapshot.\ntype UsageTokenBreakdown struct {\n\tSchemaVersion      int                       `json:"schema_version"`\n\tQuality            string                    `json:"quality"`\n\tTotalTokens        int64                     `json:"total_tokens"`\n\tInput              UsageTokenInputBreakdown  `json:"input"`\n\tOutput             UsageTokenOutputBreakdown `json:"output"`\n\tUnclassifiedTokens int64                     `json:"unclassified_tokens"`\n}\n\n''',
    'type UsageTokenBreakdown struct',
)
replace_once(
    pluginapi_types,
    '\t// FrontendAuthProvider authenticates frontend requests before proxy handling.\n',
    '\t// QuotaProvider fetches normalized per-auth quota and subscription snapshots.\n\tQuotaProvider QuotaProvider\n\t// QuotaCacheStore owns persisted quota snapshots and response observations.\n\tQuotaCacheStore QuotaCacheStore\n\t// RuntimeStateStore owns persisted per-auth scheduler statistics.\n\tRuntimeStateStore RuntimeStateStore\n\t// ProSettingsStore owns versioned Pro settings persisted outside config.yaml.\n\tProSettingsStore ProSettingsStore\n\t// FrontendAuthProvider authenticates frontend requests before proxy handling.\n',
    'QuotaProvider QuotaProvider',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_quota_api.go'),
    'type QuotaProvider interface',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_quota_cache_api.go'),
    'type QuotaCacheStore interface',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_runtime_state_api.go'),
    'type RuntimeStateStore interface',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_pro_settings_api.go'),
    'type ProSettingsStore interface',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_account_inspection_host_api.go'),
    'type HostAuthRefreshRequest struct',
)
replace_once(
    pluginapi_types,
    '\t// Email is the credential email when available.\n',
    '''\t// Revision changes whenever host-managed credential state changes.\n\tRevision int64 `json:"revision,omitempty"`\n\t// NextRefreshAfter is the next provider refresh eligibility time.\n\tNextRefreshAfter time.Time `json:"next_refresh_after,omitempty"`\n\t// DisplayName is a non-secret provider account label.\n\tDisplayName string `json:"display_name,omitempty"`\n\t// AccountID is the non-secret provider account identifier.\n\tAccountID string `json:"account_id,omitempty"`\n\t// UserID is the non-secret provider user identifier.\n\tUserID string `json:"user_id,omitempty"`\n\t// PlanType is the provider account plan identifier when available.\n\tPlanType string `json:"plan_type,omitempty"`\n\t// UsingAPI reports provider-specific official API routing when configured.\n\tUsingAPI *bool `json:"using_api,omitempty"`\n\t// VirtualSource reports that this projection came from a plugin-expanded auth source.\n\tVirtualSource string `json:"virtual_source,omitempty"`\n\t// InspectionMetadata contains an explicit non-secret provider metadata allowlist.\n\tInspectionMetadata map[string]any `json:"inspection_metadata,omitempty"`\n\t// InspectionAttributes contains an explicit non-secret attribute allowlist.\n\tInspectionAttributes map[string]string `json:"inspection_attributes,omitempty"`\n\t// InspectionUserAgent preserves the host's provider-specific probe header default.\n\tInspectionUserAgent string `json:"inspection_user_agent,omitempty"`\n\t// InspectionError is present only for health errors owned by account inspection.\n\tInspectionError *HostAuthHealthError `json:"inspection_error,omitempty"`\n\t// Email is the credential email when available.\n''',
    'Revision changes whenever host-managed credential state changes',
)

usage_translation = ROOT / 'internal/pluginhost/adapters_usage_translation.go'
add_go_import(
    usage_translation,
    f'\t"{import_path("internal/registry")}"\n',
    f'\t"{import_path("internal/requestmeta")}"\n',
)
replace_go_function(
    usage_translation,
    'func (a *usageAdapter) HandleUsage(ctx context.Context, record coreusage.Record)',
    '''func (a *usageAdapter) HandleUsage(ctx context.Context, record coreusage.Record) {
\tif a == nil {
\t\treturn
\t}
\tplugin := a.host.currentUsagePlugin(a.pluginID)
\tif plugin == nil {
\t\treturn
\t}
\tdefer func() {
\t\tif recovered := recover(); recovered != nil {
\t\t\ta.host.fusePlugin(a.pluginID, "UsagePlugin.HandleUsage", recovered)
\t\t}
\t}()
\tclient := requestmeta.GetClientRequestMetadata(ctx)
\tdetail := coreusage.EnsureTokenBreakdownForProvider(record.Detail, record.Provider, record.ExecutorType)
\tserviceTier := strings.TrimSpace(record.ServiceTier)
\tif serviceTier == "" {
\t\tserviceTier = strings.TrimSpace(record.RequestServiceTier)
\t}
\tif serviceTier == "" {
\t\tserviceTier = coreusage.ServiceTierFromContext(ctx)
\t}
\treasoningEffort := strings.TrimSpace(record.ReasoningEffort)
\tif reasoningEffort == "" {
\t\treasoningEffort = coreusage.ReasoningEffortFromContext(ctx)
\t}
\tresponseServiceTier := strings.TrimSpace(record.ResponseServiceTier)
\tif responseServiceTier == "" {
\t\tresponseServiceTier = strings.TrimSpace(detail.ResponseServiceTier)
\t}
\tfailed := record.Failed
\tstatusCode := record.Fail.StatusCode
\tif responseStatus := requestmeta.GetResponseStatus(ctx); responseStatus >= 400 {
\t\tfailed = true
\t\tif statusCode <= 0 {
\t\t\tstatusCode = responseStatus
\t\t}
\t}
\tresponseHeaders := cloneHeader(record.ResponseHeaders)
\tif len(responseHeaders) == 0 {
\t\tresponseHeaders = cloneHeader(requestmeta.GetResponseHeaders(ctx))
\t}
\tplugin.HandleUsage(ctx, pluginapi.UsageRecord{
\t\tContractVersion: pluginapi.UsageRecordContractVersion,
\t\tProvider: record.Provider, ExecutorType: record.ExecutorType, Model: record.Model,
\t\tAlias: record.Alias, APIKey: record.APIKey, AuthID: record.AuthID,
\t\tAuthIndex: record.AuthIndex, AuthType: record.AuthType, Source: record.Source,
\t\tRequestID: requestmeta.GetRequestID(ctx), Endpoint: requestmeta.GetEndpoint(ctx),
\t\tClientIP: client.ClientIP, XForwardedFor: client.XForwardedFor, UserAgent: client.UserAgent,
\t\tReasoningEffort: reasoningEffort, ServiceTier: serviceTier,
\t\tResponseServiceTier: responseServiceTier, Stream: coreusage.StreamFromContext(ctx),
\t\tAttemptIndex: record.AttemptIndex, Generate: coreusage.GenerateEnabled(record.Generate),
\t\tRequestedAt: record.RequestedAt, Latency: record.Latency, TTFT: record.TTFT,
\t\tFailed: failed,
\t\tFailure: pluginapi.UsageFailure{StatusCode: statusCode, Body: record.Fail.Body},
\t\tDetail: pluginapi.UsageDetail{
\t\t\tInputTokens: detail.InputTokens, OutputTokens: detail.OutputTokens,
\t\t\tReasoningTokens: detail.ReasoningTokens, CachedTokens: detail.CachedTokens,
\t\t\tCacheReadTokens: detail.CacheReadTokens, CacheCreationTokens: detail.CacheCreationTokens,
\t\t\tTotalTokens: detail.TotalTokens,
\t\t\tTokenBreakdown: pluginapi.UsageTokenBreakdown{
\t\t\t\tSchemaVersion: detail.TokenBreakdown.SchemaVersion,
\t\t\t\tQuality: string(detail.TokenBreakdown.Quality),
\t\t\t\tTotalTokens: detail.TokenBreakdown.TotalTokens,
\t\t\t\tInput: pluginapi.UsageTokenInputBreakdown{
\t\t\t\t\tTotalTokens: detail.TokenBreakdown.Input.TotalTokens,
\t\t\t\t\tUncachedTokens: detail.TokenBreakdown.Input.UncachedTokens,
\t\t\t\t\tCacheReadTokens: detail.TokenBreakdown.Input.CacheReadTokens,
\t\t\t\t\tCacheWriteTokens: detail.TokenBreakdown.Input.CacheWriteTokens,
\t\t\t\t},
\t\t\t\tOutput: pluginapi.UsageTokenOutputBreakdown{
\t\t\t\t\tTotalTokens: detail.TokenBreakdown.Output.TotalTokens,
\t\t\t\t\tNonReasoningTokens: detail.TokenBreakdown.Output.NonReasoningTokens,
\t\t\t\t\tReasoningTokens: detail.TokenBreakdown.Output.ReasoningTokens,
\t\t\t\t},
\t\t\t\tUnclassifiedTokens: detail.TokenBreakdown.UnclassifiedTokens,
\t\t\t},
\t\t},
\t\tResponseHeaders: responseHeaders,
\t})
}

''',
    'RequestID: requestmeta.GetRequestID(ctx)',
)
replace_once(
    pluginapi_types,
    '\t// ModelRegistrar contributes development-time model metadata to the host registry.\n',
    '\t// AuthModelFilter subtracts models from one concrete auth registration.\n\tAuthModelFilter AuthModelFilter\n\t// ModelRegistrar contributes development-time model metadata to the host registry.\n',
    'AuthModelFilter AuthModelFilter',
)
insert_before(
    pluginapi_types,
    '// ModelRegistrar registers plugin-provided models with the host.\n',
    read_text(Path(__file__).resolve().parent / 'plugin_auth_model_filter_api.go'),
    'type AuthModelFilter interface',
)

pluginabi_types = ROOT / 'sdk/pluginabi/types.go'
replace_once(
    pluginabi_types,
    '\tMethodAuthRefresh    = "auth.refresh"\n',
    '\tMethodAuthRefresh    = "auth.refresh"\n\n\tMethodQuotaIdentifier   = "quota.identifier"\n\tMethodQuotaFetch        = "quota.fetch"\n\tMethodQuotaCacheGet     = "quota_cache.get"\n\tMethodQuotaCachePut     = "quota_cache.put"\n\tMethodQuotaCacheDelete  = "quota_cache.delete"\n\tMethodQuotaCacheObserve = "quota_cache.observe"\n\tMethodRuntimeStatsGet    = "runtime_state.auth_stats.get"\n\tMethodRuntimeStatsPut    = "runtime_state.auth_stats.put"\n\tMethodRuntimeStateDelete = "runtime_state.auth.delete"\n\tMethodProSettingGet      = "pro_settings.get"\n\tMethodProSettingPut      = "pro_settings.put"\n\tMethodHostProBackupImport = "host.pro_backup.import"\n\n\tMethodAuthModelFilter = "model.filter_for_auth"\n',
    'MethodQuotaIdentifier',
)
replace_once(
    pluginabi_types,
    '\tMethodHostAuthSave           = "host.auth.save"\n',
    '''\tMethodHostAuthSave           = "host.auth.save"\n\tMethodHostAuthInspectionList = "host.auth.inspection_list"\n\tMethodHostAuthRefresh        = "host.auth.refresh"\n\tMethodHostAuthHTTPDo         = "host.auth.http.do"\n\tMethodHostAuthHealthPatch    = "host.auth.health.patch"\n\tMethodHostAuthDelete         = "host.auth.delete"\n\tMethodHostAuthQuotaFetch     = "host.auth.quota.fetch"\n''',
    'MethodHostAuthInspectionList',
)

host_callbacks = ROOT / 'internal/pluginhost/host_callbacks.go'
replace_once(
    host_callbacks,
    '''\tcase pluginabi.MethodHostAuthSave:
\t\treturn h.callHostAuthSave(ctx, request)
\tdefault:
''',
    '''\tcase pluginabi.MethodHostAuthSave:
\t\treturn h.callHostAuthSave(ctx, request)
\tcase pluginabi.MethodHostAuthInspectionList:
\t\treturn h.callHostAuthInspectionList(ctx, request)
\tcase pluginabi.MethodHostAuthRefresh:
\t\treturn h.callHostAuthRefresh(ctx, request)
\tcase pluginabi.MethodHostAuthHTTPDo:
\t\treturn h.callHostAuthHTTPDo(ctx, request)
\tcase pluginabi.MethodHostAuthHealthPatch:
\t\treturn h.callHostAuthHealthPatch(ctx, request)
\tcase pluginabi.MethodHostAuthDelete:
\t\treturn h.callHostAuthDelete(ctx, request)
\tcase pluginabi.MethodHostAuthQuotaFetch:
\t\treturn h.callHostAuthQuotaFetch(ctx, request)
\tcase pluginabi.MethodHostProBackupImport:
\t\treturn h.callHostProBackupImport(ctx, request)
\tdefault:
''',
    'case pluginabi.MethodHostProBackupImport:',
)

rpc_schema = ROOT / 'internal/pluginhost/rpc_schema.go'
replace_once(
    rpc_schema,
    '\tAuthProvider                  bool                         `json:"auth_provider"`\n',
    '\tAuthProvider                  bool                         `json:"auth_provider"`\n\tQuotaProvider                 bool                         `json:"quota_provider"`\n\tQuotaCacheStore               bool                         `json:"quota_cache_store"`\n\tRuntimeStateStore             bool                         `json:"runtime_state_store"`\n\tProSettingsStore              bool                         `json:"pro_settings_store"`\n\tAuthModelFilter               bool                         `json:"auth_model_filter"`\n',
    'QuotaProvider                 bool',
)
insert_before(
    rpc_schema,
    'type rpcAuthModelRequest struct {\n',
    '''type rpcQuotaFetchRequest struct {
\tpluginapi.QuotaFetchRequest
\tHostCallbackID string `json:"host_callback_id,omitempty"`
}

''',
    'type rpcQuotaFetchRequest struct',
)
insert_before(
    rpc_schema,
    'type rpcAuthModelRequest struct {\n',
    '''type rpcAuthModelFilterRequest struct {
\tpluginapi.AuthModelFilterRequest
\tHostCallbackID string `json:"host_callback_id,omitempty"`
}

''',
    'type rpcAuthModelFilterRequest struct',
)
replace_once(
    rpc_schema,
    '\t\tAuthProvider:                  caps.AuthProvider != nil,\n',
    '\t\tAuthProvider:                  caps.AuthProvider != nil,\n\t\tQuotaProvider:                 caps.QuotaProvider != nil,\n\t\tQuotaCacheStore:               caps.QuotaCacheStore != nil,\n\t\tRuntimeStateStore:             caps.RuntimeStateStore != nil,\n\t\tProSettingsStore:              caps.ProSettingsStore != nil,\n\t\tAuthModelFilter:               caps.AuthModelFilter != nil,\n',
    'QuotaProvider:                 caps.QuotaProvider != nil',
)

rpc_client = ROOT / 'internal/pluginhost/rpc_client.go'
insert_before(
    rpc_client,
    'type rpcFrontendAuthProvider struct {\n',
    '''type rpcQuotaProvider struct {
\t*rpcPluginAdapter
}

''',
    'type rpcQuotaProvider struct',
)
insert_before(
    rpc_client,
    'type rpcFrontendAuthProvider struct {\n',
    '''type rpcQuotaCacheStore struct {
\t*rpcPluginAdapter
}

''',
    'type rpcQuotaCacheStore struct',
)
insert_before(
    rpc_client,
    'type rpcFrontendAuthProvider struct {\n',
    '''type rpcRuntimeStateStore struct {
\t*rpcPluginAdapter
}

''',
    'type rpcRuntimeStateStore struct',
)
insert_before(
    rpc_client,
    'type rpcFrontendAuthProvider struct {\n',
    '''type rpcProSettingsStore struct {
\t*rpcPluginAdapter
}

''',
    'type rpcProSettingsStore struct',
)
insert_before(
    rpc_client,
    'type rpcFrontendAuthProvider struct {\n',
    '''type rpcAuthModelFilter struct {
\t*rpcPluginAdapter
}

''',
    'type rpcAuthModelFilter struct',
)
replace_once(
    rpc_client,
    '''\tif resp.Capabilities.FrontendAuthProvider {
\t\tplugin.Capabilities.FrontendAuthProvider = rpcFrontendAuthProvider{rpcPluginAdapter: adapter}
\t}
''',
    '''\tif resp.Capabilities.AuthModelFilter {
\t\tplugin.Capabilities.AuthModelFilter = rpcAuthModelFilter{rpcPluginAdapter: adapter}
\t}
\tif resp.Capabilities.QuotaProvider {
\t\tplugin.Capabilities.QuotaProvider = rpcQuotaProvider{rpcPluginAdapter: adapter}
\t}
\tif resp.Capabilities.QuotaCacheStore {
\t\tplugin.Capabilities.QuotaCacheStore = rpcQuotaCacheStore{rpcPluginAdapter: adapter}
\t}
\tif resp.Capabilities.RuntimeStateStore {
\t\tplugin.Capabilities.RuntimeStateStore = rpcRuntimeStateStore{rpcPluginAdapter: adapter}
\t}
\tif resp.Capabilities.ProSettingsStore {
\t\tplugin.Capabilities.ProSettingsStore = rpcProSettingsStore{rpcPluginAdapter: adapter}
\t}
\tif resp.Capabilities.FrontendAuthProvider {
\t\tplugin.Capabilities.FrontendAuthProvider = rpcFrontendAuthProvider{rpcPluginAdapter: adapter}
\t}
''',
    'plugin.Capabilities.AuthModelFilter = rpcAuthModelFilter',
)
replace_once(
    rpc_client,
    '''\tcase pluginapi.AuthRefreshRequest:
\t\treq.HTTPClient = nil
\t\treturn req
''',
    '''\tcase pluginapi.AuthRefreshRequest:
\t\treq.HTTPClient = nil
\t\treturn req
\tcase pluginapi.QuotaFetchRequest:
\t\treq.HTTPClient = nil
\t\treturn req
\tcase rpcQuotaFetchRequest:
\t\treq.HTTPClient = nil
\t\treturn req
\tcase pluginapi.AuthModelFilterRequest:
\t\treq.HTTPClient = nil
\t\treturn req
\tcase rpcAuthModelFilterRequest:
\t\treq.HTTPClient = nil
\t\treturn req
''',
    'case pluginapi.QuotaFetchRequest:',
)
insert_before(
    rpc_client,
    'func sanitizePluginMetadata(src map[string]any) map[string]any {\n',
    '''func (p rpcQuotaProvider) Identifier() string {
\treturn callPluginIdentifier(p.client, pluginabi.MethodQuotaIdentifier)
}

func (p rpcQuotaProvider) FetchQuota(ctx context.Context, req pluginapi.QuotaFetchRequest) (pluginapi.QuotaFetchResponse, error) {
\tcallbackID, closeCallback := p.openHostCallbackContext(ctx)
\tdefer closeCallback()
\treturn callPlugin[pluginapi.QuotaFetchResponse](ctx, p.client, pluginabi.MethodQuotaFetch, rpcQuotaFetchRequest{
\t\tQuotaFetchRequest: req,
\t\tHostCallbackID:    callbackID,
\t})
}

''',
    'func (p rpcQuotaProvider) FetchQuota',
)
insert_before(
    rpc_client,
    'func sanitizePluginMetadata(src map[string]any) map[string]any {\n',
    '''func (p rpcQuotaCacheStore) GetQuotaCache(ctx context.Context, req pluginapi.QuotaCacheGetRequest) (pluginapi.QuotaCacheGetResponse, error) {
\treturn callPlugin[pluginapi.QuotaCacheGetResponse](ctx, p.client, pluginabi.MethodQuotaCacheGet, req)
}

func (p rpcQuotaCacheStore) PutQuotaCache(ctx context.Context, req pluginapi.QuotaCachePutRequest) (pluginapi.QuotaCachePutResponse, error) {
\treturn callPlugin[pluginapi.QuotaCachePutResponse](ctx, p.client, pluginabi.MethodQuotaCachePut, req)
}

func (p rpcQuotaCacheStore) DeleteQuotaCache(ctx context.Context, req pluginapi.QuotaCacheDeleteRequest) (pluginapi.QuotaCacheDeleteResponse, error) {
\treturn callPlugin[pluginapi.QuotaCacheDeleteResponse](ctx, p.client, pluginabi.MethodQuotaCacheDelete, req)
}

func (p rpcQuotaCacheStore) ObserveQuota(ctx context.Context, req pluginapi.QuotaObservationRequest) (pluginapi.QuotaObservationResponse, error) {
\treturn callPlugin[pluginapi.QuotaObservationResponse](ctx, p.client, pluginabi.MethodQuotaCacheObserve, req)
}

''',
    'func (p rpcQuotaCacheStore) GetQuotaCache',
)
insert_before(
    rpc_client,
    'func sanitizePluginMetadata(src map[string]any) map[string]any {\n',
    '''func (p rpcRuntimeStateStore) GetAuthRuntimeStats(ctx context.Context, req pluginapi.AuthRuntimeStatsGetRequest) (pluginapi.AuthRuntimeStatsGetResponse, error) {
\treturn callPlugin[pluginapi.AuthRuntimeStatsGetResponse](ctx, p.client, pluginabi.MethodRuntimeStatsGet, req)
}

func (p rpcRuntimeStateStore) PutAuthRuntimeStats(ctx context.Context, req pluginapi.AuthRuntimeStatsPutRequest) (pluginapi.AuthRuntimeStatsPutResponse, error) {
\treturn callPlugin[pluginapi.AuthRuntimeStatsPutResponse](ctx, p.client, pluginabi.MethodRuntimeStatsPut, req)
}

func (p rpcRuntimeStateStore) DeleteAuthRuntimeState(ctx context.Context, req pluginapi.AuthRuntimeStateDeleteRequest) (pluginapi.AuthRuntimeStateDeleteResponse, error) {
\treturn callPlugin[pluginapi.AuthRuntimeStateDeleteResponse](ctx, p.client, pluginabi.MethodRuntimeStateDelete, req)
}

''',
    'func (p rpcRuntimeStateStore) GetAuthRuntimeStats',
)
insert_before(
    rpc_client,
    'func sanitizePluginMetadata(src map[string]any) map[string]any {\n',
    '''func (p rpcProSettingsStore) GetProSetting(ctx context.Context, req pluginapi.ProSettingGetRequest) (pluginapi.ProSettingGetResponse, error) {
\treturn callPlugin[pluginapi.ProSettingGetResponse](ctx, p.client, pluginabi.MethodProSettingGet, req)
}

func (p rpcProSettingsStore) PutProSetting(ctx context.Context, req pluginapi.ProSettingPutRequest) (pluginapi.ProSettingPutResponse, error) {
\treturn callPlugin[pluginapi.ProSettingPutResponse](ctx, p.client, pluginabi.MethodProSettingPut, req)
}

''',
    'func (p rpcProSettingsStore) GetProSetting',
)
insert_before(
    rpc_client,
    'func sanitizePluginMetadata(src map[string]any) map[string]any {\n',
    '''func (p rpcAuthModelFilter) FilterAuthModels(ctx context.Context, req pluginapi.AuthModelFilterRequest) (pluginapi.AuthModelFilterResponse, error) {
\tcallbackID, closeCallback := p.openHostCallbackContext(ctx)
\tdefer closeCallback()
\treturn callPlugin[pluginapi.AuthModelFilterResponse](ctx, p.client, pluginabi.MethodAuthModelFilter, rpcAuthModelFilterRequest{
\t\tAuthModelFilterRequest: req,
\t\tHostCallbackID:         callbackID,
\t})
}

''',
    'func (p rpcAuthModelFilter) FilterAuthModels',
)

plugin_host = ROOT / 'internal/pluginhost/host.go'
replace_once(
    plugin_host,
    '\t\tcaps.AuthProvider != nil ||\n',
    '\t\tcaps.AuthProvider != nil ||\n\t\tcaps.QuotaProvider != nil ||\n\t\tcaps.QuotaCacheStore != nil ||\n\t\tcaps.RuntimeStateStore != nil ||\n\t\tcaps.ProSettingsStore != nil ||\n\t\tcaps.AuthModelFilter != nil ||\n',
    'caps.ProSettingsStore != nil',
)
replace_once(
    plugin_host,
    '''\tdefer h.unlockApply()

\ttargets := make([]pluginUnloadTarget, 0)
\tvar loading map[string]*pluginLoadRequest
''',
    '''\tdefer h.unlockApply()

\tdetachProObservabilityBackends(h)
\ttargets := make([]pluginUnloadTarget, 0)
\tvar loading map[string]*pluginLoadRequest
''',
    'detachProObservabilityBackends(h)',
)

plugin_snapshot = ROOT / 'internal/pluginhost/snapshot.go'
replace_once(
    plugin_snapshot,
    '\tOAuthProvider string\n',
    '\tOAuthProvider string\n\tSupportsQuota bool\n\tQuotaProvider string\n\tQuotaMode     string\n',
    'SupportsQuota bool',
)
replace_once(
    plugin_snapshot,
    '''\t\tout = append(out, RegisteredPluginInfo{
''',
    '''\t\tquotaProvider := record.plugin.Capabilities.QuotaProvider
\t\tquotaProviderID := ""
\t\tquotaMode := ""
\t\tsupportsQuota := quotaProvider != nil
\t\tif quotaProvider != nil && !h.isPluginFused(record.id) {
\t\t\tif identifier, okIdentifier := h.callQuotaProviderIdentifier(record.id, quotaProvider); okIdentifier {
\t\t\t\tquotaProviderID = identifier
\t\t\t\tquotaMode = "native"
\t\t\t}
\t\t} else if identifier, okLegacy := h.legacyQuotaProviderForRecord(record); okLegacy {
\t\t\tquotaProviderID = identifier
\t\t\tquotaMode = "legacy-adapter"
\t\t\tsupportsQuota = true
\t\t}
\t\tout = append(out, RegisteredPluginInfo{
''',
    'quotaProvider := record.plugin.Capabilities.QuotaProvider',
)
replace_once(
    plugin_snapshot,
    '\t\t\tOAuthProvider: oauthProvider,\n',
    '\t\t\tOAuthProvider: oauthProvider,\n\t\t\tSupportsQuota: supportsQuota,\n\t\t\tQuotaProvider: quotaProviderID,\n\t\t\tQuotaMode: quotaMode,\n',
    'QuotaProvider: quotaProviderID',
)

management_plugins = ROOT / 'internal/api/handlers/management/plugins.go'
replace_once(
    management_plugins,
    '\tOAuthProvider    string                  `json:"oauth_provider"`\n',
    '\tOAuthProvider    string                  `json:"oauth_provider"`\n\tSupportsQuota    bool                    `json:"supports_quota"`\n\tQuotaProvider    string                  `json:"quota_provider"`\n\tQuotaMode        string                  `json:"quota_mode"`\n',
    'SupportsQuota    bool',
)
replace_once(
    management_plugins,
    '\t\t\tentry.OAuthProvider = htmlsanitize.String(info.OAuthProvider)\n',
    '\t\t\tentry.OAuthProvider = htmlsanitize.String(info.OAuthProvider)\n\t\t\tentry.SupportsQuota = info.SupportsQuota\n\t\t\tentry.QuotaProvider = htmlsanitize.String(info.QuotaProvider)\n\t\t\tentry.QuotaMode = htmlsanitize.String(info.QuotaMode)\n',
    'entry.SupportsQuota = info.SupportsQuota',
)

quota_provider_source = Path(__file__).resolve().parent / 'plugin_quota_provider.go'
quota_provider_target = ROOT / 'internal/pluginhost/quota_provider.go'
write(quota_provider_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(quota_provider_source)))
quota_provider_test_source = Path(__file__).resolve().parent / 'plugin_quota_provider_test.go'
quota_provider_test_target = ROOT / 'internal/pluginhost/quota_provider_test.go'
write(quota_provider_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(quota_provider_test_source)))

quota_cache_store_source = Path(__file__).resolve().parent / 'plugin_quota_cache_store.go'
quota_cache_store_target = ROOT / 'internal/pluginhost/quota_cache_store.go'
write(quota_cache_store_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(quota_cache_store_source)))
quota_cache_store_test_source = Path(__file__).resolve().parent / 'plugin_quota_cache_store_test.go'
quota_cache_store_test_target = ROOT / 'internal/pluginhost/quota_cache_store_test.go'
write(quota_cache_store_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(quota_cache_store_test_source)))

runtime_state_store_source = Path(__file__).resolve().parent / 'plugin_runtime_state_store.go'
runtime_state_store_target = ROOT / 'internal/pluginhost/runtime_state_store.go'
write(runtime_state_store_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(runtime_state_store_source)))
runtime_state_store_test_source = Path(__file__).resolve().parent / 'plugin_runtime_state_store_test.go'
runtime_state_store_test_target = ROOT / 'internal/pluginhost/runtime_state_store_test.go'
write(runtime_state_store_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(runtime_state_store_test_source)))

pro_settings_store_source = Path(__file__).resolve().parent / 'plugin_pro_settings_store.go'
pro_settings_store_target = ROOT / 'internal/pluginhost/pro_settings_store.go'
write(pro_settings_store_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(pro_settings_store_source)))
pro_settings_store_test_source = Path(__file__).resolve().parent / 'plugin_pro_settings_store_test.go'
pro_settings_store_test_target = ROOT / 'internal/pluginhost/pro_settings_store_test.go'
write(pro_settings_store_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(pro_settings_store_test_source)))

auth_model_filter_source = Path(__file__).resolve().parent / 'plugin_auth_model_filter.go'
auth_model_filter_target = ROOT / 'internal/pluginhost/auth_model_filter.go'
write(auth_model_filter_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(auth_model_filter_source)))
auth_model_filter_test_source = Path(__file__).resolve().parent / 'plugin_auth_model_filter_test.go'
auth_model_filter_test_target = ROOT / 'internal/pluginhost/auth_model_filter_test.go'
write(auth_model_filter_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(auth_model_filter_test_source)))
auth_model_filter_service_test_source = Path(__file__).resolve().parent / 'plugin_auth_model_filter_service_test.go'
auth_model_filter_service_test_target = ROOT / 'sdk/cliproxy/service_auth_model_filter_test.go'
write(auth_model_filter_service_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(auth_model_filter_service_test_source)))

service_models = ROOT / 'sdk/cliproxy/service_models.go'
replace_once(
    service_models,
    '''\tif ctx.Err() != nil {
\t\treturn
\t}
\tmodels = applyOAuthModelAliasForAuth(s.cfg, provider, authKind, a.Attributes, models)
''',
    '''\tif ctx.Err() != nil {
\t\treturn
\t}
\tmodels = s.applyAuthModelFilters(ctx, a, models)
\tmodels = applyOAuthModelAliasForAuth(s.cfg, provider, authKind, a.Attributes, models)
''',
    'models = s.applyAuthModelFilters(ctx, a, models)',
)
insert_before(
    service_models,
    'func (s *Service) oauthExcludedModels(provider, authKind string) []string {\n',
    '''func (s *Service) applyAuthModelFilters(ctx context.Context, auth *coreauth.Auth, models []*ModelInfo) []*ModelInfo {
\tif s == nil || s.pluginHost == nil || auth == nil || len(models) == 0 {
\t\treturn models
\t}
\treturn s.pluginHost.FilterModelsForAuth(ctx, auth, models).Models
}

''',
    'func (s *Service) applyAuthModelFilters',
)

service_executors = ROOT / 'sdk/cliproxy/service_executors.go'
replace_once(
    service_executors,
    '''\tmodels := applyExcludedModels(result.Models, activeExcluded)
\tmodels = applyOAuthModelAliasForAuth(s.cfg, providerKey, activeAuthKind, activeAuth.Attributes, models)
''',
    '''\tmodels := applyExcludedModels(result.Models, activeExcluded)
\tmodels = s.applyAuthModelFilters(ctx, activeAuth, models)
\tmodels = applyOAuthModelAliasForAuth(s.cfg, providerKey, activeAuthKind, activeAuth.Attributes, models)
''',
    'models = s.applyAuthModelFilters(ctx, activeAuth, models)',
)

legacy_gemini_quota_source = Path(__file__).resolve().parent / 'plugin_gemini_cli_quota_legacy.go'
legacy_gemini_quota_target = ROOT / 'internal/pluginhost/gemini_cli_quota_legacy.go'
write(legacy_gemini_quota_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(legacy_gemini_quota_source)))
legacy_gemini_quota_test_source = Path(__file__).resolve().parent / 'plugin_gemini_cli_quota_legacy_test.go'
legacy_gemini_quota_test_target = ROOT / 'internal/pluginhost/gemini_cli_quota_legacy_test.go'
write(legacy_gemini_quota_test_target, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(legacy_gemini_quota_test_source)))

plugin_quota_management = ROOT / 'internal/api/handlers/management/plugin_quota.go'
write(plugin_quota_management, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(Path(__file__).resolve().parent / 'plugin_quota_management.go')))
plugin_quota_management_test = ROOT / 'internal/api/handlers/management/plugin_quota_test.go'
write(plugin_quota_management_test, read_text(Path(__file__).resolve().parent / 'plugin_quota_management_test.go'))

usage_manager = ROOT / 'sdk/cliproxy/usage/manager.go'
add_go_import(usage_manager, '"net/http"\n', '\t"reflect"\n')
replace_once(
    usage_manager,
    'type serviceTierContextKey struct{}\n',
    'type serviceTierContextKey struct{}\ntype streamContextKey struct{}\n',
)
replace_once(
    usage_manager,
    '''type streamContextKey struct{}
type generateContextKey struct{}
''',
    '''type streamContextKey struct{}
type attemptTrackerContextKey struct{}
type attemptIndexContextKey struct{}
type generateContextKey struct{}

type attemptTracker struct {
\tmu   sync.Mutex
\tnext int64
}
''',
    'type attemptTrackerContextKey struct{}',
)
replace_once(
    usage_manager,
    '''\tTTFT        time.Duration
\tFailed      bool
''',
    '''\tTTFT        time.Duration
\t// AttemptIndex is the zero-based upstream attempt number for this request.
\t// Nil means the caller did not instrument attempts.
\tAttemptIndex *int64
\tFailed       bool
''',
    'AttemptIndex *int64',
)
insert_before(
    usage_manager,
    '// WithServiceTier stores the client-requested service tier for usage sinks.\n',
    '''// WithStream stores whether the client requested streaming output for usage sinks.
func WithStream(ctx context.Context, stream bool) context.Context {
\tif ctx == nil {
\t\tctx = context.Background()
\t}
\treturn context.WithValue(ctx, streamContextKey{}, stream)
}

// StreamFromContext returns whether the client requested streaming output.
func StreamFromContext(ctx context.Context) bool {
\tif ctx == nil {
\t\treturn false
\t}
\tstream, _ := ctx.Value(streamContextKey{}).(bool)
\treturn stream
}

''',
    'func WithStream(ctx context.Context, stream bool) context.Context',
)
insert_before(
    usage_manager,
    '// WithServiceTier stores the client-requested service tier for usage sinks.\n',
    '''// WithAttemptTracking attaches one request-scoped upstream-attempt counter.
func WithAttemptTracking(ctx context.Context) context.Context {
\tif ctx == nil {
\t\tctx = context.Background()
\t}
\tif tracker, ok := ctx.Value(attemptTrackerContextKey{}).(*attemptTracker); ok && tracker != nil {
\t\treturn ctx
\t}
\treturn context.WithValue(ctx, attemptTrackerContextKey{}, &attemptTracker{})
}

// NextAttemptContext allocates the next zero-based upstream-attempt index.
func NextAttemptContext(ctx context.Context) context.Context {
\tctx = WithAttemptTracking(ctx)
\ttracker, _ := ctx.Value(attemptTrackerContextKey{}).(*attemptTracker)
\ttracker.mu.Lock()
\tindex := tracker.next
\ttracker.next++
\ttracker.mu.Unlock()
\treturn context.WithValue(ctx, attemptIndexContextKey{}, index)
}

// AttemptIndexFromContext returns the current upstream-attempt index when instrumented.
func AttemptIndexFromContext(ctx context.Context) (int64, bool) {
\tif ctx == nil {
\t\treturn 0, false
\t}
\tindex, ok := ctx.Value(attemptIndexContextKey{}).(int64)
\treturn index, ok
}

''',
    'func WithAttemptTracking(ctx context.Context) context.Context',
)
replace_once(
    usage_manager,
    '''\tm.named[name] = len(m.plugins)
\tm.plugins = append(m.plugins, plugin)
\tm.pluginsMu.Unlock()
''',
    '''\tfor index, existing := range m.plugins {
\t\tif existing == nil {
\t\t\tm.named[name] = index
\t\t\tm.plugins[index] = plugin
\t\t\tm.pluginsMu.Unlock()
\t\t\treturn
\t\t}
\t}
\tm.named[name] = len(m.plugins)
\tm.plugins = append(m.plugins, plugin)
\tm.pluginsMu.Unlock()
''',
    'for index, existing := range m.plugins',
)
insert_before(
    usage_manager,
    '// Publish enqueues a usage record for processing. If no plugin is registered\n',
    '''// UnregisterNamed removes a named plugin only when the current registration
// still belongs to the supplied plugin. Passing nil removes it unconditionally.
func (m *Manager) UnregisterNamed(name string, plugin Plugin) {
\tif m == nil {
\t\treturn
\t}
\tname = strings.TrimSpace(name)
\tif name == "" {
\t\treturn
\t}
\tm.pluginsMu.Lock()
\tdefer m.pluginsMu.Unlock()
\tindex, exists := m.named[name]
\tif !exists || index < 0 || index >= len(m.plugins) {
\t\treturn
\t}
\tcurrent := m.plugins[index]
\tif plugin != nil && !samePlugin(current, plugin) {
\t\treturn
\t}
\tm.plugins[index] = nil
\tdelete(m.named, name)
}

func samePlugin(left, right Plugin) bool {
\tif left == nil || right == nil {
\t\treturn left == nil && right == nil
\t}
\tleftValue := reflect.ValueOf(left)
\trightValue := reflect.ValueOf(right)
\treturn leftValue.Type() == rightValue.Type() &&
\t\tleftValue.Type().Comparable() &&
\t\tleftValue.Interface() == rightValue.Interface()
}

''',
    'func (m *Manager) UnregisterNamed(name string, plugin Plugin)',
)
insert_before(
    usage_manager,
    '// PublishRecord publishes a record using the default manager.\n',
    '''// UnregisterNamedPlugin removes a matching named plugin from the default manager.
func UnregisterNamedPlugin(name string, plugin Plugin) { DefaultManager().UnregisterNamed(name, plugin) }

''',
    'func UnregisterNamedPlugin(name string, plugin Plugin)',
)

usage_manager_test = ROOT / 'sdk/cliproxy/usage/manager_test.go'
if 'func TestUnregisterNamedPreservesReplacement' not in read(usage_manager_test):
    write(usage_manager_test, read(usage_manager_test).rstrip() + '''

type namedLifecycleUsagePlugin struct {
\tcalls int
}

func (p *namedLifecycleUsagePlugin) HandleUsage(context.Context, Record) {
\tp.calls++
}

func TestUnregisterNamedPreservesReplacement(t *testing.T) {
\tmanager := NewManager(1)
\tfirst := &namedLifecycleUsagePlugin{}
\treplacement := &namedLifecycleUsagePlugin{}
\tmanager.RegisterNamed("lifecycle", first)
\tmanager.RegisterNamed("lifecycle", replacement)

\tmanager.UnregisterNamed("lifecycle", first)
\tmanager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
\tif replacement.calls != 1 {
\t\tt.Fatalf("replacement calls = %d, want 1", replacement.calls)
\t}

\tmanager.UnregisterNamed("lifecycle", replacement)
\tmanager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
\tif replacement.calls != 1 {
\t\tt.Fatalf("replacement calls after unregister = %d, want 1", replacement.calls)
\t}

\tmanager.RegisterNamed("lifecycle", first)
\tmanager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
\tif first.calls != 1 {
\t\tt.Fatalf("re-registered plugin calls = %d, want 1", first.calls)
\t}
}
''')

auth_conductor = ROOT / 'sdk/cliproxy/auth/conductor_execution.go'
replace_once(
    auth_conductor,
    '\tctx = coreusage.WithRequestedModelAlias(ctx, alias)\n',
    '\tctx = coreusage.WithRequestedModelAlias(ctx, alias)\n\tctx = coreusage.WithStream(ctx, opts.Stream)\n',
)
replace_once(
    auth_conductor,
    '''func (m *Manager) Execute(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tnormalized := m.normalizeProviders(providers)
''',
    '''func (m *Manager) Execute(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tctx = coreusage.WithAttemptTracking(ctx)
\tnormalized := m.normalizeProviders(providers)
''',
    'ctx = coreusage.WithAttemptTracking(ctx)\n\tnormalized := m.normalizeProviders(providers)',
)
replace_once(
    auth_conductor,
    '''func (m *Manager) ExecuteCount(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tnormalized := m.normalizeProviders(providers)
''',
    '''func (m *Manager) ExecuteCount(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tctx = coreusage.WithAttemptTracking(ctx)
\tnormalized := m.normalizeProviders(providers)
''',
    'ExecuteCount(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {\n\tctx = coreusage.WithAttemptTracking(ctx)',
)
replace_once(
    auth_conductor,
    '''func (m *Manager) ExecuteStream(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tif m.HomeEnabled() {
''',
    '''func (m *Manager) ExecuteStream(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
\treq, opts = cliproxysession.Enrich(req, opts)
\tctx = coreusage.WithAttemptTracking(ctx)
\tif m.HomeEnabled() {
''',
    'ExecuteStream(ctx context.Context, providers []string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {\n\tctx = coreusage.WithAttemptTracking(ctx)',
)

auth_conductor_stream = ROOT / 'sdk/cliproxy/auth/conductor_stream.go'
auth_conductor_home_execution = ROOT / 'sdk/cliproxy/auth/conductor_home_execution.go'
auth_conductor_home = ROOT / 'sdk/cliproxy/auth/conductor_home.go'

for target in (
    auth_conductor_stream,
    auth_conductor_home_execution,
    auth_conductor_home,
):
    add_go_import(
        target,
        f'\tcliproxyexecutor "{import_path("sdk/cliproxy/executor")}"\n',
        f'\tcoreusage "{import_path("sdk/cliproxy/usage")}"\n',
    )

for target, old_call, new_call, marker in (
    (
        auth_conductor_stream,
        '\t\tstreamResult, errStream := executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        '\t\tctx = coreusage.NextAttemptContext(ctx)\n\t\tstreamResult, errStream := executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        'ctx = coreusage.NextAttemptContext(ctx)\n\t\tstreamResult, errStream := executor.ExecuteStream',
    ),
    (
        auth_conductor_stream,
        '\t\t\t\t\tstreamResult, errStream = executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        '\t\t\t\t\tctx = coreusage.NextAttemptContext(ctx)\n\t\t\t\t\tstreamResult, errStream = executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        'ctx = coreusage.NextAttemptContext(ctx)\n\t\t\t\t\tstreamResult, errStream = executor.ExecuteStream',
    ),
    (
        auth_conductor_stream,
        '\t\t\t\t\tretryStream, retryErr := executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        '\t\t\t\t\tctx = coreusage.NextAttemptContext(ctx)\n\t\t\t\t\tretryStream, retryErr := executor.ExecuteStream(ctx, auth, execReq, execOpts)\n',
        'ctx = coreusage.NextAttemptContext(ctx)\n\t\t\t\t\tretryStream, retryErr := executor.ExecuteStream',
    ),
    (
        auth_conductor_home_execution,
        '\t\t\t\tresponse, errExecute = selection.Executor.CountTokens(execCtx, preparedAuth, execReq, execOpts)\n',
        '\t\t\t\texecCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\tresponse, errExecute = selection.Executor.CountTokens(execCtx, preparedAuth, execReq, execOpts)\n',
        'execCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\tresponse, errExecute = selection.Executor.CountTokens',
    ),
    (
        auth_conductor,
        '\t\t\tresp, errExec := executor.Execute(execCtx, auth, execReq, execOpts)\n',
        '\t\t\texecCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\tresp, errExec := executor.Execute(execCtx, auth, execReq, execOpts)\n',
        'execCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\tresp, errExec := executor.Execute',
    ),
    (
        auth_conductor,
        '\t\t\t\t\tresp, errExec = executor.Execute(execCtx, auth, execReq, execOpts)\n',
        '\t\t\t\t\texecCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\t\tresp, errExec = executor.Execute(execCtx, auth, execReq, execOpts)\n',
        'execCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\t\tresp, errExec = executor.Execute',
    ),
    (
        auth_conductor,
        '\t\t\tresp, errExec := executor.CountTokens(execCtx, auth, execReq, execOpts)\n',
        '\t\t\texecCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\tresp, errExec := executor.CountTokens(execCtx, auth, execReq, execOpts)\n',
        'execCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\tresp, errExec := executor.CountTokens',
    ),
    (
        auth_conductor,
        '\t\t\t\t\tresp, errExec = executor.CountTokens(execCtx, auth, execReq, execOpts)\n',
        '\t\t\t\t\texecCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\t\tresp, errExec = executor.CountTokens(execCtx, auth, execReq, execOpts)\n',
        'execCtx = coreusage.NextAttemptContext(execCtx)\n\t\t\t\t\tresp, errExec = executor.CountTokens',
    ),
    (
        auth_conductor_home,
        '\t\t\tresp, errExec := c.executor.Execute(creditsCtx, c.auth, execReq, creditsOpts)\n',
        '\t\t\tcreditsCtx = coreusage.NextAttemptContext(creditsCtx)\n\t\t\tresp, errExec := c.executor.Execute(creditsCtx, c.auth, execReq, creditsOpts)\n',
        'creditsCtx = coreusage.NextAttemptContext(creditsCtx)',
    ),
):
    replace_once(target, old_call, new_call, marker)

usage_helpers = ROOT / 'internal/runtime/executor/helps/usage_helpers.go'
replace_once(
    usage_helpers,
    '''func (r *UsageReporter) publishRecord(ctx context.Context, record usage.Record) {
\trecord.ResponseHeaders = internallogging.GetResponseHeaders(ctx)
\tusage.PublishRecord(ctx, record)
}
''',
    '''func (r *UsageReporter) publishRecord(ctx context.Context, record usage.Record) {
\trecord.ResponseHeaders = internallogging.GetResponseHeaders(ctx)
\tif attemptIndex, ok := usage.AttemptIndexFromContext(ctx); ok {
\t\trecord.AttemptIndex = &attemptIndex
\t}
\tusage.PublishRecord(ctx, record)
}
''',
    'record.AttemptIndex = &attemptIndex',
)

if 'func TestAttemptTrackingAllocatesZeroBasedIndexes' not in read(usage_manager_test):
    write(usage_manager_test, read(usage_manager_test).rstrip() + '''

func TestAttemptTrackingAllocatesZeroBasedIndexes(t *testing.T) {
\tctx := WithAttemptTracking(context.Background())
\tfor want := int64(0); want < 3; want++ {
\t\tattemptCtx := NextAttemptContext(ctx)
\t\tgot, ok := AttemptIndexFromContext(attemptCtx)
\t\tif !ok || got != want {
\t\t\tt.Fatalf("attempt index = %d, %t; want %d, true", got, ok, want)
\t\t}
\t}
\tif _, ok := AttemptIndexFromContext(context.Background()); ok {
\t\tt.Fatal("uninstrumented context unexpectedly has an attempt index")
\t}
}
''')

config_defaults = ROOT / 'internal/config/config_defaults.go'
replace_once(
    config_defaults,
    'DefaultPanelGitHubRepository = "https://github.com/router-for-me/Cli-Proxy-API-Management-Center"',
    f'DefaultPanelGitHubRepository = "{PRO_PANEL_REPOSITORY}"',
)

config_existing_updates = ROOT / 'internal/config/config_existing_updates.go'
write(config_existing_updates, read_text(Path(__file__).resolve().parent / 'config_existing_updates.go'))
config_existing_updates_test = ROOT / 'internal/config/config_existing_updates_test.go'
write(config_existing_updates_test, read_text(Path(__file__).resolve().parent / 'config_existing_updates_test.go'))

config_example = ROOT / 'config.example.yaml'
replace_once(
    config_example,
    '  panel-github-repository: "https://github.com/router-for-me/Cli-Proxy-API-Management-Center"',
    f'  panel-github-repository: "{PRO_PANEL_REPOSITORY}"',
)
replace_once(
    config_example,
    'plugins:\n  enabled: false\n',
    'plugins:\n  enabled: true\n',
    'plugins:\n  enabled: true',
)
replace_once(
    config_example,
    '    example:\n      enabled: true\n',
    '    example:\n      enabled: false\n',
    '    example:\n      enabled: false\n',
)
replace_once(
    config_example,
    '# Standard dynamic library plugins are trusted in-process code. They are disabled by default.',
    '# Standard dynamic library plugins are trusted in-process code. Pro builds enable them because observability persistence is plugin-owned.',
)
replace_once(
    config_example,
    '''      mode: "safe" # enum example: safe, fast

# When true, disable high-overhead request logging and HTTP middleware features to reduce per-request memory usage under high concurrency.
''',
    '''      mode: "safe" # enum example: safe, fast

    # Required Pro persistence plugin. It adopts or migrates the legacy usage.sqlite
    # before the proxy service starts, then exclusively owns usage and runtime state.
    pro-observability:
      enabled: true
      routing-strategy: "round-robin"

    # Optional Pro plugin: subtract xAI OAuth models by detected account plan.
    # oauth-model-policy:
    #   enabled: true
    #   priority: 10
    #   cache-ttl: 30m
    #   resolve-timeout: 15s
    #   providers:
    #     xai:
    #       plans:
    #         free:
    #           excluded-models: ["grok-pro-*"]
    #         supergrok:
    #           excluded-models: ["grok-4.5-*"]
    #         _unknown:
    #           excluded-models: ["grok-pro-*"]

# When true, disable high-overhead request logging and HTTP middleware features to reduce per-request memory usage under high concurrency.
''',
    'Optional Pro plugin: subtract xAI OAuth models by detected account plan.',
)
config_yaml = ROOT / 'internal/config/config_yaml.go'
insert_before(
    config_yaml,
    '// NormalizeCommentIndentation removes indentation from standalone YAML comment lines to keep them left aligned.\n',
    '// SaveConfigPreserveCommentsUpdateNestedBoolScalar updates a nested bool scalar while preserving comments and positions.\nfunc SaveConfigPreserveCommentsUpdateNestedBoolScalar(configFile string, path []string, value bool) error {\n\tdata, err := os.ReadFile(configFile)\n\tif err != nil {\n\t\treturn err\n\t}\n\tvar root yaml.Node\n\tif err = yaml.Unmarshal(data, &root); err != nil {\n\t\treturn err\n\t}\n\tif root.Kind != yaml.DocumentNode || len(root.Content) == 0 {\n\t\treturn fmt.Errorf("invalid yaml document structure")\n\t}\n\tnode := root.Content[0]\n\tfor i, key := range path {\n\t\tif i == len(path)-1 {\n\t\t\tv := getOrCreateMapValue(node, key)\n\t\t\tv.Kind = yaml.ScalarNode\n\t\t\tv.Tag = "!!bool"\n\t\t\tif value {\n\t\t\t\tv.Value = "true"\n\t\t\t} else {\n\t\t\t\tv.Value = "false"\n\t\t\t}\n\t\t} else {\n\t\t\tnext := getOrCreateMapValue(node, key)\n\t\t\tif next.Kind != yaml.MappingNode {\n\t\t\t\tnext.Kind = yaml.MappingNode\n\t\t\t\tnext.Tag = "!!map"\n\t\t\t}\n\t\t\tnode = next\n\t\t}\n\t}\n\tf, err := os.Create(configFile)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer func() { _ = f.Close() }()\n\tvar buf bytes.Buffer\n\tenc := yaml.NewEncoder(&buf)\n\tenc.SetIndent(2)\n\tif err = enc.Encode(&root); err != nil {\n\t\t_ = enc.Close()\n\t\treturn err\n\t}\n\tif err = enc.Close(); err != nil {\n\t\treturn err\n\t}\n\tdata = NormalizeCommentIndentation(buf.Bytes())\n\t_, err = f.Write(data)\n\treturn err\n}\n\n',
    'func SaveConfigPreserveCommentsUpdateNestedBoolScalar',
)
replace_go_function(
    config_yaml,
    'func SaveConfigPreserveCommentsUpdateNestedBoolScalar',
    '// SaveConfigPreserveCommentsUpdateNestedBoolScalar updates an existing bool scalar without creating missing keys.\nfunc SaveConfigPreserveCommentsUpdateNestedBoolScalar(configFile string, path []string, value bool) error {\n\t_, err := SaveConfigPreserveCommentsUpdateExistingScalars(configFile, []ExistingScalarUpdate{{Path: path, Value: value}})\n\treturn err\n}\n',
    'SaveConfigPreserveCommentsUpdateExistingScalars(configFile, []ExistingScalarUpdate',
)
insert_before(
    config_yaml,
    '// NormalizeCommentIndentation removes indentation from standalone YAML comment lines to keep them left aligned.\n',
    '// PluginAutoInstallProxyURL returns the proxy URL used by plugin store auto-install requests.\nfunc (cfg *Config) PluginAutoInstallProxyURL() string {\n\tif cfg == nil {\n\t\treturn ""\n\t}\n\treturn cfg.ProxyURL\n}\n\n// PluginAutoInstallEnabled reports whether dynamic plugins are enabled.\nfunc (cfg *Config) PluginAutoInstallEnabled() bool {\n\treturn cfg != nil && cfg.Plugins.Enabled\n}\n\n// PluginAutoInstallDir returns the normalized plugin discovery directory.\nfunc (cfg *Config) PluginAutoInstallDir() string {\n\tif cfg == nil {\n\t\treturn ""\n\t}\n\treturn cfg.Plugins.Dir\n}\n\n// PluginAutoInstallStoreSources returns configured third-party plugin registry URLs.\nfunc (cfg *Config) PluginAutoInstallStoreSources() []string {\n\tif cfg == nil || len(cfg.Plugins.StoreSources) == 0 {\n\t\treturn nil\n\t}\n\treturn append([]string(nil), cfg.Plugins.StoreSources...)\n}\n\n// PluginAutoInstallEnabledIDs returns configured plugin IDs that should be present at startup.\nfunc (cfg *Config) PluginAutoInstallEnabledIDs() []string {\n\tif cfg == nil || len(cfg.Plugins.Configs) == 0 {\n\t\treturn nil\n\t}\n\tids := make([]string, 0, len(cfg.Plugins.Configs))\n\tfor id, item := range cfg.Plugins.Configs {\n\t\tif item.Enabled == nil || !*item.Enabled {\n\t\t\tcontinue\n\t\t}\n\t\tids = append(ids, id)\n\t}\n\treturn ids\n}\n\n',
    'func (cfg *Config) PluginAutoInstallProxyURL',
)

updater = ROOT / 'internal/managementasset/updater.go'
replace_once(
    updater,
    'defaultManagementReleaseURL  = "https://api.github.com/repos/router-for-me/Cli-Proxy-API-Management-Center/releases/latest"',
    f'defaultManagementReleaseURL  = "{PRO_PANEL_RELEASE_API}"',
)
add_go_import(updater, '"net/http"\n', '\t"net/url"\n')
replace_once(updater, '\tgitURL := strings.ToLower(strings.TrimSpace(os.Getenv("GITSTORE_GIT_URL")))\n', '')
replace_once(updater, 'tok != "" && strings.Contains(gitURL, "github.com")', 'tok != "" && isGitHubReleaseURL(releaseURL)')
insert_before(
    updater,
    'func fetchLatestAsset(ctx context.Context, client *http.Client, releaseURL string) (*releaseAsset, string, error) {\n',
    '''func isGitHubReleaseURL(releaseURL string) bool {
\tparsed, err := url.Parse(strings.TrimSpace(releaseURL))
\tif err != nil || parsed.Host == "" {
\t\treturn false
\t}
\treturn strings.Contains(strings.ToLower(parsed.Host), "github.com")
}

''',
    'func isGitHubReleaseURL(releaseURL string) bool',
)
queue_go_source('internal/api/handlers/management/management_panel.go')
queue_go_source('internal/api/handlers/management/management_panel_test.go')

server_main = ROOT / 'cmd/server/main.go'
add_go_import(server_main, '"' + import_path('internal/pluginhost') + '"\n', '\t"' + import_path('internal/pluginstore') + '"\n')
replace_once(
    server_main,
    '''\tconfigaccess.Register(&cfg.SDKConfig)
\tpluginHost.ApplyConfig(context.Background(), cfg)
''',
    '''\tconfigaccess.Register(&cfg.SDKConfig)
\tcfg.EnsureProObservabilityPluginDefaults()
\tpluginstore.EnsureConfiguredPluginsInstalled(context.Background(), cfg)
\tpluginHost.ApplyConfig(context.Background(), cfg)
''',
)

queue_go_source('internal/pluginstore/autoinstall.go')

queue_go_source('internal/pluginstore/autoinstall_test.go')

queue_go_source('internal/config/pro_plugin_defaults.go')
queue_go_source('internal/config/pro_plugin_defaults_test.go')
queue_go_source('internal/cmd/pro_plugin_startup_test.go')
queue_go_source('internal/pluginhost/pro_observability_ready.go')
queue_go_source('internal/pluginhost/pro_observability_ready_test.go')
queue_go_source('internal/pluginhost/pro_backup_callbacks.go')
queue_go_source('internal/pluginhost/pro_backup_callbacks_test.go')
queue_go_source('internal/pluginhost/account_inspection_callbacks.go')
queue_go_source('internal/pluginhost/account_inspection_callbacks_test.go')

service_plugins = ROOT / 'sdk/cliproxy/service_plugins.go'
replace_once(
    service_plugins,
    '''\tif s.pluginHost != nil {
\t\ts.pluginHost.ApplyConfig(ctx, cfg)
\t}
''',
    '''\tif cfg != nil {
\t\tcfg.EnsureProObservabilityPluginDefaults()
\t}
\tif s.pluginHost != nil {
\t\ts.pluginHost.ApplyConfig(ctx, cfg)
\t}
''',
    'cfg.EnsureProObservabilityPluginDefaults()',
)

replace_once(
    ROOT / 'internal/pluginhost/auth_provider.go',
    '''\treq.RawJSON = bytes.Clone(req.RawJSON)
\tresp, errParse := provider.ParseAuth(ctx, req)
''',
    '''\treq.RawJSON = normalizePluginStorageJSON(req.Provider, bytes.Clone(req.RawJSON))
\tresp, errParse := provider.ParseAuth(ctx, req)
''',
)

replace_once(
    ROOT / 'internal/pluginhost/auth_provider.go',
    '''\tif provider != "" {
\t\tmetadata["type"] = provider
\t}
\tattributes := cloneStringMap(data.Attributes)
''',
    '''\tif provider != "" {
\t\tmetadata["type"] = provider
\t}
\tdisabled := data.Disabled || pluginAuthDisabledFromMetadata(metadata)
\tmetadata["disabled"] = disabled
\tattributes := cloneStringMap(data.Attributes)
''',
)

replace_once(
    ROOT / 'internal/pluginhost/auth_provider.go',
    '''\tstatus := coreauth.StatusActive
\tif data.Disabled {
\t\tstatus = coreauth.StatusDisabled
\t}
''',
    '''\tstatus := coreauth.StatusActive
\tif disabled {
\t\tstatus = coreauth.StatusDisabled
\t}
''',
)

replace_once(
    ROOT / 'internal/pluginhost/auth_provider.go',
    '''\t\tDisabled:         data.Disabled,
''',
    '''\t\tDisabled:         disabled,
''',
)

replace_once(
    ROOT / 'internal/pluginhost/adapters_executors.go',
    '''func storageJSONFromAuth(auth *coreauth.Auth) []byte {
\tif auth == nil {
\t\treturn nil
\t}
\tif rawProvider, okRaw := auth.Storage.(interface{ RawJSON() []byte }); okRaw {
\t\treturn bytes.Clone(rawProvider.RawJSON())
\t}
\tif len(auth.Metadata) == 0 {
\t\treturn nil
\t}
\tdata, errMarshal := json.Marshal(auth.Metadata)
\tif errMarshal != nil {
\t\treturn nil
\t}
\treturn data
}
''',
    '''func storageJSONFromAuth(auth *coreauth.Auth) []byte {
\tif auth == nil {
\t\treturn nil
\t}
\tif rawProvider, okRaw := auth.Storage.(interface{ RawJSON() []byte }); okRaw {
\t\treturn normalizePluginStorageJSON(auth.Provider, bytes.Clone(rawProvider.RawJSON()))
\t}
\tif len(auth.Metadata) == 0 {
\t\treturn nil
\t}
\tdata, errMarshal := json.Marshal(auth.Metadata)
\tif errMarshal != nil {
\t\treturn nil
\t}
\treturn normalizePluginStorageJSON(auth.Provider, data)
}
''',
)

queue_go_source('internal/pluginhost/gemini_cli_storage_compat.go')

queue_go_source('internal/pluginhost/gemini_cli_storage_compat_test.go')

queue_go_source('internal/pluginhost/management_call.go')

queue_go_source('internal/pluginhost/management_call_test.go')

queue_go_source('internal/pluginhost/pro_observability_integration_test.go')

queue_go_source('internal/pluginhost/usage_metadata_test.go')

server = ROOT / 'internal/api/server.go'
server_routes = ROOT / 'internal/api/server_routes.go'
server_management = ROOT / 'internal/api/server_management.go'
auth_files = ROOT / 'internal/api/handlers/management/auth_files.go'
auth_files_fields = ROOT / 'internal/api/handlers/management/auth_files_fields.go'
api_tools = ROOT / 'internal/api/handlers/management/api_tools.go'
routing_policy = ROOT / 'internal/api/handlers/management/routing_policy.go'
routing_policy_test = ROOT / 'internal/api/handlers/management/routing_policy_test.go'
replace_once(
    server_management,
    '\t\tmgmt.GET("/latest-version", s.mgmt.GetLatestVersion)\n',
    '\t\tmgmt.GET("/latest-version", s.mgmt.GetLatestVersion)\n\t\tmgmt.POST("/management-panel/check-update", s.mgmt.PostCheckManagementPanelUpdate)\n',
)
write(routing_policy, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(Path(__file__).resolve().parent / 'routing_policy.go')))
write(routing_policy_test, re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(Path(__file__).resolve().parent / 'routing_policy_test.go')))

replace_once(
    auth_files_fields,
    '''\tmetadata["disabled"] = disabled
\traw, errMarshal := json.Marshal(metadata)
''',
    '''\tmetadata["disabled"] = disabled
\tdelete(metadata, routingProtectionMetadataKey)
\traw, errMarshal := json.Marshal(metadata)
''',
)
replace_once(
    auth_files_fields,
    '''\tif auth.Metadata == nil {
\t\tauth.Metadata = make(map[string]any)
\t}
\tauth.Metadata["disabled"] = disabled
}
''',
    '''\tif auth.Metadata == nil {
\t\tauth.Metadata = make(map[string]any)
\t}
\tauth.Metadata["disabled"] = disabled
\tclearRoutingProtectionOwnership(auth)
}
''',
)
replace_once(
    auth_files_fields,
    '''func syncAuthFileDisabledState(auth *coreauth.Auth) {
\tif auth == nil {
\t\treturn
\t}
\tdisabled, ok := authFileBoolValue(auth.Metadata["disabled"])
''',
    '''func syncAuthFileDisabledState(auth *coreauth.Auth) {
\tif auth == nil {
\t\treturn
\t}
\tdisabled, ok := authFileBoolValue(auth.Metadata["disabled"])
\tclearRoutingProtectionOwnership(auth)
''',
)

replace_once(
    api_tools,
    '''	Data            string            `json:"data"`
}
''',
    '''	Data            string            `json:"data"`
	UseExecutorSnake *bool             `json:"use_executor"`
	UseExecutorCamel *bool             `json:"useExecutor"`
	UseExecutorPascal *bool            `json:"UseExecutor"`
}
''',
)
insert_before(
    api_tools,
    'func firstNonEmptyString(values ...*string) string {\n',
    '''func firstNonNilBool(values ...*bool) bool {
\tfor _, v := range values {
\t\tif v != nil {
\t\t\treturn *v
\t\t}
\t}
\treturn false
}

''',
    'func firstNonNilBool(values ...*bool) bool',
)
replace_once(
    api_tools,
    '''\thttpClient := &http.Client{
\t\tTimeout: defaultAPICallTimeout,
\t}
\thttpClient.Transport = h.apiCallTransport(auth)

\tresp, errDo := httpClient.Do(req)
''',
    '''\tuseExecutor := firstNonNilBool(body.UseExecutorSnake, body.UseExecutorCamel, body.UseExecutorPascal)
\tvar resp *http.Response
\tvar errDo error
\tif useExecutor {
\t\tif auth == nil {
\t\t\tc.JSON(http.StatusBadRequest, gin.H{"error": "auth not found"})
\t\t\treturn
\t\t}
\t\tif h == nil || h.authManager == nil {
\t\t\tc.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
\t\t\treturn
\t\t}
\t\tresp, errDo = h.authManager.HttpRequest(c.Request.Context(), auth, req)
\t} else {
\t\thttpClient := &http.Client{
\t\t\tTimeout: defaultAPICallTimeout,
\t\t}
\t\thttpClient.Transport = h.apiCallTransport(auth)
\t\tresp, errDo = httpClient.Do(req)
\t}
''',
)
replace_once(
    auth_files,
    '''		"unavailable":    auth.Unavailable,
		"runtime_only":   runtimeOnly,
''',
    '''		"unavailable":    auth.Unavailable,
		"last_error":     authFileLastError(auth),
		"runtime_only":   runtimeOnly,
''',
)
insert_before(
    auth_files,
    'func authAttribute(auth *coreauth.Auth, key string) string {\n',
    '''func authFileLastError(auth *coreauth.Auth) *coreauth.Error {
	if auth == nil {
		return nil
	}
	if auth.LastError != nil {
		return auth.LastError
	}
	if auth.Metadata == nil {
		return nil
	}
	raw, ok := auth.Metadata["last_error"].(map[string]any)
	if !ok {
		return nil
	}
	lastError := &coreauth.Error{
		Code:       metadataString(raw["code"]),
		Message:    metadataString(raw["message"]),
		Retryable:  metadataBool(raw["retryable"]),
		HTTPStatus: metadataInt(raw["http_status"]),
	}
	if lastError.Code == "" && lastError.Message == "" && lastError.HTTPStatus == 0 {
		return nil
	}
	return lastError
}

func metadataString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func metadataBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func metadataInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

''',
    'func authFileLastError',
)
replace_once(
    auth_files,
    '''				typeValue := gjson.GetBytes(data, "type").String()
				emailValue := gjson.GetBytes(data, "email").String()
				fileData["type"] = typeValue
				fileData["email"] = emailValue
''',
    '''				typeValue := gjson.GetBytes(data, "type").String()
				emailValue := gjson.GetBytes(data, "email").String()
				fileData["type"] = typeValue
				fileData["email"] = emailValue
				if lastErrorRaw := gjson.GetBytes(data, "last_error"); lastErrorRaw.IsObject() {
					var lastError map[string]any
					if errUnmarshal := json.Unmarshal([]byte(lastErrorRaw.Raw), &lastError); errUnmarshal == nil && len(lastError) > 0 {
						fileData["last_error"] = lastError
					}
				}
				if strings.EqualFold(strings.TrimSpace(typeValue), "codex") {
					if claims := extractCodexIDTokenClaimsFromRaw(gjson.GetBytes(data, "id_token").String()); claims != nil {
						fileData["id_token"] = claims
					}
				}
''',
)
insert_before(
    auth_files,
    'func extractCodexIDTokenClaims(auth *coreauth.Auth) gin.H {\n',
    '''func extractCodexIDTokenClaimsFromRaw(idTokenRaw string) gin.H {
	idToken := strings.TrimSpace(idTokenRaw)
	if idToken == "" {
		return nil
	}
	claims, err := codex.ParseJWTToken(idToken)
	if err != nil || claims == nil {
		return nil
	}
	return codexIDTokenClaimsEntry(claims)
}

''',
    'func extractCodexIDTokenClaimsFromRaw',
)
replace_go_function(
    auth_files,
    'func extractCodexIDTokenClaims(auth *coreauth.Auth) gin.H',
    '''func extractCodexIDTokenClaims(auth *coreauth.Auth) gin.H {
	if auth == nil || auth.Metadata == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(auth.Provider), "codex") {
		return nil
	}
	idTokenRaw, ok := auth.Metadata["id_token"].(string)
	if !ok {
		return nil
	}
	return extractCodexIDTokenClaimsFromRaw(idTokenRaw)
}

func codexIDTokenClaimsEntry(claims *codex.JWTClaims) gin.H {
	if claims == nil {
		return nil
	}
	result := gin.H{}
	if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID); v != "" {
		result["chatgpt_account_id"] = v
	}
	if v := strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType); v != "" {
		result["plan_type"] = v
	}
	if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveStart; v != nil {
		result["chatgpt_subscription_active_start"] = v
	}
	if v := claims.CodexAuthInfo.ChatgptSubscriptionActiveUntil; v != nil {
		result["chatgpt_subscription_active_until"] = v
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
''',
    'func codexIDTokenClaimsEntry',
)

patch_dir = Path(__file__).resolve().parent
embeddedusage_source = patch_dir.parent / 'embeddedusage'
embeddedusage_target = ROOT / 'internal/embeddedusage'
if embeddedusage_source.is_dir():
    queue_tree(embeddedusage_source, embeddedusage_target)
elif not embeddedusage_target.is_dir():
    raise SystemExit(f'embeddedusage source not found: {embeddedusage_source}')
ensure_go_require(ROOT / 'go.mod', 'modernc.org/sqlite', 'v1.51.0')
auth_runtime_state = ROOT / 'sdk/cliproxy/auth/auth_runtime_state.go'
write(
    auth_runtime_state,
    re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(patch_dir / 'auth_runtime_state.go')),
)
write(
    ROOT / 'sdk/cliproxy/auth/auth_runtime_state_test.go',
    re.sub(r'github\.com/router-for-me/CLIProxyAPI/v\d+', MODULE_PATH, read_text(patch_dir / 'auth_runtime_state_test.go')),
)

redisqueue_plugin = ROOT / 'internal/redisqueue/plugin.go'
queue_go_source('internal/api/embeddedusage_cutover_test.go')
queue_go_source('internal/api/usage_stream_bridge.go')
queue_go_source('internal/api/usage_stream_bridge_test.go')
queue_go_source('internal/api/management_stream_bridge.go')
queue_go_source('internal/api/management_stream_bridge_test.go')
queue_go_source('internal/api/handlers/management/pro_lifecycle.go')
replace_once(
    redisqueue_plugin,
	'func init() {\n\tcoreusage.RegisterPlugin(&usageQueuePlugin{})\n}\n',
	'func init() {}\n',
	'func init() {}',
)
replace_once(
    redisqueue_plugin,
    f'\tinternallogging "{import_path("internal/logging")}"\n',
    f'\t"{import_path("internal/requestmeta")}"\n',
    f'"{import_path("internal/requestmeta")}"',
)
redisqueue_plugin_text = read(redisqueue_plugin)
if 'internallogging.' in redisqueue_plugin_text:
    write(redisqueue_plugin, redisqueue_plugin_text.replace('internallogging.', 'requestmeta.'))
elif 'requestmeta.' not in redisqueue_plugin_text:
    raise SystemExit(f'request metadata calls not found in {redisqueue_plugin}')
replace_once(
    redisqueue_plugin,
    '\trequestID := strings.TrimSpace(requestmeta.GetRequestID(ctx))\n\treasoningEffort :=',
    '\trequestID := strings.TrimSpace(requestmeta.GetRequestID(ctx))\n\tstream := coreusage.StreamFromContext(ctx)\n\treasoningEffort :=',
    'stream := coreusage.StreamFromContext(ctx)',
)
replace_once(
    redisqueue_plugin,
    '\t\tRequestID:           requestID,\n\t\tReasoningEffort:',
    '\t\tRequestID:           requestID,\n\t\tStream:              stream,\n\t\tReasoningEffort:',
    'Stream:              stream',
)
replace_once(
    redisqueue_plugin,
    '''\t\tUserAgent:       clientRequestMetadata.UserAgent,
\t\tTokens:          tokens,
''',
    '''\t\tUserAgent:       clientRequestMetadata.UserAgent,
\t\tAttemptIndex:    record.AttemptIndex,
\t\tTokens:          tokens,
''',
    'AttemptIndex:    record.AttemptIndex',
)
redisqueue_plugin_text = read(redisqueue_plugin)
stream_field = '\tStream bool `json:"stream"`\n'
if '`json:"stream"`' not in redisqueue_plugin_text:
    request_id_field = re.compile(r'(?m)^\tRequestID[ \t]+string[ \t]+`json:"request_id"`\n')
    matches = request_id_field.findall(redisqueue_plugin_text)
    if len(matches) != 1:
        raise SystemExit(
            f'expected one request ID field in {redisqueue_plugin}, found {len(matches)}'
        )
    write(
        redisqueue_plugin,
        request_id_field.sub(lambda match: match.group(0) + stream_field, redisqueue_plugin_text, count=1),
    )
redisqueue_plugin_text = read(redisqueue_plugin)
attempt_field = '\tAttemptIndex *int64 `json:"attempt_index,omitempty"`\n'
if '`json:"attempt_index,omitempty"`' not in redisqueue_plugin_text:
    auth_index_field = re.compile(r'(?m)^\tAuthIndex[ \t]+string[ \t]+`json:"auth_index"`\n')
    matches = auth_index_field.findall(redisqueue_plugin_text)
    if len(matches) != 1:
        raise SystemExit(
            f'expected one auth index field in {redisqueue_plugin}, found {len(matches)}'
        )
    write(
        redisqueue_plugin,
        auth_index_field.sub(lambda match: match.group(0) + attempt_field, redisqueue_plugin_text, count=1),
    )
redisqueue_plugin_test = ROOT / 'internal/redisqueue/plugin_test.go'
if redisqueue_plugin_test.exists():
    text = read_text(redisqueue_plugin_test)
    text = text.replace(
        f'internallogging "{import_path("internal/logging")}"',
        f'"{import_path("internal/requestmeta")}"',
    )
    text = text.replace('internallogging.', 'requestmeta.')
    write(redisqueue_plugin_test, text)

queue_go_source('internal/requestmeta/client.go')

queue_go_source('internal/requestmeta/client_test.go')

queue_go_source('internal/requestmeta/requestid.go')

queue_go_source('internal/requestmeta/response.go')

logging_request_id = ROOT / 'internal/logging/requestid.go'
queue_go_source('internal/logging/requestid.go')

logging_request_meta = ROOT / 'internal/logging/requestmeta.go'
queue_go_source('internal/logging/requestmeta.go')

replace_go_call_block(
    server_routes,
    '\ts.engine.GET("/", func(c *gin.Context) {',
    '''\ts.engine.GET("/", func(c *gin.Context) {
\t\tc.Redirect(http.StatusTemporaryRedirect, "/management.html")
\t})
''',
    'c.Redirect(http.StatusTemporaryRedirect, "/management.html")',
)
replace_once(
    server_management,
    '''\t{
\t\tmgmt.GET("/config", s.mgmt.GetConfig)
''',
    '''\t{
\t\tmgmt.GET("/usage/stream", s.servePluginUsageStream)
\t\tmgmt.GET("/account-inspection/logs", s.servePluginAccountInspectionStream)

\t\tmgmt.GET("/config", s.mgmt.GetConfig)
''',
)
replace_once(
    server_management,
    '''\t\tmgmt.POST("/api-call", s.mgmt.APICall)\n''',
    '''\t\tmgmt.POST("/api-call", s.mgmt.APICall)\n\t\ts.mgmt.RegisterPluginQuotaRoutes(mgmt)\n\t\ts.mgmt.RegisterRoutingPolicyRoutes(mgmt)\n''',
)

handler = ROOT / 'internal/api/handlers/management/handler.go'
add_go_import(handler, '"net/http"\n', '\t"net/url"\n')
replace_once(
    handler,
    '''\tpluginReleaseCacheMu    sync.Mutex
\tpluginReleaseCache      map[string]pluginReleaseCacheEntry
}
''',
    '''\tpluginReleaseCacheMu    sync.Mutex
\tpluginReleaseCache      map[string]pluginReleaseCacheEntry
\tlifecycleContext        context.Context
\tlifecycleCancel         context.CancelFunc
\tlifecycleWG             sync.WaitGroup
\tshutdownOnce            sync.Once
}
''',
    'lifecycleContext        context.Context',
)
replace_once(
    handler,
    '''\th := &Handler{
''',
    '''\tlifecycleContext, lifecycleCancel := context.WithCancel(context.Background())

\th := &Handler{
''',
    'lifecycleContext, lifecycleCancel := context.WithCancel(context.Background())',
)
replace_once(
    handler,
    '''\t\tallowRemoteOverride: envSecret != "",
\t\tenvSecret:           envSecret,
\t}
''',
    '''\t\tallowRemoteOverride: envSecret != "",
\t\tenvSecret:           envSecret,
\t\tlifecycleContext:    lifecycleContext,
\t\tlifecycleCancel:     lifecycleCancel,
\t}
''',
    'lifecycleContext:    lifecycleContext',
)
replace_go_function(
    handler,
    'func (h *Handler) startAttemptCleanup() {',
    '''func (h *Handler) startAttemptCleanup() {
\tif h == nil || h.lifecycleContext == nil {
\t\treturn
\t}
\th.lifecycleWG.Add(1)
\tgo func() {
\t\tdefer h.lifecycleWG.Done()
\t\tticker := time.NewTicker(attemptCleanupInterval)
\t\tdefer ticker.Stop()
\t\tfor {
\t\t\tselect {
\t\t\tcase <-h.lifecycleContext.Done():
\t\t\t\treturn
\t\t\tcase <-ticker.C:
\t\t\t\th.purgeStaleAttempts()
\t\t\t}
\t\t}
\t}()
}
''',
    'case <-h.lifecycleContext.Done():',
)
replace_once(
    handler,
    '''\t\tif provided == "" {
\t\t\tprovided = c.GetHeader("X-Management-Key")
\t\t}
''',
    '''\t\tif provided == "" {
\t\t\tprovided = c.GetHeader("X-Management-Key")
\t\t}
\t\tif provided == "" {
\t\t\tprovided = managementKeyFromWebSocketProtocol(c)
\t\t}
''',
)
insert_before(
    handler,
    '''func (h *Handler) Middleware() gin.HandlerFunc {
''',
    '''func managementKeyFromWebSocketProtocol(c *gin.Context) string {
\tif !strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
\t\treturn ""
\t}
\tfor _, protocol := range strings.Split(c.GetHeader("Sec-WebSocket-Protocol"), ",") {
\t\tprotocol = strings.TrimSpace(protocol)
\t\tif !strings.HasPrefix(protocol, "cpa-management.") {
\t\t\tcontinue
\t\t}
\t\tdecoded, err := url.QueryUnescape(strings.TrimPrefix(protocol, "cpa-management."))
\t\tif err != nil {
\t\t\treturn ""
\t\t}
\t\treturn decoded
\t}
\treturn ""
}

''',
    'func managementKeyFromWebSocketProtocol(c *gin.Context) string',
)
replace_once(
    handler,
    '''\th.startAttemptCleanup()
\treturn h
''',
    '''\tstartRoutingPolicyController(h)
\th.startAttemptCleanup()
\treturn h
''',
)
replace_once(
    server,
    '''\tlog.Debug("Stopping API server...")

\tif s.keepAliveEnabled {
''',
    '''\tlog.Debug("Stopping API server...")
\tdefer func() {
\t\tif s.mgmt != nil {
\t\t\ts.mgmt.Shutdown()
\t\t}
\t}()

\tif s.keepAliveEnabled {
''',
    's.mgmt.Shutdown()',
)

run = ROOT / 'internal/cmd/run.go'
add_go_import(run, '"errors"\n', '\t"fmt"\n')
add_go_import(run, '"' + import_path('internal/config') + '"\n', '\t"' + import_path('internal/embeddedusage') + '"\n')
insert_before(
    run,
    '// StartService builds and runs the proxy service using the exported SDK.\n',
    'func applyProRequiredStartupConfig(cfg *config.Config, configPath string) {\n\tif cfg == nil {\n\t\treturn\n\t}\n\tshouldPersistUsageStatistics := !cfg.UsageStatisticsEnabled\n\tshouldPersistPanelRepository := cfg.RemoteManagement.PanelGitHubRepository != config.DefaultPanelGitHubRepository\n\tcfg.UsageStatisticsEnabled = true\n\tcfg.RemoteManagement.PanelGitHubRepository = config.DefaultPanelGitHubRepository\n\tcfg.EnsureProObservabilityPluginDefaults()\n\tif configPath == "" {\n\t\treturn\n\t}\n\tif shouldPersistUsageStatistics {\n\t\tif _, err := config.SaveConfigPreserveCommentsUpdateExistingScalars(configPath, []config.ExistingScalarUpdate{{Path: []string{"usage-statistics-enabled"}, Value: true}}); err != nil {\n\t\t\tlog.Warnf("failed to update existing usage statistics config: %v", err)\n\t\t}\n\t}\n\tif shouldPersistPanelRepository {\n\t\tif _, err := config.SaveConfigPreserveCommentsUpdateExistingScalars(configPath, []config.ExistingScalarUpdate{{Path: []string{"remote-management", "panel-github-repository"}, Value: config.DefaultPanelGitHubRepository}}); err != nil {\n\t\t\tlog.Warnf("failed to update existing panel repository config: %v", err)\n\t\t}\n\t}\n}\n\nfunc requireProObservabilityPlugin(ctx context.Context, cfg *config.Config, host *pluginhost.Host) (*pluginhost.Host, error) {\n\tif host == nil {\n\t\thost = pluginhost.New()\n\t}\n\thost.ApplyConfig(ctx, cfg)\n\tif !host.ProObservabilityReady(config.ProObservabilityPluginID) {\n\t\treturn host, fmt.Errorf("required plugin %s failed to load, migrate usage storage, or expose its complete persistence capability set", config.ProObservabilityPluginID)\n\t}\n\tembeddedusage.SetQuotaCachePluginBackend(host)\n\tembeddedusage.SetRuntimeStatePluginBackend(host)\n\tembeddedusage.SetProSettingsPluginBackend(host)\n\treturn host, nil\n}\n\n',
    'func applyProRequiredStartupConfig',
)
insert_before_nth(
    run,
    '''\tservice, err := builder.Build()
''',
    '''\tapplyProRequiredStartupConfig(cfg, configPath)
\tpreparedHost, errPrepare := requireProObservabilityPlugin(ctx, cfg, host)
\tif errPrepare != nil {
\t\tlog.Errorf("failed to prepare plugin-owned usage service: %v", errPrepare)
\t\tclose(doneCh)
\t\treturn cancelFn, doneCh
\t}
\thost = preparedHost
\tbuilder = builder.WithPluginHost(preparedHost)

''',
    2,
    'preparedHost, errPrepare := requireProObservabilityPlugin(ctx, cfg, host)',
)
insert_before_nth(
    run,
    '''\tservice, err := builder.Build()
''',
    '''\tapplyProRequiredStartupConfig(cfg, configPath)
\tpreparedHost, errPrepare := requireProObservabilityPlugin(runCtx, cfg, host)
\tif errPrepare != nil {
\t\tlog.Errorf("failed to prepare plugin-owned usage service: %v", errPrepare)
\t\treturn
\t}
\thost = preparedHost
\tbuilder = builder.WithPluginHost(preparedHost)

''',
    1,
    'requireProObservabilityPlugin(runCtx',
)

queue_go_source('sdk/cliproxy/auth/inspection_refresh.go')

auth_types = ROOT / 'sdk/cliproxy/auth/types.go'
replace_once(
    auth_types,
    '''\tSuccess int64 `json:"-"`
\tFailed  int64 `json:"-"`
''',
    '''\tSelected int64 `json:"-"`
\tSuccess  int64 `json:"-"`
\tFailed   int64 `json:"-"`
''',
    'Selected int64',
)

auth_selector = ROOT / 'sdk/cliproxy/auth/selector.go'
replace_once(
    auth_selector,
    '''type RoundRobinSelector struct {
\tmu      sync.Mutex
\tcursors map[string]int
\tmaxKeys int
}''',
    '''type RoundRobinSelector struct {
\tmu                      sync.Mutex
\tcursors                 map[string]int
\tmaxKeys                  int
\troutingCursorRestored    map[string]bool
\tpersistedRoutingCursors map[string]string
}''',
    'persistedRoutingCursors map[string]string',
)
replace_once(
    auth_selector,
    '''\ts.ensureCursorKey(key, limit)
\tindex := s.cursors[key]
\tif index >= 2_147_483_640 {
\t\tindex = 0
\t}
\ts.cursors[key] = index + 1
\ts.mu.Unlock()
\treturn available[index%len(available)], nil
}''',
    '''\ts.ensureCursorKey(key, limit)
\ts.restoreRoutingCursorLocked(provider, model, key, available)
\tindex := s.cursors[key]
\tif index >= 2_147_483_640 {
\t\tindex = 0
\t}
\ts.cursors[key] = index + 1
\tpicked := available[index%len(available)]
\ts.persistRoutingCursorLocked(provider, model, picked)
\ts.mu.Unlock()
\treturn picked, nil
}''',
    's.restoreRoutingCursorLocked(provider, model, key, available)',
)
replace_once(
    auth_selector,
    '''\tif _, ok := s.cursors[key]; !ok && len(s.cursors) >= limit {
\t\ts.cursors = make(map[string]int)
\t}
}''',
    '''\tif _, ok := s.cursors[key]; !ok && len(s.cursors) >= limit {
\t\ts.cursors = make(map[string]int)
\t\ts.routingCursorRestored = make(map[string]bool)
\t}
}''',
    's.routingCursorRestored = make(map[string]bool)',
)

auth_conductor = ROOT / 'sdk/cliproxy/auth/conductor_lifecycle.go'
replace_once(
    auth_conductor,
    '''\tauth.EnsureIndex()
\tauthClone := auth.Clone()
\tm.mu.Lock()
\tm.auths[auth.ID] = authClone
''',
    '''\tauth.EnsureIndex()
\trestoreAuthRuntimeStats(auth)
\tcleanupLegacyQuotaCacheOnRegister(auth)
\tauthClone := auth.Clone()
\tm.mu.Lock()
\tm.auths[auth.ID] = authClone
''',
    'cleanupLegacyQuotaCacheOnRegister(auth)',
)
replace_once(
    auth_conductor,
    '''\tauth.Success = existing.Success
\tauth.Failed = existing.Failed
\tauth.recentRequests = existing.recentRequests
''',
    '''\tauth.Selected = existing.Selected
\tauth.Success = existing.Success
\tauth.Failed = existing.Failed
\tauth.recentRequests = existing.recentRequests
''',
    'auth.Selected = existing.Selected',
)

auth_conductor = ROOT / 'sdk/cliproxy/auth/conductor_cooldown.go'
replace_once(
    auth_conductor,
    '''\tm.mu.Unlock()
\tif m.scheduler != nil && authSnapshot != nil {
\t\tm.scheduler.upsertAuth(authSnapshot)
\t}
''',
    '''\tm.mu.Unlock()
\tqueueAuthRuntimeStats(authSnapshot)
\tif m.scheduler != nil && authSnapshot != nil {
\t\tm.scheduler.upsertAuth(authSnapshot)
\t}
''',
    'queueAuthRuntimeStats(authSnapshot)',
)

auth_conductor = ROOT / 'sdk/cliproxy/auth/conductor_selection.go'
replace_once(
    auth_conductor,
    '''\tif m.HomeEnabled() {
\t\tauth, exec, _, err := m.pickNextViaHome(ctx, model, opts, tried)
\t\treturn auth, exec, err
\t}

\tif m.hasPluginScheduler() || !m.useSchedulerFastPath() {
\t\treturn m.pickNextLegacy(ctx, provider, model, opts, tried)
\t}
''',
    '''\tif m.HomeEnabled() {
\t\tauth, exec, _, err := m.pickNextViaHome(ctx, model, opts, tried)
\t\tif err == nil && auth != nil {
\t\t\tm.recordAuthSelected(auth.ID)
\t\t}
\t\treturn auth, exec, err
\t}

\tif m.hasPluginScheduler() || !m.useSchedulerFastPath() {
\t\tauth, exec, err := m.pickNextLegacy(ctx, provider, model, opts, tried)
\t\tif err == nil && auth != nil {
\t\t\tm.recordAuthSelected(auth.ID)
\t\t}
\t\treturn auth, exec, err
\t}
''',
    'm.recordAuthSelected(auth.ID)',
)
replace_once(
    auth_conductor,
    '''\treturn authCopy, executor, nil
}

func (m *Manager) pickNextMixedLegacy''',
    '''\tm.recordAuthSelected(authCopy.ID)
\treturn authCopy, executor, nil
}

func (m *Manager) pickNextMixedLegacy''',
    'm.recordAuthSelected(authCopy.ID)\n\treturn authCopy, executor, nil',
)
replace_once(
    auth_conductor,
    '''\t\t\tif m.routeAwareSelectionRequired(candidate, model) {
\t\t\t\tm.mu.RUnlock()
\t\t\t\treturn m.pickNextLegacy(ctx, provider, model, opts, tried)
\t\t\t}
''',
    '''\t\t\tif m.routeAwareSelectionRequired(candidate, model) {
\t\t\t\tm.mu.RUnlock()
\t\t\t\tauth, exec, err := m.pickNextLegacy(ctx, provider, model, opts, tried)
\t\t\t\tif err == nil && auth != nil {
\t\t\t\t\tm.recordAuthSelected(auth.ID)
\t\t\t\t}
\t\t\t\treturn auth, exec, err
\t\t\t}
''',
    'm.mu.RUnlock()\n\t\t\t\tauth, exec, err := m.pickNextLegacy',
)
replace_once(
    auth_conductor,
    '''\tif m.HomeEnabled() {
\t\treturn m.pickNextViaHome(ctx, model, opts, tried)
\t}

\tif m.hasPluginScheduler() || !m.useSchedulerFastPath() {
\t\treturn m.pickNextMixedLegacy(ctx, providers, model, opts, tried)
\t}
''',
    '''\tif m.HomeEnabled() {
\t\tauth, exec, providerKey, err := m.pickNextViaHome(ctx, model, opts, tried)
\t\tif err == nil && auth != nil {
\t\t\tm.recordAuthSelected(auth.ID)
\t\t}
\t\treturn auth, exec, providerKey, err
\t}

\tif m.hasPluginScheduler() || !m.useSchedulerFastPath() {
\t\tauth, exec, providerKey, err := m.pickNextMixedLegacy(ctx, providers, model, opts, tried)
\t\tif err == nil && auth != nil {
\t\t\tm.recordAuthSelected(auth.ID)
\t\t}
\t\treturn auth, exec, providerKey, err
\t}
''',
    'auth, exec, providerKey, err := m.pickNextViaHome',
)
replace_once(
    auth_conductor,
    '''\treturn authCopy, executor, providerKey, nil
}

func (m *Manager) pickNextMixed(''',
    '''\tm.recordAuthSelected(authCopy.ID)
\treturn authCopy, executor, providerKey, nil
}

func (m *Manager) pickNextMixed(''',
    'm.recordAuthSelected(authCopy.ID)\n\treturn authCopy, executor, providerKey, nil',
)
replace_once(
    auth_conductor,
    '''\t\t\tif m.routeAwareSelectionRequired(candidate, model) {
\t\t\t\tm.mu.RUnlock()
\t\t\t\treturn m.pickNextMixedLegacy(ctx, providers, model, opts, tried)
\t\t\t}
''',
    '''\t\t\tif m.routeAwareSelectionRequired(candidate, model) {
\t\t\t\tm.mu.RUnlock()
\t\t\t\tauth, exec, providerKey, err := m.pickNextMixedLegacy(ctx, providers, model, opts, tried)
\t\t\t\tif err == nil && auth != nil {
\t\t\t\t\tm.recordAuthSelected(auth.ID)
\t\t\t\t}
\t\t\t\treturn auth, exec, providerKey, err
\t\t\t}
''',
    'm.mu.RUnlock()\n\t\t\t\tauth, exec, providerKey, err := m.pickNextMixedLegacy',
)

auth_files_handler = ROOT / 'internal/api/handlers/management/auth_files.go'
auth_files_crud_handler = ROOT / 'internal/api/handlers/management/auth_files_crud.go'
add_go_import(
    auth_files_crud_handler,
    '"github.com/gin-gonic/gin"\n',
    '\t"' + import_path('internal/embeddedusage') + '"\n',
)
replace_once(
    auth_files_handler,
    '''\tentry["success"] = auth.Success
\tentry["failed"] = auth.Failed
''',
    '''\tentry["selected"] = auth.Selected
\tentry["success"] = auth.Success
\tentry["failed"] = auth.Failed
''',
    'entry["selected"] = auth.Selected',
)
replace_once(
    auth_files_crud_handler,
    '''\t\t\t\tdeleted++
\t\t\t\th.removeAuth(ctx, full)
''',
    '''\t\t\t\tdeleted++
\t\t\t\th.removeAuth(ctx, full)
\t\t\t\t_ = embeddedusage.DeleteAuthRuntimeState(ctx, "", "", name)
''',
    'DeleteAuthRuntimeState(ctx, "", "", name)',
)
replace_once(
    auth_files_crud_handler,
    '''\ttargetPath := filepath.Join(h.cfg.AuthDir, filepath.Base(name))
\ttargetID := ""
\tif targetAuth := h.findAuthForDelete(name); targetAuth != nil {
''',
    '''\ttargetPath := filepath.Join(h.cfg.AuthDir, filepath.Base(name))
\ttargetID := ""
\ttargetIndex := ""
\tif targetAuth := h.findAuthForDelete(name); targetAuth != nil {
''',
    'targetIndex := ""',
)
replace_once(
    auth_files_crud_handler,
    '''\t\ttargetID = strings.TrimSpace(targetAuth.ID)
\t\tif path := strings.TrimSpace(authAttribute(targetAuth, "path")); path != "" {
''',
    '''\t\ttargetID = strings.TrimSpace(targetAuth.ID)
\t\ttargetIndex = strings.TrimSpace(targetAuth.Index)
\t\tif path := strings.TrimSpace(authAttribute(targetAuth, "path")); path != "" {
''',
    'targetIndex = strings.TrimSpace(targetAuth.Index)',
)
replace_once(
    auth_files_crud_handler,
    '''\th.removeAuthsForPath(ctx, targetPath, targetID)
\treturn filepath.Base(name), http.StatusOK, nil
''',
    '''\th.removeAuthsForPath(ctx, targetPath, targetID)
\t_ = embeddedusage.DeleteAuthRuntimeState(ctx, targetID, targetIndex, filepath.Base(name))
\treturn filepath.Base(name), http.StatusOK, nil
''',
    'DeleteAuthRuntimeState(ctx, targetID, targetIndex',
)

auth_scheduler = ROOT / 'sdk/cliproxy/auth/scheduler.go'
add_go_import(auth_scheduler, '"context"\n', '\t"fmt"\n')
add_go_import(auth_scheduler, '"time"\n', '\t"' + import_path('internal/embeddedusage') + '"\n')
replace_once(
    auth_scheduler,
    '''\tmixedCursors        map[string]int
\tmixedWeightedStates map[string]*smoothWeightedState
}''',
    '''\tmixedCursors        map[string]int
\tmixedWeightedStates map[string]*smoothWeightedState
\tmixedRestored       map[string]bool
\tpersistedCursors    map[string]string
}''',
    'persistedCursors map[string]string',
)
replace_once(
    auth_scheduler,
    '''\tmodelShards map[string]*modelScheduler
}''',
    '''\tmodelShards      map[string]*modelScheduler
\tpersistedCursors map[string]string
}''',
    'modelShards      map[string]*modelScheduler',
)
replace_once(
    auth_scheduler,
    '''type modelScheduler struct {
\tmodelKey        string''',
    '''type modelScheduler struct {
\tproviderKey     string
\tmodelKey        string
\tpersistedCursors map[string]string''',
    'providerKey     string\n\tmodelKey',
)
replace_once(
    auth_scheduler,
    '''type readyView struct {
\tflat          []*scheduledAuth
\tcursor        int
\tweightedState smoothWeightedState
}''',
    '''type readyView struct {
\tflat          []*scheduledAuth
\tcursor        int
\tweightedState smoothWeightedState
\tcursorKey     string
\tlastAuthID    string
\tpersisted     map[string]string
}''',
    'cursorKey  string',
)
replace_once(
    auth_scheduler,
    '''func newAuthScheduler(selector Selector) *authScheduler {
\treturn &authScheduler{
\t\tstrategy:            selectorStrategy(selector),
\t\tproviders:           make(map[string]*providerScheduler),
\t\tauthProviders:       make(map[string]string),
\t\tmixedCursors:        make(map[string]int),
\t\tmixedWeightedStates: make(map[string]*smoothWeightedState),
\t}
}''',
    '''func newAuthScheduler(selector Selector) *authScheduler {
\tpersistedCursors := make(map[string]string)
\tif states, err := embeddedusage.ListRoutingCursorStates(context.Background()); err == nil {
\t\tfor _, state := range states {
\t\t\tpersistedCursors[state.CursorKey] = state.LastAuthID
\t\t}
\t}
\treturn &authScheduler{
\t\tstrategy:            selectorStrategy(selector),
\t\tproviders:           make(map[string]*providerScheduler),
\t\tauthProviders:       make(map[string]string),
\t\tmixedCursors:        make(map[string]int),
\t\tmixedWeightedStates: make(map[string]*smoothWeightedState),
\t\tmixedRestored:       make(map[string]bool),
\t\tpersistedCursors:    persistedCursors,
\t}
}''',
    'persistedCursors := make(map[string]string)',
)
insert_before(
    auth_scheduler,
    '// selectorStrategy maps a selector implementation to the scheduler semantics it should emulate.\n',
    '''func (s *authScheduler) applyImportedRuntimeState(states []embeddedusage.RoutingCursorState, auths []*Auth) {
\tif s == nil {
\t\treturn
\t}
\tpersistedCursors := make(map[string]string, len(states))
\tfor _, state := range states {
\t\tif state.CursorKey != "" && state.LastAuthID != "" {
\t\t\tpersistedCursors[state.CursorKey] = state.LastAuthID
\t\t}
\t}
\ts.mu.Lock()
\tdefer s.mu.Unlock()
\ts.persistedCursors = persistedCursors
\ts.providers = make(map[string]*providerScheduler)
\ts.authProviders = make(map[string]string)
\ts.mixedCursors = make(map[string]int)
\ts.mixedWeightedStates = make(map[string]*smoothWeightedState)
\ts.mixedRestored = make(map[string]bool)
\tnow := time.Now()
\tfor _, auth := range auths {
\t\ts.upsertAuthLocked(auth, now)
\t}
}

''',
    'func (s *authScheduler) applyImportedRuntimeState',
)
replace_once(
    auth_scheduler,
    '''\ts.mixedCursors = make(map[string]int)
\ts.mixedWeightedStates = make(map[string]*smoothWeightedState)
\tnow := time.Now()''',
    '''\ts.mixedCursors = make(map[string]int)
\ts.mixedWeightedStates = make(map[string]*smoothWeightedState)
\ts.mixedRestored = make(map[string]bool)
\tnow := time.Now()''',
    's.mixedWeightedStates = make(map[string]*smoothWeightedState)\n\ts.mixedRestored = make(map[string]bool)\n\tnow := time.Now()',
)
replace_once(
    auth_scheduler,
    '''\ts.strategy = selectorStrategy(selector)
\tclear(s.mixedCursors)
\tclear(s.mixedWeightedStates)
''',
    '''\ts.strategy = selectorStrategy(selector)
\tclear(s.mixedCursors)
\tclear(s.mixedWeightedStates)
\tclear(s.mixedRestored)
''',
    'clear(s.mixedRestored)',
)
replace_once(
    auth_scheduler,
    '''\t\tproviderState = &providerScheduler{
\t\t\tproviderKey: providerKey,
\t\t\tauths:       make(map[string]*scheduledAuthMeta),
\t\t\tmodelShards: make(map[string]*modelScheduler),
\t\t}''',
    '''\t\tproviderState = &providerScheduler{
\t\t\tproviderKey:      providerKey,
\t\t\tauths:            make(map[string]*scheduledAuthMeta),
\t\t\tmodelShards:      make(map[string]*modelScheduler),
\t\t\tpersistedCursors: s.persistedCursors,
\t\t}''',
    'persistedCursors: s.persistedCursors',
)
replace_once(
    auth_scheduler,
    '''\tshard := &modelScheduler{
\t\tmodelKey:        modelKey,
\t\tentries:         make(map[string]*scheduledAuth),
\t\treadyByPriority: make(map[int]*readyBucket),
\t}''',
    '''\tshard := &modelScheduler{
\t\tproviderKey:      p.providerKey,
\t\tmodelKey:         modelKey,
\t\tpersistedCursors: p.persistedCursors,
\t\tentries:          make(map[string]*scheduledAuth),
\t\treadyByPriority:  make(map[int]*readyBucket),
\t}''',
    'persistedCursors: p.persistedCursors',
)
replace_once(
    auth_scheduler,
    '''\t\tbucket := buildReadyBucket(entries)
''',
    '''\t\tcursorPrefix := fmt.Sprintf("single|%s|%s|%d", m.providerKey, m.modelKey, priority)
\t\tbucket := buildReadyBucket(entries, cursorPrefix, m.persistedCursors)
''',
    'cursorPrefix := fmt.Sprintf("single|%s|%s|%d"',
)
replace_once(
    auth_scheduler,
    '''\t\tif cursorState, ok := cursorStates[priority]; ok && bucket != nil {
\t\t\trestoreReadyViewCursors(&bucket.all, cursorState.all)
\t\t\trestoreReadyViewCursors(&bucket.ws, cursorState.ws)
\t\t}
\t\tm.readyByPriority[priority] = bucket
''',
    '''\t\tif cursorState, ok := cursorStates[priority]; ok && bucket != nil {
\t\t\trestoreReadyViewCursors(&bucket.all, cursorState.all)
\t\t\trestoreReadyViewCursors(&bucket.ws, cursorState.ws)
\t\t}
\t\tif bucket != nil {
\t\t\tbucket.all.restoreAfterAuthID(m.persistedCursors[cursorPrefix+"|all"])
\t\t\tbucket.ws.restoreAfterAuthID(m.persistedCursors[cursorPrefix+"|ws"])
\t\t}
\t\tm.readyByPriority[priority] = bucket
''',
    'bucket.all.restoreAfterAuthID(m.persistedCursors[cursorPrefix+"|all"])',
)
replace_once(
    auth_scheduler,
    '''func buildReadyBucket(entries []*scheduledAuth) *readyBucket {
\tbucket := &readyBucket{}
\tbucket.all = buildReadyView(entries)''',
    '''func buildReadyBucket(entries []*scheduledAuth, cursorPrefix string, persisted map[string]string) *readyBucket {
\tbucket := &readyBucket{}
\tbucket.all = buildReadyView(entries, cursorPrefix+"|all", persisted)''',
    'func buildReadyBucket(entries []*scheduledAuth, cursorPrefix string',
)
replace_once(
    auth_scheduler,
    '''\tbucket.ws = buildReadyView(wsEntries)
''',
    '''\tbucket.ws = buildReadyView(wsEntries, cursorPrefix+"|ws", persisted)
''',
    'buildReadyView(wsEntries, cursorPrefix+"|ws"',
)
replace_once(
    auth_scheduler,
    '''func buildReadyView(entries []*scheduledAuth) readyView {
\treturn readyView{flat: append([]*scheduledAuth(nil), entries...)}
}''',
    '''func buildReadyView(entries []*scheduledAuth, cursorKey string, persisted map[string]string) readyView {
\tview := readyView{flat: append([]*scheduledAuth(nil), entries...), cursorKey: cursorKey, persisted: persisted}
\tview.restoreAfterAuthID(persisted[cursorKey])
\treturn view
}

func (v *readyView) restoreAfterAuthID(lastAuthID string) {
\tlastAuthID = strings.TrimSpace(lastAuthID)
\tif lastAuthID == "" || len(v.flat) == 0 {
\t\treturn
\t}
\tv.lastAuthID = lastAuthID
\tfor index, entry := range v.flat {
\t\tif entry == nil || entry.auth == nil {
\t\t\tcontinue
\t\t}
\t\tif entry.auth.ID == lastAuthID {
\t\t\tv.cursor = index + 1
\t\t\treturn
\t\t}
\t\tif entry.auth.ID > lastAuthID {
\t\t\tv.cursor = index
\t\t\treturn
\t\t}
\t}
\tv.cursor = 0
}''',
    'func (v *readyView) restoreAfterAuthID',
)
replace_once(
    auth_scheduler,
    '''\t\tv.cursor = index + 1
\t\treturn entry
''',
    '''\t\tv.cursor = index + 1
\t\tv.lastAuthID = entry.auth.ID
\t\tif v.persisted != nil {
\t\t\tv.persisted[v.cursorKey] = entry.auth.ID
\t\t}
\t\tembeddedusage.QueueRoutingCursorState(embeddedusage.RoutingCursorState{
\t\t\tCursorKey: v.cursorKey, LastAuthID: entry.auth.ID, UpdatedAtMS: time.Now().UnixMilli(),
\t\t})
\t\treturn entry
''',
    'v.persisted[v.cursorKey] = entry.auth.ID',
)
replace_once(
    auth_scheduler,
    '''\tstartSlot := s.mixedCursors[cursorKey] % totalWeight
''',
    '''\tstartSlot := s.mixedCursors[cursorKey] % totalWeight
\tpersistedCursorKey := fmt.Sprintf("mixed|%s|%d", cursorKey, bestPriority)
\tif !s.mixedRestored[cursorKey] {
\t\ts.mixedRestored[cursorKey] = true
\t\tif lastAuthID := s.persistedCursors[persistedCursorKey]; lastAuthID != "" {
\t\t\tfor providerIndex, shard := range candidateShards {
\t\t\t\tif shard != nil {
\t\t\t\t\tif _, ok := shard.entries[lastAuthID]; ok && weights[providerIndex] > 0 {
\t\t\t\t\t\tstartSlot = segmentStarts[providerIndex]
\t\t\t\t\t\tbreak
\t\t\t\t\t}
\t\t\t\t}
\t\t\t}
\t\t}
\t}
''',
    'persistedCursorKey := fmt.Sprintf("mixed|%s|%d"',
)
replace_once(
    auth_scheduler,
    '''\t\ts.mixedCursors[cursorKey] = slot + 1
\t\treturn picked, providerKey, nil
''',
    '''\t\ts.mixedCursors[cursorKey] = slot + 1
\t\ts.persistedCursors[persistedCursorKey] = picked.ID
\t\tembeddedusage.QueueRoutingCursorState(embeddedusage.RoutingCursorState{
\t\t\tCursorKey: persistedCursorKey, LastAuthID: picked.ID, UpdatedAtMS: time.Now().UnixMilli(),
\t\t})
\t\treturn picked, providerKey, nil
''',
    's.persistedCursors[persistedCursorKey] = picked.ID',
)

flush_writes()
subprocess.run([
    'gofmt',
    '-w',
    'cmd/server/main.go',
    'internal/api/server.go',
    'internal/api/server_test.go',
    'internal/api/handlers/management/pro_lifecycle.go',
    'internal/api/handlers/management/auth_files.go',
    'internal/api/handlers/management/handler.go',
    'internal/api/handlers/management/management_panel.go',
    'internal/api/handlers/management/management_panel_test.go',
    'internal/api/handlers/management/plugin_quota.go',
    'internal/api/handlers/management/plugin_quota_test.go',
    'internal/client/claude/models/models.go',
    'internal/client/claude/models/models_test.go',
    'internal/config/sdk_config.go',
    'internal/logging/requestid.go',
    'internal/logging/requestmeta.go',
    'internal/util/claude_model.go',
    'internal/util/claude_model_test.go',
    'internal/watcher/diff/config_diff.go',
    'internal/api/handlers/management/routing_policy.go',
    'internal/api/handlers/management/routing_policy_test.go',
    'internal/api/embeddedusage_cutover_test.go',
    'internal/api/usage_stream_bridge.go',
    'internal/api/usage_stream_bridge_test.go',
    'internal/api/management_stream_bridge.go',
    'internal/api/management_stream_bridge_test.go',
    'internal/cmd/pro_plugin_startup_test.go',
    'internal/config/config_existing_updates.go',
    'internal/config/config_existing_updates_test.go',
    'internal/config/pro_plugin_defaults.go',
    'internal/config/pro_plugin_defaults_test.go',
    'internal/pluginhost/gemini_cli_storage_compat.go',
    'internal/pluginhost/gemini_cli_storage_compat_test.go',
    'internal/pluginhost/gemini_cli_quota_legacy.go',
    'internal/pluginhost/gemini_cli_quota_legacy_test.go',
    'internal/pluginhost/host.go',
    'internal/pluginhost/management_call.go',
    'internal/pluginhost/management_call_test.go',
    'internal/pluginhost/host_callbacks.go',
    'internal/pluginhost/pro_backup_callbacks.go',
    'internal/pluginhost/pro_backup_callbacks_test.go',
    'internal/pluginhost/account_inspection_callbacks.go',
    'internal/pluginhost/account_inspection_callbacks_test.go',
    'internal/pluginhost/pro_observability_integration_test.go',
    'internal/pluginhost/pro_observability_ready.go',
    'internal/pluginhost/pro_observability_ready_test.go',
    'internal/pluginhost/pro_settings_store.go',
    'internal/pluginhost/pro_settings_store_test.go',
    'internal/pluginhost/quota_cache_store.go',
    'internal/pluginhost/quota_cache_store_test.go',
    'internal/pluginhost/runtime_state_store.go',
    'internal/pluginhost/runtime_state_store_test.go',
    'internal/pluginhost/quota_provider.go',
    'internal/pluginhost/quota_provider_test.go',
    'internal/pluginhost/auth_model_filter.go',
    'internal/pluginhost/auth_model_filter_test.go',
    'internal/pluginhost/usage_metadata_test.go',
    'internal/pluginhost/rpc_client.go',
    'internal/pluginhost/rpc_schema.go',
    'internal/pluginhost/snapshot.go',
    'internal/pluginstore/autoinstall.go',
    'internal/pluginstore/autoinstall_test.go',
    'internal/redisqueue/plugin.go',
    'internal/redisqueue/plugin_test.go',
    'internal/requestmeta/client.go',
    'internal/requestmeta/client_test.go',
    'internal/requestmeta/requestid.go',
    'internal/requestmeta/response.go',
    'internal/runtime/executor/xai_executor.go',
    'internal/runtime/executor/xai_quota_observer.go',
    'internal/runtime/executor/xai_websockets_executor.go',
    'sdk/api/handlers/claude/code_handlers.go',
    'sdk/api/handlers/claude/code_handlers_model_test.go',
    'sdk/cliproxy/auth/auth_runtime_state.go',
    'sdk/cliproxy/auth/auth_runtime_state_test.go',
    'sdk/cliproxy/auth/conductor.go',
    'sdk/cliproxy/auth/scheduler.go',
    'sdk/cliproxy/auth/types.go',
    'sdk/cliproxy/service_executors.go',
    'sdk/cliproxy/service_plugins.go',
    'sdk/cliproxy/service_auth_model_filter_test.go',
    'sdk/cliproxy/service_models.go',
    'sdk/cliproxy/usage/manager.go',
    'sdk/cliproxy/usage/manager_test.go',
    'sdk/pluginabi/types.go',
    'sdk/pluginapi/types.go',
], cwd=ROOT, check=True)
subprocess.run(['go', 'mod', 'tidy'], cwd=ROOT, check=True)
