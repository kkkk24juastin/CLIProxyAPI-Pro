package profeatures

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/oauthmodelpolicy/config"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/proxypool/config"
)

func TestRuntimeUpdatesPersistOnlyToSQLite(t *testing.T) {
	ctx := startMigrationStore(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	before := []byte("host: 127.0.0.1\nport: 8317\n")
	if err := os.WriteFile(configPath, before, 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := New(ctx, configPath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	proxyCfg := proxyconfig.Default()
	proxyCfg.TakeoverEnabled = true
	if err := runtime.UpdateProxyConfig(ctx, proxyCfg); err != nil {
		t.Fatal(err)
	}
	modelCfg, err := modelconfig.Parse([]byte(`{
		"enabled": true,
		"providers": {"xai": {"plans": {"free": {"excluded-models": ["grok-pro-*"]}}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpdateModelConfig(ctx, modelCfg); err != nil {
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
	modelItem, modelFound, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceOAuthModelPolicy)
	if err != nil || !modelFound {
		t.Fatalf("model setting = found:%v err:%v", modelFound, err)
	}
	persistedModel, err := modelconfig.Parse(modelItem.Settings)
	if err != nil || !persistedModel.Enabled || len(persistedModel.Providers) != 1 {
		t.Fatalf("persisted model config = %#v err:%v", persistedModel, err)
	}
}
