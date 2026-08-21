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
	Failed               bool
	RawJSON              string
	CreatedAtMS          int64
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
