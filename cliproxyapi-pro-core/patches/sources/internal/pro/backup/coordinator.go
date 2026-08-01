// Package backup coordinates cross-module export/import hooks without making
// the observability transport own inspection or live routing state.
package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	prostate "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/state"
)

type JSONExporter func() ([]byte, bool, error)
type JSONImporter func([]byte) error
type RuntimeStateImporter func([]prostate.RoutingCursor, []prostate.AuthRuntimeStats) error

type FlushFunc func(context.Context) error
type ExportFunc func(context.Context) ([]byte, error)

// ImportPlan captures the cross-module restore barriers. Parsing remains at the
// HTTP boundary, while this coordinator owns lifecycle and application order.
type ImportPlan struct {
	FlushQueues         func(context.Context) error
	ImportDatabase      func(context.Context) error
	ReloadConfiguration func(context.Context) error
	ApplyRuntimeState   func(context.Context) error
	RestoreInspection   func(context.Context) error
	CleanupLegacy       func(context.Context) error
}

type Lifecycle struct {
	Pause  func(context.Context) error
	Resume func(context.Context) error
}

type Coordinator struct {
	mu sync.RWMutex

	scheduleExporter JSONExporter
	scheduleImporter JSONImporter
	snapshotExporter JSONExporter
	snapshotImporter JSONImporter
	runtimeImporter  RuntimeStateImporter
	legacyCleanup    func(context.Context) error
	lifecycles       []Lifecycle
}

func NewCoordinator() *Coordinator { return &Coordinator{} }

var Default = NewCoordinator()

func (c *Coordinator) SetInspectionSchedule(exporter JSONExporter, importer JSONImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.scheduleExporter, c.scheduleImporter = exporter, importer
	c.mu.Unlock()
}

func (c *Coordinator) SetInspectionSnapshot(exporter JSONExporter, importer JSONImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.snapshotExporter, c.snapshotImporter = exporter, importer
	c.mu.Unlock()
}

func (c *Coordinator) SetRuntimeStateImporter(importer RuntimeStateImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.runtimeImporter = importer
	c.mu.Unlock()
}

func (c *Coordinator) SetLegacyCleanup(cleanup func(context.Context) error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.legacyCleanup = cleanup
	c.mu.Unlock()
}

func (c *Coordinator) CleanupLegacy(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	cleanup := c.legacyCleanup
	c.mu.RUnlock()
	if cleanup == nil {
		return nil
	}
	return cleanup(ctx)
}

func (c *Coordinator) RegisterLifecycle(lifecycle Lifecycle) func() {
	if c == nil || (lifecycle.Pause == nil && lifecycle.Resume == nil) {
		return func() {}
	}
	c.mu.Lock()
	c.lifecycles = append(c.lifecycles, lifecycle)
	index := len(c.lifecycles) - 1
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		if index < len(c.lifecycles) {
			c.lifecycles[index] = Lifecycle{}
		}
		c.mu.Unlock()
	}
}

// ExecuteImport enforces the restore order shared by HTTP imports and future
// non-HTTP restore surfaces. Successfully paused modules are always resumed in
// reverse order, including when a later import phase fails.
func (c *Coordinator) ExecuteImport(ctx context.Context, plan ImportPlan) (err error) {
	if c == nil {
		return executeImportPhases(ctx, plan)
	}
	c.mu.RLock()
	lifecycles := append([]Lifecycle(nil), c.lifecycles...)
	c.mu.RUnlock()
	paused := make([]Lifecycle, 0, len(lifecycles))
	for _, lifecycle := range lifecycles {
		if lifecycle.Pause == nil && lifecycle.Resume == nil {
			continue
		}
		if lifecycle.Pause != nil {
			if err := lifecycle.Pause(ctx); err != nil {
				resumeLifecycles(ctx, paused)
				return err
			}
		}
		paused = append(paused, lifecycle)
	}
	defer func() {
		if resumeErr := resumeLifecycles(ctx, paused); err == nil {
			err = resumeErr
		}
	}()
	return executeImportPhases(ctx, plan)
}

