package management

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	proinspection "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/inspection"
	proquota "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/quota"
)

func TestListAuthFilesFromDiskIncludesInspectionAndCodexPlanMetadata(t *testing.T) {
	authDir := t.TempDir()
	fileName := "codex-user.json"
	idToken := testCodexIDToken(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id":                       "acct-1",
			"chatgpt_plan_type":                        "pro",
			"chatgpt_subscription_active_until":        float64(1790000000),
			"chatgpt_subscription_active_start":        float64(1780000000),
			"chatgpt_subscription_last_checked":        "2026-06-22T00:00:00Z",
			"rate_limit_reset_credits_available_count": float64(3),
		},
	})
	content := map[string]any{
		"type":     "codex",
		"email":    "user@example.com",
		"id_token": idToken,
		"last_error": map[string]any{
			"code":        "token_refresh_error",
			"message":     "refresh failed",
			"http_status": float64(401),
		},
	}
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("Marshal auth content error = %v", err)
	}
	if err = os.WriteFile(filepath.Join(authDir, fileName), raw, 0o600); err != nil {
		t.Fatalf("WriteFile auth content error = %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, nil)
	entry := firstAuthFileEntry(t, h)

	lastError, ok := entry["last_error"].(map[string]any)
	if !ok {
		t.Fatalf("last_error = %#v, want object", entry["last_error"])
	}
	if lastError["code"] != "token_refresh_error" || lastError["message"] != "refresh failed" {
		t.Fatalf("last_error = %#v, want token_refresh_error/refresh failed", lastError)
	}
	idTokenEntry, ok := entry["id_token"].(map[string]any)
	if !ok {
		t.Fatalf("id_token = %#v, want object", entry["id_token"])
	}
	if idTokenEntry["plan_type"] != "pro" || idTokenEntry["chatgpt_account_id"] != "acct-1" || idTokenEntry["chatgpt_subscription_active_until"] != float64(1790000000) {
		t.Fatalf("id_token entry = %#v, want codex plan/subscription claims", idTokenEntry)
	}
}

func testCodexIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "none", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("Marshal JWT header error = %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal JWT claims error = %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON) + "."
}

func TestBuildAntigravityGroupsSupportsSummaryBuckets(t *testing.T) {
	body := `{
		"groups": [{
			"displayName": "Claude/GPT",
			"description": "premium models",
			"buckets": [
				{"bucketId": "weekly", "displayName": "Weekly", "window": "weekly", "remainingFraction": 0.75, "resetTime": "2026-01-02T03:04:05Z"},
				{"bucket_id": "five-hour", "display_name": "Five hour", "window": "5h", "remaining_fraction": 0.25, "reset_time": "2026-01-01T03:04:05Z"}
			]
		}]
	}`

	groups, err := proinspection.BuildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("proinspection.BuildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	if groups[0]["id"] != "claude-gpt" {
		t.Fatalf("group id = %#v, want claude-gpt", groups[0]["id"])
	}
	buckets, ok := groups[0]["buckets"].([]map[string]any)
	if !ok || len(buckets) != 2 {
		t.Fatalf("buckets = %#v, want two parsed buckets", groups[0]["buckets"])
	}
	if _, ok := groups[0]["remainingFraction"]; ok {
		t.Fatalf("remainingFraction is present on group, want latest bucket-only shape")
	}
	if buckets[0]["id"] != "weekly" || buckets[1]["id"] != "five-hour" {
		t.Fatalf("bucket order = %q/%q, want weekly/five-hour", buckets[0]["id"], buckets[1]["id"])
	}
	used := proinspection.AntigravityGroupUsedPercent(map[string]any{"buckets": buckets})
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v, want 75", used)
	}
}

