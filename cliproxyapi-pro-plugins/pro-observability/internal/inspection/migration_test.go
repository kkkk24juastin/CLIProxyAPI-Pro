package inspection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	embeddedusage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage"
)

func startInspectionMigrationUsage(t *testing.T, root string) *embeddedusage.Service {
	t.Helper()
	usageCtx, cancelUsage := context.WithCancel(context.Background())
	usageConfig := embeddedusage.LoadConfig()
	usageConfig.DBPath = filepath.Join(root, "usage.sqlite")
	usageConfig.LegacyDBPath = usageConfig.DBPath
	usageService, err := embeddedusage.StartWithConfig(usageCtx, usageConfig)
	if err != nil {
		cancelUsage()
		t.Fatalf("start usage: %v", err)
	}
	embeddedusage.SetDefaultService(usageService)
	t.Cleanup(func() {
		embeddedusage.SetDefaultService(nil)
		cancelUsage()
		usageService.Wait()
	})
	return usageService
}

func TestStartMigratesLegacyScheduleIntoPluginSQLite(t *testing.T) {
	root := t.TempDir()
	schedulePath := filepath.Join(root, "account-inspection-schedule.json")
	if err := os.WriteFile(schedulePath, []byte(`{"enabled":false,"intervalMinutes":90,"settings":{"targetType":"codex","workers":2}}`), 0o600); err != nil {
		t.Fatalf("write legacy schedule: %v", err)
	}
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", schedulePath)
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", filepath.Join(root, "account-inspection-snapshot.json"))

	startInspectionMigrationUsage(t, root)

	inspectionService, err := Start(context.Background(), &gatewayStub{})
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

func TestStartMigratesLegacySnapshotIntoPluginSQLite(t *testing.T) {
	root := t.TempDir()
	snapshotPath := filepath.Join(root, "account-inspection-snapshot.json")
	raw := `{"version":1,"state":"completed","lastStartedAt":1000,"lastFinishedAt":2000,"settings":{"targetType":"xai"},"results":[]}`
	if err := os.WriteFile(snapshotPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", filepath.Join(root, "account-inspection-schedule.json"))
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", snapshotPath)
	startInspectionMigrationUsage(t, root)
	service, err := Start(context.Background(), &gatewayStub{})
	if err != nil {
		t.Fatalf("start inspection: %v", err)
	}
	service.Close()
	state, found, err := embeddedusage.GetAccountInspectionState(context.Background(), accountInspectionSnapshotStateKey)
	if err != nil || !found || state.SchemaVersion != accountInspectionStateSchemaVersion {
		t.Fatalf("migrated snapshot = %#v found=%v err=%v", state, found, err)
	}
}

func TestStartRejectsMalformedLegacySchedule(t *testing.T) {
	root := t.TempDir()
	schedulePath := filepath.Join(root, "account-inspection-schedule.json")
	if err := os.WriteFile(schedulePath, []byte(`{"enabled":`), 0o600); err != nil {
		t.Fatalf("write malformed schedule: %v", err)
	}
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", schedulePath)
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", filepath.Join(root, "account-inspection-snapshot.json"))
	startInspectionMigrationUsage(t, root)

	service, err := Start(context.Background(), &gatewayStub{})
	if service != nil || err == nil || !strings.Contains(err.Error(), "decode legacy account inspection schedule") {
		t.Fatalf("Start() = %#v, %v", service, err)
	}
}

func TestStartRejectsUnsupportedSQLiteStateSchema(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", filepath.Join(root, "account-inspection-schedule.json"))
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", filepath.Join(root, "account-inspection-snapshot.json"))
	startInspectionMigrationUsage(t, root)
	if _, err := embeddedusage.SetAccountInspectionState(context.Background(), accountInspectionScheduleStateKey, accountInspectionStateSchemaVersion+1, []byte(`{"enabled":false}`)); err != nil {
		t.Fatalf("seed future schedule state: %v", err)
	}

	service, err := Start(context.Background(), &gatewayStub{})
	if service != nil || err == nil || !strings.Contains(err.Error(), "unsupported account inspection schedule state schema") {
		t.Fatalf("Start() = %#v, %v", service, err)
	}
}

func TestSnapshotPersistsAndRestoresFromPluginSQLite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", filepath.Join(root, "account-inspection-schedule.json"))
	t.Setenv("ACCOUNT_INSPECTION_SNAPSHOT_PATH", filepath.Join(root, "account-inspection-snapshot.json"))
	startInspectionMigrationUsage(t, root)
	result := testInspectionProviderResult("xai-1", "xai", accountInspectionActionKeep, false, testStatusCode(502), false, "upstream error")
	result.ErrorDetail = `{"error":{"message":"raw upstream response"}}`
	source := &accountInspectionScheduler{
		lastRunSettings: defaultAccountInspectionSettings(),
		status: accountInspectionStatus{
			State: accountInspectionStateCompleted, LastStartedAt: 1000, LastFinishedAt: 2000,
			Summary: accountInspectionSummary{TotalFiles: 1, ProbeSetCount: 1, SampledCount: 1, ErrorCount: 1},
			Results: []accountInspectionResult{result},
		},
	}
	if err := source.saveResultSnapshotLocked(); err != nil {
		t.Fatalf("saveResultSnapshotLocked() error = %v", err)
	}
	restored := &accountInspectionScheduler{snapshotPath: filepath.Join(root, "missing-legacy-snapshot.json")}
	if err := restored.loadResultSnapshot(); err != nil {
		t.Fatalf("loadResultSnapshot() error = %v", err)
	}
	if !restored.status.RestoredSnapshot || restored.status.LastFinishedAt != 2000 || len(restored.status.Results) != 1 {
		t.Fatalf("restored snapshot = %#v", restored.status)
	}
	if restored.status.Results[0].ErrorDetail != result.ErrorDetail {
		t.Fatalf("restored error detail = %q", restored.status.Results[0].ErrorDetail)
	}
}

func TestSnapshotExportKeepsLastFinishedSQLiteStateDuringRun(t *testing.T) {
	root := t.TempDir()
	startInspectionMigrationUsage(t, root)
	scheduler := &accountInspectionScheduler{
		lastRunSettings: defaultAccountInspectionSettings(),
		status: accountInspectionStatus{
			State: accountInspectionStateCompleted, LastStartedAt: 1000, LastFinishedAt: 2000,
			Results: []accountInspectionResult{testInspectionResult("finished", accountInspectionActionKeep, false, nil, false, "")},
		},
	}
	if err := scheduler.saveResultSnapshotLocked(); err != nil {
		t.Fatalf("saveResultSnapshotLocked() error = %v", err)
	}
	scheduler.status = accountInspectionStatus{State: accountInspectionStateRunning, LastStartedAt: 3000}
	raw, ok, err := scheduler.exportResultSnapshot()
	if err != nil || !ok {
		t.Fatalf("exportResultSnapshot() = ok:%v err:%v", ok, err)
	}
	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil || snapshot.LastFinishedAt != 2000 || len(snapshot.Results) != 1 || snapshot.Results[0].Key != "finished" {
		t.Fatalf("exported snapshot = %#v, %v", snapshot, err)
	}
}