func executeImportPhases(ctx context.Context, plan ImportPlan) error {
	for _, phase := range []func(context.Context) error{
		plan.FlushQueues,
		plan.ImportDatabase,
		plan.ReloadConfiguration,
		plan.ApplyRuntimeState,
		plan.RestoreInspection,
		plan.CleanupLegacy,
	} {
		if phase != nil {
			if err := phase(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func resumeLifecycles(ctx context.Context, lifecycles []Lifecycle) error {
	var firstErr error
	for index := len(lifecycles) - 1; index >= 0; index-- {
		if lifecycles[index].Resume == nil {
			continue
		}
		if err := lifecycles[index].Resume(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type inspectionScheduleRecord struct {
	RecordType string          `json:"record_type"`
	Version    int             `json:"version"`
	Schedule   json.RawMessage `json:"schedule"`
	ExportedAt int64           `json:"exported_at_ms"`
}

type inspectionSnapshotRecord struct {
	RecordType string          `json:"record_type"`
	Version    int             `json:"version"`
	Snapshot   json.RawMessage `json:"snapshot"`
	ExportedAt int64           `json:"exported_at_ms"`
}

type manifestRecord struct {
	RecordType string `json:"record_type"`
	Version    int    `json:"version"`
	Records    int    `json:"records"`
	SHA256     string `json:"sha256"`
	ExportedAt int64  `json:"exported_at_ms"`
}

// ExportJSONL owns the flush/snapshot/manifest sequence while preserving the
// existing line format byte-for-byte.
func (c *Coordinator) ExportJSONL(ctx context.Context, flush FlushFunc, export ExportFunc) ([]byte, error) {
	if flush != nil {
		if err := flush(ctx); err != nil {
			return nil, err
		}
	}
	var data []byte
	var err error
	if export != nil {
		data, err = export(ctx)
		if err != nil {
			return nil, err
		}
	}
	appendRecord := func(record any) error {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		data = append(data, line...)
		data = append(data, '\n')
		return nil
	}
	if schedule, ok, err := c.ExportInspectionSchedule(); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if ok {
			if err := appendRecord(inspectionScheduleRecord{
				RecordType: "account_inspection_schedule", Version: 1,
				Schedule: schedule, ExportedAt: time.Now().UnixMilli(),
			}); err != nil {
				return nil, err
			}
		}
	}
	if snapshot, ok, err := c.ExportInspectionSnapshot(); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if ok {
			if err := appendRecord(inspectionSnapshotRecord{
				RecordType: "account_inspection_snapshot", Version: 1,
				Snapshot: snapshot, ExportedAt: time.Now().UnixMilli(),
			}); err != nil {
				return nil, err
			}
		}
	}
	digest := sha256.Sum256(data)
	manifest, err := json.Marshal(manifestRecord{
		RecordType: "backup_manifest", Version: 1,
		Records: bytes.Count(data, []byte{'\n'}), SHA256: fmt.Sprintf("%x", digest),
		ExportedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	output := make([]byte, 0, len(manifest)+1+len(data))
	output = append(output, manifest...)
	output = append(output, '\n')
	output = append(output, data...)
	return output, nil
}

func (c *Coordinator) ExportInspectionSchedule() ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	exporter := c.scheduleExporter
	c.mu.RUnlock()
	if exporter == nil {
		return nil, false, nil
	}
	return exporter()
}

func (c *Coordinator) ExportInspectionSnapshot() ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	exporter := c.snapshotExporter
	c.mu.RUnlock()
	if exporter == nil {
		return nil, false, nil
	}
	return exporter()
}

func (c *Coordinator) ImportInspectionSchedule(raw []byte) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	importer := c.scheduleImporter
	c.mu.RUnlock()
	if importer == nil {
		return nil
	}
	return importer(raw)
}

func (c *Coordinator) ImportInspectionSnapshot(raw []byte) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	importer := c.snapshotImporter
	c.mu.RUnlock()
	if importer == nil {
		return nil
	}
	return importer(raw)
}

func (c *Coordinator) HasRuntimeStateImporter() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtimeImporter != nil
}

func (c *Coordinator) ImportRuntimeState(cursors []prostate.RoutingCursor, stats []prostate.AuthRuntimeStats) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	importer := c.runtimeImporter
	c.mu.RUnlock()
	if importer == nil {
		return nil
	}
	return importer(cursors, stats)
}
