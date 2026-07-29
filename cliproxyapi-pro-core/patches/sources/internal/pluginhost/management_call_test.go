package pluginhost

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestCallManagementInvokesRegisteredBoundedRoute(t *testing.T) {
	host := New()
	record := normalizeTestCapabilityRecord(capabilityRecord{id: "observability"})
	setHostSnapshotForTest(host, true, record)
	host.managementRoutes[managementRouteKey(http.MethodGet, "/v0/management/usage/events")] = managementRouteRecord{
		pluginID: record.id,
		path:     record.path,
		version:  record.version,
		route: pluginapi.ManagementRoute{Handler: managementHandlerFunc(func(context.Context, pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Body: []byte(`{"latest_id":7}`)}, nil
		})},
	}
	resp, handled, err := host.CallManagement(context.Background(), pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/usage/events"})
	if err != nil || !handled || resp.StatusCode != http.StatusOK || string(resp.Body) != `{"latest_id":7}` {
		t.Fatalf("CallManagement() = %#v handled=%v err=%v", resp, handled, err)
	}
}
