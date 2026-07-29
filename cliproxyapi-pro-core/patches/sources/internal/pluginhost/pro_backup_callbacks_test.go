package pluginhost

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

func TestProBackupHostCallbacksBridgeScheduleExportAndImport(t *testing.T) {
	embeddedusage.SetAccountInspectionScheduleHandlers(func() ([]byte, bool, error) {
		return []byte(`{"enabled":true}`), true, nil
	}, nil)
	t.Cleanup(func() { embeddedusage.SetAccountInspectionScheduleHandlers(nil, nil) })
	ctx := withHostCallbackPluginID(context.Background(), proObservabilityPluginID)
	rawRequest, _ := json.Marshal(proBackupExportRequest{Kind: "account-inspection-schedule"})
	raw, err := New().callHostProBackupExport(ctx, rawRequest)
	if err != nil {
		t.Fatal(err)
	}
	var wrapped struct {
		OK     bool                    `json:"ok"`
		Result proBackupExportResponse `json:"result"`
	}
	if err = json.Unmarshal(raw, &wrapped); err != nil || !wrapped.OK || !wrapped.Result.Found || string(wrapped.Result.Data) != `{"enabled":true}` {
		t.Fatalf("export response = %#v raw=%s err=%v", wrapped, raw, err)
	}

	var imported string
	embeddedusage.SetAccountInspectionScheduleHandlers(nil, func(data []byte) error {
		imported = string(data)
		return nil
	})
	rawRequest, _ = json.Marshal(proBackupImportRequest{Kind: "account-inspection-schedule", Data: json.RawMessage(`{"enabled":false}`)})
	if _, err = New().callHostProBackupImport(ctx, rawRequest); err != nil || imported != `{"enabled":false}` {
		t.Fatalf("imported=%s err=%v", imported, err)
	}
}

func TestProBackupHostCallbacksRejectOtherPlugins(t *testing.T) {
	ctx := withHostCallbackPluginID(context.Background(), "other")
	raw, _ := json.Marshal(proBackupExportRequest{Kind: "account-inspection-schedule"})
	if _, err := New().callHostProBackupExport(ctx, raw); err == nil {
		t.Fatal("unauthorized plugin unexpectedly accessed Pro backup callback")
	}
}
