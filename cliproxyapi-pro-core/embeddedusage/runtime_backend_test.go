package embeddedusage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
)

type runtimeStateBackendStub struct {
	mu    sync.Mutex
	stats pluginapi.AuthRuntimeStats
}

func (s *runtimeStateBackendStub) GetAuthRuntimeStats(context.Context, pluginapi.AuthRuntimeStatsGetRequest) (pluginapi.AuthRuntimeStatsGetResponse, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return pluginapi.AuthRuntimeStatsGetResponse{Found: s.stats.AuthIndex != "", Stats: s.stats}, true, nil
}

func (s *runtimeStateBackendStub) PutAuthRuntimeStats(_ context.Context, req pluginapi.AuthRuntimeStatsPutRequest) (pluginapi.AuthRuntimeStatsPutResponse, bool, error) {
	s.mu.Lock()
	s.stats = req.Stats
	s.mu.Unlock()
	return pluginapi.AuthRuntimeStatsPutResponse{}, true, nil
}

func (s *runtimeStateBackendStub) DeleteAuthRuntimeState(context.Context, pluginapi.AuthRuntimeStateDeleteRequest) (pluginapi.AuthRuntimeStateDeleteResponse, bool, error) {
	s.mu.Lock()
	s.stats = pluginapi.AuthRuntimeStats{}
	s.mu.Unlock()
	return pluginapi.AuthRuntimeStateDeleteResponse{}, true, nil
}

func TestPluginRuntimeStateBackendRestoresAndDeletesWithoutLegacyService(t *testing.T) {
	SetDefaultService(nil)
	stub := &runtimeStateBackendStub{}
	SetRuntimeStatePluginBackend(stub)
	t.Cleanup(func() { SetRuntimeStatePluginBackend(nil) })
	if mode := RuntimeStateBackendMode(); mode != QuotaCacheBackendPlugin {
		t.Fatalf("RuntimeStateBackendMode() = %q, want plugin", mode)
	}
	QueueAuthRuntimeStats(AuthRuntimeStats{AuthIndex: "idx", AuthID: "auth", SuccessCount: 9})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stats, found, err := GetAuthRuntimeStats(context.Background(), "idx", "auth")
		if err == nil && found && stats.SuccessCount == 9 {
			if err = DeleteAuthRuntimeState(context.Background(), "auth", "idx", "a.json"); err != nil {
				t.Fatalf("DeleteAuthRuntimeState() error = %v", err)
			}
			_, found, err = GetAuthRuntimeStats(context.Background(), "idx", "auth")
			if err != nil || found {
				t.Fatalf("GetAuthRuntimeStats(after delete) found=%v err=%v", found, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued runtime stats did not reach plugin backend")
}

func TestPluginRuntimeStateBackendFlushesBeforeDetach(t *testing.T) {
	SetDefaultService(nil)
	stub := &runtimeStateBackendStub{}
	SetRuntimeStatePluginBackend(stub)
	for updatedAt := int64(1); updatedAt <= 100; updatedAt++ {
		QueueAuthRuntimeStats(AuthRuntimeStats{
			AuthIndex: "idx", AuthID: "auth", SuccessCount: updatedAt, UpdatedAtMS: updatedAt,
		})
	}
	SetRuntimeStatePluginBackend(nil)
	stub.mu.Lock()
	got := stub.stats
	stub.mu.Unlock()
	if got.SuccessCount != 100 || got.UpdatedAtMS != 100 {
		t.Fatalf("flushed stats = %#v, want latest snapshot", got)
	}
	runtimePluginBackendState.RLock()
	queue, backend := runtimePluginBackendState.queue, runtimePluginBackendState.plugin
	runtimePluginBackendState.RUnlock()
	if queue != nil || backend != nil {
		t.Fatalf("backend remained attached after flush: queue=%v backend=%T", queue != nil, backend)
	}
}
