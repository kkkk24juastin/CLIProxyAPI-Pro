package pluginhost

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type readyUsagePlugin struct{}

func (readyUsagePlugin) HandleUsage(context.Context, pluginapi.UsageRecord) {}

type readyManagementAPI struct{}

func (readyManagementAPI) RegisterManagement(context.Context, pluginapi.ManagementRegistrationRequest) (pluginapi.ManagementRegistrationResponse, error) {
	return pluginapi.ManagementRegistrationResponse{}, nil
}

type readyScheduler struct{}

func (readyScheduler) Pick(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
	return pluginapi.SchedulerPickResponse{}, nil
}

func TestProObservabilityReadyRequiresCompleteCapabilitySet(t *testing.T) {
	host := New()
	store := &testProSettingsStore{}
	caps := pluginapi.Capabilities{
		UsagePlugin: readyUsagePlugin{}, ManagementAPI: readyManagementAPI{}, Scheduler: readyScheduler{},
		QuotaCacheStore: &testQuotaCacheStore{}, RuntimeStateStore: &testRuntimeStateStore{}, ProSettingsStore: store,
	}
	setHostSnapshotForTest(host, true, capabilityRecord{id: "pro-observability", plugin: pluginapi.Plugin{
		Metadata:     pluginapi.Metadata{Name: "observability", Version: "1", Author: "test", GitHubRepository: "https://example.com"},
		Capabilities: caps,
	}})
	if !host.ProObservabilityReady("pro-observability") {
		t.Fatal("complete plugin was not ready")
	}
	caps.ProSettingsStore = nil
	setHostSnapshotForTest(host, true, capabilityRecord{id: "pro-observability", plugin: pluginapi.Plugin{
		Metadata:     pluginapi.Metadata{Name: "observability", Version: "1", Author: "test", GitHubRepository: "https://example.com"},
		Capabilities: caps,
	}})
	if host.ProObservabilityReady("pro-observability") {
		t.Fatal("partial plugin unexpectedly passed readiness gate")
	}
}
