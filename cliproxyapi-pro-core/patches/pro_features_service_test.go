package cliproxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	proapp "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/app"
	internalregistry "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	runtimeexecutor "github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestBuiltInOAuthPolicyConstrainsRegistrationAndSelection(t *testing.T) {
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
		Namespace: embeddedusage.ProSettingNamespaceOAuthPolicy, SchemaVersion: 1, Settings: settings,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.ParseConfigBytes([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := proapp.New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{cfg: cfg, proApp: runtime, coreManager: manager}
	freeAuth := &coreauth.Auth{
		ID: "xai-free-auth", Provider: "xai", Status: coreauth.StatusActive,
		Attributes: map[string]string{"auth_kind": "oauth"},
		Metadata:   map[string]any{"access_token": "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"tier":0}`)) + ".signature"},
	}
	superGrokAuth := &coreauth.Auth{
		ID: "xai-supergrok-auth", Provider: "xai", Status: coreauth.StatusActive,
		Attributes: map[string]string{"auth_kind": "oauth"},
		Metadata:   map[string]any{"access_token": "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"tier":1}`)) + ".signature"},
	}

	modelRegistry := internalregistry.GetGlobalRegistry()
	modelRegistry.UnregisterClient(freeAuth.ID)
	modelRegistry.UnregisterClient(superGrokAuth.ID)
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(freeAuth.ID)
		modelRegistry.UnregisterClient(superGrokAuth.ID)
	})
	if _, err := manager.Register(ctx, freeAuth); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(ctx, superGrokAuth); err != nil {
		t.Fatal(err)
	}
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
	service.registerExecutorForAuth(freeAuth, false)
	selected, err := manager.SelectAuth(ctx, "xai", superGrokOnlyModel, cliproxyexecutor.Options{})
	if err != nil || selected == nil || selected.ID != superGrokAuth.ID {
		t.Fatalf("selected auth = %#v, %v; want %s", selected, err, superGrokAuth.ID)
	}
	if _, found := runtime.OAuthPolicy().EffectivePolicy(freeAuth.ID); !found {
		t.Fatal("free auth account policy was not recorded before removal")
	}
	service.applyCoreAuthRemoval(ctx, freeAuth.ID)
	if _, found := runtime.OAuthPolicy().EffectivePolicy(freeAuth.ID); found {
		t.Fatal("removed auth retained its effective account policy")
	}
}

func TestOAuthPolicyInFlightRegistrationCannotRestoreRemovedAuthModels(t *testing.T) {
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
		"providers": {"claude": {"plans": {
			"pro": {"excluded-models": ["claude-opus-4-1-20250805"]}
		}}}
	}`)
	if err = embeddedusage.SetProSetting(ctx, embeddedusage.ProSetting{
		Namespace: embeddedusage.ProSettingNamespaceOAuthPolicy, SchemaVersion: 1, Settings: settings,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ParseConfigBytes([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := proapp.New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(runtimeexecutor.NewClaudeExecutor(cfg))
	service := &Service{cfg: cfg, proApp: runtime, coreManager: manager}
	auth := &coreauth.Auth{
		ID: "claude-in-flight-removal", FileName: "claude-in-flight-removal.json", Provider: "claude", Status: coreauth.StatusActive,
		Attributes: map[string]string{"auth_kind": "oauth"}, Metadata: map[string]any{"access_token": "claude-token"},
	}
	if _, err = manager.Register(ctx, auth); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	requestCtx := context.WithValue(ctx, "cliproxy.roundtripper", roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"account":{"has_claude_pro":true}}`)),
		}, nil
	}))
	done := make(chan struct{})
	go func() {
		service.registerModelsForAuth(requestCtx, auth)
		close(done)
	}()
	<-started
	service.applyCoreAuthRemoval(ctx, auth.ID)
	close(release)
	<-done
	if _, found := manager.GetByID(auth.ID); found {
		t.Fatal("removed auth was restored to the manager")
	}
	if models := internalregistry.GetGlobalRegistry().GetModelsForClient(auth.ID); len(models) != 0 {
		t.Fatalf("removed auth models were restored by in-flight registration: %#v", models)
	}
}

func TestQueuedModelRegistrationCannotReplaceRecreatedAuthModels(t *testing.T) {
	ctx := context.Background()
	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{coreManager: manager}
	stale := &coreauth.Auth{ID: "reused-auth-id", Provider: "claude", Status: coreauth.StatusActive}
	if _, err := manager.Register(ctx, stale); err != nil {
		t.Fatal(err)
	}
	queuedCtx := service.beginAuthModelRegistration(ctx, stale.ID)
	service.applyCoreAuthRemoval(ctx, stale.ID)

	replacement := &coreauth.Auth{ID: stale.ID, Provider: "xai", Status: coreauth.StatusActive}
	if _, err := manager.Register(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	modelRegistry := internalregistry.GetGlobalRegistry()
	t.Cleanup(func() { modelRegistry.UnregisterClient(stale.ID) })
	service.registerModelsForAuth(ctx, replacement)
	service.registerModelsForAuth(queuedCtx, stale)

	models := modelRegistry.GetModelsForClient(stale.ID)
	if len(models) == 0 {
		t.Fatal("recreated xAI auth has no registered models")
	}
	for _, model := range models {
		if model != nil && strings.HasPrefix(model.ID, "claude-") {
			t.Fatalf("queued stale snapshot replaced recreated auth models: %#v", models)
		}
	}
}

func TestStalePluginAuthUpdateCannotReplaceRecreatedAuth(t *testing.T) {
	ctx := context.Background()
	manager := coreauth.NewManager(nil, nil, nil)
	service := &Service{coreManager: manager}
	stale := &coreauth.Auth{ID: "reused-plugin-auth-id", Provider: "claude", Label: "old", Status: coreauth.StatusActive}
	if _, err := manager.Register(ctx, stale); err != nil {
		t.Fatal(err)
	}
	queuedCtx := service.beginAuthModelRegistration(ctx, stale.ID)
	service.applyCoreAuthRemoval(ctx, stale.ID)

	replacement := &coreauth.Auth{ID: stale.ID, Provider: "xai", Label: "replacement", Status: coreauth.StatusActive}
	if _, err := manager.Register(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	staleUpdate := stale.Clone()
	staleUpdate.Label = "stale-plugin-update"
	if _, committed := service.updateAuthForCurrentModelRegistration(queuedCtx, stale, staleUpdate); committed {
		t.Fatal("stale plugin auth update committed after the auth ID was recreated")
	}
	current, found := manager.GetByID(stale.ID)
	if !found || current.Provider != "xai" || current.Label != "replacement" {
		t.Fatalf("recreated auth was overwritten by stale plugin update: %#v", current)
	}
}
