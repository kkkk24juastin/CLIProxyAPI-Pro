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

type inspectionRegistration struct {
	owner    uint64
	exporter JSONExporter
	importer JSONImporter
}

type runtimeRegistration struct {
	owner    uint64
	importer RuntimeStateImporter
}

type cleanupRegistration struct {
	owner   uint64
	cleanup func(context.Context) error
}

type lifecycleRegistration struct {
	owner     uint64
	lifecycle Lifecycle
}

type Coordinator struct {
	mu       sync.RWMutex
	importMu sync.Mutex

	scheduleExporter JSONExporter
	scheduleImporter JSONImporter
	scheduleOwner    uint64
	scheduleOwners   []inspectionRegistration
	snapshotExporter JSONExporter
	snapshotImporter JSONImporter
	snapshotOwner    uint64
	snapshotOwners   []inspectionRegistration
	runtimeImporter  RuntimeStateImporter
	runtimeOwner     uint64
	runtimeOwners    []runtimeRegistration
	legacyCleanup    func(context.Context) error
	cleanupOwner     uint64
	cleanupOwners    []cleanupRegistration
	nextOwner        uint64
	lifecycles       []lifecycleRegistration
}

func NewCoordinator() *Coordinator { return &Coordinator{} }

var Default = NewCoordinator()

func (c *Coordinator) SetInspectionSchedule(exporter JSONExporter, importer JSONImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.nextOwner++
	c.scheduleOwner = c.nextOwner
	c.scheduleOwners = nil
	c.scheduleExporter, c.scheduleImporter = exporter, importer
	c.mu.Unlock()
}

// RegisterInspectionSchedule installs lifecycle-owned inspection hooks. The
// returned function only removes this exact registration, so an older owner
// cannot clear a newer replacement during shutdown.
func (c *Coordinator) RegisterInspectionSchedule(exporter JSONExporter, importer JSONImporter) func() {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.nextOwner++
	owner := c.nextOwner
	c.scheduleOwner = owner
	c.scheduleExporter, c.scheduleImporter = exporter, importer
	c.scheduleOwners = append(c.scheduleOwners, inspectionRegistration{owner: owner, exporter: exporter, importer: importer})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		if c.scheduleOwner == owner {
			c.scheduleOwners = removeInspectionRegistration(c.scheduleOwners, owner)
			if count := len(c.scheduleOwners); count > 0 {
				current := c.scheduleOwners[count-1]
				c.scheduleOwner = current.owner
				c.scheduleExporter, c.scheduleImporter = current.exporter, current.importer
			} else {
				c.scheduleExporter, c.scheduleImporter = nil, nil
				c.scheduleOwner = 0
			}
		} else {
			c.scheduleOwners = removeInspectionRegistration(c.scheduleOwners, owner)
		}
		c.mu.Unlock()
	}
}

func (c *Coordinator) SetInspectionSnapshot(exporter JSONExporter, importer JSONImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.nextOwner++
	c.snapshotOwner = c.nextOwner
	c.snapshotOwners = nil
	c.snapshotExporter, c.snapshotImporter = exporter, importer
	c.mu.Unlock()
}

func (c *Coordinator) RegisterInspectionSnapshot(exporter JSONExporter, importer JSONImporter) func() {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.nextOwner++
	owner := c.nextOwner
	c.snapshotOwner = owner
	c.snapshotExporter, c.snapshotImporter = exporter, importer
	c.snapshotOwners = append(c.snapshotOwners, inspectionRegistration{owner: owner, exporter: exporter, importer: importer})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		if c.snapshotOwner == owner {
			c.snapshotOwners = removeInspectionRegistration(c.snapshotOwners, owner)
			if count := len(c.snapshotOwners); count > 0 {
				current := c.snapshotOwners[count-1]
				c.snapshotOwner = current.owner
				c.snapshotExporter, c.snapshotImporter = current.exporter, current.importer
			} else {
				c.snapshotExporter, c.snapshotImporter = nil, nil
				c.snapshotOwner = 0
			}
		} else {
			c.snapshotOwners = removeInspectionRegistration(c.snapshotOwners, owner)
		}
		c.mu.Unlock()
	}
}

func (c *Coordinator) SetRuntimeStateImporter(importer RuntimeStateImporter) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.nextOwner++
	c.runtimeOwner = c.nextOwner
	c.runtimeOwners = nil
	c.runtimeImporter = importer
	c.mu.Unlock()
}

