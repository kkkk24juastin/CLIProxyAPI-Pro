package inspection

import (
	"reflect"
	"testing"
)

func TestShouldInspectCandidate(t *testing.T) {
	tests := []struct {
		hasAuth  bool
		apiKey   bool
		provider string
		target   string
		want     bool
	}{
		{hasAuth: true, provider: "codex", target: ProviderAll, want: true},
		{hasAuth: true, provider: " XAI ", target: "xai", want: true},
		{hasAuth: true, provider: "codex", target: "xai", want: false},
		{hasAuth: true, apiKey: true, provider: "codex", target: ProviderAll, want: false},
		{hasAuth: false, provider: "codex", target: ProviderAll, want: false},
		{hasAuth: true, provider: "unknown", target: ProviderAll, want: false},
	}
	for _, tt := range tests {
		if got := ShouldInspectCandidate(tt.hasAuth, tt.apiKey, tt.provider, tt.target); got != tt.want {
			t.Fatalf("ShouldInspectCandidate(%v, %v, %q, %q) = %v, want %v", tt.hasAuth, tt.apiKey, tt.provider, tt.target, got, tt.want)
		}
	}
}

func TestSampleUsesCopyAndStableSeed(t *testing.T) {
	source := []int{1, 2, 3, 4, 5}
	first := Sample(source, 3, 42)
	second := Sample(source, 3, 42)
	if !reflect.DeepEqual(first, second) || len(first) != 3 {
		t.Fatalf("samples = %#v and %#v", first, second)
	}
	if !reflect.DeepEqual(source, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("source mutated: %#v", source)
	}
	if got := Sample(source, 0, 42); &got[0] != &source[0] {
		t.Fatal("unbounded sample unexpectedly copied source")
	}
}

func TestProviderLimiters(t *testing.T) {
	limiters := ProviderLimiters(map[string]struct{}{"codex": {}, "xai": {}}, 2)
	if len(limiters) != 2 || cap(limiters["codex"]) != 2 || cap(limiters["xai"]) != 2 {
		t.Fatalf("limiters = %#v", limiters)
	}
}
