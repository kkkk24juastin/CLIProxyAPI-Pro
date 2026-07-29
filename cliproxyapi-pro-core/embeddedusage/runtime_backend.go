package embeddedusage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

type RuntimeStatePluginBackend interface {
	GetAuthRuntimeStats(context.Context, pluginapi.AuthRuntimeStatsGetRequest) (pluginapi.AuthRuntimeStatsGetResponse, bool, error)
	PutAuthRuntimeStats(context.Context, pluginapi.AuthRuntimeStatsPutRequest) (pluginapi.AuthRuntimeStatsPutResponse, bool, error)
	DeleteAuthRuntimeState(context.Context, pluginapi.AuthRuntimeStateDeleteRequest) (pluginapi.AuthRuntimeStateDeleteResponse, bool, error)
}

var runtimePluginBackendState struct {
	sync.RWMutex
	plugin     RuntimeStatePluginBackend
	queue      chan runtimePluginMutation
	stop       chan struct{}
	done       chan struct{}
	overflowMu sync.Mutex
	overflow   map[string]AuthRuntimeStats
}

var runtimePluginBackendLifecycleMu sync.Mutex

type runtimePluginMutation struct {
	stats  *AuthRuntimeStats
	delete *pluginapi.AuthRuntimeStateDeleteRequest
	done   chan error
}

func SetRuntimeStatePluginBackend(backend RuntimeStatePluginBackend) {
	runtimePluginBackendLifecycleMu.Lock()
	defer runtimePluginBackendLifecycleMu.Unlock()
	runtimePluginBackendState.Lock()
	stop := runtimePluginBackendState.stop
	done := runtimePluginBackendState.done
	if stop != nil {
		close(stop)
	}
	runtimePluginBackendState.plugin = nil
	runtimePluginBackendState.queue = nil
	runtimePluginBackendState.stop = nil
	runtimePluginBackendState.done = nil
	runtimePluginBackendState.Unlock()
	if done != nil {
		<-done
	}
	if backend == nil {
		return
	}
	runtimePluginBackendState.Lock()
	runtimePluginBackendState.plugin = backend
	runtimePluginBackendState.queue = make(chan runtimePluginMutation, 8192)
	runtimePluginBackendState.stop = make(chan struct{})
	runtimePluginBackendState.done = make(chan struct{})
	if runtimePluginBackendState.overflow == nil {
		runtimePluginBackendState.overflow = make(map[string]AuthRuntimeStats)
	}
	queue := runtimePluginBackendState.queue
	stop = runtimePluginBackendState.stop
	done = runtimePluginBackendState.done
	runtimePluginBackendState.Unlock()
	go runRuntimePluginWriter(backend, queue, stop, done)
}

func RuntimeStateBackendMode() string {
	return QuotaCacheBackendPlugin
}

func runtimeStatePluginBackend() RuntimeStatePluginBackend {
	runtimePluginBackendState.RLock()
	defer runtimePluginBackendState.RUnlock()
	return runtimePluginBackendState.plugin
}

func runRuntimePluginWriter(backend RuntimeStatePluginBackend, queue <-chan runtimePluginMutation, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	process := func(mutation runtimePluginMutation) {
		var err error
		if backend == nil {
			err = fmt.Errorf("plugin runtime state backend is not available")
		} else if mutation.stats != nil {
			_, handled, putErr := backend.PutAuthRuntimeStats(context.Background(), pluginapi.AuthRuntimeStatsPutRequest{Stats: authRuntimeStatsToPlugin(*mutation.stats)})
			err = putErr
			if err == nil && !handled {
				err = fmt.Errorf("plugin runtime state backend is not available")
			}
		} else if mutation.delete != nil {
			_, handled, deleteErr := backend.DeleteAuthRuntimeState(context.Background(), *mutation.delete)
			err = deleteErr
			if err == nil && !handled {
				err = fmt.Errorf("plugin runtime state backend is not available")
			}
		}
		if mutation.done != nil {
			mutation.done <- err
			close(mutation.done)
		} else if err != nil {
			log.WithError(err).Warn("plugin runtime state write failed")
		}
	}
	drainOverflow := func() {
		runtimePluginBackendState.overflowMu.Lock()
		overflow := runtimePluginBackendState.overflow
		runtimePluginBackendState.overflow = make(map[string]AuthRuntimeStats)
		runtimePluginBackendState.overflowMu.Unlock()
		for _, item := range overflow {
			item := item
			process(runtimePluginMutation{stats: &item})
		}
	}
	for {
		select {
		case mutation := <-queue:
			process(mutation)
		case <-ticker.C:
			drainOverflow()
		case <-stop:
			for {
				select {
				case mutation := <-queue:
					process(mutation)
				default:
					drainOverflow()
					return
				}
			}
		}
	}
}

func QueueAuthRuntimeStats(item AuthRuntimeStats) {
	if item.AuthIndex == "" || item.AuthID == "" {
		return
	}
	if item.UpdatedAtMS <= 0 {
		item.UpdatedAtMS = time.Now().UnixMilli()
	}
	if legacyCompatibilityServiceAvailable() {
		queueLegacyAuthRuntimeStats(item)
		return
	}
	runtimePluginBackendState.RLock()
	queue := runtimePluginBackendState.queue
	if queue == nil {
		runtimePluginBackendState.RUnlock()
		log.Warn("plugin runtime state backend is not configured")
		return
	}
	select {
	case queue <- runtimePluginMutation{stats: &item}:
		runtimePluginBackendState.RUnlock()
	default:
		runtimePluginBackendState.overflowMu.Lock()
		current, ok := runtimePluginBackendState.overflow[item.AuthIndex]
		if !ok || item.UpdatedAtMS >= current.UpdatedAtMS {
			runtimePluginBackendState.overflow[item.AuthIndex] = item
		}
		runtimePluginBackendState.overflowMu.Unlock()
		runtimePluginBackendState.RUnlock()
	}
}

