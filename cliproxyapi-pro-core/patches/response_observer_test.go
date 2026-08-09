package helps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/requestmeta"
)

type observedUpstreamResponse struct {
	status  int
	header  http.Header
	body    string
}

func TestResponseObserverRunsWhenRequestLoggingIsDisabled(t *testing.T) {
	var observations []observedUpstreamResponse
	ctx := requestmeta.WithUpstreamResponseObserver(context.Background(), func(
		_ context.Context,
		status int,
		header http.Header,
		body []byte,
	) {
		observations = append(observations, observedUpstreamResponse{status, header, string(body)})
	})
	header := make(http.Header)
	header.Set("X-RateLimit-Remaining", "0")

	RecordAPIResponseMetadata(ctx, &config.Config{}, http.StatusTooManyRequests, header)
	AppendAPIResponseChunk(ctx, &config.Config{}, []byte(`{"error":"quota"}`))

	if len(observations) != 2 {
		t.Fatalf("observations = %#v, want metadata and body", observations)
	}
	if observations[0].status != http.StatusTooManyRequests || observations[0].header.Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("metadata observation = %#v", observations[0])
	}
	if observations[1].status != 0 || observations[1].body != `{"error":"quota"}` {
		t.Fatalf("body observation = %#v", observations[1])
	}
}

func TestWebsocketUpgradeRejectionIsObservedOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	var observations []observedUpstreamResponse
	ctx = requestmeta.WithUpstreamResponseObserver(ctx, func(
		_ context.Context,
		status int,
		header http.Header,
		body []byte,
	) {
		observations = append(observations, observedUpstreamResponse{status, header, string(body)})
	})
	header := make(http.Header)
	header.Set("Retry-After", "30")
	body := []byte(`{"error":"rate limit"}`)
	cfg := &config.Config{}
	cfg.RequestLog = true

	RecordAPIWebsocketUpgradeRejection(
		ctx,
		cfg,
		UpstreamRequestLog{URL: "https://example.test/ws", Method: http.MethodGet},
		http.StatusTooManyRequests,
		header,
		body,
	)

	if len(observations) != 1 {
		t.Fatalf("observations = %#v, want exactly one", observations)
	}
	got := observations[0]
	if got.status != http.StatusTooManyRequests || got.header.Get("Retry-After") != "30" || got.body != string(body) {
		t.Fatalf("rejection observation = %#v", got)
	}
}
