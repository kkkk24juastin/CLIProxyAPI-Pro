package pluginhost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func inspectionCallbackTestHost(t *testing.T) (*Host, *coreauth.Auth) {
	t.Helper()
	authDir := t.TempDir()
	path := filepath.Join(authDir, "codex-inspection.json")
	if err := os.WriteFile(path, []byte(`{"type":"codex","email":"inspect@example.com"}`), 0o600); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	auth := &coreauth.Auth{
		ID: "codex-inspection", Provider: "codex", FileName: filepath.Base(path),
		Status: coreauth.StatusActive, UpdatedAt: time.Now().UTC(),
		Attributes: map[string]string{"path": path, "source": path},
		Metadata: map[string]any{
			"email": "inspect@example.com", "name": "Inspection Account",
			"account_id": "account-1", "plan_type": "plus",
		},
	}
	auth.EnsureIndex()
	host := New()
	host.runtimeConfig = &config.Config{AuthDir: authDir}
	manager := coreauth.NewManager(nil, nil, nil)
	host.SetAuthManager(manager)
	registered, err := manager.Register(context.Background(), auth)
	if err != nil {
		t.Fatalf("register auth: %v", err)
	}
	return host, registered
}

func proObservabilityCallbackContext() context.Context {
	return withHostCallbackPluginID(context.Background(), proObservabilityPluginID)
}

func TestAccountInspectionCallbacksRejectOtherPlugins(t *testing.T) {
	host, _ := inspectionCallbackTestHost(t)
	if _, err := host.callFromPlugin(withHostCallbackPluginID(context.Background(), "other"), pluginabi.MethodHostAuthInspectionList, nil); err == nil {
		t.Fatal("inspection auth callback accepted an unrelated plugin")
	}
}

func TestAccountInspectionListReturnsSafeRuntimeMetadata(t *testing.T) {
	host, auth := inspectionCallbackTestHost(t)
	auth.Metadata["access_token"] = "secret-access-token"
	auth.Metadata["id_token"] = "secret-id-token"
	raw, err := host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthInspectionList, nil)
	if err != nil {
		t.Fatalf("inspection list: %v", err)
	}
	response, err := decodeRPCEnvelope[pluginapi.HostAuthInspectionListResponse](raw)
	if err != nil {
		t.Fatalf("decode inspection list: %v", err)
	}
	if len(response.Auths) != 1 {
		t.Fatalf("auths = %#v", response.Auths)
	}
	entry := response.Auths[0]
	if entry.AuthIndex != auth.Index || entry.Email != "inspect@example.com" || entry.DisplayName != "Inspection Account" || entry.AccountID != "account-1" || entry.PlanType != "plus" {
		t.Fatalf("entry = %#v", entry)
	}
	encoded, _ := json.Marshal(entry)
	if string(encoded) == "" || json.Valid(encoded) == false {
		t.Fatalf("invalid entry JSON: %s", encoded)
	}
	if strings.Contains(string(encoded), "secret-access-token") || strings.Contains(string(encoded), "secret-id-token") {
		t.Fatalf("inspection entry leaked credential material: %s", encoded)
	}
}

func TestAccountInspectionHealthPatchOwnsAndClearsError(t *testing.T) {
	host, auth := inspectionCallbackTestHost(t)
	request := pluginapi.HostAuthHealthPatchRequest{
		AuthIndex: auth.Index, ExpectedRevision: authRevision(auth),
		Error: &pluginapi.HostAuthHealthError{Code: "inspection_http_error", Message: "HTTP 401", HTTPStatus: 401},
	}
	rawRequest, _ := json.Marshal(request)
	if _, err := host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthHealthPatch, rawRequest); err != nil {
		t.Fatalf("set inspection error: %v", err)
	}
	current, err := host.authByIndex(auth.Index)
	if err != nil {
		t.Fatalf("current auth: %v", err)
	}
	if current.Status != coreauth.StatusError || !current.Unavailable || inspectionErrorSource(current) != "account_inspection" {
		t.Fatalf("patched auth = %#v", current)
	}
	clearRequest, _ := json.Marshal(pluginapi.HostAuthHealthPatchRequest{AuthIndex: auth.Index, ClearError: true})
	if _, err = host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthHealthPatch, clearRequest); err != nil {
		t.Fatalf("clear inspection error: %v", err)
	}
	current, _ = host.authByIndex(auth.Index)
	if current.Status != coreauth.StatusActive || current.Unavailable || inspectionErrorSource(current) != "" {
		t.Fatalf("cleared auth = %#v", current)
	}
}

func TestAccountInspectionHealthPatchRejectsStaleRevision(t *testing.T) {
	host, auth := inspectionCallbackTestHost(t)
	disabled := true
	rawRequest, _ := json.Marshal(pluginapi.HostAuthHealthPatchRequest{
		AuthIndex: auth.Index, ExpectedRevision: authRevision(auth) + 1, Disabled: &disabled,
	})
	if _, err := host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthHealthPatch, rawRequest); err == nil {
		t.Fatal("health patch accepted a stale auth revision")
	}
}

func TestAccountInspectionEnablePreservesNonInspectionError(t *testing.T) {
	host, auth := inspectionCallbackTestHost(t)
	auth.LastError = &coreauth.Error{Code: "provider_failure", Message: "provider unavailable", HTTPStatus: 503}
	auth.Status = coreauth.StatusError
	auth.StatusMessage = "provider unavailable"
	auth.Unavailable = true
	auth.Metadata["last_error"] = map[string]any{"source": "provider", "message": "provider unavailable"}
	if _, err := host.currentAuthManager().Update(context.Background(), auth); err != nil {
		t.Fatalf("seed non-inspection error: %v", err)
	}
	current, err := host.authByIndex(auth.Index)
	if err != nil {
		t.Fatalf("current auth: %v", err)
	}
	disabled := false
	rawRequest, _ := json.Marshal(pluginapi.HostAuthHealthPatchRequest{AuthIndex: auth.Index, ExpectedRevision: authRevision(current), Disabled: &disabled, ClearError: true})
	if _, err = host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthHealthPatch, rawRequest); err != nil {
		t.Fatalf("enable auth: %v", err)
	}
	current, _ = host.authByIndex(auth.Index)
	if current.Status != coreauth.StatusError || !current.Unavailable || current.LastError == nil || inspectionErrorSource(current) != "provider" {
		t.Fatalf("non-inspection error was changed: %#v", current)
	}
}
