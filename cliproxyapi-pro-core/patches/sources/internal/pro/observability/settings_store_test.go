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
			Data:     []byte(`{"status":"success","groups":[],"subscription":{"plan":"pro","tierId":"g1-pro-tier"}}`),
			CachedAt: now - 200, ObservedAt: now - 100,
		},
		{
			ID: "quota-provider:antigravity:idx", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
			Data:     []byte(`{"schema_version":1,"provider":"antigravity","items":[],"plan":{"id":"g1-ultra-tier","kind":"ultra"}}`),
			CachedAt: now, ObservedAt: now,
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
	if !found || snapshot.ObservedAtMS != now-100 || !strings.Contains(string(snapshot.Data), `"plan":"pro"`) {
		t.Fatalf("snapshot = %#v, found = %t", snapshot, found)
	}
}

func TestSettingsStoreIgnoresPluginSnapshotThatAuthCardCannotHydrate(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UnixMilli()
	entry := QuotaCacheEntry{
		ID: "quota-provider:antigravity:idx", Provider: "antigravity", FileName: "account.json", AuthIndex: "idx",
		Data:     []byte(`{"schema_version":1,"provider":"antigravity","items":[],"plan":{"id":"g1-ultra-tier","kind":"ultra"}}`),
		CachedAt: now, ObservedAt: now,
	}
	if err := store.SetQuotaCache(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	SetDefaultService(&Service{store: store})
	t.Cleanup(func() { SetDefaultService(nil) })

	if snapshot, found, err := (SettingsStore{}).GetPlanSnapshot(context.Background(), "antigravity", "account.json", "idx"); err != nil || found {
		t.Fatalf("snapshot = %#v, found = %t, error = %v", snapshot, found, err)
	}
	entries, err := store.GetQuotaCache(context.Background(), "antigravity", "account.json")
	if err != nil || len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("persisted entries = %#v, error = %v", entries, err)
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

func TestAuthCardQuotaSnapshotCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		raw      string
		want     bool
	}{
		{name: "antigravity inspection", provider: "antigravity", raw: `{"status":"success","groups":[]}`, want: true},
		{name: "antigravity plugin", provider: "antigravity", raw: `{"schema_version":1,"items":[],"plan":{"kind":"ultra"}}`},
		{name: "codex inspection", provider: "codex", raw: `{"status":"success","windows":[]}`, want: true},
		{name: "gemini inspection", provider: "gemini-cli", raw: `{"status":"success","buckets":[]}`, want: true},
		{name: "gemini normalized plugin", provider: "gemini-cli", raw: `{"schema_version":1,"items":[]}`, want: true},
		{name: "kimi inspection", provider: "kimi", raw: `{"status":"success","rows":[]}`, want: true},
		{name: "xai inspection", provider: "xai", raw: `{"status":"success","billing":{}}`, want: true},
		{name: "error state", provider: "antigravity", raw: `{"status":"error"}`, want: true},
		{name: "missing shape", provider: "antigravity", raw: `{"status":"success"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAuthCardQuotaSnapshotCompatible(test.provider, []byte(test.raw)); got != test.want {
				t.Fatalf("isAuthCardQuotaSnapshotCompatible() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSettingsStoreDoesNotUseBoundSnapshotWithoutAuthIndex(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetQuotaCache(context.Background(), QuotaCacheEntry{
		ID: "quota-provider:kimi:idx", Provider: "kimi", FileName: "account.json", AuthIndex: "idx",
		Data: []byte(`{"status":"success","rows":[],"planType":"team"}`), CachedAt: time.Now().UnixMilli(),
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