func TestBuildAntigravityGroupsCanonicalizesLatestGroups(t *testing.T) {
	body := `{
		"groups": [
			{
				"buckets": [
					{"bucketId": "gemini-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": "2026-06-20T00:39:10Z", "remainingFraction": 0.9997293},
					{"bucketId": "gemini-5h", "displayName": "Five Hour Limit", "window": "5h", "resetTime": "2026-06-17T15:04:15Z", "remainingFraction": 1}
				],
				"displayName": "Gemini Models",
				"description": "Models within this group: Gemini Flash, Gemini Pro"
			},
			{
				"buckets": [
					{"bucketId": "3p-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": "2026-06-24T04:38:44Z", "remainingFraction": 0.9914995},
					{"bucketId": "3p-5h", "displayName": "Five Hour Limit", "window": "5h", "resetTime": "2026-06-17T12:12:15Z", "remainingFraction": 0.999886}
				],
				"displayName": "Claude and GPT models",
				"description": "Models within this group: Claude Opus, Claude Sonnet, GPT-OSS"
			}
		]
	}`

	groups, err := proinspection.BuildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("proinspection.BuildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups len = %d, want 2", len(groups))
	}
	if groups[0]["id"] != "gemini" || groups[1]["id"] != "claude-gpt" {
		t.Fatalf("group ids = %#v/%#v, want gemini/claude-gpt", groups[0]["id"], groups[1]["id"])
	}
	used := proinspection.AntigravityUsedPercent(groups, accountInspectionAntigravityQuotaModeClaudeGpt)
	if used == nil || math.Abs(*used-0.85005) > 0.0001 {
		t.Fatalf("claude-gpt used percent = %v, want about 0.85005", used)
	}
}

