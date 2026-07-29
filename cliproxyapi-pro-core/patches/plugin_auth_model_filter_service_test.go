package cliproxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
	internalregistry "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestOAuthModelPolicyXAIRegistersPerPlanModelsAndConstrainsSelection(t *testing.T) {
	pluginBinary := os.Getenv("CLIPROXY_OAUTH_MODEL_POLICY_PLUGIN")
	if pluginBinary == "" {
		t.Skip("CLIPROXY_OAUTH_MODEL_POLICY_PLUGIN is not set")
	}

	pluginRoot := t.TempDir()
	pluginDir := filepath.Join(pluginRoot, runtime.GOOS, runtime.GOARCH)
	if errMkdir := os.MkdirAll(pluginDir, 0o755); errMkdir != nil {
		t.Fatalf("create plugin directory: %v", errMkdir)
	}
	pluginTarget := filepath.Join(pluginDir, "oauth-model-policy"+pluginhost.PluginExtension(runtime.GOOS))
	pluginBytes, errRead := os.ReadFile(pluginBinary)
	if errRead != nil {
		t.Fatalf("read oauth-model-policy plugin: %v", errRead)
	}
	if errWrite := os.WriteFile(pluginTarget, pluginBytes, 0o755); errWrite != nil {
		t.Fatalf("stage oauth-model-policy plugin: %v", errWrite)
	}

	cfg, errConfig := config.ParseConfigBytes([]byte(fmt.Sprintf(`
plugins:
  enabled: true
  dir: %q
  configs:
    oauth-model-policy:
      enabled: true
      providers:
        xai:
          plans:
            free:
              excluded-models: ["grok-imagine-video"]
            supergrok:
              excluded-models: ["grok-imagine-image"]
`, pluginRoot)))
	if errConfig != nil {
		t.Fatalf("parse plugin config: %v", errConfig)
	}

	host := pluginhost.New()
	host.ApplyConfig(context.Background(), cfg)
	t.Cleanup(host.ShutdownAll)
	if !host.PluginRegistered("oauth-model-policy") {
		t.Fatal("oauth-model-policy plugin was not registered")
	}

	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{cfg: cfg, pluginHost: host, coreManager: manager}
	freeAuth := &coreauth.Auth{
		ID:       "xai-free-auth",
		Provider: "xai",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "oauth",
			"plan_type": "free",
		},
	}
	superGrokAuth := &coreauth.Auth{
		ID:       "xai-supergrok-auth",
		Provider: "xai",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "oauth",
			"plan_type": "supergrok",
		},
	}

	modelRegistry := internalregistry.GetGlobalRegistry()
	modelRegistry.UnregisterClient(freeAuth.ID)
	modelRegistry.UnregisterClient(superGrokAuth.ID)
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(freeAuth.ID)
		modelRegistry.UnregisterClient(superGrokAuth.ID)
	})

	service.registerModelsForAuth(context.Background(), freeAuth)
	service.registerModelsForAuth(context.Background(), superGrokAuth)

	const freeOnlyModel = "grok-imagine-image"
	const superGrokOnlyModel = "grok-imagine-video"
	if !modelRegistry.ClientSupportsModel(freeAuth.ID, freeOnlyModel) || modelRegistry.ClientSupportsModel(freeAuth.ID, superGrokOnlyModel) {
		t.Fatalf("free auth model set was not filtered by its plan: %#v", modelRegistry.GetModelsForClient(freeAuth.ID))
	}
	if modelRegistry.ClientSupportsModel(superGrokAuth.ID, freeOnlyModel) || !modelRegistry.ClientSupportsModel(superGrokAuth.ID, superGrokOnlyModel) {
		t.Fatalf("SuperGrok auth model set was not filtered by its plan: %#v", modelRegistry.GetModelsForClient(superGrokAuth.ID))
	}
	available := modelRegistry.GetAvailableModels("openai")
	if !availableModelContains(available, freeOnlyModel) || !availableModelContains(available, superGrokOnlyModel) {
		t.Fatalf("aggregated /v1/models set is missing plan-specific models: %#v", available)
	}

	if _, errRegister := manager.Register(context.Background(), freeAuth); errRegister != nil {
		t.Fatalf("register free auth: %v", errRegister)
	}
	if _, errRegister := manager.Register(context.Background(), superGrokAuth); errRegister != nil {
		t.Fatalf("register SuperGrok auth: %v", errRegister)
	}
	service.registerExecutorForAuth(freeAuth, false)

	selected, errSelect := manager.SelectAuth(context.Background(), "xai", superGrokOnlyModel, cliproxyexecutor.Options{})
	if errSelect != nil {
		t.Fatalf("select auth for %s: %v", superGrokOnlyModel, errSelect)
	}
	if selected == nil || selected.ID != superGrokAuth.ID {
		t.Fatalf("selected auth for %s = %#v, want %s", superGrokOnlyModel, selected, superGrokAuth.ID)
	}
	selected, errSelect = manager.SelectAuth(context.Background(), "xai", freeOnlyModel, cliproxyexecutor.Options{})
	if errSelect != nil {
		t.Fatalf("select auth for %s: %v", freeOnlyModel, errSelect)
	}
	if selected == nil || selected.ID != freeAuth.ID {
		t.Fatalf("selected auth for %s = %#v, want %s", freeOnlyModel, selected, freeAuth.ID)
	}
}

func availableModelContains(models []map[string]any, modelID string) bool {
	for _, model := range models {
		if id, _ := model["id"].(string); id == modelID {
			return true
		}
	}
	return false
}
