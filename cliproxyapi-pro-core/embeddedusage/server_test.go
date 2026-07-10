package embeddedusage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage/internalusage"
)

func TestUsageStreamPushesInsertedEventsWithoutPollingDelay(t *testing.T) {
	store := openTestStore(t)
	server := httptest.NewServer(testUsageRouter(store))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/usage/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("stream request error = %v", err)
	}
	defer response.Body.Close()

	payloads := make(chan internalusage.Payload, 1)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		isUsageEvent := false
		for scanner.Scan() {
			line := scanner.Text()
			if line == "event: usage" {
				isUsageEvent = true
				continue
			}
			if isUsageEvent && strings.HasPrefix(line, "data: ") {
				var payload internalusage.Payload
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload) == nil {
					payloads <- payload
				}
				return
			}
		}
	}()

	insertTestUsageEvents(t, store, testUsageEvent(0, false, 10))
	select {
	case payload := <-payloads:
		if payload.LatestID != 1 || payload.TotalRequests != 1 {
			t.Fatalf("stream payload = %+v, want inserted event", payload)
		}
	case <-ctx.Done():
		t.Fatal("stream did not push inserted event before polling interval")
	}
}

func testUsageRouter(store *Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	server := NewServer(Config{QueryLimit: 50000, BatchSize: 100}, store)
	group := router.Group("/usage")
	server.RegisterGinRoutes(group)
	return router
}

func decodeUsagePayload(t *testing.T, recorder *httptest.ResponseRecorder) internalusage.Payload {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var payload internalusage.Payload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, recorder.Body.String())
	}
	return payload
}

func TestHandleUsageReturnsFullSummaryWithLimitedDetails(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
		testUsageEvent(2, false, 30),
	)
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage?limit=1", nil)
	router.ServeHTTP(recorder, request)
	payload := decodeUsagePayload(t, recorder)

	if payload.TotalRequests != 3 || payload.SuccessCount != 2 || payload.FailureCount != 1 || payload.TotalTokens != 60 {
		t.Fatalf("summary = %+v, want total=3 success=2 failure=1 tokens=60", payload)
	}
	if payload.DetailsCount != 1 || payload.DetailsLimit != 1 || !payload.DetailsLimited {
		t.Fatalf("detail metadata = count:%d limit:%d limited:%v, want 1/1/true", payload.DetailsCount, payload.DetailsLimit, payload.DetailsLimited)
	}
}

func TestHandleUsageEventsDetailsLimitedTracksRemainingRows(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
		testUsageEvent(2, false, 30),
	)
	router := testUsageRouter(store)

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodGet, "/usage/events?after_id=0&limit=2", nil)
	router.ServeHTTP(firstRecorder, firstRequest)
	firstPayload := decodeUsagePayload(t, firstRecorder)

	if firstPayload.DetailsCount != 2 || firstPayload.DetailsLimit != 2 || !firstPayload.DetailsLimited {
		t.Fatalf("first page detail metadata = count:%d limit:%d limited:%v, want 2/2/true", firstPayload.DetailsCount, firstPayload.DetailsLimit, firstPayload.DetailsLimited)
	}

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodGet, "/usage/events?after_id=2&limit=2", nil)
	router.ServeHTTP(secondRecorder, secondRequest)
	secondPayload := decodeUsagePayload(t, secondRecorder)

	if secondPayload.DetailsCount != 1 || secondPayload.DetailsLimit != 2 || secondPayload.DetailsLimited {
		t.Fatalf("second page detail metadata = count:%d limit:%d limited:%v, want 1/2/false", secondPayload.DetailsCount, secondPayload.DetailsLimit, secondPayload.DetailsLimited)
	}
}

func TestHandleUsageEventsDoesNotMarkExactFinalPageLimited(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
	)
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/events?after_id=0&limit=2", nil)
	router.ServeHTTP(recorder, request)
	payload := decodeUsagePayload(t, recorder)

	if payload.DetailsCount != 2 || payload.DetailsLimit != 2 || payload.DetailsLimited {
		t.Fatalf("detail metadata = count:%d limit:%d limited:%v, want 2/2/false", payload.DetailsCount, payload.DetailsLimit, payload.DetailsLimited)
	}
	if payload.LatestID != 2 {
		t.Fatalf("latest_id = %d, want 2", payload.LatestID)
	}
}

