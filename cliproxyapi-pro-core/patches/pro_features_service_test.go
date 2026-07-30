package cliproxy

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/profeatures"
	internalregistry "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestBuiltInOAuthModelPolicyConstrainsRegistrationAndSelection(t *testing.T) {
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	t.Setenv("USAGE_SERVICE_ENABLED", "true")
	ctx, cancel := context.WithCancel(context.Background())
	usageService, err := embeddedusage.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	embeddedusage.SetDefaultService(usageService)
	t.Cleanup(func() {
		embeddedusage.SetDefaultService(nil)
		cancel()
	})
	settings := json.RawMessage(`{
		"enabled": true,
		"cache-ttl": "30m",
		"resolve-timeout": "15s",
		"providers": {"xai": {"plans": {
			"free": {"excluded-models": ["grok-imagine-video"]},
			"supergrok": {"excluded-models": ["grok-imagine-image"]}
		}}}
	}`)
	if err := embeddedusage.SetProSetting(ctx, embeddedusage.ProSetting{
		Namespace: embeddedusage.ProSettingNamespaceOAuthModelPolicy, SchemaVersion: 1, Settings: settings,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.ParseConfigBytes([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := profeatures.New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{cfg: cfg, proFeatures: runtime, coreManager: manager}
	freeAuth := &coreauth.Auth{
		ID: "xai-free-auth", Provider: "xai", Status: coreauth.StatusActive,
		Attributes: map[string]string{"auth_kind": "oauth", "plan_type": "free"},
	}
	superGrokAuth := &coreauth.Auth{
		ID: "xai-supergrok-auth", Provider: "xai", Status: coreauth.StatusActive,
		Attributes: map[string]string{"auth_kind": "oauth", "plan_type": "supergrok"},
	}

	modelRegistry := internalregistry.GetGlobalRegistry()
	modelRegistry.UnregisterClient(freeAuth.ID)
	modelRegistry.UnregisterClient(superGrokAuth.ID)
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(freeAuth.ID)
		modelRegistry.UnregisterClient(superGrokAuth.ID)
	})
	service.registerModelsForAuth(ctx, freeAuth)
	service.registerModelsForAuth(ctx, superGrokAuth)

	const freeOnlyModel = "grok-imagine-image"
	const superGrokOnlyModel = "grok-imagine-video"
	if !modelRegistry.ClientSupportsModel(freeAuth.ID, freeOnlyModel) || modelRegistry.ClientSupportsModel(freeAuth.ID, superGrokOnlyModel) {
		t.Fatalf("free auth model set was not filtered: %#v", modelRegistry.GetModelsForClient(freeAuth.ID))
	}
	if modelRegistry.ClientSupportsModel(superGrokAuth.ID, freeOnlyModel) || !modelRegistry.ClientSupportsModel(superGrokAuth.ID, superGrokOnlyModel) {
		t.Fatalf("SuperGrok auth model set was not filtered: %#v", modelRegistry.GetModelsForClient(superGrokAuth.ID))
	}
	if _, err := manager.Register(ctx, freeAuth); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(ctx, superGrokAuth); err != nil {
		t.Fatal(err)
	}
	service.registerExecutorForAuth(freeAuth, false)
	selected, err := manager.SelectAuth(ctx, "xai", superGrokOnlyModel, cliproxyexecutor.Options{})
	if err != nil || selected == nil || selected.ID != superGrokAuth.ID {
		t.Fatalf("selected auth = %#v, %v; want %s", selected, err, superGrokAuth.ID)
	}
}