func GetAuthRuntimeStats(ctx context.Context, authIndex, authID string) (AuthRuntimeStats, bool, error) {
	if legacyCompatibilityServiceAvailable() {
		return getLegacyAuthRuntimeStats(ctx, authIndex, authID)
	}
	backend := runtimeStatePluginBackend()
	if backend == nil {
		return AuthRuntimeStats{}, false, fmt.Errorf("plugin runtime state backend is not available")
	}
	resp, handled, err := backend.GetAuthRuntimeStats(ctx, pluginapi.AuthRuntimeStatsGetRequest{AuthIndex: authIndex, AuthID: authID})
	if err != nil {
		return AuthRuntimeStats{}, false, err
	}
	if !handled {
		return AuthRuntimeStats{}, false, fmt.Errorf("plugin runtime state backend is not available")
	}
	return authRuntimeStatsFromPlugin(resp.Stats), resp.Found, nil
}

func DeleteAuthRuntimeState(ctx context.Context, authID, authIndex, fileName string) error {
	if legacyCompatibilityServiceAvailable() {
		return deleteLegacyAuthRuntimeState(ctx, authID, authIndex, fileName)
	}
	runtimePluginBackendState.RLock()
	queue := runtimePluginBackendState.queue
	if queue == nil {
		runtimePluginBackendState.RUnlock()
		return fmt.Errorf("plugin runtime state backend is not configured")
	}
	runtimePluginBackendState.overflowMu.Lock()
	for key, item := range runtimePluginBackendState.overflow {
		if (authIndex != "" && item.AuthIndex == authIndex) ||
			(authID != "" && item.AuthID == authID) ||
			(fileName != "" && item.FileName == fileName) {
			delete(runtimePluginBackendState.overflow, key)
		}
	}
	runtimePluginBackendState.overflowMu.Unlock()
	done := make(chan error, 1)
	request := &pluginapi.AuthRuntimeStateDeleteRequest{AuthID: authID, AuthIndex: authIndex, FileName: fileName}
	select {
	case queue <- runtimePluginMutation{delete: request, done: done}:
		runtimePluginBackendState.RUnlock()
	case <-ctx.Done():
		runtimePluginBackendState.RUnlock()
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func queueLegacyAuthRuntimeStats(item AuthRuntimeStats) {
	enqueueRuntimeState(runtimeStateMutation{stats: &item})
}

func getLegacyAuthRuntimeStats(ctx context.Context, authIndex, authID string) (AuthRuntimeStats, bool, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return AuthRuntimeStats{}, false, nil
	}
	return globalService.store.GetAuthRuntimeStats(ctx, authIndex, authID)
}

func deleteLegacyAuthRuntimeState(ctx context.Context, authID, authIndex, fileName string) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	service := globalService
	queue := globalStateQueue
	if service == nil || service.store == nil {
		return nil
	}
	if queue == nil {
		return service.store.DeleteAuthRuntimeState(ctx, authID, authIndex, fileName)
	}
	deletion := &runtimeStateDelete{authID: authID, authIndex: authIndex, fileName: fileName, updatedAt: time.Now().UnixMilli(), done: make(chan error, 1)}
	select {
	case queue <- runtimeStateMutation{delete: deletion}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-deletion.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func authRuntimeStatsToPlugin(item AuthRuntimeStats) pluginapi.AuthRuntimeStats {
	out := pluginapi.AuthRuntimeStats{
		AuthIndex: item.AuthIndex, AuthID: item.AuthID, FileName: item.FileName,
		IdentityFingerprint: item.IdentityFingerprint, SelectedCount: item.SelectedCount,
		SuccessCount: item.SuccessCount, FailureCount: item.FailureCount, UpdatedAtMS: item.UpdatedAtMS,
		RecentBuckets: make([]pluginapi.RuntimeRequestBucket, 0, len(item.RecentBuckets)),
	}
	for _, bucket := range item.RecentBuckets {
		out.RecentBuckets = append(out.RecentBuckets, pluginapi.RuntimeRequestBucket{BucketID: bucket.BucketID, Success: bucket.Success, Failed: bucket.Failed})
	}
	return out
}

func authRuntimeStatsFromPlugin(item pluginapi.AuthRuntimeStats) AuthRuntimeStats {
	out := AuthRuntimeStats{
		AuthIndex: item.AuthIndex, AuthID: item.AuthID, FileName: item.FileName,
		IdentityFingerprint: item.IdentityFingerprint, SelectedCount: item.SelectedCount,
		SuccessCount: item.SuccessCount, FailureCount: item.FailureCount, UpdatedAtMS: item.UpdatedAtMS,
		RecentBuckets: make([]RuntimeRequestBucket, 0, len(item.RecentBuckets)),
	}
	for _, bucket := range item.RecentBuckets {
		out.RecentBuckets = append(out.RecentBuckets, RuntimeRequestBucket{BucketID: bucket.BucketID, Success: bucket.Success, Failed: bucket.Failed})
	}
	return out
}
