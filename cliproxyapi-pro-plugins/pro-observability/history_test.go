package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func historyTestEvents(t *testing.T, count int) []usageEvent {
	t.Helper()
	base, err := usageEventFromRPC(testUsageRecord(t), time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	events := make([]usageEvent, count)
	for index := range events {
		event := base
		event.RequestID = "history-request-" + string(rune('a'+index))
		event.TimestampMS += int64(index)
		event.Timestamp = time.UnixMilli(event.TimestampMS).UTC().Format(time.RFC3339Nano)
		event.Failed = index%2 == 1
		event.EventHash = buildEventHash(event)
		events[index] = event
	}
	return events
}

func prepareHistoryReader(t *testing.T, events []usageEvent) string {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
	store, err := openUsageStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if _, err := store.insertEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	config := "read-enabled: true\ndatabase-path: " + databasePath + "\n"
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config)); err != nil {
		t.Fatal(err)
	}
	return databasePath
}

func decodeUsageResponse(t *testing.T, response managementResponse) usagePayload {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("usage response = %d %s", response.StatusCode, response.Body)
	}
	var payload usagePayload
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func usageDetailIDs(payload usagePayload) []int64 {
	ids := make([]int64, 0, payload.DetailsCount)
	for _, api := range payload.APIs {
		for _, model := range api.Models {
			for _, detail := range model.Details {
				ids = append(ids, detail.ID)
			}
		}
	}
	return ids
}

func TestShadowHistoryCursorKeepsStableSnapshot(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	events := historyTestEvents(t, 6)
	databasePath := prepareHistoryReader(t, events[:5])

	first := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"direction": []string{"before"},
		"limit":     []string{"2"},
	}))
	if got := usageDetailIDs(first); len(got) != 2 || got[0] != 5 || got[1] != 4 {
		t.Fatalf("first page ids = %v, want [5 4]", got)
	}
	if first.MatchedTotal != 5 || first.SnapshotMaxID != 5 || first.PageCursor == "" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page metadata = %#v", first)
	}

	writer, err := openUsageStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.insertEvent(context.Background(), events[5]); err != nil {
		t.Fatal(err)
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}

	second := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"cursor": []string{first.NextCursor},
		"limit":  []string{"2"},
	}))
	if got := usageDetailIDs(second); len(got) != 2 || got[0] != 3 || got[1] != 2 {
		t.Fatalf("second page ids = %v, want [3 2]", got)
	}
	if second.MatchedTotal != 5 || second.SnapshotMaxID != 5 || !second.HasMore || second.NextCursor == "" {
		t.Fatalf("second page metadata = %#v", second)
	}

	returnedFirst := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"cursor": []string{first.PageCursor},
		"limit":  []string{"2"},
	}))
	if got := usageDetailIDs(returnedFirst); len(got) != 2 || got[0] != 5 || got[1] != 4 {
		t.Fatalf("returned first page ids = %v, want [5 4]", got)
	}

	third := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"cursor": []string{second.NextCursor},
		"limit":  []string{"2"},
	}))
	if got := usageDetailIDs(third); len(got) != 1 || got[0] != 1 {
		t.Fatalf("third page ids = %v, want [1]", got)
	}
	if third.MatchedTotal != 5 || third.SnapshotMaxID != 5 || third.HasMore || third.NextCursor != "" {
		t.Fatalf("third page metadata = %#v", third)
	}
}

func TestShadowHistoryCursorPreservesStructuredFilters(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	events := historyTestEvents(t, 4)
	events[0].Provider = "alpha"
	events[0].Model = "model-a"
	events[0].APIKeyHash = "key-a"
	events[0].RequestID = "needle-request"
	events[0].Failed = false
	events[1].Provider = "alpha"
	events[1].Model = "model-a"
	events[1].APIKeyHash = "key-a"
	events[1].ErrorMessage = "needle failure"
	events[1].Failed = true
	events[2].Provider = "beta"
	events[2].Model = "model-b"
	events[2].APIKeyHash = "key-b"
	events[2].Failed = true
	events[3].Provider = "alpha"
	events[3].Model = "model-a"
	events[3].APIKeyHash = "key-a"
	events[3].AuthIndex = "auth-meta"
	events[3].Failed = true
	for index := range events {
		events[index].EventHash = buildEventHash(events[index])
	}
	prepareHistoryReader(t, events)

	first := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"direction":           []string{"before"},
		"limit":               []string{"1"},
		"provider":            []string{"alpha"},
		"model":               []string{"model-a"},
		"api_key_hash":        []string{"key-a"},
		"status":              []string{"failed"},
		"search":              []string{"needle"},
		"search_auth_indexes": []string{"auth-meta"},
	}))
	if got := usageDetailIDs(first); len(got) != 1 || got[0] != 4 {
		t.Fatalf("filtered first page ids = %v, want [4]", got)
	}
	if first.MatchedTotal != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("filtered first page metadata = %#v", first)
	}

	second := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"cursor": []string{first.NextCursor},
		"limit":  []string{"1"},
	}))
	if got := usageDetailIDs(second); len(got) != 1 || got[0] != 2 {
		t.Fatalf("filtered second page ids = %v, want [2]", got)
	}
	if second.MatchedTotal != 2 || second.HasMore {
		t.Fatalf("filtered second page metadata = %#v", second)
	}
}

func TestShadowHistoryCursorSupportsAuthAndUnknownPolicyFilters(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	events := historyTestEvents(t, 3)
	events[0].AuthIndex = "auth-a"
	events[0].PolicyMode = ""
	events[1].AuthIndex = "auth-b"
	events[1].PolicyMode = "profile"
	events[2].AuthIndex = "auth-c"
	events[2].PolicyMode = ""
	for index := range events {
		events[index].EventHash = buildEventHash(events[index])
	}
	prepareHistoryReader(t, events)

	payload := decodeUsageResponse(t, callManagementQuery(t, managementEventsPath, url.Values{
		"direction":   []string{"before"},
		"auth_index":  []string{"auth-a,auth-c"},
		"policy_mode": []string{"unknown"},
		"limit":       []string{"10"},
	}))
	if got := usageDetailIDs(payload); len(got) != 2 || got[0] != 3 || got[1] != 1 {
		t.Fatalf("auth/policy filtered ids = %v, want [3 1]", got)
	}
}