func TestBuildAntigravityGroupsSupportsWrappedBody(t *testing.T) {
	body := `{
		"body": "{\"groups\":[{\"displayName\":\"Claude/GPT\",\"buckets\":[{\"bucketId\":\"weekly\",\"displayName\":\"Weekly\",\"window\":\"weekly\",\"remainingFraction\":0.5}]}]}"
	}`

	groups, err := proinspection.BuildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("proinspection.BuildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	buckets, ok := groups[0]["buckets"].([]map[string]any)
	if !ok || len(buckets) != 1 || buckets[0]["remainingFraction"] != 0.5 {
		t.Fatalf("wrapped buckets = %#v, want one 0.5 bucket", groups[0]["buckets"])
	}
}

func TestBuildAntigravitySubscriptionMapsPaidTierPlan(t *testing.T) {
	payload := map[string]any{
		"currentTier": map[string]any{"id": "free-tier", "name": "Free"},
		"paidTier": map[string]any{
			"id":   "g1-ultra-tier",
			"name": "Ultra",
			"availableCredits": []any{
				map[string]any{
					"creditType":                  "AI",
					"creditAmount":                float64(20),
					"minimumCreditAmountForUsage": "1",
				},
			},
		},
	}

	subscription := proinspection.BuildAntigravitySubscription(payload)
	if subscription == nil {
		t.Fatal("proinspection.BuildAntigravitySubscription() = nil, want subscription")
	}
	if subscription["plan"] != "ultra" || subscription["tierId"] != "g1-ultra-tier" || subscription["source"] != "paid" {
		t.Fatalf("subscription = %#v, want ultra paid tier", subscription)
	}
	credits, ok := subscription["availableCredits"].([]map[string]any)
	if !ok || len(credits) != 1 || credits[0]["creditType"] != "AI" {
		t.Fatalf("availableCredits = %#v, want AI credit entry", subscription["availableCredits"])
	}
}

func TestAntigravityUsedPercentFallsBackWhenClaudeGroupNameChanges(t *testing.T) {
	groups := []map[string]any{{
		"id":    "quota-group-1",
		"label": "Premium Models",
		"buckets": []map[string]any{{
			"id":                "weekly",
			"label":             "Weekly",
			"remainingFraction": 0.35,
		}},
	}}

	used := proinspection.AntigravityUsedPercent(groups, accountInspectionAntigravityQuotaModeClaudeGpt)
	if used == nil || *used != 65 {
		t.Fatalf("used percent = %v, want 65 fallback from buckets", used)
	}
	if model := proinspection.SelectAntigravityDeepProbeModel(""); model != "claude-sonnet-4-6" {
		t.Fatalf("deep probe model = %q, want default claude-sonnet-4-6", model)
	}
}

func TestBuildAntigravityGroupsRejectsLegacyModelsShape(t *testing.T) {
	body := `{
		"models": {
			"claude-sonnet-4-6": {"quotaInfo": {"remainingFraction": 0.4, "resetTime": "2026-01-02T03:04:05Z"}},
			"gpt-oss-120b-medium": {"quota_info": {"remaining_fraction": 0.8}}
		}
	}`

	if _, err := proinspection.BuildAntigravityGroups(body); err == nil {
		t.Fatalf("proinspection.BuildAntigravityGroups() error = nil, want legacy models shape rejected")
	}
}

func TestBuildCodexWindowsClassifiesTeamMonthlyWindows(t *testing.T) {
	body := `{
		"rate_limit": {
			"primary_window": {"limit_window_seconds": 18000, "used_percent": 12.5, "reset_after_seconds": 60},
			"secondary_window": {"limit_window_seconds": 2592000, "used_percent": 42.5, "reset_after_seconds": 120},
			"allowed": true
		},
		"code_review_rate_limit": {
			"primary_window": {"limit_window_seconds": 18000, "used_percent": 5},
			"secondary_window": {"limit_window_seconds": 2419200, "used_percent": 88}
		},
		"additional_rate_limits": [{
			"limit_name": "Premium Tokens",
			"rate_limit": {
				"primary_window": {"limit_window_seconds": 18000, "used_percent": 11},
				"secondary_window": {"limit_window_seconds": 2678400, "used_percent": 22}
			}
		}]
	}`

	_, windows, used := proinspection.BuildCodexWindows(body)
	if used == nil || *used != 88 {
		t.Fatalf("used percent = %v, want 88", used)
	}
	labelsByID := make(map[string]string)
	for _, window := range windows {
		id, _ := window["id"].(string)
		labelKey, _ := window["labelKey"].(string)
		labelsByID[id] = labelKey
	}
	if labelsByID["monthly"] != "codex_quota.team_secondary_window" {
		t.Fatalf("monthly label = %q, want team secondary", labelsByID["monthly"])
	}
	if labelsByID["code-review-monthly"] != "codex_quota.code_review_team_secondary_window" {
		t.Fatalf("code review monthly label = %q, want code review team secondary", labelsByID["code-review-monthly"])
	}
	if labelsByID["premium-tokens-monthly-0"] != "codex_quota.additional_team_secondary_window" {
		t.Fatalf("additional monthly label = %q, want additional team secondary", labelsByID["premium-tokens-monthly-0"])
	}
}

func TestCodexQuotaStateValuesIncludesSubscriptionAndResetCredits(t *testing.T) {
	auth := &coreauth.Auth{
		Metadata: map[string]any{
			"id_token": map[string]any{
				"chatgpt_subscription_active_until": float64(1790000000),
			},
		},
		Attributes: map[string]string{
			"plan_type": "plus",
		},
	}
	payload := map[string]any{
		"rate_limit_reset_credits": map[string]any{
			"available_count": float64(2),
		},
	}
	windows := []map[string]any{{"id": "five-hour"}}

	values := codexQuotaStateValues(auth, payload, windows, `{"rate_limit":{}}`)
	if values["planType"] != "plus" {
		t.Fatalf("planType = %#v, want plus", values["planType"])
	}
	if values["subscriptionActiveUntil"] != float64(1790000000) {
		t.Fatalf("subscriptionActiveUntil = %#v, want id token timestamp", values["subscriptionActiveUntil"])
	}
	if values["rateLimitResetCreditsAvailableCount"] != float64(2) {
		t.Fatalf("rateLimitResetCreditsAvailableCount = %#v, want 2", values["rateLimitResetCreditsAvailableCount"])
	}
	if values["rawShapeHash"] == "" {
		t.Fatalf("rawShapeHash = %q, want populated", values["rawShapeHash"])
	}
}

func TestBuildXAIBillingSummaryParsesBillingConfig(t *testing.T) {
	body := `{
		"config": {
			"monthlyLimit": {"val": 10000},
			"used": {"val": 2500},
			"onDemandCap": {"val": 5000},
			"billingPeriodStart": "2026-06-01T00:00:00Z",
			"billingPeriodEnd": "2026-07-01T00:00:00Z"
		}
	}`

	billing, used, err := proquota.BuildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("proquota.BuildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 25 {
		t.Fatalf("used percent = %v, want 25", used)
	}
	if billing["monthlyLimitCents"] != 10000.0 || billing["usedCents"] != 2500.0 || billing["onDemandCapCents"] != 5000.0 {
		t.Fatalf("billing cents = %+v, want parsed cent values", billing)
	}
	if billing["billingPeriodEnd"] != "2026-07-01T00:00:00Z" {
		t.Fatalf("billing period end = %#v", billing["billingPeriodEnd"])
	}
	if billing["usedPercent"] != 25.0 {
		t.Fatalf("billing usedPercent = %#v, want 25", billing["usedPercent"])
	}
	if billing["includedUsedCents"] != 2500.0 {
		t.Fatalf("includedUsedCents = %#v, want 2500", billing["includedUsedCents"])
	}
	if billing["onDemandUsedCents"] != 0.0 {
		t.Fatalf("onDemandUsedCents = %#v, want 0", billing["onDemandUsedCents"])
	}
}

func TestBuildXAIBillingSummarySupportsSnakeCaseAndNumericValues(t *testing.T) {
	body := `{
		"config": {
			"monthly_limit": 8000,
			"used": 6000,
			"on_demand_cap": 12000,
			"billing_period_end": "2026-07-01T00:00:00Z"
		}
	}`

	billing, used, err := proquota.BuildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("proquota.BuildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v, want 75", used)
	}
	if billing["monthlyLimitCents"] != 8000.0 || billing["usedCents"] != 6000.0 || billing["onDemandCapCents"] != 12000.0 {
		t.Fatalf("billing cents = %+v, want parsed snake_case numeric values", billing)
	}
}