func TestHandleUsageEventsMaxLimitUsesSentinel(t *testing.T) {
	store := openTestStore(t)
	events := make([]internalusage.Event, usageEventsPageLimit+1)
	for index := range events {
		events[index] = testUsageEvent(index, index%2 == 0, int64(index+1))
	}
	insertTestUsageEvents(t, store, events...)
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/events?after_id=0&limit=5000", nil)
	router.ServeHTTP(recorder, request)
	payload := decodeUsagePayload(t, recorder)

	if payload.DetailsCount != int64(usageEventsPageLimit) || payload.DetailsLimit != int64(usageEventsPageLimit) || !payload.DetailsLimited {
		t.Fatalf("detail metadata = count:%d limit:%d limited:%v, want %d/%d/true", payload.DetailsCount, payload.DetailsLimit, payload.DetailsLimited, usageEventsPageLimit, usageEventsPageLimit)
	}
	if payload.LatestID != int64(usageEventsPageLimit) {
		t.Fatalf("latest_id = %d, want %d", payload.LatestID, usageEventsPageLimit)
	}
}

func TestLoadUsageEventPageUsesSentinelForStreamBatches(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, false, 20),
		testUsageEvent(2, false, 30),
	)
	server := NewServer(Config{QueryLimit: 50000, BatchSize: 2}, store)

	events, limit, detailsLimited, err := server.loadUsageEventPage(context.Background(), 0, server.cfg.BatchSize)
	if err != nil {
		t.Fatalf("loadUsageEventPage() error = %v", err)
	}

	if len(events) != 2 || limit != 2 || !detailsLimited {
		t.Fatalf("page = len:%d limit:%d limited:%v, want 2/2/true", len(events), limit, detailsLimited)
	}
	payload := usagePayloadWithDetailLimit(events, limit, detailsLimited)
	if payload.DetailsCount != 2 || payload.DetailsLimit != 2 || !payload.DetailsLimited || payload.LatestID != 2 {
		t.Fatalf("payload = %+v, want details 2/2/true latest_id=2", payload)
	}
}

