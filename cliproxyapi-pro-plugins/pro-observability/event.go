package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var endpointPattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD)\s+(\S+)`)

func usageEventFromRPC(raw []byte, now time.Time) (usageEvent, error) {
	var record usageRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return usageEvent{}, fmt.Errorf("decode usage record: %w", err)
	}
	if now.IsZero() {
		now = time.Now()
	}
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = now
	}
	timestamp = timestamp.UTC()

	provider := fallbackString(record.Provider, "unknown")
	executorType := fallbackString(record.ExecutorType, "unknown")
	model := fallbackString(record.Model, "unknown")
	alias := fallbackString(record.Alias, model)
	authType := fallbackString(record.AuthType, "unknown")
	endpoint, method, path := normalizeEndpoint(record.Endpoint)

	breakdown := tokenBreakdownFromRPC(record.TokenBreakdown)
	if !breakdown.valid() {
		breakdown = unclassifiedTokenBreakdown(record.Detail.TotalTokens)
	}
	inputTokens := record.Detail.InputTokens
	outputTokens := record.Detail.OutputTokens
	reasoningTokens := record.Detail.ReasoningTokens
	cachedTokens := record.Detail.CachedTokens
	cacheReadTokens := record.Detail.CacheReadTokens
	cacheWriteTokens := record.Detail.CacheCreationTokens
	cacheTokens := cacheReadTokens + cacheWriteTokens
	totalTokens := record.Detail.TotalTokens
	if breakdown.Quality == "complete" {
		inputTokens = breakdown.Input.TotalTokens
		outputTokens = breakdown.Output.TotalTokens
		reasoningTokens = breakdown.Output.ReasoningTokens
		cacheReadTokens = breakdown.Input.CacheReadTokens
		cacheWriteTokens = breakdown.Input.CacheWriteTokens
		cachedTokens = cacheReadTokens
		cacheTokens = cacheReadTokens + cacheWriteTokens
		totalTokens = breakdown.TotalTokens
	} else if totalTokens <= 0 {
		totalTokens = breakdown.TotalTokens
	}
	breakdownJSON, err := json.Marshal(breakdown)
	if err != nil {
		return usageEvent{}, fmt.Errorf("encode token breakdown: %w", err)
	}

	latencyMS := record.Latency.Milliseconds()
	ttftMS := record.TTFT.Milliseconds()
	statusCode := record.Failure.StatusCode
	failed := record.Failed || statusCode >= http.StatusBadRequest
	if statusCode == 0 && !failed {
		statusCode = http.StatusOK
	} else if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	errorMessage := ""
	if failed {
		errorMessage = summarizeErrorMessage(record.Failure.Body)
	}

	rawJSON, err := redactedRawJSON(raw)
	if err != nil {
		return usageEvent{}, err
	}
	event := usageEvent{
		RequestID:            strings.TrimSpace(record.RequestID),
		TimestampMS:          timestamp.UnixMilli(),
		Timestamp:            timestamp.Format(time.RFC3339Nano),
		Provider:             provider,
		ExecutorType:         executorType,
		Model:                model,
		Alias:                alias,
		Endpoint:             endpoint,
		Method:               method,
		Path:                 path,
		AuthType:             authType,
		AuthIndex:            strings.TrimSpace(record.AuthIndex),
		Source:               maskSource(record.Source),
		SourceHash:           hashString(record.Source),
		APIKeyHash:           hashString(record.APIKey),
		APIKeyPolicyID:       strings.TrimSpace(record.APIKeyPolicyID),
		ProfileID:            strings.TrimSpace(record.ProfileID),
		ProfileNameSnapshot:  strings.TrimSpace(record.ProfileNameSnapshot),
		PolicyMode:           strings.TrimSpace(record.PolicyMode),
		RequestedModel:       strings.TrimSpace(record.RequestedModel),
		EffectiveModel:       strings.TrimSpace(record.EffectiveModel),
		ClientIP:             limitRunes(record.ClientIP, 256),
		XForwardedFor:        limitRunes(record.XForwardedFor, 2048),
		UserAgent:            limitRunes(record.UserAgent, 1024),
		InputTokens:          inputTokens,
		OutputTokens:         outputTokens,
		ReasoningTokens:      reasoningTokens,
		CachedTokens:         cachedTokens,
		CacheTokens:          cacheTokens,
		CacheReadTokens:      cacheReadTokens,
		CacheWriteTokens:     cacheWriteTokens,
		TotalTokens:          totalTokens,
		AccountingVersion:    breakdown.SchemaVersion,
		AccountingQuality:    breakdown.Quality,
		UncachedInputTokens:  breakdown.Input.UncachedTokens,
		UnclassifiedTokens:   breakdown.UnclassifiedTokens,
		TokenBreakdownJSON:   string(breakdownJSON),
		LatencyMS:            &latencyMS,
		TTFTMS:               &ttftMS,
		StatusCode:           &statusCode,
		ErrorMessage:         errorMessage,
		UpstreamRequestID:    responseHeader(record.ResponseHeaders, "x-upstream-request-id", "x-request-id", "openai-request-id", "anthropic-request-id", "cf-ray"),
		RetryAfter:           responseHeader(record.ResponseHeaders, "retry-after"),
		AttemptIndex:         cloneInt64(record.AttemptIndex),
		Stream:               record.Stream,
		ReasoningEffort:      strings.TrimSpace(record.ReasoningEffort),
		ServiceTier:          strings.TrimSpace(record.ServiceTier),
		EffectiveServiceTier: strings.TrimSpace(record.ResponseServiceTier),
		Speed:                strings.TrimSpace(record.Speed),
		EffectiveSpeed:       strings.TrimSpace(record.ResponseSpeed),
		Failed:               failed,
		RawJSON:              rawJSON,
		CreatedAtMS:          now.UnixMilli(),
	}
	event.EventHash = buildEventHash(event)
	return event, nil
}

func tokenBreakdownFromRPC(value rpcTokenBreakdown) tokenBreakdown {
	return tokenBreakdown{
		SchemaVersion: value.SchemaVersion,
		Quality:       strings.TrimSpace(value.Quality),
		TotalTokens:   value.TotalTokens,
		Input: tokenInputBreakdown{
			TotalTokens:      value.Input.TotalTokens,
			UncachedTokens:   value.Input.UncachedTokens,
			CacheReadTokens:  value.Input.CacheReadTokens,
			CacheWriteTokens: value.Input.CacheWriteTokens,
		},
		Output: tokenOutputBreakdown{
			TotalTokens:        value.Output.TotalTokens,
			NonReasoningTokens: value.Output.NonReasoningTokens,
			ReasoningTokens:    value.Output.ReasoningTokens,
		},
		UnclassifiedTokens: value.UnclassifiedTokens,
	}
}

func unclassifiedTokenBreakdown(total int64) tokenBreakdown {
	if total < 0 {
		return tokenBreakdown{SchemaVersion: 2, Quality: "inconsistent"}
	}
	if total == 0 {
		return tokenBreakdown{SchemaVersion: 2, Quality: "complete"}
	}
	return tokenBreakdown{
		SchemaVersion:      2,
		Quality:            "unclassified",
		TotalTokens:        total,
		UnclassifiedTokens: total,
	}
}

func normalizeEndpoint(value string) (string, string, string) {
	endpoint := strings.TrimSpace(value)
	if endpoint == "" {
		return "-", "", ""
	}
	match := endpointPattern.FindStringSubmatch(endpoint)
	if len(match) != 3 {
		return endpoint, "", ""
	}
	return endpoint, strings.ToUpper(match[1]), match[2]
}

func redactedRawJSON(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("decode usage record for redaction: %w", err)
	}
	redacted, err := json.Marshal(redactValue(decoded))
	if err != nil {
		return "", fmt.Errorf("encode redacted usage record: %w", err)
	}
	return string(redacted), nil
}

func redactValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			if isSecretKey(key) {
				result[key] = "[redacted]"
				continue
			}
			result[key] = redactValue(child)
		}
		return result
	case []any:
		result := make([]any, 0, len(item))
		for _, child := range item {
			result = append(result, redactValue(child))
		}
		return result
	default:
		return value
	}
}

func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	return normalized == "api_key" || normalized == "apikey" ||
		normalized == "authorization" || normalized == "cookie" ||
		normalized == "set_cookie" || normalized == "access_token" ||
		normalized == "refresh_token" || normalized == "token" ||
		strings.Contains(normalized, "secret")
}

func hashString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

func buildEventHash(event usageEvent) string {
	parts := []string{
		event.RequestID,
		event.Timestamp,
		event.Endpoint,
		event.Model,
		event.AuthIndex,
		event.SourceHash,
		event.APIKeyHash,
		event.APIKeyPolicyID,
		event.ProfileID,
		event.PolicyMode,
		event.RequestedModel,
		event.EffectiveModel,
		event.ServiceTier,
		event.EffectiveServiceTier,
		event.Speed,
		event.EffectiveSpeed,
		strconv.FormatInt(event.InputTokens, 10),
		strconv.FormatInt(event.OutputTokens, 10),
		strconv.FormatInt(event.ReasoningTokens, 10),
		strconv.FormatInt(maxInt64(event.CachedTokens, event.CacheTokens), 10),
		strconv.FormatBool(event.Failed),
	}
	if event.LatencyMS != nil {
		parts = append(parts, strconv.FormatInt(*event.LatencyMS, 10))
	}
	if event.AttemptIndex != nil {
		parts = append(parts, strconv.FormatInt(*event.AttemptIndex, 10))
	}
	return hashString(strings.Join(parts, "|"))
}

func maskSource(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "@") {
		parts := strings.SplitN(trimmed, "@", 2)
		prefix := parts[0]
		if len(prefix) > 3 {
			prefix = prefix[:3]
		}
		return prefix + "***@" + parts[1]
	}
	if looksSecret(trimmed) {
		if len(trimmed) <= 8 {
			return "m:****"
		}
		return "m:" + trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
	}
	return trimmed
}

func looksSecret(value string) bool {
	if strings.ContainsAny(value, " /\\") {
		return false
	}
	return strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "AIza") || len(value) >= 32
}

func summarizeErrorMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err == nil {
		if nested, ok := payload["error"].(map[string]any); ok {
			if message := strings.TrimSpace(fmt.Sprint(nested["message"])); message != "<nil>" && message != "" {
				value = message
			}
		} else if message := strings.TrimSpace(fmt.Sprint(payload["message"])); message != "<nil>" && message != "" {
			value = message
		}
	}
	if len(value) > 240 {
		return value[:240]
	}
	return value
}

func responseHeader(headers http.Header, names ...string) string {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[normalizeHeaderName(name)] = true
	}
	for name, values := range headers {
		if !wanted[normalizeHeaderName(name)] {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func normalizeHeaderName(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
}

func limitRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func fallbackString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
