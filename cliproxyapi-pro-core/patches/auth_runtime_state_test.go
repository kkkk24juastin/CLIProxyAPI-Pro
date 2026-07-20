package auth

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage"
)

type runtimeStateTestStore struct {
	saved *Auth
}

func (s *runtimeStateTestStore) List(context.Context) ([]*Auth, error) {
	return nil, nil
}

func (s *runtimeStateTestStore) Save(_ context.Context, auth *Auth) (string, error) {
	s.saved = auth.Clone()
	return auth.FileName, nil
}

func (s *runtimeStateTestStore) Delete(context.Context, string) error {
	return nil
}

func runtimeStateTestEntries(ids ...string) []*scheduledAuth {
	entries := make([]*scheduledAuth, 0, len(ids))
	for _, id := range ids {
		auth := &Auth{ID: id}
		entries = append(entries, &scheduledAuth{auth: auth})
	}
	return entries
}

func TestReadyViewRestoresSuccessorOfLastSelectedAuth(t *testing.T) {
	const key = "single|codex|gpt-5|0|all"
	persisted := map[string]string{key: "auth-b"}
	view := buildReadyView(runtimeStateTestEntries("auth-a", "auth-b", "auth-c"), key, persisted)
	picked := view.pickRoundRobin(nil)
	if picked == nil || picked.auth == nil || picked.auth.ID != "auth-c" {
		t.Fatalf("restored pick = %#v, want auth-c", picked)
	}
}

func TestReadyViewRestoresNextSortedAuthWhenSavedAuthIsMissing(t *testing.T) {
	const key = "single|codex|gpt-5|0|all"
	persisted := map[string]string{key: "auth-b"}
	view := buildReadyView(runtimeStateTestEntries("auth-a", "auth-c", "auth-d"), key, persisted)
	picked := view.pickRoundRobin(nil)
	if picked == nil || picked.auth == nil || picked.auth.ID != "auth-c" {
		t.Fatalf("restored missing-auth pick = %#v, want auth-c", picked)
	}
}

func TestApplyImportedRuntimeStateUpdatesRunningManager(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	registered, err := manager.Register(context.Background(), &Auth{
		ID: "auth-a", Provider: "codex", FileName: "auth-a.json",
		Metadata: map[string]any{"email": "user@example.com"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	now := time.Now()
	cursorKey := "single|codex|gpt-5|0|all"
	err = manager.ApplyImportedRuntimeState(
		[]embeddedusage.RoutingCursorState{{CursorKey: cursorKey, LastAuthID: registered.ID, UpdatedAtMS: now.UnixMilli()}},
		[]embeddedusage.AuthRuntimeStats{{
			AuthIndex: registered.Index, AuthID: registered.ID, FileName: registered.FileName,
			SelectedCount: 9, SuccessCount: 7, FailureCount: 2, UpdatedAtMS: now.UnixMilli(),
			RecentBuckets: []embeddedusage.RuntimeRequestBucket{{BucketID: recentRequestBucketID(now), Success: 4, Failed: 1}},
		}},
	)
	if err != nil {
		t.Fatalf("ApplyImportedRuntimeState() error = %v", err)
	}
	got, ok := manager.GetByID(registered.ID)
	if !ok || got == nil {
		t.Fatal("imported auth not found")
	}
	if got.Selected != 9 || got.Success != 7 || got.Failed != 2 {
		t.Fatalf("runtime totals = selected:%d success:%d failed:%d", got.Selected, got.Success, got.Failed)
	}
	buckets := got.RecentRequestsSnapshot(now)
	latest := buckets[len(buckets)-1]
	if latest.Success != 4 || latest.Failed != 1 {
		t.Fatalf("latest bucket = %+v, want success=4 failed=1", latest)
	}
	manager.scheduler.mu.Lock()
	lastAuthID := manager.scheduler.persistedCursors[cursorKey]
	manager.scheduler.mu.Unlock()
	if lastAuthID != registered.ID {
		t.Fatalf("persisted cursor = %q, want %q", lastAuthID, registered.ID)
	}
}

func TestRegisterRemovesLegacyQuotaCacheFromOrdinaryAuthPersistence(t *testing.T) {
	store := &runtimeStateTestStore{}
	manager := NewManager(store, nil, nil)
	registered, err := manager.Register(context.Background(), &Auth{
		ID: "auth-legacy", Provider: "codex", FileName: "auth-legacy.json",
		Metadata: map[string]any{
			"email":       "user@example.com",
			"quota_cache": map[string]any{"status": "success"},
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered == nil || registered.Metadata["email"] != "user@example.com" {
		t.Fatalf("registered auth = %+v", registered)
	}
	if _, ok := registered.Metadata["quota_cache"]; ok {
		t.Fatalf("registered quota_cache = %#v, want removed", registered.Metadata["quota_cache"])
	}
	if store.saved == nil {
		t.Fatal("auth was not persisted")
	}
	if _, ok := store.saved.Metadata["quota_cache"]; ok {
		t.Fatalf("persisted quota_cache = %#v, want removed", store.saved.Metadata["quota_cache"])
	}
}
