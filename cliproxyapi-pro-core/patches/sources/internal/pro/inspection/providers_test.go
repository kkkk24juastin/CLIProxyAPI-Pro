package inspection

import "testing"

func TestAntigravityParserBuildsCanonicalGroups(t *testing.T) {
	groups, err := BuildAntigravityGroups(`{
		"groups":[{"displayName":"Claude and GPT models","description":"premium models","buckets":[
			{"bucketId":"weekly","window":"weekly","remainingFraction":0.75,"resetTime":"2026-01-08T00:00:00Z"},
			{"bucketId":"five-hour","window":"5h","remainingFraction":0.25,"resetTime":"2026-01-01T05:00:00Z"}
		]}]
	}`)
	if err != nil || len(groups) != 1 || groups[0]["id"] != "claude-gpt" {
		t.Fatalf("groups/error = %+v / %v", groups, err)
	}
	used := AntigravityUsedPercent(groups, AntigravityQuotaModeClaudeGPT)
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v", used)
	}
	buckets := groups[0]["buckets"].([]map[string]any)
	if buckets[0]["resetAtMs"] != int64(1767830400000) || buckets[0]["periodHours"] != float64(168) {
		t.Fatalf("weekly timeline fields = %+v", buckets[0])
	}
	if buckets[1]["resetAtMs"] != int64(1767243600000) || buckets[1]["periodHours"] != float64(5) {
		t.Fatalf("five-hour timeline fields = %+v", buckets[1])
	}
}

func TestClaudeAndCodexWindowParsers(t *testing.T) {
	claude, extra, err := BuildClaudeWindows(`{
		"five_hour":{"utilization":25,"resets_at":"2026-01-01T00:00:00Z"},
		"seven_day":{"utilization":50,"resets_at":"2026-01-07T00:00:00Z"},
		"extra_usage":{"is_enabled":true}
	}`)
	if err != nil || len(claude) != 2 || extra == nil {
		t.Fatalf("claude windows/extra/error = %+v / %+v / %v", claude, extra, err)
	}
	if claude[0]["resetAtMs"] != int64(1767225600000) || claude[0]["periodHours"] != float64(5) {
		t.Fatalf("claude five-hour timeline fields = %+v", claude[0])
	}
	if claude[1]["resetAtMs"] != int64(1767744000000) || claude[1]["periodHours"] != float64(168) {
		t.Fatalf("claude weekly timeline fields = %+v", claude[1])
	}

	_, codex, used := BuildCodexWindows(`{
		"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":12,"reset_at":1767225600},"secondary_window":{"limit_window_seconds":604800,"used_percent":42,"reset_at":1767744000}},
		"code_review_rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":5,"reset_at":1767243600}}
	}`)
	if len(codex) != 3 || used == nil || *used != 42 {
		t.Fatalf("codex windows/used = %+v / %v", codex, used)
	}
	if codex[0]["resetAtMs"] != int64(1767225600000) || codex[0]["periodHours"] != float64(5) {
		t.Fatalf("codex five-hour timeline fields = %+v", codex[0])
	}
	if codex[1]["resetAtMs"] != int64(1767744000000) || codex[1]["periodHours"] != float64(168) {
		t.Fatalf("codex weekly timeline fields = %+v", codex[1])
	}
}

func TestKimiParserNormalizesLimits(t *testing.T) {
	rows, used, err := BuildKimiRows(`{
		"limits":[{"name":"Weekly","limit":100,"used":40,"reset_at":"2026-01-08T00:00:00Z","window":{"duration":1,"timeUnit":"WEEKS"}}]
	}`)
	if err != nil || len(rows) != 1 || rows[0]["limit"] != 100 || rows[0]["used"] != 40 {
		t.Fatalf("rows/error = %+v / %v", rows, err)
	}
	if used == nil || *used != 40 {
		t.Fatalf("used percent = %v", used)
	}
	if rows[0]["resetAtMs"] != int64(1767830400000) || rows[0]["periodHours"] != float64(168) {
		t.Fatalf("kimi timeline fields = %+v", rows[0])
	}
}
