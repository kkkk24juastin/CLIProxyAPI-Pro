package inspection

import "testing"

func TestQuotaDecisionHonorsDisabledAndThreshold(t *testing.T) {
	used := 100.0
	if got := QuotaDecision(false, &used, true, 100); got.Action != ActionDisable || !got.IsQuota {
		t.Fatalf("enabled exhausted decision = %+v", got)
	}
	if got := QuotaDecision(true, &used, true, 100); got.Action != ActionKeep || !got.IsQuota {
		t.Fatalf("disabled exhausted decision = %+v", got)
	}
}

func TestHealthyDecisionReenablesDisabledAccount(t *testing.T) {
	if got := HealthyDecision(true); got.Action != ActionEnable {
		t.Fatalf("healthy decision = %+v", got)
	}
}
