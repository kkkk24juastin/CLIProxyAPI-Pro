package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementRouteCallerFunc func(context.Context, pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error)

func (f managementRouteCallerFunc) CallManagement(ctx context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error) {
	return f(ctx, req)
}

func TestPollPluginUsageEventsForwardsCursor(t *testing.T) {
	caller := managementRouteCallerFunc(func(_ context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error) {
		if req.Method != http.MethodGet || req.Path != "/v0/management/usage/events" || req.Query.Get("after_id") != "41" {
			t.Fatalf("request = %#v", req)
		}
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Body: []byte(`{"latest_id":42,"generation":7}`)}, true, nil
	})
	body, cursor, err := pollPluginUsageEvents(context.Background(), caller, 41)
	if err != nil || cursor.LatestID != 42 || cursor.Generation != 7 || len(body) == 0 {
		t.Fatalf("pollPluginUsageEvents() cursor=%#v body=%s err=%v", cursor, body, err)
	}
}

func TestPollPluginUsageEventsRequiresCompatibilityRoute(t *testing.T) {
	caller := managementRouteCallerFunc(func(context.Context, pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error) {
		return pluginapi.ManagementResponse{}, false, nil
	})
	if _, _, err := pollPluginUsageEvents(context.Background(), caller, 0); err == nil {
		t.Fatal("pollPluginUsageEvents() error = nil")
	}
}