func TestHandleUsageAggregatesReturnsBuckets(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store,
		testUsageEvent(0, false, 10),
		testUsageEvent(1, true, 20),
	)
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/aggregates?interval=hour&group_by=provider,model&limit=10", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Items        []UsageAggregateBucket `json:"items"`
		LatestID     int64                  `json:"latest_id"`
		SnapshotAtMS int64                  `json:"snapshot_at_ms"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("aggregate items len = %d, want 1", len(payload.Items))
	}
	if payload.LatestID != 2 || payload.SnapshotAtMS <= 0 {
		t.Fatalf("aggregate metadata = latest:%d snapshot:%d, want latest=2 and timestamp", payload.LatestID, payload.SnapshotAtMS)
	}
	item := payload.Items[0]
	if item.Provider != "test" || item.Model != "model" || item.TotalRequests != 2 || item.FailureCount != 1 || item.TotalTokens != 30 {
		t.Fatalf("aggregate item = %+v, want totals by provider/model", item)
	}
}

func TestUsagePayloadDetailsIncludeEventID(t *testing.T) {
	store := openTestStore(t)
	insertTestUsageEvents(t, store, testUsageEvent(0, false, 10))
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/events?after_id=0&limit=1", nil)
	router.ServeHTTP(recorder, request)
	payload := decodeUsagePayload(t, recorder)

	for _, api := range payload.APIs {
		for _, model := range api.Models {
			if len(model.Details) != 1 || model.Details[0].ID != 1 {
				t.Fatalf("details = %+v, want event id 1", model.Details)
			}
			return
		}
	}
	t.Fatal("usage payload did not contain details")
}

func TestUsageExportImportPreservesAntigravitySubscriptionQuotaCache(t *testing.T) {
	sourceStore := openTestStore(t)
	sourceRouter := testUsageRouter(sourceStore)
	quotaState := map[string]any{
		"status":        "success",
		"schemaVersion": float64(2),
		"parserVersion": float64(3),
		"plan":          "ultra",
		"planType":      "ultra",
		"subscription": map[string]any{
			"plan":     "ultra",
			"tierId":   "g1-ultra-tier",
			"tierName": "Ultra",
			"availableCredits": []any{
				map[string]any{"creditType": "AI", "creditAmount": float64(20)},
			},
		},
		"groups": []any{
			map[string]any{
				"id": "claude-gpt",
				"buckets": []any{
					map[string]any{"id": "weekly", "remainingFraction": float64(0.5)},
				},
			},
		},
	}
	rawQuotaState, err := json.Marshal(quotaState)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := sourceStore.SetQuotaCache(context.Background(), QuotaCacheEntry{
		Provider:   "antigravity",
		FileName:   "antigravity-user.json",
		Data:       rawQuotaState,
		CachedAt:   1,
		AccessedAt: 1,
		Version:    1,
	}); err != nil {
		t.Fatalf("SetQuotaCache() error = %v", err)
	}

	exportRecorder := httptest.NewRecorder()
	exportRequest := httptest.NewRequest(http.MethodGet, "/usage/export", nil)
	sourceRouter.ServeHTTP(exportRecorder, exportRequest)
	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body=%s", exportRecorder.Code, exportRecorder.Body.String())
	}

	targetStore := openTestStore(t)
	targetRouter := testUsageRouter(targetStore)
	importRecorder := httptest.NewRecorder()
	importRequest := httptest.NewRequest(http.MethodPost, "/usage/import", bytes.NewReader(exportRecorder.Body.Bytes()))
	targetRouter.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	entries, err := targetStore.GetQuotaCache(context.Background(), "antigravity", "antigravity-user.json")
	if err != nil {
		t.Fatalf("GetQuotaCache() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("quota cache entries len = %d, want 1", len(entries))
	}
	var restored map[string]any
	if err := json.Unmarshal(entries[0].Data, &restored); err != nil {
		t.Fatalf("json.Unmarshal(restored) error = %v", err)
	}
	subscription, ok := restored["subscription"].(map[string]any)
	if !ok {
		t.Fatalf("subscription = %#v, want object", restored["subscription"])
	}
	if restored["planType"] != "ultra" || subscription["plan"] != "ultra" || subscription["tierId"] != "g1-ultra-tier" {
		t.Fatalf("restored quota = %#v, want antigravity ultra subscription preserved", restored)
	}
}

func TestUsageExportImportPreservesUpstreamDiagnostics(t *testing.T) {
	sourceStore := openTestStore(t)
	sourceRouter := testUsageRouter(sourceStore)
	event := testUsageEvent(0, true, 42)
	event.Provider = "antigravity"
	event.ExecutorType = "AntigravityExecutor"
	event.Model = "gemini-claude-opus-4-5-thinking"
	event.Alias = "claude-opus-4-5"
	event.ErrorCode = "rate_limit"
	event.ErrorMessage = "too many requests"
	event.UpstreamRequestID = "upstream-req-1"
	event.RetryAfter = "30"
	insertTestUsageEvents(t, sourceStore, event)

	exportRecorder := httptest.NewRecorder()
	exportRequest := httptest.NewRequest(http.MethodGet, "/usage/export", nil)
	sourceRouter.ServeHTTP(exportRecorder, exportRequest)
	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body=%s", exportRecorder.Code, exportRecorder.Body.String())
	}

	targetStore := openTestStore(t)
	targetRouter := testUsageRouter(targetStore)
	importRecorder := httptest.NewRecorder()
	importRequest := httptest.NewRequest(http.MethodPost, "/usage/import", bytes.NewReader(exportRecorder.Body.Bytes()))
	targetRouter.ServeHTTP(importRecorder, importRequest)
	if importRecorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200; body=%s", importRecorder.Code, importRecorder.Body.String())
	}

	recent, err := targetStore.RecentEvents(context.Background(), 1)
	if err != nil {
		t.Fatalf("RecentEvents() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("RecentEvents() len = %d, want 1", len(recent))
	}
	got := recent[0]
	if got.Provider != "antigravity" || got.ExecutorType != "AntigravityExecutor" || got.Alias != "claude-opus-4-5" {
		t.Fatalf("provider metadata = provider:%q executor:%q alias:%q", got.Provider, got.ExecutorType, got.Alias)
	}
	if got.ErrorCode != "rate_limit" || got.ErrorMessage != "too many requests" || got.UpstreamRequestID != "upstream-req-1" || got.RetryAfter != "30" {
		t.Fatalf("diagnostics = code:%q message:%q rid:%q retry:%q", got.ErrorCode, got.ErrorMessage, got.UpstreamRequestID, got.RetryAfter)
	}
}

func TestHandleStatusIncludesDeadLetterSamples(t *testing.T) {
	store := openTestStore(t)
	if err := store.AddDeadLetter(context.Background(), "bad payload", errTestParse); err != nil {
		t.Fatalf("AddDeadLetter() error = %v", err)
	}
	router := testUsageRouter(store)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/usage/status", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		DeadLetters       int64              `json:"deadLetters"`
		DeadLetterSamples []DeadLetterSample `json:"deadLetterSamples"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.DeadLetters != 1 || len(payload.DeadLetterSamples) != 1 || payload.DeadLetterSamples[0].Error == "" {
		t.Fatalf("status payload = %+v, want dead letter sample", payload)
	}
}
