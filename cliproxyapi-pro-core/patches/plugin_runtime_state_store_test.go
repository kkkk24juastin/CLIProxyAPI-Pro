package pluginhost

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type testRuntimeStateStore struct{ stats pluginapi.AuthRuntimeStats }

func (s *testRuntimeStateStore) GetAuthRuntimeStats(context.Context, pluginapi.AuthRuntimeStatsGetRequest) (pluginapi.AuthRuntimeStatsGetResponse, error) {
	return pluginapi.AuthRuntimeStatsGetResponse{Found: s.stats.AuthIndex != "", Stats: s.stats}, nil
}

func (s *testRuntimeStateStore) PutAuthRuntimeStats(_ context.Context, req pluginapi.AuthRuntimeStatsPutRequest) (pluginapi.AuthRuntimeStatsPutResponse, error) {
	s.stats = req.Stats
	return pluginapi.AuthRuntimeStatsPutResponse{}, nil
}

func (s *testRuntimeStateStore) DeleteAuthRuntimeState(context.Context, pluginapi.AuthRuntimeStateDeleteRequest) (pluginapi.AuthRuntimeStateDeleteResponse, error) {
	s.stats = pluginapi.AuthRuntimeStats{}
	return pluginapi.AuthRuntimeStateDeleteResponse{}, nil
}

func TestRuntimeStateStoreFacadeRestoresAndDeletesAuthStats(t *testing.T) {
	store := &testRuntimeStateStore{}
	host := New()
	setHostSnapshotForTest(host, true, capabilityRecord{id: "observability", plugin: pluginapi.Plugin{
		Metadata:     pluginapi.Metadata{Name: "observability", Version: "1", Author: "test", GitHubRepository: "https://example.com"},
		Capabilities: pluginapi.Capabilities{RuntimeStateStore: store},
	}})
	stats := pluginapi.AuthRuntimeStats{AuthIndex: "idx", AuthID: "auth", SuccessCount: 7}
	if _, handled, err := host.PutAuthRuntimeStats(context.Background(), pluginapi.AuthRuntimeStatsPutRequest{Stats: stats}); err != nil || !handled {
		t.Fatalf("PutAuthRuntimeStats() handled=%v err=%v", handled, err)
	}
	got, handled, err := host.GetAuthRuntimeStats(context.Background(), pluginapi.AuthRuntimeStatsGetRequest{AuthIndex: "idx"})
	if err != nil || !handled || !got.Found || got.Stats.SuccessCount != 7 {
		t.Fatalf("GetAuthRuntimeStats() = %#v handled=%v err=%v", got, handled, err)
	}
	if _, handled, err = host.DeleteAuthRuntimeState(context.Background(), pluginapi.AuthRuntimeStateDeleteRequest{AuthIndex: "idx"}); err != nil || !handled {
		t.Fatalf("DeleteAuthRuntimeState() handled=%v err=%v", handled, err)
	}
}
