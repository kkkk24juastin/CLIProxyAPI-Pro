package inspection

import "testing"

func TestAntigravityParserBuildsCanonicalGroups(t *testing.T) {
	groups, err := BuildAntigravityGroups(`{
		"groups":[{"displayName":"Claude and GPT models","description":"premium models","buckets":[
			{"bucketId":"weekly","window":"weekly","remainingFraction":0.75},
			{"bucketId":"five-hour","window":"5h","remainingFraction":0.25}
		]}]
	}`)
	if err != nil || len(groups) != 1 || groups[0]["id"] != "claude-gpt" {
		t.Fatalf("groups/error = %+v / %v", groups, err)
	}
	used := AntigravityUsedPercent(groups, AntigravityQuotaModeClaudeGPT)
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v", used)
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

	_, codex, used := BuildCodexWindows(`{
		"rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":12},"secondary_window":{"limit_window_seconds":604800,"used_percent":42}},
		"code_review_rate_limit":{"primary_window":{"limit_window_seconds":18000,"used_percent":5}}
	}`)
	if len(codex) != 3 || used == nil || *used != 42 {
		t.Fatalf("codex windows/used = %+v / %v", codex, used)
	}
}

func TestKimiParserNormalizesLimits(t *testing.T) {
	rows, used, err := BuildKimiRows(`{
		"limits":[{"name":"Weekly","limit":100,"used":40,"reset_in":3600}]
	}`)
	if err != nil || len(rows) != 1 || rows[0]["limit"] != 100 || rows[0]["used"] != 40 {
		t.Fatalf("rows/error = %+v / %v", rows, err)
	}
	if used == nil || *used != 40 {
		t.Fatalf("used percent = %v", used)
	}
}
