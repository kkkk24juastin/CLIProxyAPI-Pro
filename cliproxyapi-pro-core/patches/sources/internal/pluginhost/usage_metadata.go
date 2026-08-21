package pluginhost

import (
	"context"

	apikeypolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/requestmeta"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// enrichRPCUsageRecord preserves the request and accounting context that the
// in-process monitoring sink receives today. Keeping this bridge in the RPC
// adapter lets an external UsagePlugin reach storage parity without teaching
// the generic usage bus about Pro policy types.
func enrichRPCUsageRecord(ctx context.Context, record pluginapi.UsageRecord) pluginapi.UsageRecord {
	if source, ok := coreusage.RecordFromContext(ctx); ok {
		record.AccessTokenSHA256 = source.AccessTokenSHA256
		record.AttemptIndex = cloneInt64Pointer(source.AttemptIndex)
		record.ResponseServiceTier = source.ResponseServiceTier
		record.Speed = source.Speed
		record.ResponseSpeed = source.ResponseSpeed
		record.AccountingVersion = coreusage.TokenAccountingSchemaVersion
		record.TokenBreakdown = pluginTokenBreakdown(source.Detail.TokenBreakdown)
	}
	record.RequestID = requestmeta.GetRequestID(ctx)
	record.Endpoint = requestmeta.GetEndpoint(ctx)
	record.Stream = coreusage.StreamFromContext(ctx)
	if record.RequestedModel == "" {
		record.RequestedModel = coreusage.RequestedModelAliasFromContext(ctx)
	}
	client := requestmeta.GetClientRequestMetadata(ctx)
	record.ClientIP = client.ClientIP
	record.XForwardedFor = client.XForwardedFor
	record.UserAgent = client.UserAgent
	if decision, ok := apikeypolicy.DecisionFromContext(ctx); ok {
		attribution := decision.UsageAttribution()
		record.PolicyMode = attribution.PolicyMode
		record.APIKeyPolicyID = attribution.APIKeyPolicyID
		record.ProfileID = attribution.ProfileID
		record.ProfileNameSnapshot = attribution.ProfileName
		if attribution.RequestedModel != "" {
			record.RequestedModel = attribution.RequestedModel
		}
		record.EffectiveModel = attribution.EffectiveModel
	}
	return record
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func pluginTokenBreakdown(value coreusage.TokenBreakdown) pluginapi.UsageTokenBreakdown {
	return pluginapi.UsageTokenBreakdown{
		SchemaVersion: value.SchemaVersion,
		Quality:       string(value.Quality),
		TotalTokens:   value.TotalTokens,
		Input: pluginapi.UsageTokenInputBreakdown{
			TotalTokens:      value.Input.TotalTokens,
			UncachedTokens:   value.Input.UncachedTokens,
			CacheReadTokens:  value.Input.CacheReadTokens,
			CacheWriteTokens: value.Input.CacheWriteTokens,
		},
		Output: pluginapi.UsageTokenOutputBreakdown{
			TotalTokens:        value.Output.TotalTokens,
			NonReasoningTokens: value.Output.NonReasoningTokens,
			ReasoningTokens:    value.Output.ReasoningTokens,
		},
		UnclassifiedTokens: value.UnclassifiedTokens,
	}
}
