package usage

import (
	"context"
	"testing"
)

func TestSpeedContextRoundTrip(t *testing.T) {
	ctx := WithSpeed(context.Background(), " fast ")
	if got := SpeedFromContext(ctx); got != "fast" {
		t.Fatalf("SpeedFromContext() = %q, want fast", got)
	}
	if got := SpeedFromContext(context.Background()); got != "" {
		t.Fatalf("SpeedFromContext(background) = %q, want empty", got)
	}
}
