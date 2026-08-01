package backup

import (
	"bytes"
	"context"
	"errors"
	"strings"
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
	if err := coordinator.ImportInspectionSchedule(wantSchedule); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ImportInspectionSnapshot(wantSnapshot); err != nil {
		t.Fatal(err)
	}
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
	if !coordinator.HasRuntimeStateImporter() {
		t.Fatal("runtime importer not registered")
	}
	if err := coordinator.ImportRuntimeState(wantCursors, wantStats); err != nil {
		t.Fatal(err)
	}
	if len(importedCursors) != 1 || importedCursors[0] != wantCursors[0] || len(importedStats) != 1 || importedStats[0].AuthID != "auth-a" {
		t.Fatalf("runtime import = %#v %#v", importedCursors, importedStats)
	}
}

func TestExecuteImportUsesStablePhaseOrderAndReverseResume(t *testing.T) {
	coordinator := NewCoordinator()
	var calls []string
	coordinator.RegisterLifecycle(Lifecycle{
		Pause:  func(context.Context) error { calls = append(calls, "pause-observability"); return nil },
		Resume: func(context.Context) error { calls = append(calls, "resume-observability"); return nil },
	})
	coordinator.RegisterLifecycle(Lifecycle{
		Pause:  func(context.Context) error { calls = append(calls, "pause-inspection"); return nil },
		Resume: func(context.Context) error { calls = append(calls, "resume-inspection"); return nil },
	})
	phase := func(name string) func(context.Context) error {
		return func(context.Context) error { calls = append(calls, name); return nil }
	}
	err := coordinator.ExecuteImport(context.Background(), ImportPlan{
		FlushQueues:         phase("flush"),
		ImportDatabase:      phase("database"),
		ReloadConfiguration: phase("reload"),
		ApplyRuntimeState:   phase("runtime"),
		RestoreInspection:   phase("inspection"),
		CleanupLegacy:       phase("cleanup"),
	})
	if err != nil {
		t.Fatalf("ExecuteImport() error = %v", err)
	}
	want := "pause-observability,pause-inspection,flush,database,reload,runtime,inspection,cleanup,resume-inspection,resume-observability"
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("phase order = %q, want %q", got, want)
	}
}

func TestExecuteImportResumesAfterPhaseFailure(t *testing.T) {
	coordinator := NewCoordinator()
	var calls []string
	coordinator.RegisterLifecycle(Lifecycle{
		Pause:  func(context.Context) error { calls = append(calls, "pause"); return nil },
		Resume: func(context.Context) error { calls = append(calls, "resume"); return nil },
	})
	wantErr := errors.New("database failed")
	err := coordinator.ExecuteImport(context.Background(), ImportPlan{
		FlushQueues:    func(context.Context) error { calls = append(calls, "flush"); return nil },
		ImportDatabase: func(context.Context) error { calls = append(calls, "database"); return wantErr },
		ReloadConfiguration: func(context.Context) error {
			calls = append(calls, "reload")
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteImport() error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(calls, ","); got != "pause,flush,database,resume" {
		t.Fatalf("failure phase order = %q", got)
	}
}

func TestExecuteImportResumesLifecycleWhosePauseWasCanceled(t *testing.T) {
	coordinator := NewCoordinator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	paused := true
	var resumed bool
	coordinator.RegisterLifecycle(Lifecycle{
		Pause: func(ctx context.Context) error { return ctx.Err() },
		Resume: func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Fatal("resume received canceled cleanup context")
			}
			paused = false
			resumed = true
			return nil
		},
	})
	if err := coordinator.ExecuteImport(ctx, ImportPlan{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteImport() error = %v, want context canceled", err)
	}
	if paused || !resumed {
		t.Fatalf("failed pause was not resumed: paused=%v resumed=%v", paused, resumed)
	}
}

func TestOwnedHookUnregisterDoesNotClearNewerRegistration(t *testing.T) {
	coordinator := NewCoordinator()
	unregisterOld := coordinator.RegisterInspectionSchedule(
		func() ([]byte, bool, error) { return []byte(`{"owner":"old"}`), true, nil }, nil,
	)
	unregisterNew := coordinator.RegisterInspectionSchedule(
		func() ([]byte, bool, error) { return []byte(`{"owner":"new"}`), true, nil }, nil,
	)
	unregisterOld()
	got, ok, err := coordinator.ExportInspectionSchedule()
	if err != nil || !ok || string(got) != `{"owner":"new"}` {
		t.Fatalf("newer hook after old unregister = %s, %v, %v", got, ok, err)
	}
	unregisterNew()
	if _, ok, err := coordinator.ExportInspectionSchedule(); err != nil || ok {
		t.Fatalf("hook after owner unregister = _, %v, %v; want absent", ok, err)
	}
}

func TestOwnedHookUnregisterRestoresOlderLiveRegistration(t *testing.T) {
	coordinator := NewCoordinator()
	unregisterOld := coordinator.RegisterInspectionSchedule(
		func() ([]byte, bool, error) { return []byte(`{"owner":"old"}`), true, nil }, nil,
	)
	unregisterNew := coordinator.RegisterInspectionSchedule(
		func() ([]byte, bool, error) { return []byte(`{"owner":"new"}`), true, nil }, nil,
	)
	unregisterNew()
	got, ok, err := coordinator.ExportInspectionSchedule()
	if err != nil || !ok || string(got) != `{"owner":"old"}` {
		t.Fatalf("older hook after new unregister = %s, %v, %v", got, ok, err)
	}
	unregisterOld()
}

func TestExportJSONLFlushesAndIncludesInspectionBeforeManifest(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.SetInspectionSchedule(func() ([]byte, bool, error) {
		return []byte(`{"enabled":true}`), true, nil
	}, nil)
	var calls []string
	data, err := coordinator.ExportJSONL(context.Background(),
		func(context.Context) error { calls = append(calls, "flush"); return nil },
		func(context.Context) ([]byte, error) {
			calls = append(calls, "snapshot")
			return []byte("{\"record_type\":\"usage\"}\n"), nil
		},
	)
	if err != nil {
		t.Fatalf("ExportJSONL() error = %v", err)
	}
	if got := strings.Join(calls, ","); got != "flush,snapshot" {
		t.Fatalf("export order = %q", got)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], `"record_type":"backup_manifest"`) ||
		!strings.Contains(lines[2], `"record_type":"account_inspection_schedule"`) {
		t.Fatalf("unexpected JSONL records: %q", lines)
	}
}
