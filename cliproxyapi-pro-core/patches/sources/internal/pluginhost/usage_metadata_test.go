package pluginhost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	apikeypolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/requestmeta"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type usageMetadataCaptureClient struct {
	method  string
	payload []byte
}

func (c *usageMetadataCaptureClient) Call(_ context.Context, method string, payload []byte) ([]byte, error) {
	c.method = method
	c.payload = append([]byte(nil), payload...)
	return marshalRPCResult(rpcEmptyResponse{})
}

func (*usageMetadataCaptureClient) Shutdown() {}

func TestEnrichRPCUsageRecordPreservesMonitoringAttribution(t *testing.T) {
	attempt := int64(3)
	breakdown := coreusage.NewSubsetTokenBreakdown(11, 2, 1, 7, 3, 18)
	source := coreusage.Record{
		Provider:            "codex",
		Model:               "gpt-5",
		AccessTokenSHA256:   "token-sha256",
		AttemptIndex:        &attempt,
		ResponseServiceTier: "priority",
		Speed:               "fast",
		ResponseSpeed:       "fast",
		RequestedAt:         time.Unix(123, 0),
		Detail: coreusage.Detail{
			InputTokens:    11,
			OutputTokens:   7,
			TotalTokens:    18,
			TokenBreakdown: breakdown,
		},
	}
	ctx := coreusage.WithRecordSnapshot(context.Background(), source)
	ctx = requestmeta.WithRequestID(ctx, "request-1")
	ctx = requestmeta.WithEndpoint(ctx, "POST /v1/responses")
	ctx = requestmeta.WithClientRequestMetadata(ctx, requestmeta.ClientRequestMetadata{
		ClientIP: "192.0.2.1", XForwardedFor: "198.51.100.2", UserAgent: "contract-test",
	})
	ctx = coreusage.WithStream(ctx, true)
	ctx = coreusage.WithRequestedModelAlias(ctx, "fallback-alias")
	ctx = apikeypolicy.WithDecision(ctx, apikeypolicy.RequestPolicyDecision{
		Mode: apikeypolicy.ModeProfile,
		Snapshot: &apikeypolicy.RequestPolicySnapshot{
			PolicyID: "policy-1", ProfileID: "profile-1", ProfileName: "Production",
			RequestedModel: "smart", EffectiveModel: "gpt-5",
		},
	})

	got := enrichRPCUsageRecord(ctx, pluginapi.UsageRecord{Provider: "codex", Model: "gpt-5"})
	if got.RequestID != "request-1" || got.Endpoint != "POST /v1/responses" || !got.Stream {
		t.Fatalf("request metadata = %#v", got)
	}
	if got.ClientIP != "192.0.2.1" || got.XForwardedFor != "198.51.100.2" || got.UserAgent != "contract-test" {
		t.Fatalf("client metadata = %#v", got)
	}
	if got.PolicyMode != apikeypolicy.ModeProfile || got.APIKeyPolicyID != "policy-1" || got.ProfileID != "profile-1" || got.ProfileNameSnapshot != "Production" {
		t.Fatalf("policy attribution = %#v", got)
	}
	if got.RequestedModel != "smart" || got.EffectiveModel != "gpt-5" {
		t.Fatalf("model attribution = %#v", got)
	}
	if got.AccessTokenSHA256 != "token-sha256" || got.AttemptIndex == nil || *got.AttemptIndex != 3 {
		t.Fatalf("auth attempt metadata = %#v", got)
	}
	if got.ResponseServiceTier != "priority" || got.Speed != "fast" || got.ResponseSpeed != "fast" {
		t.Fatalf("response metadata = %#v", got)
	}
	if got.AccountingVersion != coreusage.TokenAccountingSchemaVersion || got.TokenBreakdown.TotalTokens != 18 || got.TokenBreakdown.Input.CacheWriteTokens != 1 || got.TokenBreakdown.Output.ReasoningTokens != 3 {
		t.Fatalf("token accounting = %#v", got.TokenBreakdown)
	}
}

func TestEnrichRPCUsageRecordDoesNotInventProfileAttribution(t *testing.T) {
	ctx := apikeypolicy.WithDecision(context.Background(), apikeypolicy.PassthroughDecision())
	got := enrichRPCUsageRecord(ctx, pluginapi.UsageRecord{})
	if got.PolicyMode != apikeypolicy.ModePassthrough || got.APIKeyPolicyID != "" || got.ProfileID != "" || got.ProfileNameSnapshot != "" {
		t.Fatalf("passthrough attribution = %#v", got)
	}
}

