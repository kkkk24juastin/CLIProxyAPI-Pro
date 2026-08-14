package observability

import (
	"context"
	"testing"
	"time"
)

func TestSettingsStoreReturnsAllMatchingPlanSnapshotsNewestFirst(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UnixMilli()
	entries := []QuotaCacheEntry{
		{
			ID: "antigravity:account.json", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
			Data: []byte(`{"status":"success","subscription":{"plan":"ultra"}}`), CachedAt: now - 1, ObservedAt: now - 1,
		},
		{
			ID: "quota-provider:antigravity:idx", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
			Data: []byte(`{"schema_version":1,"plan":{"id":"standard-tier","kind":"unknown"}}`), CachedAt: now, ObservedAt: now,
		},
		{
			ID: "quota-provider:antigravity:other", Provider: "antigravity", FileName: "account.json", AuthIndex: "other",
			Data: []byte(`{"schema_version":1,"plan":{"kind":"free"}}`), CachedAt: now + 1, ObservedAt: now + 1,
		},
	}
	for _, entry := range entries {
		if err := store.SetQuotaCache(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	SetDefaultService(&Service{store: store})
	t.Cleanup(func() { SetDefaultService(nil) })

	snapshots, err := (SettingsStore{}).GetPlanSnapshots(context.Background(), "antigravity", "account.json", "idx")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 || snapshots[0].ObservedAtMS != now || snapshots[1].ObservedAtMS != now-1 {
		t.Fatalf("snapshots = %#v", snapshots)
	}
}

func TestSettingsStoreDoesNotUseBoundSnapshotWithoutAuthIndex(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetQuotaCache(context.Background(), QuotaCacheEntry{
		ID: "quota-provider:kimi:idx", Provider: "kimi", FileName: "account.json", AuthIndex: "idx",
		Data: []byte(`{"schema_version":1,"plan":{"kind":"team"}}`), CachedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	SetDefaultService(&Service{store: store})
	t.Cleanup(func() { SetDefaultService(nil) })
	snapshots, err := (SettingsStore{}).GetPlanSnapshots(context.Background(), "kimi", "account.json", "")
	if err != nil || len(snapshots) != 0 {
		t.Fatalf("snapshots = %#v, error = %v", snapshots, err)
	}
}
