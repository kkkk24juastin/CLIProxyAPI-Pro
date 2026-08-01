package backup

import (
	"bytes"
	"testing"

	prostate "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/state"
)

func TestCoordinatorKeepsModuleStatePortsIndependent(t *testing.T) {
	coordinator := NewCoordinator()
	wantSchedule := []byte(`{"enabled":true}`)
	wantSnapshot := []byte(`{"status":"completed"}`)
	var importedSchedule, importedSnapshot []byte
	coordinator.SetInspectionSchedule(
		func() ([]byte, bool, error) { return wantSchedule, true, nil },
		func(raw []byte) error { importedSchedule = append([]byte(nil), raw...); return nil },
	)
	coordinator.SetInspectionSnapshot(
		func() ([]byte, bool, error) { return wantSnapshot, true, nil },
		func(raw []byte) error { importedSnapshot = append([]byte(nil), raw...); return nil },
	)

	if got, ok, err := coordinator.ExportInspectionSchedule(); err != nil || !ok || !bytes.Equal(got, wantSchedule) {
		t.Fatalf("schedule export = %s, %v, %v", got, ok, err)
	}
	if got, ok, err := coordinator.ExportInspectionSnapshot(); err != nil || !ok || !bytes.Equal(got, wantSnapshot) {
		t.Fatalf("snapshot export = %s, %v, %v", got, ok, err)
	}
	if err := coordinator.ImportInspectionSchedule(wantSchedule); err != nil { t.Fatal(err) }
	if err := coordinator.ImportInspectionSnapshot(wantSnapshot); err != nil { t.Fatal(err) }
	if !bytes.Equal(importedSchedule, wantSchedule) || !bytes.Equal(importedSnapshot, wantSnapshot) {
		t.Fatalf("imported schedule=%s snapshot=%s", importedSchedule, importedSnapshot)
	}

	var importedCursors []prostate.RoutingCursor
	var importedStats []prostate.AuthRuntimeStats
	coordinator.SetRuntimeStateImporter(func(cursors []prostate.RoutingCursor, stats []prostate.AuthRuntimeStats) error {
		importedCursors = append([]prostate.RoutingCursor(nil), cursors...)
		importedStats = append([]prostate.AuthRuntimeStats(nil), stats...)
		return nil
	})
	wantCursors := []prostate.RoutingCursor{{CursorKey: "single|codex", LastAuthID: "auth-a"}}
	wantStats := []prostate.AuthRuntimeStats{{AuthIndex: "idx-a", AuthID: "auth-a", SelectedCount: 2}}
	if !coordinator.HasRuntimeStateImporter() { t.Fatal("runtime importer not registered") }
	if err := coordinator.ImportRuntimeState(wantCursors, wantStats); err != nil { t.Fatal(err) }
	if len(importedCursors) != 1 || importedCursors[0] != wantCursors[0] || len(importedStats) != 1 || importedStats[0].AuthID != "auth-a" {
		t.Fatalf("runtime import = %#v %#v", importedCursors, importedStats)
	}
}
