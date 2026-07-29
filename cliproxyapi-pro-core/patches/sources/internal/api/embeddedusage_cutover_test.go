package api

import "testing"

func TestPluginUsageOwnsManagementRoutesAndCoreKeepsOnlyStreamBridge(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "test-management-key")
	server := newTestServer(t)
	routes := server.registeredManagementRouteKeys()
	if _, exists := routes["GET /v0/management/usage/status"]; exists {
		t.Fatal("Core still reserves the plugin-owned usage status route")
	}
	if _, exists := routes["GET /v0/management/usage/stream"]; !exists {
		t.Fatal("Core SSE transport bridge is missing")
	}
	if _, exists := routes["GET /v0/management/config"]; !exists {
		t.Fatal("unrelated management routes were not registered")
	}
}
