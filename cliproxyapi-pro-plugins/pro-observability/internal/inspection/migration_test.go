package inspection

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	embeddedusage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage"
)

func TestStartMigratesLegacyScheduleIntoPluginSQLite(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "usage.sqlite")
	schedulePath := filepath.Join(root, "account-inspection-schedule.json")
	if err := os.WriteFile(schedulePath, []byte(`{"enabled":false,"intervalMinutes":90,"settings":{"targetType":"codex","workers":2}}`), 0o600); err != nil {
		t.Fatalf("write legacy schedule: %v", err)
	}
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", schedulePath)
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", filepath.Join(root, "account-inspection-snapshot.json"))

	usageCtx, cancelUsage := context.WithCancel(context.Background())
	usageConfig := embeddedusage.LoadConfig()
	usageConfig.DBPath = dbPath
	usageConfig.LegacyDBPath = dbPath
	usageService, err := embeddedusage.StartWithConfig(usageCtx, usageConfig)
	if err != nil {
		t.Fatalf("start usage: %v", err)
	}
	embeddedusage.SetDefaultService(usageService)
	defer func() {
		embeddedusage.SetDefaultService(nil)
		cancelUsage()
		usageService.Wait()
	}()

	inspectionService, err := Start(context.Background(), &gatewayStub{}, Config{})
	if err != nil {
		t.Fatalf("start inspection: %v", err)
	}
	inspectionService.Close()
	state, found, err := embeddedusage.GetAccountInspectionState(context.Background(), accountInspectionScheduleStateKey)
	if err != nil || !found {
		t.Fatalf("migrated state = %#v found=%v err=%v", state, found, err)
	}
	if string(state.Payload) == "" || state.SchemaVersion != accountInspectionStateSchemaVersion {
		t.Fatalf("migrated state = %#v", state)
	}
}
