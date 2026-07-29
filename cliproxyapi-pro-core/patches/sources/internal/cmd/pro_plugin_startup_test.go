package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
)

func TestRequireProObservabilityPluginBlocksServiceStartupWhenMissing(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Dir = t.TempDir()
	applyProRequiredStartupConfig(cfg, "")
	host := pluginhost.New()
	t.Cleanup(host.ShutdownAll)
	_, err := requireProObservabilityPlugin(context.Background(), cfg, host)
	if err == nil || !strings.Contains(err.Error(), config.ProObservabilityPluginID) {
		t.Fatalf("requireProObservabilityPlugin() error = %v", err)
	}
}
