package redisqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/requestmeta"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func apiKeyPolicyUsageContext(status int) context.Context {
	decision := apikeypolicy.RequestPolicyDecision{Mode: apikeypolicy.ModeProfile, Snapshot: &apikeypolicy.RequestPolicySnapshot{
		PolicyID: "policy-start", ProfileID: "profile-start", ProfileName: "Profile at request start",
		RequestedModel: "smart", EffectiveModel: "gpt-5",
	}}
	ctx := apikeypolicy.WithDecision(context.Background(), decision)
	ctx = requestmeta.WithResponseStatusHolder(ctx)
	requestmeta.SetResponseStatus(ctx, status)
	return ctx
}

func TestUsageQueuePolicyAttributionIsFrozenForEveryTerminalOutcome(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		record coreusage.Record
	}{
		{name: "success", status: http.StatusOK, record: coreusage.Record{Provider: "codex", Model: "gpt-5"}},
		{name: "ordinary failure", status: http.StatusBadGateway, record: coreusage.Record{Provider: "codex", Model: "gpt-5", Failed: true, Fail: coreusage.Failure{StatusCode: http.StatusBadGateway, Body: "upstream failed"}}},
		{name: "stream cancellation", status: 499, record: coreusage.Record{Provider: "codex", Model: "gpt-5", Failed: true, Fail: coreusage.Failure{StatusCode: 499, Body: "context canceled"}}},
		{name: "response translation failure", status: http.StatusBadGateway, record: coreusage.Record{Provider: "codex", Model: "gpt-5", Failed: true, Fail: coreusage.Failure{StatusCode: http.StatusBadGateway, Body: "response translation failed"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withEnabledQueue(t, func() {
				ctx := apiKeyPolicyUsageContext(test.status)
				(&usageQueuePlugin{}).HandleUsage(ctx, test.record)
				payload := popSinglePayload(t)
				requireStringField(t, payload, "policy_mode", "profile")
				requireStringField(t, payload, "api_key_policy_id", "policy-start")
				requireStringField(t, payload, "profile_id", "profile-start")
				requireStringField(t, payload, "profile_name_snapshot", "Profile at request start")
				requireStringField(t, payload, "requested_model", "smart")
				requireStringField(t, payload, "effective_model", "gpt-5")
				if items := PopOldest(10); len(items) != 0 {
					t.Fatalf("terminal outcome emitted %d duplicate usage records", len(items))
				}
			})
		})
	}
}

func TestUsageQueueRetryAttemptsKeepOneFrozenProfileAttributionEach(t *testing.T) {
	withEnabledQueue(t, func() {
		ctx := apiKeyPolicyUsageContext(http.StatusOK)
		for attempt := int64(0); attempt < 2; attempt++ {
			current := attempt
			(&usageQueuePlugin{}).HandleUsage(ctx, coreusage.Record{
				Provider: "codex", Model: "gpt-5", AttemptIndex: &current,
				Failed: attempt == 0, Fail: coreusage.Failure{StatusCode: http.StatusBadGateway, Body: "retryable"},
			})
		}
		items := PopOldest(10)
		if len(items) != 2 {
			t.Fatalf("retry usage records=%d, want 2 attempts", len(items))
		}
		for _, item := range items {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(item, &payload); err != nil {
				t.Fatal(err)
			}
			requireStringField(t, payload, "api_key_policy_id", "policy-start")
			requireStringField(t, payload, "profile_id", "profile-start")
			requireStringField(t, payload, "profile_name_snapshot", "Profile at request start")
		}
	})
}
