package embeddedusage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage/internalusage"
)

func TestServiceWaitFlushesAndClosesSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	cfg := LoadConfig()
	cfg.DBPath = path
	cfg.LegacyDBPath = path
	cfg.PollInterval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	service, err := StartWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("StartWithConfig() error = %v", err)
	}
	payload, err := json.Marshal(testUsageEvent(0, false, 10))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.IngestRaw(context.Background(), payload); err != nil {
		t.Fatalf("IngestRaw() error = %v", err)
	}
	cancel()
	service.Wait()
	if service.store.db != nil {
		t.Fatal("SQLite connection is still open after Service.Wait")
	}
	reopened := openStoreAt(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	events, _, err := reopened.Counts(context.Background())
	if err != nil || events != 1 {
		t.Fatalf("events after shutdown = %d, %v; want 1", events, err)
	}
}

func TestCollectorRetriesPoppedBatchAfterSQLiteFailure(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := store.db.ExecContext(ctx, `create trigger fail_usage_insert before insert on usage_events begin select raise(abort, 'forced usage write failure'); end`); err != nil {
		t.Fatalf("create trigger error = %v", err)
	}
	payload, err := json.Marshal(testUsageEvent(0, false, 10))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	service := &Service{
		ctx: ctx, cfg: Config{BatchSize: 10, PollInterval: 5 * time.Millisecond},
		store: store, events: make(chan internalusage.Event, 16),
	}
	done := make(chan struct{})
	go func() {
		service.collect(ctx)
		close(done)
	}()
	if err := service.IngestRaw(ctx, payload); err != nil {
		t.Fatalf("IngestRaw() error = %v", err)
	}

	// Allow several failed persistence attempts so the item has definitely left the upstream queue.
	time.Sleep(75 * time.Millisecond)
	if _, err := store.db.ExecContext(ctx, `drop trigger fail_usage_insert`); err != nil {
		t.Fatalf("drop trigger error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, _, countErr := store.Counts(ctx)
		if countErr == nil && events == 1 {
			cancel()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatal("collector did not retry the popped batch after SQLite recovered")
}
