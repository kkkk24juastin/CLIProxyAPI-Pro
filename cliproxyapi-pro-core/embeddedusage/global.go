package embeddedusage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	probackup "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/backup"
	prostate "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/state"
)

var globalService *Service
var proSettingConsumers = make(map[string]proSettingConsumerRegistration)
var proSettingConsumerGeneration uint64
var globalStateMu sync.RWMutex
var globalStateWriter *prostate.Writer

func SetDefaultService(service *Service) {
	globalStateMu.Lock()
	if globalStateWriter != nil {
		globalStateWriter.Close()
	}
	globalStateWriter = nil
	globalService = service
	if service != nil && service.store != nil {
		globalStateWriter = prostate.NewWriter(service.store)
	}
	globalStateMu.Unlock()
}

func stopRuntimeStateWriter(service *Service) {
	globalStateMu.Lock()
	if globalService != service {
		globalStateMu.Unlock()
		return
	}
	if globalStateWriter != nil {
		globalStateWriter.Close()
	}
	globalStateWriter = nil
	globalService = nil
	globalStateMu.Unlock()
}

func flushRuntimeStateWrites(ctx context.Context, store *Store) error {
	if ctx == nil {
		ctx = context.Background()
	}
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	var writer *prostate.Writer
	if globalService != nil && globalService.store == store {
		writer = globalStateWriter
	}
	if writer == nil {
		return nil
	}
	return writer.Flush(ctx)
}

func SetAccountInspectionScheduleHandlers(exporter func() ([]byte, bool, error), importer func([]byte) error) {
	probackup.Default.SetInspectionSchedule(exporter, importer)
}

func SetAccountInspectionSnapshotHandlers(exporter func() ([]byte, bool, error), importer func([]byte) error) {
	probackup.Default.SetInspectionSnapshot(exporter, importer)
}

func SetAuthRuntimeStateImportHandler(importer func([]RoutingCursorState, []AuthRuntimeStats) error) {
	probackup.Default.SetRuntimeStateImporter(importer)
}

func SetLegacyQuotaCleanupHandler(cleanup func(context.Context) error) {
	probackup.Default.SetLegacyCleanup(cleanup)
}

type proSettingConsumerRegistration struct {
	generation uint64
	apply      func(context.Context, ProSetting) error
}

// RegisterProSettingConsumer installs one lifecycle-bound runtime consumer for a
// Pro setting namespace. The returned function unregisters only this exact
// registration, so a stopped owner cannot remove a newer replacement.
func RegisterProSettingConsumer(namespace string, apply func(context.Context, ProSetting) error) func() {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || apply == nil {
		return func() {}
	}
	globalStateMu.Lock()
	proSettingConsumerGeneration++
	generation := proSettingConsumerGeneration
	proSettingConsumers[namespace] = proSettingConsumerRegistration{generation: generation, apply: apply}
	globalStateMu.Unlock()
	return func() {
		globalStateMu.Lock()
		current, ok := proSettingConsumers[namespace]
		if ok && current.generation == generation {
			delete(proSettingConsumers, namespace)
		}
		globalStateMu.Unlock()
	}
}

func ApplyImportedProSettings(ctx context.Context, settings []ProSetting) error {
	for _, item := range settings {
		globalStateMu.RLock()
		consumer := proSettingConsumers[item.Namespace]
		globalStateMu.RUnlock()
		if consumer.apply == nil {
			continue
		}
		if err := consumer.apply(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func defaultServer() *Server {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil {
		return nil
	}
	return globalService.Server()
}

func SetQuotaCache(ctx context.Context, entry QuotaCacheEntry) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return globalService.store.SetQuotaCache(ctx, entry)
}

func GetQuotaCache(ctx context.Context, provider, fileName string) ([]QuotaCacheEntry, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return nil, fmt.Errorf("usage service is not available")
	}
	return globalService.store.GetQuotaCache(ctx, provider, fileName)
}

func GetProSetting(ctx context.Context, namespace string) (ProSetting, bool, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return ProSetting{}, false, nil
	}
	return globalService.store.GetProSetting(ctx, namespace)
}

func SetProSetting(ctx context.Context, item ProSetting) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return globalService.store.SetProSetting(ctx, item)
}

func QueueRoutingCursorState(state RoutingCursorState) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalStateWriter != nil {
		globalStateWriter.QueueRoutingCursor(state)
	}
}

func GetRoutingCursorState(ctx context.Context, cursorKey string) (RoutingCursorState, bool, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return RoutingCursorState{}, false, nil
	}
	return globalService.store.GetRoutingCursorState(ctx, cursorKey)
}

func ListRoutingCursorStates(ctx context.Context) ([]RoutingCursorState, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return nil, nil
	}
	return globalService.store.ListRoutingCursorStates(ctx)
}

func QueueAuthRuntimeStats(item AuthRuntimeStats) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalStateWriter != nil {
		globalStateWriter.QueueAuthRuntimeStats(item)
	}
}

func GetAuthRuntimeStats(ctx context.Context, authIndex, authID string) (AuthRuntimeStats, bool, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return AuthRuntimeStats{}, false, nil
	}
	return globalService.store.GetAuthRuntimeStats(ctx, authIndex, authID)
}

func DeleteAuthRuntimeState(ctx context.Context, authID, authIndex, fileName string) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	service := globalService
	writer := globalStateWriter
	if service == nil || service.store == nil {
		return nil
	}
	if writer == nil {
		return service.store.DeleteAuthRuntimeState(ctx, authID, authIndex, fileName)
	}
	return writer.Delete(ctx, authID, authIndex, fileName)
}
