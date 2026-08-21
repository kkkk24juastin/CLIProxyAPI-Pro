package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

const (
	pluginID            = "pro-observability"
	pluginName          = "Pro Observability"
	pluginVersion       = "0.1.0"
	pluginAuthor        = "sfun"
	pluginRepository    = "https://github.com/ssfun/CLIProxyAPI-Pro"
	pluginSchemaVersion = 2

	managementStatusPath = "/plugins/pro-observability/status"
	managementUsagePath  = "/plugins/pro-observability/usage"
	managementEventsPath = "/plugins/pro-observability/usage/events"
	defaultDatabasePath  = "/CLIProxyAPI/usage/usage.sqlite"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      registrationMetadata     `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationMetadata struct {
	Name             string `json:"Name"`
	Version          string `json:"Version"`
	Author           string `json:"Author"`
	GitHubRepository string `json:"GitHubRepository"`
}

type registrationCapabilities struct {
	UsagePlugin   bool `json:"usage_plugin"`
	ManagementAPI bool `json:"management_api"`
}

type managementRegistration struct {
	Routes []managementRoute `json:"routes,omitempty"`
}

type managementRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

type managementRequest struct {
	Method         string      `json:"Method"`
	Path           string      `json:"Path"`
	Headers        http.Header `json:"Headers"`
	Query          url.Values  `json:"Query"`
	Body           []byte      `json:"Body"`
	HostCallbackID string      `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

// usageRecord mirrors the latest Core pluginapi.UsageRecord JSON shape.
// Upstream intentionally has no JSON tags, so RPC field names are exported Go
// names rather than snake_case names.
type usageRecord struct {
	Provider            string
	ExecutorType        string
	Model               string
	Alias               string
	APIKey              string
	AuthID              string
	AuthIndex           string
	AuthType            string
	Source              string
	RequestID           string
	Endpoint            string
	AccessTokenSHA256   string
	ClientIP            string
	XForwardedFor       string
	UserAgent           string
	APIKeyPolicyID      string
	ProfileID           string
	ProfileNameSnapshot string
	PolicyMode          string
	RequestedModel      string
	EffectiveModel      string
	ReasoningEffort     string
	ServiceTier         string
	ResponseServiceTier string
	Speed               string
	ResponseSpeed       string
	Generate            bool
	RequestedAt         time.Time
	Latency             time.Duration
	TTFT                time.Duration
	AttemptIndex        *int64
	Stream              bool
	Failed              bool
	Failure             usageFailure
	Detail              usageDetail
	AccountingVersion   int
	TokenBreakdown      rpcTokenBreakdown
	ResponseHeaders     http.Header
}

type usageFailure struct {
	StatusCode int
	Body       string
}

type usageDetail struct {
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

type rpcTokenInputBreakdown struct {
	TotalTokens      int64
	UncachedTokens   int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

type rpcTokenOutputBreakdown struct {
	TotalTokens        int64
	NonReasoningTokens int64
	ReasoningTokens    int64
}

type rpcTokenBreakdown struct {
	SchemaVersion      int
	Quality            string
	TotalTokens        int64
	Input              rpcTokenInputBreakdown
	Output             rpcTokenOutputBreakdown
	UnclassifiedTokens int64
}

type tokenInputBreakdown struct {
	TotalTokens      int64 `json:"total_tokens"`
	UncachedTokens   int64 `json:"uncached_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type tokenOutputBreakdown struct {
	TotalTokens        int64 `json:"total_tokens"`
	NonReasoningTokens int64 `json:"non_reasoning_tokens"`
	ReasoningTokens    int64 `json:"reasoning_tokens"`
}

type tokenBreakdown struct {
	SchemaVersion      int                  `json:"schema_version"`
	Quality            string               `json:"quality"`
	TotalTokens        int64                `json:"total_tokens"`
	Input              tokenInputBreakdown  `json:"input"`
	Output             tokenOutputBreakdown `json:"output"`
	UnclassifiedTokens int64                `json:"unclassified_tokens"`
}

func (breakdown tokenBreakdown) valid() bool {
	if breakdown.SchemaVersion != 2 {
		return false
	}
	switch breakdown.Quality {
	case "complete", "inconsistent", "unclassified":
	default:
		return false
	}
	if breakdown.TotalTokens < 0 || breakdown.UnclassifiedTokens < 0 ||
		breakdown.Input.TotalTokens < 0 || breakdown.Input.UncachedTokens < 0 ||
		breakdown.Input.CacheReadTokens < 0 || breakdown.Input.CacheWriteTokens < 0 ||
		breakdown.Output.TotalTokens < 0 || breakdown.Output.NonReasoningTokens < 0 ||
		breakdown.Output.ReasoningTokens < 0 {
		return false
	}
	if breakdown.Input.TotalTokens != breakdown.Input.UncachedTokens+breakdown.Input.CacheReadTokens+breakdown.Input.CacheWriteTokens ||
		breakdown.Output.TotalTokens != breakdown.Output.NonReasoningTokens+breakdown.Output.ReasoningTokens ||
		breakdown.TotalTokens != breakdown.Input.TotalTokens+breakdown.Output.TotalTokens+breakdown.UnclassifiedTokens {
		return false
	}
	return breakdown.Quality != "complete" || breakdown.UnclassifiedTokens == 0
}

type usageEvent struct {
	ID                   int64
	RequestID            string
	EventHash            string
	TimestampMS          int64
	Timestamp            string
	Provider             string
	ExecutorType         string
	Model                string
	Alias                string
	Endpoint             string
	Method               string
	Path                 string
	AuthType             string
	AuthIndex            string
	Source               string
	SourceHash           string
	APIKeyHash           string
	APIKeyPolicyID       string
	ProfileID            string
	ProfileNameSnapshot  string
	PolicyMode           string
	RequestedModel       string
	EffectiveModel       string
	ClientIP             string
	XForwardedFor        string
	UserAgent            string
	InputTokens          int64
	OutputTokens         int64
	ReasoningTokens      int64
	CachedTokens         int64
	CacheTokens          int64
	CacheReadTokens      int64
	CacheWriteTokens     int64
	TotalTokens          int64
	AccountingVersion    int
	AccountingQuality    string
	UncachedInputTokens  int64
	UnclassifiedTokens   int64
	TokenBreakdownJSON   string
	LatencyMS            *int64
	TTFTMS               *int64
	StatusCode           *int
	ErrorCode            string
	ErrorMessage         string
	UpstreamRequestID    string
	RetryAfter           string
	AttemptIndex         *int64
	Stream               bool
	ReasoningEffort      string
	ServiceTier          string
	EffectiveServiceTier string
	Speed                string
	EffectiveSpeed       string
	EstimatedCost        *float64
	PriceRuleID          int64
	CostBreakdownJSON    string
	Failed               bool
	RawJSON              string
	CreatedAtMS          int64
}

type usageTokens struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	CacheTokens      int64 `json:"cache_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	CacheInputTokens int64 `json:"cache_input_tokens,omitempty"`
	TotalTokens      int64 `json:"total_tokens"`
}

type usageDetailPayload struct {
	ID                   int64           `json:"id,omitempty"`
	RequestID            string          `json:"request_id,omitempty"`
	Timestamp            string          `json:"timestamp"`
	Source               string          `json:"source"`
	AuthIndex            string          `json:"auth_index,omitempty"`
	APIKeyHash           string          `json:"api_key_hash,omitempty"`
	APIKeyPolicyID       string          `json:"api_key_policy_id,omitempty"`
	ProfileID            string          `json:"profile_id,omitempty"`
	ProfileNameSnapshot  string          `json:"profile_name_snapshot,omitempty"`
	PolicyMode           string          `json:"policy_mode,omitempty"`
	RequestedModel       string          `json:"requested_model,omitempty"`
	EffectiveModel       string          `json:"effective_model,omitempty"`
	ClientIP             string          `json:"client_ip,omitempty"`
	XForwardedFor        string          `json:"x_forwarded_for,omitempty"`
	UserAgent            string          `json:"user_agent,omitempty"`
	Provider             string          `json:"provider,omitempty"`
	ExecutorType         string          `json:"executor_type,omitempty"`
	Alias                string          `json:"alias,omitempty"`
	AuthType             string          `json:"auth_type,omitempty"`
	LatencyMS            *int64          `json:"latency_ms,omitempty"`
	TTFTMS               *int64          `json:"ttft_ms,omitempty"`
	StatusCode           *int            `json:"status_code,omitempty"`
	ErrorCode            string          `json:"error_code,omitempty"`
	ErrorMessage         string          `json:"error_message,omitempty"`
	UpstreamRequestID    string          `json:"upstream_request_id,omitempty"`
	RetryAfter           string          `json:"retry_after,omitempty"`
	AttemptIndex         *int64          `json:"attempt_index,omitempty"`
	Stream               bool            `json:"stream"`
	ReasoningEffort      string          `json:"reasoning_effort,omitempty"`
	ServiceTier          string          `json:"service_tier,omitempty"`
	EffectiveServiceTier string          `json:"effective_service_tier,omitempty"`
	Speed                string          `json:"speed,omitempty"`
	EffectiveSpeed       string          `json:"effective_speed,omitempty"`
	EstimatedCost        *float64        `json:"estimated_cost,omitempty"`
	PriceRuleID          int64           `json:"price_rule_id,omitempty"`
	CostBreakdown        json.RawMessage `json:"cost_breakdown,omitempty"`
	AccountingVersion    int             `json:"accounting_version,omitempty"`
	AccountingQuality    string          `json:"accounting_quality,omitempty"`
	TokenBreakdown       tokenBreakdown  `json:"token_breakdown"`
	Tokens               usageTokens     `json:"tokens"`
	Failed               bool            `json:"failed"`
}

type usageModelAggregate struct {
	Details []usageDetailPayload `json:"details"`
}

type usageAPIAggregate struct {
	Models map[string]*usageModelAggregate `json:"models"`
}

type usagePayload struct {
	TotalRequests  int64                         `json:"total_requests"`
	SuccessCount   int64                         `json:"success_count"`
	FailureCount   int64                         `json:"failure_count"`
	TotalTokens    int64                         `json:"total_tokens"`
	LatestID       int64                         `json:"latest_id"`
	Generation     int64                         `json:"generation"`
	ResetAtMS      int64                         `json:"reset_at_ms,omitempty"`
	DetailsCount   int64                         `json:"details_count,omitempty"`
	DetailsLimit   int64                         `json:"details_limit,omitempty"`
	DetailsLimited bool                          `json:"details_limited,omitempty"`
	MatchedTotal   int64                         `json:"matched_total,omitempty"`
	SnapshotMaxID  int64                         `json:"snapshot_max_id,omitempty"`
	PageCursor     string                        `json:"page_cursor,omitempty"`
	NextCursor     string                        `json:"next_cursor,omitempty"`
	HasMore        bool                          `json:"has_more,omitempty"`
	APIs           map[string]*usageAPIAggregate `json:"apis"`
}

type insertResult struct {
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"`
}

type usageSummary struct {
	LatestEventID int64 `json:"latestEventId"`
	TotalRequests int64 `json:"totalRequests"`
	SuccessCount  int64 `json:"successCount"`
	FailureCount  int64 `json:"failureCount"`
	TotalTokens   int64 `json:"totalTokens"`
	Generation    int64 `json:"generation"`
	ResetAtMS     int64 `json:"resetAtMs"`
	UpdatedAtMS   int64 `json:"updatedAtMs"`
}
