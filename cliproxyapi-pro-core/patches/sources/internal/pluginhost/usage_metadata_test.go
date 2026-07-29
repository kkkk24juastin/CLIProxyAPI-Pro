package pluginhost

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/requestmeta"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type usageMetadataPluginFunc func(context.Context, pluginapi.UsageRecord)

func (f usageMetadataPluginFunc) HandleUsage(ctx context.Context, record pluginapi.UsageRecord) {
	f(ctx, record)
}

func TestUsageAdapterPublishesObservabilityMetadata(t *testing.T) {
	var captured pluginapi.UsageRecord
	plugin := usageMetadataPluginFunc(func(_ context.Context, record pluginapi.UsageRecord) {
		captured = record
	})
	host := newHostWithRecords(capabilityRecord{
		id: "usage-metadata",
		plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{
			UsagePlugin: plugin,
		}},
	})
	adapter := &usageAdapter{host: host, pluginID: "usage-metadata"}
	ctx := requestmeta.WithRequestID(context.Background(), "request-42")
	ctx = requestmeta.WithEndpoint(ctx, "POST /v1/responses")
	ctx = requestmeta.WithClientRequestMetadata(ctx, requestmeta.ClientRequestMetadata{
		ClientIP: "192.0.2.10", XForwardedFor: "198.51.100.2", UserAgent: "canary-client/1.0",
	})
	ctx = requestmeta.WithResponseStatusHolder(ctx)
	ctx = requestmeta.WithResponseHeadersHolder(ctx)
	requestmeta.SetResponseStatus(ctx, http.StatusTooManyRequests)
	requestmeta.SetResponseHeaders(ctx, http.Header{"X-Request-Id": []string{"upstream-42"}})
	ctx = coreusage.WithStream(ctx, true)
	ctx = coreusage.WithServiceTier(ctx, "priority")
	ctx = coreusage.WithReasoningEffort(ctx, "high")
	attempt := int64(2)
	adapter.HandleUsage(ctx, coreusage.Record{
		Provider: "codex", ExecutorType: "codex", Model: "gpt-test", AttemptIndex: &attempt,
		ResponseServiceTier: "default",
		Detail:              coreusage.Detail{InputTokens: 30, OutputTokens: 10, CacheReadTokens: 5, TotalTokens: 40},
	})

	if captured.ContractVersion != pluginapi.UsageRecordContractVersion ||
		captured.RequestID != "request-42" || captured.Endpoint != "POST /v1/responses" ||
		captured.ClientIP != "192.0.2.10" || captured.XForwardedFor != "198.51.100.2" ||
		captured.UserAgent != "canary-client/1.0" {
		t.Fatalf("request metadata = %#v", captured)
	}
	if !captured.Stream || captured.AttemptIndex == nil || *captured.AttemptIndex != 2 ||
		captured.ReasoningEffort != "high" || captured.ServiceTier != "priority" ||
		captured.ResponseServiceTier != "default" {
		t.Fatalf("request semantics = %#v", captured)
	}
	if !captured.Failed || captured.Failure.StatusCode != http.StatusTooManyRequests ||
		captured.ResponseHeaders.Get("X-Request-Id") != "upstream-42" {
		t.Fatalf("response metadata = %#v", captured)
	}
	if captured.Detail.TokenBreakdown.SchemaVersion != coreusage.TokenAccountingSchemaVersion ||
		captured.Detail.TokenBreakdown.TotalTokens != 40 {
		t.Fatalf("token breakdown = %#v", captured.Detail.TokenBreakdown)
	}
}
