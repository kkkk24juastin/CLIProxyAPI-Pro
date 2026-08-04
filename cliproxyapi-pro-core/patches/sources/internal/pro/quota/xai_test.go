package quota

import (
	"net/http"
	"testing"
	"time"
)

func TestBuildXAIMutationFromWebsocketRateLimitHeaders(t *testing.T) {
	mutation, ok, err := BuildXAIMutation(XAIObservation{
		FileName: "xai.json", Model: "grok-free", Status: http.StatusSwitchingProtocols,
		Header: http.Header{
			"X-Ratelimit-Limit-Tokens":     {"100"},
			"X-Ratelimit-Remaining-Tokens": {"40"},
		},
		ObservedAt: time.Unix(100, 0),
	})
	if err != nil || !ok {
		t.Fatalf("BuildXAIMutation() = %#v, %v, %v", mutation, ok, err)
	}
	if mutation.Provider != "xai" || mutation.FileName != "xai.json" || mutation.Version != 2 {
		t.Fatalf("mutation = %#v", mutation)
	}
}

func TestMergeXAIStatePreservesNewerFreeQuota(t *testing.T) {
	existing := map[string]any{"billing": map[string]any{
		"freeQuota": map[string]any{"observedAt": float64(200), "remainingTokens": float64(10)},
	}}
	incoming := map[string]any{"billing": map[string]any{
		"planType":  "free",
		"freeQuota": map[string]any{"observedAt": float64(100), "remainingTokens": float64(80)},
	}}
	merged := MergeXAIState(existing, incoming)
	billing := merged["billing"].(map[string]any)
	if billing["planType"] != "free" || billing["freeQuota"].(map[string]any)["remainingTokens"] != float64(10) {
		t.Fatalf("merged billing = %#v", billing)
	}
}
