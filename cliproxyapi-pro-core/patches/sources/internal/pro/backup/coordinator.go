// Package backup coordinates cross-module export/import hooks without making
// the observability transport own inspection or live routing state.
package backup

import (
	"sync"

	prostate "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/state"
)

type JSONExporter func() ([]byte, bool, error)
type JSONImporter func([]byte) error
type RuntimeStateImporter func([]prostate.RoutingCursor, []prostate.AuthRuntimeStats) error

type Coordinator struct {
	mu sync.RWMutex

	scheduleExporter JSONExporter
	scheduleImporter JSONImporter
	snapshotExporter JSONExporter
	snapshotImporter JSONImporter
	runtimeImporter  RuntimeStateImporter
}

func NewCoordinator() *Coordinator { return &Coordinator{} }

var Default = NewCoordinator()

func (c *Coordinator) SetInspectionSchedule(exporter JSONExporter, importer JSONImporter) {
	if c == nil { return }
	c.mu.Lock()
	c.scheduleExporter, c.scheduleImporter = exporter, importer
	c.mu.Unlock()
}

func (c *Coordinator) SetInspectionSnapshot(exporter JSONExporter, importer JSONImporter) {
	if c == nil { return }
	c.mu.Lock()
	c.snapshotExporter, c.snapshotImporter = exporter, importer
	c.mu.Unlock()
}

func (c *Coordinator) SetRuntimeStateImporter(importer RuntimeStateImporter) {
	if c == nil { return }
	c.mu.Lock()
	c.runtimeImporter = importer
	c.mu.Unlock()
}

func (c *Coordinator) ExportInspectionSchedule() ([]byte, bool, error) {
	if c == nil { return nil, false, nil }
	c.mu.RLock(); exporter := c.scheduleExporter; c.mu.RUnlock()
	if exporter == nil { return nil, false, nil }
	return exporter()
}

func (c *Coordinator) ExportInspectionSnapshot() ([]byte, bool, error) {
	if c == nil { return nil, false, nil }
	c.mu.RLock(); exporter := c.snapshotExporter; c.mu.RUnlock()
	if exporter == nil { return nil, false, nil }
	return exporter()
}

func (c *Coordinator) ImportInspectionSchedule(raw []byte) error {
	if c == nil { return nil }
	c.mu.RLock(); importer := c.scheduleImporter; c.mu.RUnlock()
	if importer == nil { return nil }
	return importer(raw)
}

func (c *Coordinator) ImportInspectionSnapshot(raw []byte) error {
	if c == nil { return nil }
	c.mu.RLock(); importer := c.snapshotImporter; c.mu.RUnlock()
	if importer == nil { return nil }
	return importer(raw)
}

func (c *Coordinator) HasRuntimeStateImporter() bool {
	if c == nil { return false }
	c.mu.RLock(); defer c.mu.RUnlock()
	return c.runtimeImporter != nil
}

func (c *Coordinator) ImportRuntimeState(cursors []prostate.RoutingCursor, stats []prostate.AuthRuntimeStats) error {
	if c == nil { return nil }
	c.mu.RLock(); importer := c.runtimeImporter; c.mu.RUnlock()
	if importer == nil { return nil }
	return importer(cursors, stats)
}
