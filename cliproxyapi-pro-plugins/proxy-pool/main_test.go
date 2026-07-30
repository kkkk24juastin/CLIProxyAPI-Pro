package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestPluginRegistrationTitle(t *testing.T) {
	if got := pluginRegistration().Metadata.Name; got != "Pro Proxy Pool" {
		t.Fatalf("plugin title = %q, want %q", got, "Pro Proxy Pool")
	}
}

func TestManagementRegistrationIncludesResourcePage(t *testing.T) {
	raw, err := handleMethod(pluginabi.MethodManagementRegister, nil)
	if err != nil {
		t.Fatalf("handle management registration: %v", err)
	}
	var response envelope
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var registration managementRegistration
	if err = json.Unmarshal(response.Result, &registration); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if len(registration.Resources) != 1 || registration.Resources[0].Path != "/ui" {
		t.Fatalf("resources = %#v, want /ui", registration.Resources)
	}
}

func TestManagementResourcePageUsesAuthenticatedParentBridge(t *testing.T) {
	rawRequest, err := json.Marshal(managementRequest{Method: http.MethodGet, Path: "/v0/resource/plugins/proxy-pool/ui"})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	raw, err := handleMethod(pluginabi.MethodManagementHandle, rawRequest)
	if err != nil {
		t.Fatalf("handle resource page: %v", err)
	}
	var responseEnvelope envelope
	if err = json.Unmarshal(raw, &responseEnvelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var response pluginapi.ManagementResponse
	if err = json.Unmarshal(responseEnvelope.Result, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	body := string(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Headers.Get("Content-Type"), "text/html") {
		t.Fatalf("response = %#v", response)
	}
	for _, marker := range []string{"cliproxy-plugin-resource", "proxy_pool.title", "/pro/proxy-pool/status"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("resource page missing marker %q", marker)
		}
	}
}
