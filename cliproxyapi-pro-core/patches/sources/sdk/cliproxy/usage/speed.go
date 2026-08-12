package usage

import (
	"context"
	"strings"
)

type speedContextKey struct{}

// WithSpeed stores the client-requested inference speed for usage sinks.
func WithSpeed(ctx context.Context, speed string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	speed = strings.TrimSpace(speed)
	if speed == "" {
		return ctx
	}
	return context.WithValue(ctx, speedContextKey{}, speed)
}

// SpeedFromContext returns the client-requested inference speed.
func SpeedFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	raw := ctx.Value(speedContextKey{})
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}
