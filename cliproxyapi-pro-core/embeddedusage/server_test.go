package embeddedusage

import (
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
