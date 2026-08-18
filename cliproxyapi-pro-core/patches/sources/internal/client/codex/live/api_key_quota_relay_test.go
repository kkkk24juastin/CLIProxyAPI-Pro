package live

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
)

func TestRealtimeRelayAdmitsAndSettlesEveryResponseTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamEvents := make(chan string, 4)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			messageType, payload, errRead := conn.ReadMessage()
			if errRead != nil {
				return
			}
			upstreamEvents <- string(payload)
			if strings.Contains(string(payload), `"type":"response.create"`) {
				_ = conn.WriteMessage(messageType, []byte(`{"type":"response.done","response":{"id":"resp_1","usage":{"input_tokens":4,"output_tokens":5,"total_tokens":9}}}`))
			}
		}
	}))
	defer upstreamServer.Close()

	var admissions atomic.Int64
	settled := make(chan int64, 1)
	baseDecision := apikeypolicy.RequestPolicyDecision{Mode: apikeypolicy.ModeProfile, Snapshot: &apikeypolicy.RequestPolicySnapshot{
		PolicyID: "policy", ProfileID: "profile", Quota: &apikeypolicy.Quota{Enabled: true, Epoch: 1},
	}}
	baseCtx := apikeypolicy.WithDecision(context.Background(), baseDecision)
	baseCtx = apikeypolicy.WithQuotaAdmission(baseCtx, func(_ context.Context, decision apikeypolicy.RequestPolicyDecision) (apikeypolicy.RequestPolicyDecision, error) {
		turn := admissions.Add(1)
		if turn > 1 {
			return apikeypolicy.RequestPolicyDecision{}, &apikeypolicy.QuotaExceededError{Metric: "requests", Used: 1, Limit: 1}
		}
		decision.Snapshot.QuotaAdmissionID = "admission-1"
		decision.Snapshot.QuotaSettlement = func(_ context.Context, _ string, tokens int64) error {
			settled <- tokens
			return nil
		}
		return decision, nil
	})

	router := gin.New()
	router.GET("/v1/realtime", func(c *gin.Context) {
		upstreamURL := "ws" + strings.TrimPrefix(upstreamServer.URL, "http")
		upstream, _, errDial := websocket.DefaultDialer.Dial(upstreamURL, nil)
		if errDial != nil {
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		downstream, errUpgrade := upgrader.Upgrade(c.Writer, c.Request, nil)
		if errUpgrade != nil {
			upstream.Close()
			return
		}
		_ = relayRealtimeWebsockets(baseCtx, downstream, upstream)
	})
	downstreamServer := httptest.NewServer(router)
	defer downstreamServer.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(downstreamServer.URL, "http")+"/v1/realtime", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	if err = connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"session.update"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-upstreamEvents:
		if !strings.Contains(event, "session.update") || admissions.Load() != 0 {
			t.Fatalf("control event=%s admissions=%d", event, admissions.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("control event was not relayed")
	}
	if err = connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-upstreamEvents:
		if !strings.Contains(event, "response.create") {
			t.Fatalf("turn event=%s", event)
		}
	case <-time.After(time.Second):
		t.Fatal("response.create was not relayed")
	}
	_, done, err := connection.ReadMessage()
	if err != nil || !strings.Contains(string(done), "response.done") {
		t.Fatalf("done=%s error=%v", done, err)
	}
	select {
	case tokens := <-settled:
		if tokens != 9 {
			t.Fatalf("settled tokens=%d", tokens)
		}
	case <-time.After(time.Second):
		t.Fatal("realtime usage was not settled")
	}
	if err = connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)); err != nil {
		t.Fatal(err)
	}
	_, quotaError, err := connection.ReadMessage()
	if err != nil || !strings.Contains(string(quotaError), "api_key_quota_exceeded") {
		t.Fatalf("quota error=%s error=%v", quotaError, err)
	}
	select {
	case event := <-upstreamEvents:
		if strings.Contains(event, "response.create") {
			t.Fatalf("rejected turn reached upstream: %s", event)
		}
	case <-time.After(50 * time.Millisecond):
	}
	if admissions.Load() != 2 {
		t.Fatalf("admissions=%d", admissions.Load())
	}
}