func (c *Coordinator) RegisterRuntimeStateImporter(importer RuntimeStateImporter) func() {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.nextOwner++
	owner := c.nextOwner
	c.runtimeOwner = owner
	c.runtimeImporter = importer
	c.runtimeOwners = append(c.runtimeOwners, runtimeRegistration{owner: owner, importer: importer})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		if c.runtimeOwner == owner {
			c.runtimeOwners = removeRuntimeRegistration(c.runtimeOwners, owner)
			if count := len(c.runtimeOwners); count > 0 {
				current := c.runtimeOwners[count-1]
				c.runtimeOwner, c.runtimeImporter = current.owner, current.importer
			} else {
				c.runtimeImporter = nil
				c.runtimeOwner = 0
			}
		} else {
			c.runtimeOwners = removeRuntimeRegistration(c.runtimeOwners, owner)
		}
		c.mu.Unlock()
	}
}

func (c *Coordinator) SetLegacyCleanup(cleanup func(context.Context) error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.nextOwner++
	c.cleanupOwner = c.nextOwner
	c.cleanupOwners = nil
	c.legacyCleanup = cleanup
	c.mu.Unlock()
}

func (c *Coordinator) RegisterLegacyCleanup(cleanup func(context.Context) error) func() {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.nextOwner++
	owner := c.nextOwner
	c.cleanupOwner = owner
	c.legacyCleanup = cleanup
	c.cleanupOwners = append(c.cleanupOwners, cleanupRegistration{owner: owner, cleanup: cleanup})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		if c.cleanupOwner == owner {
			c.cleanupOwners = removeCleanupRegistration(c.cleanupOwners, owner)
			if count := len(c.cleanupOwners); count > 0 {
				current := c.cleanupOwners[count-1]
				c.cleanupOwner, c.legacyCleanup = current.owner, current.cleanup
			} else {
				c.legacyCleanup = nil
				c.cleanupOwner = 0
			}
		} else {
			c.cleanupOwners = removeCleanupRegistration(c.cleanupOwners, owner)
		}
		c.mu.Unlock()
	}
}

func removeInspectionRegistration(items []inspectionRegistration, owner uint64) []inspectionRegistration {
	for index := range items {
		if items[index].owner == owner {
			return append(items[:index], items[index+1:]...)
		}
	}
	return items
}

func removeRuntimeRegistration(items []runtimeRegistration, owner uint64) []runtimeRegistration {
	for index := range items {
		if items[index].owner == owner {
			return append(items[:index], items[index+1:]...)
		}
	}
	return items
}

func removeCleanupRegistration(items []cleanupRegistration, owner uint64) []cleanupRegistration {
	for index := range items {
		if items[index].owner == owner {
			return append(items[:index], items[index+1:]...)
		}
	}
	return items
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
	c.nextOwner++
	owner := c.nextOwner
	c.lifecycles = append(c.lifecycles, lifecycleRegistration{owner: owner, lifecycle: lifecycle})
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		for index := range c.lifecycles {
			if c.lifecycles[index].owner == owner {
				c.lifecycles = append(c.lifecycles[:index], c.lifecycles[index+1:]...)
				break
			}
		}
		c.mu.Unlock()
	}
}

// ExecuteImport enforces the restore order shared by HTTP imports and future
// non-HTTP restore surfaces. Successfully paused modules are always resumed in
// reverse order, including when a later import phase fails.
func (c *Coordinator) ExecuteImport(ctx context.Context, plan ImportPlan) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return executeImportPhases(ctx, plan)
	}
	c.importMu.Lock()
	defer c.importMu.Unlock()
	cleanupCtx := context.WithoutCancel(ctx)
	c.mu.RLock()
	registrations := append([]lifecycleRegistration(nil), c.lifecycles...)
	c.mu.RUnlock()
	lifecycles := make([]Lifecycle, 0, len(registrations))
	for _, registration := range registrations {
		lifecycles = append(lifecycles, registration.lifecycle)
	}
	paused := make([]Lifecycle, 0, len(lifecycles))
	for _, lifecycle := range lifecycles {
		if lifecycle.Pause == nil && lifecycle.Resume == nil {
			continue
		}
		if lifecycle.Pause != nil {
			if err := lifecycle.Pause(ctx); err != nil {
				// Pause implementations may have already closed their admission
				// gate before waiting for active work. Include the failing lifecycle
				// in cleanup and never use the canceled request context to resume.
				resumeLifecycles(cleanupCtx, append(paused, lifecycle))
				return err
			}
		}
		paused = append(paused, lifecycle)
	}
	defer func() {
		if resumeErr := resumeLifecycles(cleanupCtx, paused); err == nil {
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
