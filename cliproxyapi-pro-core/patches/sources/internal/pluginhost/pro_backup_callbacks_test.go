package pluginhost

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

func TestProBackupHostCallbacksApplyRuntimeStateImport(t *testing.T) {
	ctx := withHostCallbackPluginID(context.Background(), proObservabilityPluginID)
	var imported []embeddedusage.AuthRuntimeStats
	embeddedusage.SetAuthRuntimeStateImportHandler(func(_ []embeddedusage.RoutingCursorState, stats []embeddedusage.AuthRuntimeStats) error {
		imported = append([]embeddedusage.AuthRuntimeStats(nil), stats...)
		return nil
	})
	t.Cleanup(func() { embeddedusage.SetAuthRuntimeStateImportHandler(nil) })
	rawRequest, _ := json.Marshal(proBackupImportRequest{Kind: "runtime-state", RuntimeStats: []embeddedusage.AuthRuntimeStats{{AuthIndex: "auth-1", SuccessCount: 3}}})
	if _, err := New().callHostProBackupImport(ctx, rawRequest); err != nil || len(imported) != 1 || imported[0].SuccessCount != 3 {
		t.Fatalf("imported=%#v err=%v", imported, err)
	}
}

func TestProBackupHostCallbacksRejectOtherPlugins(t *testing.T) {
	ctx := withHostCallbackPluginID(context.Background(), "other")
	raw, _ := json.Marshal(proBackupImportRequest{Kind: "runtime-state"})
	if _, err := New().callHostProBackupImport(ctx, raw); err == nil {
		t.Fatal("unauthorized plugin unexpectedly accessed Pro backup callback")
	}
}
