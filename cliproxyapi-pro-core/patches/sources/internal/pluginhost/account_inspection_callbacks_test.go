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
	host.runtimeConfig = &config.Config{
		AuthDir:             authDir,
		CodexHeaderDefaults: config.CodexHeaderDefaults{UserAgent: "custom-codex-agent/1.0"},
	}
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
	auth.Label = "safe label"
	auth.Metadata["access_token"] = "secret-access-token"
	auth.Metadata["id_token"] = "secret-id-token"
	auth.Metadata["note"] = "private operator note"
	auth.Metadata["base_url"] = "https://user:base-secret@example.com/v1?token=query-secret#fragment"
	auth.Attributes["base_url"] = "https://user:base-secret@example.com/v1?token=query-secret#fragment"
	auth.Metadata["last_error"] = map[string]any{
		"source": "account_inspection", "code": "inspection_http_error", "message": "HTTP 401", "http_status": 401,
	}
	auth.LastError = &coreauth.Error{Code: "inspection_http_error", Message: "HTTP 401", HTTPStatus: 401}
	var err error
	auth, err = host.currentAuthManager().Update(context.Background(), auth)
	if err != nil {
		t.Fatalf("update inspection auth fixture: %v", err)
	}
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
	if entry.InspectionUserAgent != "custom-codex-agent/1.0" {
		t.Fatalf("inspection user agent = %q", entry.InspectionUserAgent)
	}
	if entry.InspectionMetadata["base_url"] != "https://example.com/v1" || entry.InspectionAttributes["base_url"] != "https://example.com/v1" {
		t.Fatalf("sanitized base URLs = %#v / %#v", entry.InspectionMetadata, entry.InspectionAttributes)
	}
	if entry.InspectionError == nil || entry.InspectionError.Code != "inspection_http_error" || entry.InspectionError.HTTPStatus != 401 {
		t.Fatalf("inspection error = %#v", entry.InspectionError)
	}
	encoded, _ := json.Marshal(entry)
	if string(encoded) == "" || json.Valid(encoded) == false {
		t.Fatalf("invalid entry JSON: %s", encoded)
	}
	if strings.Contains(string(encoded), "secret-access-token") || strings.Contains(string(encoded), "secret-id-token") {
		t.Fatalf("inspection entry leaked credential material: %s", encoded)
	}
	if strings.Contains(string(encoded), auth.Attributes["path"]) || strings.Contains(string(encoded), "private operator note") ||
		strings.Contains(string(encoded), "base-secret") || strings.Contains(string(encoded), "query-secret") {
		t.Fatalf("inspection entry leaked host-only path or note: %s", encoded)
	}
}

func TestAccountInspectionListDoesNotProjectForeignErrors(t *testing.T) {
	host, auth := inspectionCallbackTestHost(t)
	auth.Metadata["last_error"] = map[string]any{
		"source": "provider", "code": "provider_failure", "message": "private provider detail", "http_status": 503,
	}
	auth.LastError = &coreauth.Error{Code: "provider_failure", Message: "private provider detail", HTTPStatus: 503}
	var err error
	auth, err = host.currentAuthManager().Update(context.Background(), auth)
	if err != nil {
		t.Fatalf("update foreign-error fixture: %v", err)
	}
	raw, err := host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthInspectionList, nil)
	if err != nil {
		t.Fatalf("inspection list: %v", err)
	}
	response, err := decodeRPCEnvelope[pluginapi.HostAuthInspectionListResponse](raw)
	if err != nil || len(response.Auths) != 1 {
		t.Fatalf("decode inspection list: %#v, %v", response, err)
	}
	if response.Auths[0].InspectionError != nil {
		t.Fatalf("foreign error leaked into inspection projection: %#v", response.Auths[0].InspectionError)
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

func TestAccountInspectionDeleteRejectsSharedVirtualSource(t *testing.T) {
	host, primary := inspectionCallbackTestHost(t)
	sourcePath := primary.Attributes["path"]
	coreauth.MarkPluginVirtualAuth(primary, sourcePath, 0)
	primary, err := host.currentAuthManager().Update(context.Background(), primary)
	if err != nil {
		t.Fatalf("mark primary virtual auth: %v", err)
	}
	secondary := &coreauth.Auth{
		ID: "codex-inspection-secondary", Provider: "codex", FileName: "codex-secondary.json",
		Status: coreauth.StatusActive, UpdatedAt: time.Now().UTC(),
		Attributes: map[string]string{"path": sourcePath, "source": sourcePath},
		Metadata:   map[string]any{"email": "secondary@example.com"},
	}
	coreauth.MarkPluginVirtualAuth(secondary, sourcePath, 1)
	if _, err = host.currentAuthManager().Register(context.Background(), secondary); err != nil {
		t.Fatalf("register secondary virtual auth: %v", err)
	}
	rawRequest, _ := json.Marshal(pluginapi.HostAuthDeleteRequest{
		AuthIndex: primary.Index, ExpectedRevision: authRevision(primary),
	})
	if _, err = host.callFromPlugin(proObservabilityCallbackContext(), pluginabi.MethodHostAuthDelete, rawRequest); err == nil || !strings.Contains(err.Error(), "shared source file") {
		t.Fatalf("delete shared virtual source error = %v", err)
	}
	if _, err = os.Stat(sourcePath); err != nil {
		t.Fatalf("shared source was removed: %v", err)
	}
}
