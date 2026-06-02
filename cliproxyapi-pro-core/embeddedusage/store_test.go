package embeddedusage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage/internalusage"
)

func testUsageEvent(index int, failed bool, totalTokens int64) internalusage.Event {
	timestamp := time.Unix(1_700_000_000+int64(index), 0).UTC()
	return internalusage.Event{
		RequestID:   "request-" + string(rune('a'+index)),
		EventHash:   "event-hash-" + string(rune('a'+index)),
		TimestampMS: timestamp.UnixMilli(),
		Timestamp:   timestamp.Format(time.RFC3339Nano),
		Provider:    "test",
		Model:       "model",
		Endpoint:    "POST /v1/test",
		Method:      "POST",
		Path:        "/v1/test",
		TotalTokens: totalTokens,
		Failed:      failed,
		CreatedAtMS: timestamp.UnixMilli(),
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	return openTestStoreAt(t, filepath.Join(t.TempDir(), "usage.sqlite"))
}

func openTestStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}

func insertTestUsageEvents(t *testing.T, store *Store, events ...internalusage.Event) {
	t.Helper()
	result, err := store.InsertEvents(context.Background(), events)
	if err != nil {
		t.Fatalf("InsertEvents() error = %v", err)
	}
	if result.Inserted != len(events) {
		t.Fatalf("InsertEvents() inserted = %d, want %d", result.Inserted, len(events))
	}
}

func TestUsageSummaryRespectsCursorLimit(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
		testUsageEvent(2, false, 30),
	)

	recent, err := store.RecentEvents(ctx, 1)
	if err != nil {
		t.Fatalf("RecentEvents() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("RecentEvents() len = %d, want 1", len(recent))
	}

	latestID, _, err := store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}
	summary, err := store.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}

	if summary.TotalRequests != 3 || summary.SuccessCount != 2 || summary.FailureCount != 1 || summary.TotalTokens != 60 {
		t.Fatalf("UsageSummary() = %+v, want total=3 success=2 failure=1 tokens=60", summary)
	}
}

func TestUsageSummaryStopsAtCursor(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
	)
	cursorID, _, err := store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}

	insertTestUsageEvents(t, store, testUsageEvent(2, false, 30))
	summary, err := store.UsageSummary(ctx, cursorID)
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}

	if summary.TotalRequests != 2 || summary.SuccessCount != 1 || summary.FailureCount != 1 || summary.TotalTokens != 30 {
		t.Fatalf("UsageSummary() = %+v, want total=2 success=1 failure=1 tokens=30", summary)
	}
}

func TestUsageSummaryZeroCursorIsEmpty(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store, testUsageEvent(0, false, 10))
	summary, err := store.UsageSummary(ctx, 0)
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}

	if summary.TotalRequests != 0 || summary.SuccessCount != 0 || summary.FailureCount != 0 || summary.TotalTokens != 0 {
		t.Fatalf("UsageSummary() = %+v, want empty summary", summary)
	}
}

func TestEventsAfterAllowsSentinelLimit(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	events := make([]internalusage.Event, usageEventsSentinelLimit)
	for index := range events {
		events[index] = testUsageEvent(index, false, int64(index+1))
	}
	insertTestUsageEvents(t, store, events...)

	recent, err := store.EventsAfter(ctx, 0, usageEventsSentinelLimit)
	if err != nil {
		t.Fatalf("EventsAfter() error = %v", err)
	}
	if len(recent) != usageEventsSentinelLimit {
		t.Fatalf("EventsAfter() len = %d, want %d", len(recent), usageEventsSentinelLimit)
	}
}

