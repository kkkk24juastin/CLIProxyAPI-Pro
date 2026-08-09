package requestmeta

import (
	"context"
	"net/http"
)

// UpstreamResponseObserver receives transport-neutral upstream response
// fragments. Status/headers and body chunks can arrive in separate calls.
type UpstreamResponseObserver func(context.Context, int, http.Header, []byte)

type upstreamResponseObserverKey struct{}

// WithUpstreamResponseObserver appends an observer without discarding an
// observer already attached by another host feature.
func WithUpstreamResponseObserver(ctx context.Context, observer UpstreamResponseObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx
	}
	previous, _ := ctx.Value(upstreamResponseObserverKey{}).(UpstreamResponseObserver)
	if previous == nil {
		return context.WithValue(ctx, upstreamResponseObserverKey{}, observer)
	}
	return context.WithValue(ctx, upstreamResponseObserverKey{}, UpstreamResponseObserver(
		func(callCtx context.Context, status int, headers http.Header, body []byte) {
			previous(callCtx, status, headers, body)
			observer(callCtx, status, headers, body)
		},
	))
}

// WithoutUpstreamResponseObserver suppresses observers for nested helper calls
// that would otherwise report the same response fragment twice.
func WithoutUpstreamResponseObserver(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, upstreamResponseObserverKey{}, UpstreamResponseObserver(nil))
}

// ObserveUpstreamResponse publishes one response fragment when an observer is
// attached. It is intentionally a no-op on ordinary upstream requests.
func ObserveUpstreamResponse(ctx context.Context, status int, headers http.Header, body []byte) {
	if ctx == nil {
		return
	}
	observer, _ := ctx.Value(upstreamResponseObserverKey{}).(UpstreamResponseObserver)
	if observer != nil {
		observer(ctx, status, headers, body)
	}
}
