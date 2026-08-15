package observability

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSettingsStoreUsesAuthCardPreferredPlanSnapshot(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UnixMilli()
	entries := []QuotaCacheEntry{
		{
			ID: "antigravity:account.json", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
			Data:     []byte(`{"status":"success","groups":[],"subscription":{"plan":"ultra","tierId":"g1-ultra-tier"}}`),
			CachedAt: now - 200, ObservedAt: now - 100,
		},
		{
			ID: "quota-provider:antigravity:idx", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
			Data:     []byte(`{"schema_version":1,"plan":{"id":"antigravity-starter-quota","kind":"antigravity"}}`),
			CachedAt: now, ObservedAt: now - 300,
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

	snapshot, found, err := (SettingsStore{}).GetPlanSnapshot(context.Background(), "antigravity", "account.json", "idx")
	if err != nil {
		t.Fatal(err)
	}
	if !found || snapshot.ObservedAtMS != now-100 || !strings.Contains(string(snapshot.Data), `"plan":"ultra"`) {
		t.Fatalf("snapshot = %#v, found = %t", snapshot, found)
	}
}

func TestSettingsStoreMatchesAuthCardGeminiCanonicalPreference(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UnixMilli()
	entries := []QuotaCacheEntry{
		{
			ID: "gemini-cli:account.json", Provider: "gemini-cli", FileName: "account.json", AuthIndex: "idx",
			Data: []byte(`{"status":"success","buckets":[],"tierId":"free-tier"}`), CachedAt: now, ObservedAt: now,
		},
		{
			ID: "quota-provider:gemini-cli:idx", Provider: "gemini-cli", FileName: "account.json", AuthIndex: "idx",
			Data: []byte(`{"schema_version":1,"items":[],"plan":{"id":"g1-pro-tier","kind":"pro"}}`), CachedAt: now - 100, ObservedAt: now - 100,
		},
	}
	for _, entry := range entries {
		if err := store.SetQuotaCache(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	SetDefaultService(&Service{store: store})
	t.Cleanup(func() { SetDefaultService(nil) })

	snapshot, found, err := (SettingsStore{}).GetPlanSnapshot(context.Background(), "gemini-cli", "account.json", "idx")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !strings.Contains(string(snapshot.Data), `"id":"g1-pro-tier"`) {
		t.Fatalf("snapshot = %#v, found = %t", snapshot, found)
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
	snapshot, found, err := (SettingsStore{}).GetPlanSnapshot(context.Background(), "kimi", "account.json", "")
	if err != nil || found {
		t.Fatalf("snapshot = %#v, found = %t, error = %v", snapshot, found, err)
	}
}