func TestUsageSummaryCacheInvalidatesAfterInsert(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store, testUsageEvent(0, false, 10))
	latestID, _, err := store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}
	firstSummary, err := store.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() first error = %v", err)
	}
	if firstSummary.TotalRequests != 1 || firstSummary.TotalTokens != 10 {
		t.Fatalf("first UsageSummary() = %+v, want total=1 tokens=10", firstSummary)
	}

	insertTestUsageEvents(t, store, testUsageEvent(1, true, 20))
	latestID, _, err = store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() second error = %v", err)
	}
	secondSummary, err := store.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() second error = %v", err)
	}
	if secondSummary.TotalRequests != 2 || secondSummary.SuccessCount != 1 || secondSummary.FailureCount != 1 || secondSummary.TotalTokens != 30 {
		t.Fatalf("second UsageSummary() = %+v, want total=2 success=1 failure=1 tokens=30", secondSummary)
	}
}

func TestUsageSummaryPersistsAcrossStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	ctx := context.Background()

	store := openTestStoreAt(t, path)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
	)
	latestID, _, err := store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened := openTestStoreAt(t, path)
	summary, err := reopened.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() after reopen error = %v", err)
	}
	if summary.TotalRequests != 2 || summary.SuccessCount != 1 || summary.FailureCount != 1 || summary.TotalTokens != 30 {
		t.Fatalf("UsageSummary() after reopen = %+v, want total=2 success=1 failure=1 tokens=30", summary)
	}

	var persistedRequests int64
	if err := reopened.db.QueryRowContext(ctx, `select total_requests from usage_summary where id = 1`).Scan(&persistedRequests); err != nil {
		t.Fatalf("usage_summary lookup error = %v", err)
	}
	if persistedRequests != 2 {
		t.Fatalf("usage_summary total_requests = %d, want 2", persistedRequests)
	}
}

func TestUsageSummaryUpdatesAfterDeleteEventsBefore(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
		testUsageEvent(2, false, 30),
	)
	deleted, err := store.DeleteEventsBefore(ctx, testUsageEvent(2, false, 30).TimestampMS)
	if err != nil {
		t.Fatalf("DeleteEventsBefore() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteEventsBefore() deleted = %d, want 2", deleted)
	}
	latestID, _, err := store.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}
	summary, err := store.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}
	if summary.TotalRequests != 1 || summary.SuccessCount != 1 || summary.FailureCount != 0 || summary.TotalTokens != 30 {
		t.Fatalf("UsageSummary() after delete = %+v, want total=1 success=1 failure=0 tokens=30", summary)
	}
}

func TestOpenStoreRebuildsStaleUsageSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	ctx := context.Background()

	store := openTestStoreAt(t, path)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
	)
	if _, err := store.db.ExecContext(ctx, `update usage_summary set latest_event_id = 0, total_requests = 0, success_count = 0, failure_count = 0, total_tokens = 0 where id = 1`); err != nil {
		t.Fatalf("corrupt usage_summary error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened := openTestStoreAt(t, path)
	latestID, _, err := reopened.LatestCursor(ctx)
	if err != nil {
		t.Fatalf("LatestCursor() error = %v", err)
	}
	summary, err := reopened.UsageSummary(ctx, latestID)
	if err != nil {
		t.Fatalf("UsageSummary() error = %v", err)
	}
	if summary.TotalRequests != 2 || summary.SuccessCount != 1 || summary.FailureCount != 1 || summary.TotalTokens != 30 {
		t.Fatalf("rebuilt UsageSummary() = %+v, want total=2 success=1 failure=1 tokens=30", summary)
	}
}

func TestRecentEventsUsesRecentIndex(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
		testUsageEvent(2, false, 30),
	)

	rows, err := store.db.QueryContext(ctx, `explain query plan select
		id, request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms
		from usage_events indexed by idx_usage_events_recent
		order by timestamp_ms desc, id desc
		limit ?`, 2)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN error = %v", err)
	}
	defer rows.Close()

	planLines := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan query plan error = %v", err)
		}
		planLines = append(planLines, strings.ToLower(detail))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query plan rows error = %v", err)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, "idx_usage_events_recent") {
		t.Fatalf("RecentEvents query plan = %q, want idx_usage_events_recent", plan)
	}
	if strings.Contains(plan, "temp b-tree") {
		t.Fatalf("RecentEvents query plan = %q, want no temp b-tree sort", plan)
	}
}
