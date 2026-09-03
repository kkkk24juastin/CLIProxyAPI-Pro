package auth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type accountPolicyRefreshExecutor struct{ err error }

func (accountPolicyRefreshExecutor) Identifier() string { return "codex" }

func (accountPolicyRefreshExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (accountPolicyRefreshExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e accountPolicyRefreshExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	if e.err != nil {
		return nil, e.err
	}
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	auth.Metadata["access_token"] = "refreshed-token"
	return auth, nil
}

func (accountPolicyRefreshExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (accountPolicyRefreshExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestAccountPolicyResolverAffectsSchedulerWithoutMutatingBaseAuth(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	low := &Auth{ID: "low", Provider: "codex", Status: StatusActive, Attributes: map[string]string{"auth_kind": "oauth"}}
	high := &Auth{ID: "high", Provider: "codex", Status: StatusActive, Attributes: map[string]string{"auth_kind": "oauth"}}
	for _, auth := range []*Auth{low, high} {
		registry.GetGlobalRegistry().RegisterClient(auth.ID, "codex", []*registry.ModelInfo{{ID: "gpt-test"}})
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(auth.ID) })
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatal(err)
		}
	}
	priority := 100
	manager.SetAccountPolicyResolver(func(auth *Auth) *Auth {
		clone := auth.Clone()
		RememberAccountPolicyBase(clone)
		clone.Attributes["priority"] = "10"
		clone.Attributes[AttributeWeight] = "2"
		clone.Prefix = "low-policy"
		if clone.ID == "high" {
			clone.Attributes["priority"] = strconv.Itoa(priority)
			clone.Attributes[AttributeWeight] = "7"
			clone.Prefix = "high-policy"
		}
		return clone
	})
	assertScheduledAccountPolicy(t, manager, "high", 100, 7, "high-policy")
	priority = 200
	manager.RefreshSchedulerEntry("high")
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	manager.RefreshSchedulerAll()
	picked, err := manager.scheduler.pickSingle(context.Background(), "codex", "gpt-test", cliproxyexecutor.Options{}, nil)
	if err != nil || picked == nil || picked.ID != "high" {
		t.Fatalf("picked = %#v, %v", picked, err)
	}
	manager.MarkResult(context.Background(), Result{AuthID: "high", Provider: "codex", Model: "gpt-test", Success: true})
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	updated, _ := manager.GetByID("high")
	updated.StatusMessage = "inspection state write"
	if _, err = manager.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	manager.RegisterExecutor(accountPolicyRefreshExecutor{})
	if _, refreshed, errRefresh := manager.ForceRefreshForInspection(context.Background(), "high"); errRefresh != nil || !refreshed {
		t.Fatalf("ForceRefreshForInspection() refreshed=%v error=%v, want success", refreshed, errRefresh)
	}
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	manager.RegisterExecutor(accountPolicyRefreshExecutor{err: errors.New("transient refresh failure")})
	if _, _, errRefresh := manager.ForceRefreshForInspection(context.Background(), "high"); errRefresh == nil {
		t.Fatal("ForceRefreshForInspection() accepted a failed refresh")
	}
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	if _, errRefresh := manager.refreshAuthForRequest(context.Background(), "high", ""); errRefresh == nil {
		t.Fatal("refreshAuthForRequest() accepted a failed refresh")
	}
	assertScheduledAccountPolicy(t, manager, "high", 200, 7, "high-policy")
	picked, err = manager.scheduler.pickSingle(context.Background(), "codex", "gpt-test", cliproxyexecutor.Options{}, nil)
	if err != nil || picked == nil || picked.ID != "high" {
		t.Fatalf("picked after runtime updates = %#v, %v", picked, err)
	}
	stored, _ := manager.GetByID("high")
	if stored.Prefix != "" || stored.Attributes["priority"] != "" || stored.Attributes[AttributeWeight] != "" {
		t.Fatalf("base auth was mutated: %#v", stored.Attributes)
	}
}

func assertScheduledAccountPolicy(t *testing.T, manager *Manager, authID string, wantPriority int, wantWeight int64, wantPrefix string) {
	t.Helper()
	manager.scheduler.mu.Lock()
	defer manager.scheduler.mu.Unlock()
	provider := manager.scheduler.providers["codex"]
	if provider == nil || provider.auths[authID] == nil {
		t.Fatalf("scheduled auth %q is missing", authID)
	}
	meta := provider.auths[authID]
	if meta.priority != wantPriority || meta.weight != wantWeight || meta.auth.Prefix != wantPrefix {
		t.Fatalf("scheduled auth policy = priority:%d weight:%d prefix:%q, want priority:%d weight:%d prefix:%q", meta.priority, meta.weight, meta.auth.Prefix, wantPriority, wantWeight, wantPrefix)
	}
}

func TestUpdateStripsRuntimeAccountPolicyMarkers(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	base := &Auth{ID: "auth-1", Provider: "codex", Prefix: "base", Status: StatusActive, Attributes: map[string]string{"auth_kind": "oauth", "priority": "2", AttributeWeight: "3"}}
	if _, err := manager.Register(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	overlay := base.Clone()
	RememberAccountPolicyBase(overlay)
	overlay.Prefix = "policy"
	overlay.Attributes["priority"] = "100"
	overlay.Attributes[AttributeWeight] = "9"
	if _, err := manager.Update(context.Background(), overlay); err != nil {
		t.Fatal(err)
	}
	stored, _ := manager.GetByID(base.ID)
	if stored.Prefix != "base" || stored.Attributes["priority"] != "2" || stored.Attributes[AttributeWeight] != "3" {
		t.Fatalf("stored auth retained runtime policy: %#v", stored)
	}
	for _, marker := range []string{accountPolicyBasePrefix, accountPolicyBasePriority, accountPolicyBaseWeight} {
		if _, found := stored.Attributes[marker]; found {
			t.Fatalf("marker %q was persisted", marker)
		}
	}
}
