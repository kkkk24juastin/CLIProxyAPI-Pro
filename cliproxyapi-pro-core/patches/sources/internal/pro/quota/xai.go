// Package quota owns provider quota parsing and merge policy independently of
// the shared SQLite adapter and request transport.
package quota

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// CacheParserVersion is the shared version of normalized quota cache
	// records. Provider-specific parser versions may diverge from it later.
	CacheParserVersion = 5
	XAIParserVersion   = CacheParserVersion
)

type XAIObservation struct {
	FileName   string
	AuthIndex  string
	Email      string
	Label      string
	Model      string
	Status     int
	Header     http.Header
	Body       []byte
	ObservedAt time.Time
}

type CacheMutation struct {
	ID                  string
	Provider            string
	FileName            string
	AuthIndex           string
	IdentityFingerprint string
	Data                json.RawMessage
	CachedAt            int64
	ObservedAt          int64
	AccessedAt          int64
	Version             int
}

var (
	xaiFreeQuotaUsagePattern = regexp.MustCompile(`(?i)tokens\s*\(actual/limit\)\s*:\s*([0-9]+)\s*/\s*([0-9]+)`)
	xaiFreeQuotaModelPattern = regexp.MustCompile(`(?i)for\s+model\s+([a-z0-9._-]+)`)
)

func BuildXAIMutation(observation XAIObservation) (CacheMutation, bool, error) {
	fileName := strings.TrimSpace(observation.FileName)
	if fileName == "" {
		return CacheMutation{}, false, nil
	}
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	var freeQuota map[string]any
	if XAIHeadersStatus(observation.Status) {
		freeQuota = XAIRateLimitSnapshot(observation.Header, observation.Model, observedAt)
	}
	if XAIFreeQuotaExhausted(observation.Body) {
		freeQuota = XAIExhaustedQuotaSnapshot(observation.Body, observation.Model, observedAt)
	}
	if freeQuota == nil {
		return CacheMutation{}, false, nil
	}
	now := observedAt.UnixMilli()
	raw, err := json.Marshal(map[string]any{
		"status": "success", "schemaVersion": 2, "parserVersion": XAIParserVersion,
		"cachedAt": now, "billing": map[string]any{"freeQuota": freeQuota},
	})
	if err != nil {
		return CacheMutation{}, false, err
	}
	fingerprintSource := strings.Join([]string{
		"xai", strings.ToLower(fileName),
		strings.ToLower(strings.TrimSpace(observation.Email)),
		strings.ToLower(strings.TrimSpace(observation.Label)),
	}, "|")
	fingerprint := sha256.Sum256([]byte(fingerprintSource))
	return CacheMutation{
		ID: "xai:" + fileName, Provider: "xai", FileName: fileName,
		AuthIndex:           strings.TrimSpace(observation.AuthIndex),
		IdentityFingerprint: hex.EncodeToString(fingerprint[:]),
		Data:                raw, CachedAt: now, ObservedAt: now, AccessedAt: now, Version: 2,
	}, true, nil
}

func MergeXAIState(existing, incoming map[string]any) map[string]any {
	merged := cloneMap(existing)
	for key, value := range incoming {
		merged[key] = value
	}
	existingBilling, _ := existing["billing"].(map[string]any)
	incomingBilling, _ := incoming["billing"].(map[string]any)
	if existingBilling == nil && incomingBilling == nil {
		return merged
	}
	billing := cloneMap(existingBilling)
	for key, value := range incomingBilling {
		billing[key] = value
	}
	existingFree, _ := existingBilling["freeQuota"].(map[string]any)
	incomingFree, _ := incomingBilling["freeQuota"].(map[string]any)
	if existingFree != nil && (incomingFree == nil || observedAt(existingFree) > observedAt(incomingFree)) {
		billing["freeQuota"] = existingFree
	}
	merged["billing"] = billing
	return merged
}

func XAIHeadersStatus(status int) bool {
	return status == http.StatusSwitchingProtocols || (status >= 200 && status < 300)
}

func XAIRateLimitSnapshot(header http.Header, model string, observed time.Time) map[string]any {
	limitTokens, okLimit := headerInt64(header, "x-ratelimit-limit-tokens")
	remainingTokens, okRemaining := headerInt64(header, "x-ratelimit-remaining-tokens")
	if !okLimit || !okRemaining || limitTokens <= 0 {
		return nil
	}
	if remainingTokens > limitTokens {
		remainingTokens = limitTokens
	}
	snapshot := map[string]any{
		"source": "rate_limit_headers", "windowKind": "rolling_24h",
		"usedTokens": limitTokens - remainingTokens, "limitTokens": limitTokens,
		"remainingTokens": remainingTokens, "observedAt": observed.UnixMilli(),
		"exhausted": remainingTokens == 0,
	}
	if value, ok := headerInt64(header, "x-ratelimit-limit-requests"); ok {
		snapshot["limitRequests"] = value
	}
	if value, ok := headerInt64(header, "x-ratelimit-remaining-requests"); ok {
		snapshot["remainingRequests"] = value
	}
	if model = strings.TrimSpace(model); model != "" {
		snapshot["model"] = model
	}
	return snapshot
}

func XAIExhaustedQuotaSnapshot(body []byte, fallbackModel string, observed time.Time) map[string]any {
	snapshot := map[string]any{
		"source": "free_usage_exhausted", "windowKind": "rolling_24h",
		"observedAt": observed.UnixMilli(), "exhausted": true,
	}
	if matches := xaiFreeQuotaUsagePattern.FindSubmatch(body); len(matches) == 3 {
		used, usedErr := strconv.ParseInt(string(matches[1]), 10, 64)
		limit, limitErr := strconv.ParseInt(string(matches[2]), 10, 64)
		if usedErr == nil && limitErr == nil && limit > 0 {
			snapshot["usedTokens"] = used
			snapshot["limitTokens"] = limit
			snapshot["remainingTokens"] = int64(0)
		}
	}
	model := strings.TrimSpace(jsonField(body, "model"))
	if model == "" {
		if matches := xaiFreeQuotaModelPattern.FindSubmatch(body); len(matches) == 2 {
			model = string(matches[1])
		}
	}
	if model == "" {
		model = strings.TrimSpace(fallbackModel)
	}
	model = strings.TrimRight(model, ".,;:!?\"'()[]{}")
	if model != "" {
		snapshot["model"] = model
	}
	return snapshot
}

func XAIFreeQuotaExhausted(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "subscription:free-usage-exhausted") ||
		strings.Contains(lower, "used all the included free usage")
}

func cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func observedAt(value map[string]any) int64 {
	switch typed := value["observedAt"].(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func headerInt64(header http.Header, key string) (int64, bool) {
	if header == nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(header.Get(key)), 10, 64)
	return value, err == nil && value >= 0
}

func jsonField(body []byte, key string) string {
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	text, _ := value[key].(string)
	return text
}
