package usage

import (
	"context"
	"testing"
)

func TestSkipMonitoringContext(t *testing.T) {
	if SkipMonitoringFromContext(context.Background()) {
		t.Fatal("SkipMonitoringFromContext(background) = true, want false")
	}
	if !SkipMonitoringFromContext(WithSkipMonitoring(context.Background())) {
		t.Fatal("SkipMonitoringFromContext(marked) = false, want true")
	}
}

type namedLifecycleUsagePlugin struct {
	calls int
}

func (p *namedLifecycleUsagePlugin) HandleUsage(context.Context, Record) {
	p.calls++
}

func TestUnregisterNamedPreservesReplacement(t *testing.T) {
	manager := NewManager(1)
	first := &namedLifecycleUsagePlugin{}
	replacement := &namedLifecycleUsagePlugin{}
	manager.RegisterNamed("lifecycle", first)
	manager.RegisterNamed("lifecycle", replacement)

	manager.UnregisterNamed("lifecycle", first)
	manager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
	if replacement.calls != 1 {
		t.Fatalf("replacement calls = %d, want 1", replacement.calls)
	}

	manager.UnregisterNamed("lifecycle", replacement)
	manager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
	if replacement.calls != 1 {
		t.Fatalf("replacement calls after unregister = %d, want 1", replacement.calls)
	}

	manager.RegisterNamed("lifecycle", first)
	manager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
	if first.calls != 1 {
		t.Fatalf("re-registered plugin calls = %d, want 1", first.calls)
	}
}

func TestUnregisterNamedRestoresOlderLiveOwner(t *testing.T) {
	manager := NewManager(1)
	first := &namedLifecycleUsagePlugin{}
	replacement := &namedLifecycleUsagePlugin{}
	manager.RegisterNamed("lifecycle", first)
	manager.RegisterNamed("lifecycle", replacement)

	manager.UnregisterNamed("lifecycle", replacement)
	manager.dispatch(queueItem{ctx: context.Background(), record: Record{}})
	if first.calls != 1 || replacement.calls != 0 {
		t.Fatalf("restored owner calls = first:%d replacement:%d", first.calls, replacement.calls)
	}
}

func TestAttemptTrackingAllocatesZeroBasedIndexes(t *testing.T) {
	ctx := WithAttemptTracking(context.Background())
	for want := int64(0); want < 3; want++ {
		attemptCtx := NextAttemptContext(ctx)
		got, ok := AttemptIndexFromContext(attemptCtx)
		if !ok || got != want {
			t.Fatalf("attempt index = %d, %t; want %d, true", got, ok, want)
		}
	}
	if _, ok := AttemptIndexFromContext(context.Background()); ok {
		t.Fatal("uninstrumented context unexpectedly has an attempt index")
	}
}