func TestBuildXAIBillingSummarySupportsWeeklyCreditsShape(t *testing.T) {
	body := `{
		"config": {
			"currentPeriod": {
				"type": "weekly",
				"start": "2026-07-06T00:00:00Z",
				"end": "2026-07-13T00:00:00Z"
			},
			"creditUsagePercent": 64.5,
			"productUsage": [
				{"product": "Grok", "usagePercent": 50},
				{"product": "Think", "usage_percent": "82.25"}
			]
		}
	}`

	billing, used, err := proquota.BuildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("proquota.BuildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 64.5 {
		t.Fatalf("used percent = %v, want 64.5", used)
	}
	if billing["periodType"] != "weekly" || billing["usagePercent"] != 64.5 {
		t.Fatalf("weekly billing = %+v, want weekly usage percent", billing)
	}
	if billing["periodEnd"] != "2026-07-13T00:00:00Z" {
		t.Fatalf("periodEnd = %#v, want weekly period end", billing["periodEnd"])
	}
	usage, ok := billing["productUsage"].([]map[string]any)
	if !ok || len(usage) != 2 {
		t.Fatalf("productUsage = %#v, want 2 normalized items", billing["productUsage"])
	}
	if usage[1]["usagePercent"] != 82.25 {
		t.Fatalf("second usagePercent = %#v, want 82.25", usage[1]["usagePercent"])
	}
}

func TestXAIPlanTypeFromMonthlyBilling(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "free missing limit", body: `{"config": {}}`, want: "free"},
		{name: "free ignores on demand cap", body: `{"config": {"onDemandCap": {"val": 20000}}}`, want: "free"},
		{name: "free zero limit", body: `{"config": {"monthlyLimit": {"val": 0}}}`, want: "free"},
		{name: "supergrok", body: `{"config": {"monthlyLimit": {"val": 15000}}}`, want: "supergrok"},
		{name: "x premium plus", body: `{"config": {"monthlyLimit": {"val": 20000}}}`, want: "x-premium-plus"},
		{name: "supergrok heavy", body: `{"config": {"monthlyLimit": {"val": 150000}}}`, want: "supergrok-heavy"},
		{name: "unknown paid", body: `{"config": {"monthlyLimit": {"val": 99000}}}`, want: "paid-unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := proquota.XAIPlanTypeFromBillingBody(http.StatusOK, tt.body)
			if !known || got != tt.want {
				t.Fatalf("proquota.XAIPlanTypeFromBillingBody() = %q, %v; want %q, true", got, known, tt.want)
			}
		})
	}
	if got, known := proquota.XAIPlanTypeFromBillingBody(http.StatusUnauthorized, `{"config": {}}`); known || got != "" {
		t.Fatalf("failed billing inferred plan = %q, %v", got, known)
	}
}

func TestXAISummaryUsedPercentUsesFreeQuotaOnlyForFreePlan(t *testing.T) {
	freeQuota := map[string]any{"usedTokens": 75.0, "limitTokens": 100.0, "exhausted": false}
	free := proquota.EmptyXAIBillingSummary()
	free["planType"] = "free"
	free["freeQuota"] = freeQuota
	if got := proquota.XAISummaryUsedPercent(free); got == nil || *got != 75 {
		t.Fatalf("free used percent = %v, want 75", got)
	}
	paid := proquota.EmptyXAIBillingSummary()
	paid["planType"] = "x-premium-plus"
	paid["freeQuota"] = map[string]any{"exhausted": true}
	if got := proquota.XAISummaryUsedPercent(paid); got != nil {
		t.Fatalf("paid free-model exhaustion used percent = %v, want nil", got)
	}
}
