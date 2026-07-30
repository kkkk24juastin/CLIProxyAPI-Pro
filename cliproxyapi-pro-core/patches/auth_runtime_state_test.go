package auth

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
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

func TestModelSchedulerRebuildKeepsImportedCursorAuthoritative(t *testing.T) {
	const key = "single|codex|gpt-5|0|all"
	persisted := map[string]string{key: "auth-b"}
	model := &modelScheduler{
		providerKey:      "codex",
		modelKey:         "gpt-5",
		persistedCursors: persisted,
		entries:          make(map[string]*scheduledAuth),
		readyByPriority:  make(map[int]*readyBucket),
	}
	for _, id := range []string{"auth-a", "auth-b", "auth-c"} {
		auth := &Auth{ID: id, Provider: "codex"}
		model.upsertEntryLocked(&scheduledAuthMeta{auth: auth, providerKey: "codex"}, time.Now())
	}
	picked := model.pickReadyAtPriorityLocked(false, 0, schedulerStrategyRoundRobin, nil)
	if picked == nil || picked.ID != "auth-c" {
		t.Fatalf("restored pick after incremental rebuild = %#v, want auth-c", picked)
	}
}

func TestImportedCursorRestoresLegacyRoundRobinSelector(t *testing.T) {
	selector := &RoundRobinSelector{}
	manager := NewManager(nil, selector, nil)
	key := legacyRoundRobinCursorKey("codex", "")
	if err := manager.ApplyImportedRuntimeState([]embeddedusage.RoutingCursorState{{
		CursorKey: key, LastAuthID: "auth-b", UpdatedAtMS: time.Now().UnixMilli(),
	}}, nil); err != nil {
		t.Fatalf("ApplyImportedRuntimeState() error = %v", err)
	}
	picked, err := selector.Pick(context.Background(), "codex", "", cliproxyexecutor.Options{}, []*Auth{
		{ID: "auth-a", Provider: "codex"},
		{ID: "auth-b", Provider: "codex"},
		{ID: "auth-c", Provider: "codex"},
	})
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if picked == nil || picked.ID != "auth-c" {
		t.Fatalf("legacy restored pick = %#v, want auth-c", picked)
	}
	if got := selector.persistedRoutingCursors[key]; got != "auth-c" {
		t.Fatalf("persisted legacy cursor = %q, want auth-c", got)
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

func TestApplyImportedRuntimeStateExactIndexOverridesFingerprintDrift(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	registered, err := manager.Register(context.Background(), &Auth{
		ID: "xai-auth", Provider: "xai", FileName: "xai-auth.json",
		Metadata: map[string]any{"email": "user@example.com", "sub": "current-subject"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	now := time.Now()
	err = manager.ApplyImportedRuntimeState(nil, []embeddedusage.AuthRuntimeStats{{
		AuthIndex:           registered.Index,
		AuthID:              registered.ID,
		FileName:            registered.FileName,
		IdentityFingerprint: "historical-xai-fingerprint",
		SelectedCount:       11,
		SuccessCount:        8,
		FailureCount:        3,
		UpdatedAtMS:         now.UnixMilli(),
		RecentBuckets: []embeddedusage.RuntimeRequestBucket{{
			BucketID: recentRequestBucketID(now), Success: 5, Failed: 2,
		}},
	}})
	if err != nil {
		t.Fatalf("ApplyImportedRuntimeState() error = %v", err)
	}

	got, ok := manager.GetByID(registered.ID)
	if !ok || got == nil {
		t.Fatal("imported xai auth not found")
	}
	if got.Selected != 11 || got.Success != 8 || got.Failed != 3 {
		t.Fatalf("runtime totals = selected:%d success:%d failed:%d", got.Selected, got.Success, got.Failed)
	}
	buckets := got.RecentRequestsSnapshot(now)
	latest := buckets[len(buckets)-1]
	if latest.Success != 5 || latest.Failed != 2 {
		t.Fatalf("latest bucket = %+v, want success=5 failed=2", latest)
	}
}

func TestApplyImportedRuntimeStateIDFallbackKeepsFingerprintGuard(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	registered, err := manager.Register(context.Background(), &Auth{
		ID: "auth-replaced", Provider: "xai", FileName: "xai-replaced.json",
		Metadata: map[string]any{"email": "replacement@example.com", "sub": "replacement-subject"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err = manager.ApplyImportedRuntimeState(nil, []embeddedusage.AuthRuntimeStats{{
		AuthIndex:           "historical-index",
		AuthID:              registered.ID,
		IdentityFingerprint: "historical-credential-fingerprint",
		SelectedCount:       7,
		SuccessCount:        6,
		FailureCount:        1,
		UpdatedAtMS:         time.Now().UnixMilli(),
	}})
	if err != nil {
		t.Fatalf("ApplyImportedRuntimeState() error = %v", err)
	}

	got, ok := manager.GetByID(registered.ID)
	if !ok || got == nil {
		t.Fatal("registered auth not found")
	}
	if got.Selected != 0 || got.Success != 0 || got.Failed != 0 {
		t.Fatalf("ID fallback applied mismatched stats = selected:%d success:%d failed:%d", got.Selected, got.Success, got.Failed)
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
