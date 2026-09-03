package quota

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestXAIPaidHealthSummary(t *testing.T) {
	summary := XAIPaidHealthSummary()
	if summary["mode"] != "paid-health" || summary["planType"] != "paid" || summary["healthStatus"] != "chat-ok" {
		t.Fatalf("summary = %#v", summary)
	}
	if _, exists := summary["freeQuota"]; exists {
		t.Fatalf("summary contains freeQuota: %#v", summary)
	}
}

func TestXAIBillingParserCombinesWeeklyAndMonthlyShapes(t *testing.T) {
	weekly, used, err := BuildXAIBillingSummary(`{
		"config":{"current_period":{"type":"weekly","start":"2026-07-06T00:00:00Z","end":"2026-07-13T00:00:00Z"},"credit_usage_percent":10,"product_usage":[{"product":"Grok","usage_percent":25}]}
	}`)
	if err != nil || used == nil || *used != 10 {
		t.Fatalf("weekly/used/error = %+v / %v / %v", weekly, used, err)
	}
	monthly, _, err := BuildXAIBillingSummary(`{
		"config":{"monthly_limit":150000,"used":160000,"on_demand_cap":20000,"billing_period_end":"2026-08-01T00:00:00Z"}
	}`)
	if err != nil {
		t.Fatalf("monthly error = %v", err)
	}
	merged := MergeXAIBillingSummaries(weekly, monthly)
	if merged["periodType"] != "weekly" || merged["monthlyLimitCents"] != 150000.0 || merged["onDemandUsedPercent"] != 50.0 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged["resetAtMs"] != int64(1783900800000) || merged["periodHours"] != float64(168) {
		t.Fatalf("merged timeline fields = %+v", merged)
	}
	if merged["periodEnd"] != "2026-07-13T00:00:00Z" {
		t.Fatalf("merged period borrowed monthly clock = %+v", merged)
	}
}

func TestCacheParserVersionCoversCodexWindowClassification(t *testing.T) {
	if CacheParserVersion != 7 {
		t.Fatalf("CacheParserVersion = %d, want 7", CacheParserVersion)
	}
}

func TestXAIPlanAndFreeQuotaSemantics(t *testing.T) {
	plan, known := XAIPlanTypeFromBillingBody(200, `{"config":{"monthlyLimit":{"val":20000}}}`)
	if !known || plan != "paid-unknown" {
		t.Fatalf("plan = %q/%v", plan, known)
	}
	free := EmptyXAIBillingSummary()
	free["planType"] = "free"
	free["freeQuota"] = map[string]any{"usedTokens": 75.0, "limitTokens": 100.0}
	if used := XAISummaryUsedPercent(free); used == nil || *used != 75 {
		t.Fatalf("free used percent = %v", used)
	}
	paid := EmptyXAIBillingSummary()
	paid["planType"] = plan
	paid["freeQuota"] = map[string]any{"exhausted": true}
	if used := XAISummaryUsedPercent(paid); used != nil {
		t.Fatalf("paid free quota leaked into used percent: %v", used)
	}
}

func TestXAIPlanTypeFromAccessToken(t *testing.T) {
	tests := []struct {
		tier int
		want string
	}{
		{tier: 0, want: "free"},
		{tier: 1, want: "supergrok"},
		{tier: 2, want: "x-basic"},
		{tier: 3, want: "x-premium"},
		{tier: 4, want: "x-premium-plus"},
		{tier: 5, want: "supergrok-heavy"},
		{tier: 6, want: "supergrok-lite"},
		{tier: 9, want: "paid-unknown"},
	}
	for _, tt := range tests {
		payload := []byte(fmt.Sprintf(`{"tier":%v}`, tt.tier))
		token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
		if got, known := XAIPlanTypeFromAccessToken(token); !known || got != tt.want {
			t.Fatalf("XAIPlanTypeFromAccessToken(tier=%v) = %q, %v; want %q, true", tt.tier, got, known, tt.want)
		}
	}
	for _, token := range []string{"", "not-a-jwt", "header.invalid.signature"} {
		if got, known := XAIPlanTypeFromAccessToken(token); known || got != "" {
			t.Fatalf("XAIPlanTypeFromAccessToken(%q) = %q, %v; want unknown", token, got, known)
		}
	}
	for _, claims := range []string{`{}`, `{"tier":-1}`, `{"tier":1.5}`} {
		token := "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
		if got, known := XAIPlanTypeFromAccessToken(token); known || got != "" {
			t.Fatalf("XAIPlanTypeFromAccessToken(%s) = %q, %v; want unknown", claims, got, known)
		}
	}
}
