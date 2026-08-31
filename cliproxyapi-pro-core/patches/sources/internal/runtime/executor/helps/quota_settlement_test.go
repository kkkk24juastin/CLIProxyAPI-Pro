package helps

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

func TestUsageReporterSettlesQuotaSynchronouslyWithDistinctRecordIDs(t *testing.T) {
	type settlement struct {
		eventID string
		tokens  int64
	}
	settlements := make([]settlement, 0, 2)
	ctx := apikeypolicy.WithQuotaSettlement(context.Background(), func(_ context.Context, eventID string, totalTokens int64) error {
		settlements = append(settlements, settlement{eventID: eventID, tokens: totalTokens})
		return nil
	})
	reporter := NewUsageReporter(ctx, "codex", "gpt-5", nil)
	reporter.Publish(ctx, usage.Detail{TotalTokens: 3})
	reporter.PublishAdditionalModel(ctx, "gpt-image-1", usage.Detail{TotalTokens: 4})

	if len(settlements) != 2 {
		t.Fatalf("synchronous quota settlements = %#v, want two before Publish returns", settlements)
	}
	if settlements[0].tokens != 3 || settlements[1].tokens != 4 {
		t.Fatalf("quota settlement tokens = %#v", settlements)
	}
	if settlements[0].eventID == "" || settlements[0].eventID == settlements[1].eventID {
		t.Fatalf("quota settlement event IDs = %#v, want distinct stable record identities", settlements)
	}
}

func TestUsageReporterPassesAuthTypeAndServiceTiersToCostSettlement(t *testing.T) {
	var settled apikeypolicy.QuotaUsageDelta
	ctx := apikeypolicy.WithQuotaUsageSettlement(context.Background(), func(_ context.Context, _ string, usageDelta apikeypolicy.QuotaUsageDelta) error {
		settled = usageDelta
		return nil
	})
	auth := &cliproxyauth.Auth{Provider: "codex", Attributes: map[string]string{"auth_kind": "oauth"}}
	reporter := NewUsageReporter(usage.WithServiceTier(ctx, "fast"), "codex", "gpt-5.6-sol", auth)
	reporter.Publish(ctx, usage.Detail{InputTokens: 8, OutputTokens: 2, TotalTokens: 10, ResponseServiceTier: "default"})

	if settled.Provider != "codex" || settled.AuthType != "oauth" || settled.Model != "gpt-5.6-sol" ||
		settled.ServiceTier != "fast" || settled.EffectiveServiceTier != "default" || settled.TotalTokens != 10 {
		t.Fatalf("settled usage = %#v, want Codex OAuth tier metadata", settled)
	}
}
