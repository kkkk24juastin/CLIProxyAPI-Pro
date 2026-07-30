package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	page, err := pollPluginManagementEvents(context.Background(), caller, "/v0/management/account-inspection/events", url.Values{"details": {"1"}}, 5)
	if err != nil {
		t.Fatalf("poll events: %v", err)
	}
	if page.Sequence != 7 || len(page.Messages) != 1 {
		t.Fatalf("page = %#v", page)
	}
	if caller.request.Path != "/v0/management/account-inspection/events" || caller.request.Query.Get("after_sequence") != "5" || caller.request.Query.Get("details") != "1" {
		t.Fatalf("request = %#v", caller.request)
	}
}

func TestPluginManagementWebSocketEchoesManagementProtocol(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v0/management/account-inspection/logs", nil)
	request.Header.Set("Sec-WebSocket-Protocol", "unrelated, cpa-management.secret-value")
	header := pluginManagementWebSocketResponseHeader(request)
	if got := header.Get("Sec-WebSocket-Protocol"); got != "cpa-management.secret-value" {
		t.Fatalf("response protocol = %q", got)
	}
}

func TestPluginManagementEventQueryDoesNotForwardCredentials(t *testing.T) {
	query := sanitizedPluginManagementEventQuery(url.Values{
		"details": {"1"}, "result_page": {"2"}, "key": {"management-secret"}, "access_token": {"token-secret"},
	})
	if query.Get("details") != "1" || query.Get("result_page") != "2" {
		t.Fatalf("inspection filters were lost: %#v", query)
	}
	if query.Get("key") != "" || query.Get("access_token") != "" {
		t.Fatalf("credential query leaked to plugin: %#v", query)
	}
}
