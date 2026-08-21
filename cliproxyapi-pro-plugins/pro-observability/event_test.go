package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testUsageRecord(t *testing.T) []byte {
	t.Helper()
	attempt := int64(2)
	record := usageRecord{
		Provider:            "codex",
		ExecutorType:        "codex",
		Model:               "gpt-5",
		Alias:               "smart",
		APIKey:              "sk-client-secret",
		AuthID:              "auth-1",
		AuthIndex:           "codex:1",
		AuthType:            "oauth",
		Source:              "very-long-secret-source-value-1234567890",
		RequestID:           "request-1",
		Endpoint:            "POST /v1/responses",
		AccessTokenSHA256:   strings.Repeat("a", 64),
		ClientIP:            "192.0.2.1",
		XForwardedFor:       "198.51.100.2",
		UserAgent:           "plugin-contract-test",
		APIKeyPolicyID:      "policy-1",
		ProfileID:           "profile-1",
		ProfileNameSnapshot: "Production",
		PolicyMode:          "profile",
		RequestedModel:      "smart",
		EffectiveModel:      "gpt-5",
		ReasoningEffort:     "high",
		ServiceTier:         "auto",
		ResponseServiceTier: "priority",
		Speed:               "fast",
		ResponseSpeed:       "fast",
		Generate:            true,
		RequestedAt:         time.Date(2026, 8, 21, 1, 2, 3, 456_000_000, time.UTC),
		Latency:             3 * time.Second,
		TTFT:                time.Second,
		AttemptIndex:        &attempt,
		Stream:              true,
		Failed:              true,
		Failure: usageFailure{
			StatusCode: http.StatusTooManyRequests,
			Body:       `{"error":{"message":"rate limited"}}`,
		},
		Detail: usageDetail{
			InputTokens: 11, OutputTokens: 7, ReasoningTokens: 3,
			CachedTokens: 2, CacheReadTokens: 2, CacheCreationTokens: 1, TotalTokens: 18,
		},
		AccountingVersion: 2,
		TokenBreakdown: rpcTokenBreakdown{
			SchemaVersion: 2, Quality: "complete", TotalTokens: 18,
			Input: rpcTokenInputBreakdown{
				TotalTokens: 11, UncachedTokens: 8, CacheReadTokens: 2, CacheWriteTokens: 1,
			},
			Output: rpcTokenOutputBreakdown{
				TotalTokens: 7, NonReasoningTokens: 4, ReasoningTokens: 3,
			},
		},
		ResponseHeaders: http.Header{
			"OpenAI-Request-ID": []string{"upstream-1"},
			"Retry-After":       []string{"30"},
		},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUsageEventFromRPCPreservesMonitoringContract(t *testing.T) {
	raw := testUsageRecord(t)
	event, err := usageEventFromRPC(raw, time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if event.RequestID != "request-1" || event.Endpoint != "POST /v1/responses" || event.Method != "POST" || event.Path != "/v1/responses" {
		t.Fatalf("request metadata = %#v", event)
	}
	if event.APIKeyPolicyID != "policy-1" || event.ProfileID != "profile-1" || event.ProfileNameSnapshot != "Production" || event.PolicyMode != "profile" {
		t.Fatalf("policy attribution = %#v", event)
	}
	if event.ClientIP != "192.0.2.1" || event.XForwardedFor != "198.51.100.2" || event.UserAgent != "plugin-contract-test" {
		t.Fatalf("client metadata = %#v", event)
	}
	if event.AccountingVersion != 2 || event.AccountingQuality != "complete" || event.InputTokens != 11 || event.OutputTokens != 7 || event.ReasoningTokens != 3 || event.CacheTokens != 3 || event.TotalTokens != 18 {
		t.Fatalf("accounting = %#v", event)
	}
	if event.StatusCode == nil || *event.StatusCode != http.StatusTooManyRequests || event.ErrorMessage != "rate limited" || event.UpstreamRequestID != "upstream-1" || event.RetryAfter != "30" {
		t.Fatalf("outcome = %#v", event)
	}
	if event.AttemptIndex == nil || *event.AttemptIndex != 2 || !event.Stream || event.LatencyMS == nil || *event.LatencyMS != 3000 || event.TTFTMS == nil || *event.TTFTMS != 1000 {
		t.Fatalf("timing = %#v", event)
	}
	if event.SourceHash != hashString("very-long-secret-source-value-1234567890") || event.APIKeyHash != hashString("sk-client-secret") {
		t.Fatalf("sensitive hashes = %#v", event)
	}
	if strings.Contains(event.RawJSON, "sk-client-secret") || !strings.Contains(event.RawJSON, `"APIKey":"[redacted]"`) {
		t.Fatalf("raw JSON was not redacted: %s", event.RawJSON)
	}
	const hostCompatibleEventHash = "780b08cba7fb46adddda3657d18b484b25b9e5a2e6041a77031b1128dbc47f32"
	if event.EventHash != hostCompatibleEventHash || event.EventHash != buildEventHash(event) {
		t.Fatalf("event hash = %q", event.EventHash)
	}
}

func TestUsageEventFromRPCUsesSafeUnclassifiedFallback(t *testing.T) {
	record := usageRecord{
		Model:       "custom",
		RequestedAt: time.Unix(100, 0).UTC(),
		Detail:      usageDetail{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	event, err := usageEventFromRPC(raw, time.Unix(101, 0))
	if err != nil {
		t.Fatal(err)
	}
	if event.AccountingQuality != "unclassified" || event.UnclassifiedTokens != 15 || event.TotalTokens != 15 {
		t.Fatalf("fallback accounting = %#v", event)
	}
}
