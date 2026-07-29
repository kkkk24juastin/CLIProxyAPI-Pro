package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestLifecycleAndAuthModelFilterEnvelope(t *testing.T) {
	configRaw, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte(`
providers:
  xai:
    plans:
      free:
        excluded-models: ["grok-pro-*"]
`)})
	registeredRaw, errRegister := configurePlugin(configRaw)
	if errRegister != nil {
		t.Fatalf("configurePlugin() error = %v", errRegister)
	}
	registeredEnvelope := envelope{}
	if errUnmarshal := json.Unmarshal(registeredRaw, &registeredEnvelope); errUnmarshal != nil {
		t.Fatalf("decode registration envelope: %v", errUnmarshal)
	}
	registered := registration{}
	if !registeredEnvelope.OK || json.Unmarshal(registeredEnvelope.Result, &registered) != nil {
		t.Fatalf("registration envelope = %s", registeredRaw)
	}
	if !registered.Capabilities.AuthModelFilter || !registered.Capabilities.ManagementAPI || registered.Metadata.Version != pluginVersion {
		t.Fatalf("registration = %#v", registered)
	}

	requestRaw, _ := json.Marshal(authModelFilterRequest{
		AuthID:       "xai-free",
		AuthProvider: "xai",
		AuthKind:     "oauth",
		Attributes:   map[string]string{"plan_type": "free"},
		Models:       []pluginapi.ModelInfo{{ID: "grok-pro-1"}, {ID: "grok-basic"}},
	})
	responseRaw, errFilter := filterAuthModels(requestRaw)
	if errFilter != nil {
		t.Fatalf("filterAuthModels() error = %v", errFilter)
	}
	responseEnvelope := envelope{}
	if errUnmarshal := json.Unmarshal(responseRaw, &responseEnvelope); errUnmarshal != nil {
		t.Fatalf("decode filter envelope: %v", errUnmarshal)
	}
	response := authModelFilterResponse{}
	if !responseEnvelope.OK || json.Unmarshal(responseEnvelope.Result, &response) != nil {
		t.Fatalf("filter envelope = %s", responseRaw)
	}
	if !response.Handled || len(response.ExcludedModelIDs) != 1 || response.ExcludedModelIDs[0] != "grok-pro-1" {
		t.Fatalf("filter response = %#v", response)
	}
}

func TestManagementResourceRegistrationAndPage(t *testing.T) {
	registeredRaw, errRegister := handleMethod(pluginabi.MethodManagementRegister, nil)
	if errRegister != nil {
		t.Fatalf("management register error = %v", errRegister)
	}
	registeredEnvelope := envelope{}
	if errUnmarshal := json.Unmarshal(registeredRaw, &registeredEnvelope); errUnmarshal != nil {
		t.Fatalf("decode management registration envelope: %v", errUnmarshal)
	}
	registered := managementRegistration{}
	if !registeredEnvelope.OK || json.Unmarshal(registeredEnvelope.Result, &registered) != nil {
		t.Fatalf("management registration envelope = %s", registeredRaw)
	}
	if len(registered.Resources) != 1 || registered.Resources[0].Path != "/ui" || registered.Resources[0].Menu == "" {
		t.Fatalf("management resources = %#v", registered.Resources)
	}

	requestRaw, _ := json.Marshal(managementRequest{Method: http.MethodGet, Path: "/v0/resource/plugins/oauth-model-policy/ui"})
	pageRaw, errPage := handleMethod(pluginabi.MethodManagementHandle, requestRaw)
	if errPage != nil {
		t.Fatalf("management handle error = %v", errPage)
	}
	pageEnvelope := envelope{}
	if errUnmarshal := json.Unmarshal(pageRaw, &pageEnvelope); errUnmarshal != nil {
		t.Fatalf("decode management page envelope: %v", errUnmarshal)
	}
	page := pluginapi.ManagementResponse{}
	if !pageEnvelope.OK || json.Unmarshal(pageEnvelope.Result, &page) != nil {
		t.Fatalf("management page envelope = %s", pageRaw)
	}
	if page.StatusCode != http.StatusOK || !strings.Contains(string(page.Body), "cliproxy-plugin-resource") || !strings.Contains(string(page.Body), "excluded-models") {
		t.Fatalf("management page status=%d body=%q", page.StatusCode, page.Body)
	}
}
