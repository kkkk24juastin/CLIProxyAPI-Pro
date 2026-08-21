package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func callManagementQuery(t *testing.T, path string, query url.Values) managementResponse {
	t.Helper()
	rawRequest, err := json.Marshal(managementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management" + path,
		Query:  query,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := dispatchMethod(methodManagementHandle, rawRequest)
	if err != nil {
		t.Fatal(err)
	}
	return decodeEnvelopeResult[managementResponse](t, raw)
}

func prepareShadowReaderDatabase(t *testing.T) string {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
	store, err := openUsageStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := usageEventFromRPC(testUsageRecord(t), time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.insertEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.RequestID = "request-2"
	second.TimestampMS++
	second.Timestamp = time.UnixMilli(second.TimestampMS).UTC().Format(time.RFC3339Nano)
	second.Failed = false
	statusOK := http.StatusOK
	second.StatusCode = &statusOK
	second.ErrorMessage = ""
	second.EventHash = buildEventHash(second)
	if _, err := store.insertEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	return databasePath
}

func TestShadowReaderReturnsHostCompatibleUsageAndIncrementalEvents(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	databasePath := prepareShadowReaderDatabase(t)
	config := "enabled: true\npriority: 100\nread-enabled: true\ndatabase-path: " + databasePath + "\n"
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config)); err != nil {
		t.Fatal(err)
	}
	status := activeRuntime.status(t.Context())
	if !status.ReadEnabled || status.WriteEnabled || !status.StoreOpen || status.MigrationMode != "shadow-reader" {
		t.Fatalf("reader status = %#v", status)
	}
	if _, err := dispatchMethod(methodUsageHandle, testUsageRecord(t)); err != nil {
		t.Fatal(err)
	}

	usageResponse := callManagementQuery(t, managementUsagePath, url.Values{"limit": []string{"1"}})
	if usageResponse.StatusCode != http.StatusOK || !strings.Contains(string(usageResponse.Body), `"total_requests":2`) || strings.Contains(string(usageResponse.Body), "TotalRequests") {
		t.Fatalf("usage response = %d %s", usageResponse.StatusCode, usageResponse.Body)
	}
	var usage usagePayload
	if err := json.Unmarshal(usageResponse.Body, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.TotalRequests != 2 || usage.SuccessCount != 1 || usage.FailureCount != 1 || usage.TotalTokens != 36 || usage.LatestID != 2 || usage.DetailsCount != 1 || usage.DetailsLimit != 1 || !usage.DetailsLimited || usage.Generation != 1 {
		t.Fatalf("usage payload = %#v", usage)
	}
	details := usage.APIs["POST /v1/responses"].Models["gpt-5"].Details
	if len(details) != 1 || details[0].ID != 2 || details[0].RequestID != "request-2" || details[0].APIKeyPolicyID != "policy-1" || details[0].Tokens.CacheWriteTokens != 1 {
		t.Fatalf("usage details = %#v", details)
	}

	eventsResponse := callManagementQuery(t, managementEventsPath, url.Values{"after_id": []string{"0"}, "limit": []string{"1"}})
	if eventsResponse.StatusCode != http.StatusOK {
		t.Fatalf("events response = %d %s", eventsResponse.StatusCode, eventsResponse.Body)
	}
	var events usagePayload
	if err := json.Unmarshal(eventsResponse.Body, &events); err != nil {
		t.Fatal(err)
	}
	if events.TotalRequests != 1 || events.LatestID != 1 || events.DetailsLimit != 1 || !events.DetailsLimited || events.Generation != 1 {
		t.Fatalf("events payload = %#v", events)
	}
	if status := activeRuntime.status(t.Context()); status.Summary == nil || status.Summary.TotalRequests != 2 {
		t.Fatalf("read-only usage handler changed summary: %#v", status)
	}
}

func TestShadowReaderRejectsInvalidHistoryCursor(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	response := callManagementQuery(t, managementEventsPath, url.Values{"cursor": []string{"not-a-cursor"}})
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "invalid usage cursor") {
		t.Fatalf("history response = %d %s", response.StatusCode, response.Body)
	}
}

func TestShadowQueriesStayUnavailableByDefault(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, "enabled: true\n")); err != nil {
		t.Fatal(err)
	}
	response := callManagementQuery(t, managementUsagePath, nil)
	if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(response.Body), "reader is disabled") {
		t.Fatalf("disabled response = %d %s", response.StatusCode, response.Body)
	}
}

func TestShadowQueriesUseHostEnvironmentDefaults(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	t.Setenv("USAGE_QUERY_LIMIT", "1")
	t.Setenv("USAGE_BATCH_SIZE", "1")
	databasePath := prepareShadowReaderDatabase(t)
	config := "read-enabled: true\ndatabase-path: " + databasePath + "\n"
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config)); err != nil {
		t.Fatal(err)
	}

	usageResponse := callManagementQuery(t, managementUsagePath, nil)
	var usage usagePayload
	if err := json.Unmarshal(usageResponse.Body, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.DetailsCount != 1 || usage.DetailsLimit != 1 || !usage.DetailsLimited {
		t.Fatalf("usage environment defaults = %#v", usage)
	}

	eventsResponse := callManagementQuery(t, managementEventsPath, nil)
	var events usagePayload
	if err := json.Unmarshal(eventsResponse.Body, &events); err != nil {
		t.Fatal(err)
	}
	if events.DetailsCount != 1 || events.DetailsLimit != 1 || !events.DetailsLimited {
		t.Fatalf("events environment defaults = %#v", events)
	}
}
