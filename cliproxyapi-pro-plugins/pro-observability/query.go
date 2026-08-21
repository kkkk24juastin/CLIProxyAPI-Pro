package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func handleUsageQuery(query url.Values) ([]byte, error) {
	defaultLimit := positiveEnvironmentInt("USAGE_QUERY_LIMIT", 50000)
	limit := queryInt(query, "limit", defaultLimit)
	if limit <= 0 {
		limit = defaultLimit
	}
	return withReadableStore(func(store *usageStore) (managementResponse, error) {
		latestID, err := store.latestCursor(context.Background())
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		events, err := store.recentEvents(context.Background(), limit)
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		payload := buildUsagePayload(events)
		payload.LatestID = maxInt64(payload.LatestID, latestID)
		state, err := store.summary(context.Background())
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		summary, err := store.summaryAt(context.Background(), payload.LatestID)
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		payload.TotalRequests = summary.TotalRequests
		payload.SuccessCount = summary.SuccessCount
		payload.FailureCount = summary.FailureCount
		payload.TotalTokens = summary.TotalTokens
		payload.Generation = state.Generation
		payload.ResetAtMS = state.ResetAtMS
		payload.DetailsLimit = int64(limit)
		payload.DetailsLimited = summary.TotalRequests > payload.DetailsCount
		return jsonManagementResponse(http.StatusOK, payload)
	})
}

func handleUsageEventsQuery(query url.Values) ([]byte, error) {
	if strings.TrimSpace(query.Get("cursor")) != "" || strings.EqualFold(strings.TrimSpace(query.Get("direction")), "before") {
		return handleUsageHistoryQuery(query)
	}
	afterID := queryInt64(query, "after_id", 0)
	limit := queryInt(query, "limit", positiveEnvironmentInt("USAGE_BATCH_SIZE", 100))
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	return withReadableStore(func(store *usageStore) (managementResponse, error) {
		events, err := store.eventsAfter(context.Background(), afterID, limit+1)
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		detailsLimited := len(events) > limit
		if detailsLimited {
			events = events[:limit]
		}
		summary, err := store.summary(context.Background())
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		payload := buildUsagePayload(events)
		payload.Generation = summary.Generation
		payload.ResetAtMS = summary.ResetAtMS
		payload.DetailsLimit = int64(limit)
		payload.DetailsLimited = detailsLimited
		return jsonManagementResponse(http.StatusOK, payload)
	})
}

func positiveEnvironmentInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func withReadableStore(run func(*usageStore) (managementResponse, error)) ([]byte, error) {
	activeRuntime.mu.RLock()
	defer activeRuntime.mu.RUnlock()
	if activeRuntime.store == nil || !activeRuntime.config.ReadEnabled {
		response, err := jsonManagementResponse(http.StatusServiceUnavailable, map[string]string{"error": "observability plugin reader is disabled"})
		if err != nil {
			return nil, err
		}
		return okEnvelope(response)
	}
	response, err := run(activeRuntime.store)
	if err != nil {
		return nil, err
	}
	return okEnvelope(response)
}

func jsonManagementResponse(status int, payload any) (managementResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return managementResponse{}, err
	}
	return managementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       body,
	}, nil
}

func queryInt(values url.Values, key string, fallback int) int {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func queryInt64(values url.Values, key string, fallback int64) int64 {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func buildUsagePayload(events []usageEvent) usagePayload {
	payload := usagePayload{
		DetailsCount: int64(len(events)),
		APIs:         make(map[string]*usageAPIAggregate),
	}
	for _, event := range events {
		payload.TotalRequests++
		if event.Failed {
			payload.FailureCount++
		} else {
			payload.SuccessCount++
		}
		payload.TotalTokens += event.TotalTokens
		if event.ID > payload.LatestID {
			payload.LatestID = event.ID
		}
		endpoint := event.Endpoint
		if endpoint == "" {
			endpoint = "-"
		}
		api := payload.APIs[endpoint]
		if api == nil {
			api = &usageAPIAggregate{Models: make(map[string]*usageModelAggregate)}
			payload.APIs[endpoint] = api
		}
		model := event.Model
		if model == "" {
			model = "-"
		}
		modelAggregate := api.Models[model]
		if modelAggregate == nil {
			modelAggregate = &usageModelAggregate{}
			api.Models[model] = modelAggregate
		}
		var costBreakdown json.RawMessage
		if json.Valid([]byte(event.CostBreakdownJSON)) {
			costBreakdown = json.RawMessage(event.CostBreakdownJSON)
		}
		breakdown := tokenBreakdown{}
		if json.Valid([]byte(event.TokenBreakdownJSON)) {
			_ = json.Unmarshal([]byte(event.TokenBreakdownJSON), &breakdown)
		}
		cacheInputTokens := int64(0)
		if event.AccountingQuality == "complete" {
			cacheInputTokens = event.InputTokens
		}
		modelAggregate.Details = append(modelAggregate.Details, usageDetailPayload{
			ID:                   event.ID,
			RequestID:            event.RequestID,
			Timestamp:            event.Timestamp,
			Source:               event.Source,
			AuthIndex:            event.AuthIndex,
			APIKeyHash:           event.APIKeyHash,
			APIKeyPolicyID:       event.APIKeyPolicyID,
			ProfileID:            event.ProfileID,
			ProfileNameSnapshot:  event.ProfileNameSnapshot,
			PolicyMode:           event.PolicyMode,
			RequestedModel:       event.RequestedModel,
			EffectiveModel:       event.EffectiveModel,
			ClientIP:             event.ClientIP,
			XForwardedFor:        event.XForwardedFor,
			UserAgent:            event.UserAgent,
			Provider:             event.Provider,
			ExecutorType:         event.ExecutorType,
			Alias:                event.Alias,
			AuthType:             event.AuthType,
			LatencyMS:            cloneInt64(event.LatencyMS),
			TTFTMS:               cloneInt64(event.TTFTMS),
			StatusCode:           cloneInt(event.StatusCode),
			ErrorCode:            event.ErrorCode,
			ErrorMessage:         event.ErrorMessage,
			UpstreamRequestID:    event.UpstreamRequestID,
			RetryAfter:           event.RetryAfter,
			AttemptIndex:         cloneInt64(event.AttemptIndex),
			Stream:               event.Stream,
			ReasoningEffort:      event.ReasoningEffort,
			ServiceTier:          event.ServiceTier,
			EffectiveServiceTier: event.EffectiveServiceTier,
			Speed:                event.Speed,
			EffectiveSpeed:       event.EffectiveSpeed,
			EstimatedCost:        cloneFloat64(event.EstimatedCost),
			PriceRuleID:          event.PriceRuleID,
			CostBreakdown:        costBreakdown,
			AccountingVersion:    event.AccountingVersion,
			AccountingQuality:    event.AccountingQuality,
			TokenBreakdown:       breakdown,
			Failed:               event.Failed,
			Tokens: usageTokens{
				InputTokens:      event.InputTokens,
				OutputTokens:     event.OutputTokens,
				ReasoningTokens:  event.ReasoningTokens,
				CachedTokens:     event.CachedTokens,
				CacheTokens:      event.CacheTokens,
				CacheReadTokens:  event.CacheReadTokens,
				CacheWriteTokens: event.CacheWriteTokens,
				CacheInputTokens: cacheInputTokens,
				TotalTokens:      event.TotalTokens,
			},
		})
	}
	return payload
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
