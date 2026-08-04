package inspection

import (
	"math/rand"
	"strings"
)

func ShouldInspectCandidate(hasAuth, apiKey bool, provider, targetType string) bool {
	if !hasAuth || apiKey {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	targetType = strings.ToLower(strings.TrimSpace(targetType))
	if !IsSupportedProvider(provider) {
		return false
	}
	return targetType == ProviderAll || targetType == provider
}

func Sample[T any](items []T, sampleSize int, seed int64) []T {
	if sampleSize <= 0 || sampleSize >= len(items) {
		return items
	}
	out := append([]T(nil), items...)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out[:sampleSize]
}

func ProviderLimiters(providers map[string]struct{}, concurrency int) map[string]chan struct{} {
	if concurrency <= 0 {
		concurrency = 1
	}
	limiters := make(map[string]chan struct{}, len(providers))
	for provider := range providers {
		limiters[provider] = make(chan struct{}, concurrency)
	}
	return limiters
}