func TestRPCUsageHandleSerializesEnrichedRecord(t *testing.T) {
	attempt := int64(1)
	source := coreusage.Record{
		Provider: "codex", ExecutorType: "codex", Model: "gpt-5", Alias: "smart",
		APIKey: "api-key", AuthID: "auth-1", AuthIndex: "codex:1", AuthType: "oauth", Source: "sdk",
		AttemptIndex:      &attempt,
		AccessTokenSHA256: "safe-token-hash",
		ReasoningEffort:   "high", ServiceTier: "auto", ResponseServiceTier: "priority",
		Speed: "fast", ResponseSpeed: "fast",
		RequestedAt: time.Unix(456, 0), Latency: 3 * time.Second, TTFT: time.Second,
		Failed: true, Fail: coreusage.Failure{StatusCode: 429, Body: "limited"},
		Detail: coreusage.Detail{
			InputTokens: 4, OutputTokens: 2, CachedTokens: 1, CacheReadTokens: 1, TotalTokens: 6,
			TokenBreakdown: coreusage.NewSubsetTokenBreakdown(4, 1, 0, 2, 0, 6),
		},
	}
	ctx := coreusage.WithRecordSnapshot(context.Background(), source)
	ctx = requestmeta.WithRequestID(ctx, "rpc-request")
	ctx = requestmeta.WithEndpoint(ctx, "POST /v1/responses")
	ctx = requestmeta.WithClientRequestMetadata(ctx, requestmeta.ClientRequestMetadata{
		ClientIP: "192.0.2.10", XForwardedFor: "198.51.100.10", UserAgent: "rpc-test",
	})
	ctx = coreusage.WithStream(ctx, true)
	ctx = apikeypolicy.WithDecision(ctx, apikeypolicy.RequestPolicyDecision{
		Mode: apikeypolicy.ModeProfile,
		Snapshot: &apikeypolicy.RequestPolicySnapshot{
			PolicyID: "policy-rpc", ProfileID: "profile-rpc", ProfileName: "RPC",
			RequestedModel: "smart", EffectiveModel: "gpt-5",
		},
	})
	client := &usageMetadataCaptureClient{}
	rpcAdapter := &rpcPluginAdapter{client: client}
	host := newHostWithRecords(capabilityRecord{
		id:     "observability",
		plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{UsagePlugin: rpcAdapter}},
	})
	adapter := &usageAdapter{host: host, pluginID: "observability"}

	adapter.HandleUsage(ctx, source)
	if client.method != "usage.handle" {
		t.Fatalf("method = %q", client.method)
	}
	var got pluginapi.UsageRecord
	if err := json.Unmarshal(client.payload, &got); err != nil {
		t.Fatalf("decode payload: %v; payload=%s", err, client.payload)
	}
	if got.Provider != "codex" || got.ExecutorType != "codex" || got.Model != "gpt-5" || got.Alias != "smart" || got.AuthID != "auth-1" || got.AuthIndex != "codex:1" {
		t.Fatalf("base rpc record = %#v", got)
	}
	if got.RequestID != "rpc-request" || got.Endpoint != "POST /v1/responses" || !got.Stream || got.AccessTokenSHA256 != "safe-token-hash" || got.AttemptIndex == nil || *got.AttemptIndex != 1 {
		t.Fatalf("rpc record = %#v", got)
	}
	if got.ClientIP != "192.0.2.10" || got.XForwardedFor != "198.51.100.10" || got.UserAgent != "rpc-test" {
		t.Fatalf("rpc client metadata = %#v", got)
	}
	if got.APIKeyPolicyID != "policy-rpc" || got.ProfileID != "profile-rpc" || got.ProfileNameSnapshot != "RPC" || got.PolicyMode != apikeypolicy.ModeProfile || got.RequestedModel != "smart" || got.EffectiveModel != "gpt-5" {
		t.Fatalf("rpc policy attribution = %#v", got)
	}
	if got.ResponseServiceTier != "priority" || got.Speed != "fast" || got.ResponseSpeed != "fast" || !got.Failed || got.Failure.StatusCode != 429 {
		t.Fatalf("rpc outcome metadata = %#v", got)
	}
	if got.TokenBreakdown.TotalTokens != 6 || got.TokenBreakdown.Input.CacheReadTokens != 1 {
		t.Fatalf("rpc token breakdown = %#v", got.TokenBreakdown)
	}
}
