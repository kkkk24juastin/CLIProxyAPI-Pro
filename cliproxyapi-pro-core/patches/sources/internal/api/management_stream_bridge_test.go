package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementEventCallerStub struct{ request pluginapi.ManagementRequest }

func (s *managementEventCallerStub) CallManagement(_ context.Context, request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, bool, error) {
	s.request = request
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Body: []byte(`{"sequence":7,"messages":[{"type":"status"}]}`)}, true, nil
}

func TestPollPluginManagementEvents(t *testing.T) {
	caller := &managementEventCallerStub{}
	page, err := pollPluginManagementEvents(context.Background(), caller, "/v0/management/account-inspection/events", 5)
	if err != nil {
		t.Fatalf("poll events: %v", err)
	}
	if page.Sequence != 7 || len(page.Messages) != 1 {
		t.Fatalf("page = %#v", page)
	}
	if caller.request.Path != "/v0/management/account-inspection/events" || caller.request.Query.Get("after_sequence") != "5" {
		t.Fatalf("request = %#v", caller.request)
	}
}
