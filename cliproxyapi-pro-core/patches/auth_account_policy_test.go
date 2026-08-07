package auth

import (
	"context"
	"strconv"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

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
		if clone.ID == "high" {
			clone.Attributes["priority"] = strconv.Itoa(priority)
			clone.Attributes[AttributeWeight] = "7"
		}
		return clone
	})
	assertScheduledAccountPolicy(t, manager, "high", 100, 7)
	priority = 200
	manager.RefreshSchedulerEntry("high")
	assertScheduledAccountPolicy(t, manager, "high", 200, 7)
	manager.RefreshSchedulerAll()
	picked, err := manager.scheduler.pickSingle(context.Background(), "codex", "gpt-test", cliproxyexecutor.Options{}, nil)
	if err != nil || picked == nil || picked.ID != "high" {
		t.Fatalf("picked = %#v, %v", picked, err)
	}
	stored, _ := manager.GetByID("high")
	if stored.Attributes["priority"] != "" {
		t.Fatalf("base auth was mutated: %#v", stored.Attributes)
	}
}

func assertScheduledAccountPolicy(t *testing.T, manager *Manager, authID string, wantPriority int, wantWeight int64) {
	t.Helper()
	manager.scheduler.mu.Lock()
	defer manager.scheduler.mu.Unlock()
	provider := manager.scheduler.providers["codex"]
	if provider == nil || provider.auths[authID] == nil {
		t.Fatalf("scheduled auth %q is missing", authID)
	}
	meta := provider.auths[authID]
	if meta.priority != wantPriority || meta.weight != wantWeight {
		t.Fatalf("scheduled auth policy = priority:%d weight:%d, want priority:%d weight:%d", meta.priority, meta.weight, wantPriority, wantWeight)
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
