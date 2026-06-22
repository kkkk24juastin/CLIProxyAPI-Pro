package embeddedusage

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage/internalusage"
)

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
		Items []UsageAggregateBucket `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("aggregate items len = %d, want 1", len(payload.Items))
	}
	item := payload.Items[0]
	if item.Provider != "test" || item.Model != "model" || item.TotalRequests != 2 || item.FailureCount != 1 || item.TotalTokens != 30 {
		t.Fatalf("aggregate item = %+v, want totals by provider/model", item)
	}
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
