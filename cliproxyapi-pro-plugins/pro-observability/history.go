package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const usageHistoryStartCursorValue = int64(1<<63 - 1)

type usageHistoryCursor struct {
	SnapshotMaxID     int64  `json:"snapshot_max_id"`
	MatchedTotal      int64  `json:"matched_total"`
	BeforeTimestamp   int64  `json:"before_timestamp_ms"`
	BeforeID          int64  `json:"before_id"`
	FromMS            int64  `json:"from_ms,omitempty"`
	ToMS              int64  `json:"to_ms,omitempty"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	AuthIndex         string `json:"auth_index,omitempty"`
	SearchAuthIndexes string `json:"search_auth_indexes,omitempty"`
	APIKeyHash        string `json:"api_key_hash,omitempty"`
	APIKeyPolicyID    string `json:"api_key_policy_id,omitempty"`
	ProfileID         string `json:"profile_id,omitempty"`
	PolicyMode        string `json:"policy_mode,omitempty"`
	Status            string `json:"status,omitempty"`
	Search            string `json:"search,omitempty"`
}

type usageEventQueryOptions struct {
	SnapshotMaxID     int64
	BeforeTimestamp   int64
	BeforeID          int64
	FromMS            int64
	ToMS              int64
	Provider          string
	Model             string
	AuthIndex         string
	SearchAuthIndexes string
	APIKeyHash        string
	APIKeyPolicyID    string
	ProfileID         string
	PolicyMode        string
	Failed            *bool
	Search            string
	Limit             int
	MatchedTotal      int64
	SkipCount         bool
}

type usageEventQueryPage struct {
	Events       []usageEvent
	MatchedTotal int64
	HasMore      bool
}

func handleUsageHistoryQuery(query url.Values) ([]byte, error) {
	limit := boundedUsageEventLimit(queryInt(query, "limit", 100))
	cursorValue := strings.TrimSpace(query.Get("cursor"))
	status := ""
	var options usageEventQueryOptions
	if cursorValue != "" {
		cursor, err := decodeUsageHistoryCursor(cursorValue)
		if err != nil {
			return envelopedManagementJSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		options = usageEventQueryOptionsFromCursor(cursor, limit)
		status = cursor.Status
	} else {
		failed, normalizedStatus := usageStatusFilter(query.Get("status"))
		status = normalizedStatus
		options = usageEventQueryOptions{
			FromMS:            queryInt64(query, "from_ms", 0),
			ToMS:              queryInt64(query, "to_ms", 0),
			Provider:          strings.TrimSpace(query.Get("provider")),
			Model:             strings.TrimSpace(query.Get("model")),
			AuthIndex:         strings.TrimSpace(query.Get("auth_index")),
			SearchAuthIndexes: strings.TrimSpace(query.Get("search_auth_indexes")),
			APIKeyHash:        strings.TrimSpace(query.Get("api_key_hash")),
			APIKeyPolicyID:    strings.TrimSpace(query.Get("api_key_policy_id")),
			ProfileID:         strings.TrimSpace(query.Get("profile_id")),
			PolicyMode:        strings.TrimSpace(query.Get("policy_mode")),
			Failed:            failed,
			Search:            strings.TrimSpace(query.Get("search")),
			Limit:             limit,
		}
	}

	return withReadableStore(func(store *usageStore) (managementResponse, error) {
		if cursorValue == "" {
			latestID, err := store.latestCursor(context.Background())
			if err != nil {
				return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			if latestID <= 0 {
				payload := buildUsagePayload(nil)
				state, stateErr := store.summary(context.Background())
				if stateErr != nil {
					return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": stateErr.Error()})
				}
				payload.Generation = state.Generation
				payload.ResetAtMS = state.ResetAtMS
				payload.DetailsLimit = int64(limit)
				return jsonManagementResponse(http.StatusOK, payload)
			}
			options.SnapshotMaxID = latestID
		}

		page, err := store.queryHistoryEvents(context.Background(), options)
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		pageCursor := cursorValue
		if pageCursor == "" {
			pageCursor = encodeUsageHistoryCursor(usageHistoryCursorFromOptions(
				options,
				status,
				page.MatchedTotal,
				usageEvent{TimestampMS: usageHistoryStartCursorValue, ID: usageHistoryStartCursorValue},
			))
		}
		payload, err := buildUsageHistoryPayload(store, page, options, status, pageCursor, limit)
		if err != nil {
			return jsonManagementResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return jsonManagementResponse(http.StatusOK, payload)
	})
}

func envelopedManagementJSON(status int, payload any) ([]byte, error) {
	response, err := jsonManagementResponse(status, payload)
	if err != nil {
		return nil, err
	}
	return okEnvelope(response)
}

func boundedUsageEventLimit(requested int) int {
	if requested <= 0 || requested > 5000 {
		return 5000
	}
	return requested
}

func encodeUsageHistoryCursor(cursor usageHistoryCursor) string {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeUsageHistoryCursor(value string) (usageHistoryCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return usageHistoryCursor{}, fmt.Errorf("invalid usage cursor")
	}
	var cursor usageHistoryCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return usageHistoryCursor{}, fmt.Errorf("invalid usage cursor")
	}
	if cursor.SnapshotMaxID <= 0 || cursor.BeforeTimestamp <= 0 || cursor.BeforeID <= 0 {
		return usageHistoryCursor{}, fmt.Errorf("invalid usage cursor")
	}
	return cursor, nil
}

func usageStatusFilter(value string) (*bool, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "failed", "failure":
		failed := true
		return &failed, "failed"
	case "success", "succeeded":
		failed := false
		return &failed, "success"
	default:
		return nil, ""
	}
}

func usageEventQueryOptionsFromCursor(cursor usageHistoryCursor, limit int) usageEventQueryOptions {
	failed, _ := usageStatusFilter(cursor.Status)
	return usageEventQueryOptions{
		SnapshotMaxID:     cursor.SnapshotMaxID,
		BeforeTimestamp:   cursor.BeforeTimestamp,
		BeforeID:          cursor.BeforeID,
		FromMS:            cursor.FromMS,
		ToMS:              cursor.ToMS,
		Provider:          cursor.Provider,
		Model:             cursor.Model,
		AuthIndex:         cursor.AuthIndex,
		SearchAuthIndexes: cursor.SearchAuthIndexes,
		APIKeyHash:        cursor.APIKeyHash,
		APIKeyPolicyID:    cursor.APIKeyPolicyID,
		ProfileID:         cursor.ProfileID,
		PolicyMode:        cursor.PolicyMode,
		Failed:            failed,
		Search:            cursor.Search,
		Limit:             limit,
		MatchedTotal:      cursor.MatchedTotal,
		SkipCount:         true,
	}
}

func usageHistoryCursorFromOptions(options usageEventQueryOptions, status string, matchedTotal int64, event usageEvent) usageHistoryCursor {
	return usageHistoryCursor{
		SnapshotMaxID:     options.SnapshotMaxID,
		MatchedTotal:      matchedTotal,
		BeforeTimestamp:   event.TimestampMS,
		BeforeID:          event.ID,
		FromMS:            options.FromMS,
		ToMS:              options.ToMS,
		Provider:          options.Provider,
		Model:             options.Model,
		AuthIndex:         options.AuthIndex,
		SearchAuthIndexes: options.SearchAuthIndexes,
		APIKeyHash:        options.APIKeyHash,
		APIKeyPolicyID:    options.APIKeyPolicyID,
		ProfileID:         options.ProfileID,
		PolicyMode:        options.PolicyMode,
		Status:            status,
		Search:            options.Search,
	}
}

func buildUsageHistoryPayload(store *usageStore, page usageEventQueryPage, options usageEventQueryOptions, status, pageCursor string, limit int) (usagePayload, error) {
	payload := buildUsagePayload(page.Events)
	state, err := store.summary(context.Background())
	if err != nil {
		return usagePayload{}, err
	}
	payload.Generation = state.Generation
	payload.ResetAtMS = state.ResetAtMS
	payload.DetailsLimit = int64(limit)
	payload.DetailsLimited = page.HasMore
	payload.MatchedTotal = page.MatchedTotal
	payload.SnapshotMaxID = options.SnapshotMaxID
	payload.PageCursor = pageCursor
	payload.HasMore = page.HasMore
	if page.HasMore && len(page.Events) > 0 {
		payload.NextCursor = encodeUsageHistoryCursor(usageHistoryCursorFromOptions(
			options,
			status,
			page.MatchedTotal,
			page.Events[len(page.Events)-1],
		))
	}
	return payload, nil
}
