package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeEnvelopeResult[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var response envelope
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Error != nil {
		t.Fatalf("envelope = %s", raw)
	}
	var result T
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func lifecyclePayload(t *testing.T, config string) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(config), SchemaVersion: pluginSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParsePluginConfigAcceptsHostScopedAndFullYAML(t *testing.T) {
	for name, raw := range map[string]string{
		"host scoped":   "enabled: true\npriority: 100\nwrite-enabled: true\ndatabase-path: /tmp/scoped.sqlite\n",
		"full document": "plugins:\n  configs:\n    pro-observability:\n      write-enabled: true\n      database-path: /tmp/full.sqlite\n",
	} {
		t.Run(name, func(t *testing.T) {
			config, err := parsePluginConfig([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if !config.WriteEnabled || !strings.HasPrefix(config.DatabasePath, "/tmp/") {
				t.Fatalf("config = %#v", config)
			}
		})
	}
}

func TestPluginDefaultsToDisabledWriterAndRegistersNoResourceUI(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
	config := "enabled: true\npriority: 100\ndatabase-path: " + databasePath + "\n"
	raw, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config))
	if err != nil {
		t.Fatal(err)
	}
	registered := decodeEnvelopeResult[registration](t, raw)
	if !registered.Capabilities.UsagePlugin || !registered.Capabilities.ManagementAPI || registered.SchemaVersion != pluginSchemaVersion {
		t.Fatalf("registration = %#v", registered)
	}
	if _, err := dispatchMethod(methodUsageHandle, testUsageRecord(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("disabled writer created database: %v", err)
	}
	raw, err = dispatchMethod(methodManagementRegister, nil)
	if err != nil {
		t.Fatal(err)
	}
	management := decodeEnvelopeResult[managementRegistration](t, raw)
	if len(management.Routes) != 1 || management.Routes[0].Method != http.MethodGet || management.Routes[0].Path != managementStatusPath {
		t.Fatalf("management registration = %#v", management)
	}
	if strings.Contains(string(raw), "resources") || strings.Contains(string(raw), "menu") {
		t.Fatalf("plugin unexpectedly registered UI resources: %s", raw)
	}
}

func TestPluginOptInWriterAndAuthenticatedStatus(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
	config := "enabled: true\npriority: 100\nwrite-enabled: true\ndatabase-path: " + databasePath + "\n"
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config)); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchMethod(methodUsageHandle, testUsageRecord(t)); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(managementRequest{Method: http.MethodGet, Path: "/v0/management" + managementStatusPath})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := dispatchMethod(methodManagementHandle, request)
	if err != nil {
		t.Fatal(err)
	}
	response := decodeEnvelopeResult[managementResponse](t, raw)
	if response.StatusCode != http.StatusOK || response.Headers.Get("Content-Type") != "application/json" {
		t.Fatalf("management response = %#v", response)
	}
	var status runtimeStatus
	if err := json.Unmarshal(response.Body, &status); err != nil {
		t.Fatal(err)
	}
	if !status.WriteEnabled || !status.StoreOpen || status.MigrationMode != "opt-in-writer" || status.Summary == nil || status.Summary.TotalRequests != 1 {
		t.Fatalf("runtime status = %#v", status)
	}
}

func TestInvalidReconfigureKeepsCurrentWriter(t *testing.T) {
	shutdownRuntime()
	t.Cleanup(shutdownRuntime)
	databasePath := filepath.Join(t.TempDir(), "usage.sqlite")
	config := "enabled: true\npriority: 100\nwrite-enabled: true\ndatabase-path: " + databasePath + "\n"
	if _, err := dispatchMethod(methodPluginRegister, lifecyclePayload(t, config)); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchMethod(methodPluginReconfigure, lifecyclePayload(t, "plugins: [invalid")); err == nil {
		t.Fatal("invalid reconfigure error = nil")
	}
	if _, err := dispatchMethod(methodUsageHandle, testUsageRecord(t)); err != nil {
		t.Fatal(err)
	}
	status := activeRuntime.status(t.Context())
	if !status.WriteEnabled || !status.StoreOpen || status.Summary == nil || status.Summary.TotalRequests != 1 {
		t.Fatalf("runtime after invalid reconfigure = %#v", status)
	}
}
