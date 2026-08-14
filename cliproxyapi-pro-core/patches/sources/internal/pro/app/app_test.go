package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/config"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func TestAppModulesPersistSettingsOnlyToSQLite(t *testing.T) {
	ctx := startMigrationStore(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	before := []byte("host: 127.0.0.1\nport: 8317\n")
	if err := os.WriteFile(configPath, before, 0o600); err != nil {
		t.Fatal(err)
	}

	proApp, err := New(ctx, configPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proApp.Close)

	proxyCfg := proxyconfig.Default()
	proxyCfg.TakeoverEnabled = true
	if err := proApp.UpdateProxyConfig(ctx, proxyCfg); err != nil {
		t.Fatal(err)
	}
	modelCfg, err := modelconfig.Parse([]byte(`{
		"enabled": true,
		"providers": {"xai": {"plans": {"free": {"excluded-models": ["grok-pro-*"]}}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := proApp.UpdateOAuthConfig(ctx, modelCfg); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("runtime settings changed config.yaml:\n%s", after)
	}
	proxyItem, proxyFound, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceProxyPool)
	if err != nil || !proxyFound {
		t.Fatalf("proxy setting = found:%v err:%v", proxyFound, err)
	}
	persistedProxy, err := proxyconfig.Parse(proxyItem.Settings)
	if err != nil || !persistedProxy.TakeoverEnabled {
		t.Fatalf("persisted proxy config = %#v err:%v", persistedProxy, err)
	}
	modelItem, modelFound, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceOAuthPolicy)
	if err != nil || !modelFound {
		t.Fatalf("model setting = found:%v err:%v", modelFound, err)
	}
	persistedModel, err := modelconfig.Parse(modelItem.Settings)
	if err != nil || !persistedModel.Enabled || len(persistedModel.Providers) != 1 {
		t.Fatalf("persisted model config = %#v err:%v", persistedModel, err)
	}
}

func TestAPIKeyPolicyLifecycleIsIndependentOfUsageService(t *testing.T) {
	t.Setenv("USAGE_SERVICE_ENABLED", "false")
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "policy-without-usage.sqlite"))
	ctx := context.Background()
	registry.GetGlobalRegistry().RegisterClient("policy-without-usage-client", "codex", []*registry.ModelInfo{{ID: "policy-without-usage-model"}})
	t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient("policy-without-usage-client") })
	identity, err := apikeypolicy.NewAuthenticatedAPIKeyIdentity("policy-without-usage-key")
	if err != nil {
		t.Fatal(err)
	}
	first, err := New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.APIKeyPolicy().Create(ctx, identity, "No usage", apikeypolicy.ProfileInput{
		Name: "Restricted", Providers: []string{"codex"}, Models: []string{"policy-without-usage-model"},
	}); err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.Close()

	second, err := New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	decision, err := second.APIKeyPolicy().Decide(identity)
	if err != nil || decision.Mode != apikeypolicy.ModeProfile || decision.Snapshot == nil || decision.Snapshot.ProfileName != "Restricted" {
		t.Fatalf("decision=%#v error=%v", decision, err)
	}
}
