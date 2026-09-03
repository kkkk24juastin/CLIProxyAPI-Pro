package requestmeta

import (
	"context"
	"net/http"
	"testing"
)

func TestUpstreamResponseObserversComposeAndCanBeSuppressed(t *testing.T) {
	calls := make([]string, 0, 2)
	ctx := WithUpstreamResponseObserver(context.Background(), func(_ context.Context, status int, _ http.Header, body []byte) {
		calls = append(calls, "first")
		if status != http.StatusTooManyRequests || string(body) != "quota" {
			t.Fatalf("first observation = status:%d body:%q", status, body)
		}
	})
	ctx = WithUpstreamResponseObserver(ctx, func(context.Context, int, http.Header, []byte) {
		calls = append(calls, "second")
	})

	ObserveUpstreamResponse(ctx, http.StatusTooManyRequests, http.Header{"X-Test": {"1"}}, []byte("quota"))
	if len(calls) != 2 || calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("observer calls = %#v", calls)
	}
	ObserveUpstreamResponse(WithoutUpstreamResponseObserver(ctx), http.StatusOK, nil, nil)
	if len(calls) != 2 {
		t.Fatalf("suppressed observer calls = %#v", calls)
	}
}
