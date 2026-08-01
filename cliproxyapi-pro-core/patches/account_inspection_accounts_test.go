package management

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	proinspection "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/inspection"
	proquota "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/quota"
)

func TestSyncAuthInspectionLastErrorClearsMetadata(t *testing.T) {
	auth := &coreauth.Auth{
		LastError: &coreauth.Error{Code: "token_refresh_error", Message: "old refresh failed"},
		Metadata: map[string]any{
			"last_error": map[string]any{"code": "token_refresh_error", "message": "old refresh failed"},
			"email":      "user@example.com",
		},
	}

	syncAuthInspectionLastError(auth, nil)

	if auth.LastError != nil {
		t.Fatalf("LastError = %#v, want nil", auth.LastError)
	}
	if _, ok := auth.Metadata["last_error"]; ok {
		t.Fatalf("metadata last_error = %#v, want removed", auth.Metadata["last_error"])
	}
	if auth.Metadata["email"] != "user@example.com" {
		t.Fatalf("metadata email = %#v, want preserved", auth.Metadata["email"])
	}
}

func TestSyncInspectionAuthErrorPersistsLastErrorMetadata(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	registered, err := manager.Register(context.Background(), &coreauth.Auth{
		Provider: "codex",
		ID:       "codex-user",
		FileName: "codex-user.json",
		Metadata: map[string]any{"email": "user@example.com"},
	})
	if err != nil {
		t.Fatalf("Register auth error = %v", err)
	}

	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	scheduler.syncInspectionAuthError(context.Background(), accountFromAuth(registered), "token_refresh_error", "refresh failed", 0)

	var got *coreauth.Auth
	for _, auth := range manager.List() {
		if auth.ID == registered.ID {
			got = auth
			break
		}
	}
	if got == nil {
		t.Fatal("updated auth not found")
	}
	if got.Status != coreauth.StatusError || !got.Unavailable || got.StatusMessage != "refresh failed" {
		t.Fatalf("updated status = status:%q unavailable:%v message:%q, want error/unavailable/refresh failed", got.Status, got.Unavailable, got.StatusMessage)
	}
	if got.LastError == nil || got.LastError.Code != "token_refresh_error" || got.LastError.Message != "refresh failed" {
		t.Fatalf("LastError = %#v, want token_refresh_error/refresh failed", got.LastError)
	}
	lastError, ok := got.Metadata["last_error"].(map[string]any)
	if !ok {
		t.Fatalf("metadata last_error = %#v, want object", got.Metadata["last_error"])
	}
	if lastError["code"] != "token_refresh_error" || lastError["message"] != "refresh failed" {
		t.Fatalf("metadata last_error = %#v, want token_refresh_error/refresh failed", lastError)
	}
}

func TestCleanupLegacyQuotaCacheFromOrdinaryAuthPersistsJSONRemoval(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "codex-user.json")
	manager := coreauth.NewManager(&accountInspectionAuthStore{path: authPath}, nil, nil)
	registered, err := manager.Register(context.Background(), &coreauth.Auth{
		Provider: "codex",
		ID:       "codex-user",
		FileName: "codex-user.json",
		Metadata: map[string]any{
			"email":       "user@example.com",
			"quota_cache": map[string]any{"status": "success", "cachedAt": float64(123)},
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	if err := scheduler.cleanupLegacyQuotaCacheFromAuth(context.Background(), accountFromAuth(registered)); err != nil {
		t.Fatalf("cleanupLegacyQuotaCacheFromAuth() error = %v", err)
	}
	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := persisted["quota_cache"]; ok {
		t.Fatalf("persisted quota_cache = %#v, want removed", persisted["quota_cache"])
	}
	if persisted["email"] != "user@example.com" {
		t.Fatalf("persisted email = %#v, want preserved", persisted["email"])
	}
	got, ok := manager.GetByID(registered.ID)
	if !ok || got == nil {
		t.Fatal("updated auth not found")
	}
	if _, ok := got.Metadata["quota_cache"]; ok {
		t.Fatalf("runtime quota_cache = %#v, want removed", got.Metadata["quota_cache"])
	}
}

func TestCleanupLegacyQuotaCachesPreservesPluginVirtualSourceMetadata(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "gemini-cli.json")
	legacyQuota := map[string]any{"status": "success", "cachedAt": float64(123)}
	if err := os.WriteFile(authPath, []byte(`{"type":"gemini-cli","email":"user@example.com","project_id":"project-a","project_ids":["project-a","project-b"],"quota_cache":{"status":"success","cachedAt":123}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	primary := &coreauth.Auth{
		Provider: "gemini-cli",
		ID:       "gemini-cli-primary",
		FileName: "gemini-cli.json",
		Metadata: map[string]any{
			"type":        "gemini-cli",
			"email":       "user@example.com",
			"project_id":  "project-a",
			"quota_cache": legacyQuota,
		},
		Attributes: map[string]string{"path": authPath, "project_id": "project-a"},
	}
	coreauth.MarkPluginVirtualAuth(primary, authPath, 0)
	secondary := &coreauth.Auth{
		Provider: "gemini-cli",
		ID:       "gemini-cli-project-b",
		FileName: "user-project-b.json",
		Metadata: map[string]any{
			"type":        "gemini-cli",
			"email":       "user@example.com",
			"project_id":  "project-b",
			"virtual":     true,
			"quota_cache": legacyQuota,
		},
		Attributes: map[string]string{"path": authPath, "project_id": "project-b", "runtime_only": "true"},
	}
	coreauth.MarkPluginVirtualAuth(secondary, authPath, 1)
	if _, err := manager.Register(context.Background(), secondary); err != nil {
		t.Fatalf("Register(secondary) error = %v", err)
	}
	if _, err := manager.Register(context.Background(), primary); err != nil {
		t.Fatalf("Register(primary) error = %v", err)
	}

	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	scheduler.cleanupLegacyQuotaCaches(context.Background())

	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := persisted["quota_cache"]; ok {
		t.Fatalf("persisted quota_cache = %#v, want removed", persisted["quota_cache"])
	}
	if persisted["project_id"] != "project-a" {
		t.Fatalf("persisted project_id = %#v, want project-a", persisted["project_id"])
	}
	projectIDs, ok := persisted["project_ids"].([]any)
	if !ok || len(projectIDs) != 2 || projectIDs[0] != "project-a" || projectIDs[1] != "project-b" {
		t.Fatalf("persisted project_ids = %#v, want original project list", persisted["project_ids"])
	}
	for _, auth := range manager.List() {
		if auth != nil && auth.Metadata != nil {
			if _, ok := auth.Metadata["quota_cache"]; ok {
				t.Fatalf("runtime auth %q quota_cache = %#v, want removed", auth.ID, auth.Metadata["quota_cache"])
			}
		}
	}
}

func TestClearInspectionAuthErrorClearsMetadataOnlyError(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	registered, err := manager.Register(context.Background(), &coreauth.Auth{
		Provider:      "antigravity",
		ID:            "antigravity-user",
		FileName:      "antigravity-user.json",
		Status:        coreauth.StatusActive,
		StatusMessage: "",
		Unavailable:   false,
		Metadata: map[string]any{
			"email": "user@example.com",
			"last_error": map[string]any{
				"code":        "inspection_probe_error",
				"http_status": 0,
				"message":     "antigravity quota unavailable",
				"retryable":   false,
			},
		},
	})
	if err != nil {
		t.Fatalf("Register auth error = %v", err)
	}

	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	scheduler.clearInspectionAuthError(context.Background(), accountFromAuth(registered))

	var got *coreauth.Auth
	for _, auth := range manager.List() {
		if auth.ID == registered.ID {
			got = auth
			break
		}
	}
	if got == nil {
		t.Fatal("updated auth not found")
	}
	if got.LastError != nil {
		t.Fatalf("LastError = %#v, want nil", got.LastError)
	}
	if _, ok := got.Metadata["last_error"]; ok {
		t.Fatalf("metadata last_error = %#v, want removed", got.Metadata["last_error"])
	}
	if got.Status != coreauth.StatusActive || got.StatusMessage != "" || got.Unavailable {
		t.Fatalf("status = %q message=%q unavailable=%v, want active/empty/false", got.Status, got.StatusMessage, got.Unavailable)
	}
	if got.Metadata["email"] != "user@example.com" {
		t.Fatalf("metadata email = %#v, want preserved", got.Metadata["email"])
	}
}

func TestAutoActionConfirmationDelaysExecution(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	result := testInspectionResult("quota", accountInspectionActionDisable, false, nil, true, "")
	settings := proinspection.DefaultSettings()
	settings.AutoExecuteConfirmations = 2
	settings.AutoExecuteQuotaLimitDisable = true

	action := proinspection.AutoActionForResult(result, settings)
	if action != accountInspectionActionDisable {
		t.Fatalf("proinspection.AutoActionForResult() = %q, want disable", action)
	}
	confirmed, count, required := scheduler.confirmAutoAction(result, action, settings.AutoExecuteConfirmations)
	if confirmed || count != 1 || required != 2 {
		t.Fatalf("first confirmation = confirmed:%v count:%d required:%d, want false/1/2", confirmed, count, required)
	}
	confirmed, count, required = scheduler.confirmAutoAction(result, action, settings.AutoExecuteConfirmations)
	if !confirmed || count != 2 || required != 2 {
		t.Fatalf("second confirmation = confirmed:%v count:%d required:%d, want true/2/2", confirmed, count, required)
	}
	scheduler.clearAutoActionConfirmation(result)
	confirmed, count, required = scheduler.confirmAutoAction(result, action, settings.AutoExecuteConfirmations)
	if confirmed || count != 1 || required != 2 {
		t.Fatalf("confirmation after clear = confirmed:%v count:%d required:%d, want false/1/2", confirmed, count, required)
	}
}

func TestExecuteActionDisablesGeminiCLIPluginVirtualSourceFile(t *testing.T) {
	authDir := t.TempDir()
	authPath := filepath.Join(authDir, "gemini-cli.json")
	if err := os.WriteFile(authPath, []byte(`{"type":"gemini-cli","email":"user@example.com","project_id":"project-a","project_ids":["project-a","project-b"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile auth error = %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	primary := &coreauth.Auth{
		Provider:   "gemini-cli",
		ID:         "gemini-cli-primary",
		FileName:   "gemini-cli.json",
		Metadata:   map[string]any{"type": "gemini-cli", "email": "user@example.com", "project_id": "project-a"},
		Attributes: map[string]string{"path": authPath, "project_id": "project-a"},
		Storage:    &accountInspectionTestStorage{},
	}
	coreauth.MarkPluginVirtualAuth(primary, authPath, 0)
	secondary := &coreauth.Auth{
		Provider:   "gemini-cli",
		ID:         "gemini-cli-project-b",
		FileName:   "user-project-b.json",
		Metadata:   map[string]any{"type": "gemini-cli", "email": "user@example.com", "project_id": "project-b", "virtual": true},
		Attributes: map[string]string{"path": authPath, "project_id": "project-b", "runtime_only": "true"},
		Storage:    &accountInspectionTestStorage{},
	}
	coreauth.MarkPluginVirtualAuth(secondary, authPath, 1)
	registeredSecondary, err := manager.Register(context.Background(), secondary)
	if err != nil {
		t.Fatalf("Register secondary error = %v", err)
	}
	if _, err = manager.Register(context.Background(), primary); err != nil {
		t.Fatalf("Register primary error = %v", err)
	}

	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	err = scheduler.executeAction(context.Background(), accountInspectionResult{AuthIndex: registeredSecondary.Index}, accountInspectionActionDisable)
	if err != nil {
		t.Fatalf("executeAction(disable) error = %v", err)
	}

	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("ReadFile auth error = %v", err)
	}
	var saved map[string]any
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("saved auth invalid JSON: %v", err)
	}
	if saved["disabled"] != true {
		t.Fatalf("saved disabled = %#v, want true in source auth file", saved["disabled"])
	}
	if saved["project_id"] != "project-a" {
		t.Fatalf("saved project_id = %#v, want primary project preserved", saved["project_id"])
	}
	projectIDs, ok := saved["project_ids"].([]any)
	if !ok || len(projectIDs) != 2 || projectIDs[0] != "project-a" || projectIDs[1] != "project-b" {
		t.Fatalf("saved project_ids = %#v, want original source project_ids preserved", saved["project_ids"])
	}
}

func TestManualActionsBindIdentityToCurrentSnapshot(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	registeredA, err := manager.Register(context.Background(), &coreauth.Auth{Provider: "codex", ID: "auth-a", FileName: "a.json", Metadata: map[string]any{"email": "a@example.com"}})
	if err != nil {
		t.Fatalf("Register(auth-a) error = %v", err)
	}
	registeredB, err := manager.Register(context.Background(), &coreauth.Auth{Provider: "codex", ID: "auth-b", FileName: "b.json", Metadata: map[string]any{"email": "b@example.com"}})
	if err != nil {
		t.Fatalf("Register(auth-b) error = %v", err)
	}
	resultA := accountFromAuth(registeredA).baseResult()
	scheduler := &accountInspectionScheduler{
		h:      &Handler{authManager: manager},
		status: accountInspectionStatus{Results: []accountInspectionResult{resultA}},
	}
	outcomes, err := scheduler.executeManualActions(context.Background(), []accountInspectionActionItem{{
		Key:       resultA.Key,
		FileName:  registeredB.FileName,
		AuthIndex: registeredB.Index,
		Provider:  registeredB.Provider,
		Action:    accountInspectionActionDisable,
	}})
	if err != nil {
		t.Fatalf("executeManualActions() error = %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Success || outcomes[0].AuthIndex != registeredA.Index || outcomes[0].FileName != registeredA.FileName {
		t.Fatalf("outcomes = %+v, want canonical auth-a identity", outcomes)
	}
	gotA, _ := manager.GetByID(registeredA.ID)
	gotB, _ := manager.GetByID(registeredB.ID)
	if gotA == nil || !gotA.Disabled {
		t.Fatalf("auth-a = %#v, want disabled", gotA)
	}
	if gotB == nil || gotB.Disabled {
		t.Fatalf("auth-b = %#v, want enabled", gotB)
	}
}

func TestExecuteActionRejectsStaleRuntimeIdentity(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	registeredA, _ := manager.Register(context.Background(), &coreauth.Auth{Provider: "codex", ID: "auth-a", FileName: "a.json", Metadata: map[string]any{}})
	registeredB, _ := manager.Register(context.Background(), &coreauth.Auth{Provider: "codex", ID: "auth-b", FileName: "b.json", Metadata: map[string]any{}})
	result := accountFromAuth(registeredA).baseResult()
	result.AuthIndex = registeredB.Index
	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	if err := scheduler.executeAction(context.Background(), result, accountInspectionActionDisable); !errors.Is(err, errAccountInspectionResultStale) {
		t.Fatalf("executeAction() error = %v, want stale result", err)
	}
	gotA, _ := manager.GetByID(registeredA.ID)
	gotB, _ := manager.GetByID(registeredB.ID)
	if gotA.Disabled || gotB.Disabled {
		t.Fatalf("auths mutated after stale action: a=%v b=%v", gotA.Disabled, gotB.Disabled)
	}
}

func TestExecuteActionRejectsSharedPluginVirtualSourceDelete(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "gemini-cli.json")
	if err := os.WriteFile(authPath, []byte(`{"type":"gemini-cli","project_ids":["project-a","project-b"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	manager := coreauth.NewManager(nil, nil, nil)
	primary := &coreauth.Auth{Provider: "gemini-cli", ID: "primary", FileName: "gemini-cli.json", Metadata: map[string]any{}, Attributes: map[string]string{"path": authPath}}
	secondary := &coreauth.Auth{Provider: "gemini-cli", ID: "secondary", FileName: "project-b.json", Metadata: map[string]any{}, Attributes: map[string]string{"path": authPath, "runtime_only": "true"}}
	coreauth.MarkPluginVirtualAuth(primary, authPath, 0)
	coreauth.MarkPluginVirtualAuth(secondary, authPath, 1)
	registeredPrimary, _ := manager.Register(context.Background(), primary)
	_, _ = manager.Register(context.Background(), secondary)
	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	if err := scheduler.executeAction(context.Background(), accountFromAuth(registeredPrimary).baseResult(), accountInspectionActionDelete); !errors.Is(err, errAccountInspectionSharedSourceDelete) {
		t.Fatalf("executeAction(delete) error = %v, want shared-source rejection", err)
	}
	if _, err := os.Stat(authPath); err != nil {
		t.Fatalf("shared source file was removed: %v", err)
	}
}

func TestPluginVirtualInspectionErrorStaysOnTargetIdentity(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "gemini-cli.json")
	if err := os.WriteFile(authPath, []byte(`{"type":"gemini-cli","project_ids":["project-a","project-b"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	manager := coreauth.NewManager(nil, nil, nil)
	primary := &coreauth.Auth{Provider: "gemini-cli", ID: "primary", FileName: "gemini-cli.json", Metadata: map[string]any{"project_id": "project-a"}, Attributes: map[string]string{"path": authPath}}
	secondary := &coreauth.Auth{Provider: "gemini-cli", ID: "secondary", FileName: "project-b.json", Metadata: map[string]any{"project_id": "project-b"}, Attributes: map[string]string{"path": authPath, "runtime_only": "true"}}
	coreauth.MarkPluginVirtualAuth(primary, authPath, 0)
	coreauth.MarkPluginVirtualAuth(secondary, authPath, 1)
	registeredPrimary, _ := manager.Register(context.Background(), primary)
	registeredSecondary, _ := manager.Register(context.Background(), secondary)
	scheduler := &accountInspectionScheduler{h: &Handler{authManager: manager}}
	scheduler.syncInspectionAuthError(context.Background(), accountFromAuth(registeredSecondary), "inspection_probe_error", "project-b failed", 0)

	gotPrimary, _ := manager.GetByID(registeredPrimary.ID)
	gotSecondary, _ := manager.GetByID(registeredSecondary.ID)
	if gotPrimary.LastError != nil || gotPrimary.Status == coreauth.StatusError {
		t.Fatalf("primary auth was polluted: %#v", gotPrimary)
	}
	if gotSecondary.LastError == nil || gotSecondary.LastError.Code != "inspection_probe_error" || gotSecondary.Status != coreauth.StatusError {
		t.Fatalf("secondary auth error = %#v, want scoped inspection error", gotSecondary)
	}
	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(raw), "last_error") {
		t.Fatalf("shared source contains identity-local error: %s", raw)
	}
}

func TestEnablePreservesNonInspectionLastError(t *testing.T) {
	auth := &coreauth.Auth{
		Disabled:    true,
		Status:      coreauth.StatusDisabled,
		LastError:   &coreauth.Error{Code: "upstream_refresh_error", Message: "refresh failed"},
		Metadata:    map[string]any{"last_error": map[string]any{"code": "upstream_refresh_error", "message": "refresh failed"}},
		Unavailable: true,
	}
	setProAuthDisabledState(auth, false)
	if auth.Disabled || auth.Status != coreauth.StatusError || !auth.Unavailable {
		t.Fatalf("enabled auth state = disabled:%v status:%q unavailable:%v", auth.Disabled, auth.Status, auth.Unavailable)
	}
	if auth.LastError == nil || auth.LastError.Code != "upstream_refresh_error" {
		t.Fatalf("LastError = %#v, want preserved upstream error", auth.LastError)
	}
	if _, ok := auth.Metadata["last_error"]; !ok {
		t.Fatal("metadata last_error was removed")
	}
}

func TestQuotaSuccessStateIncludesParserMetadata(t *testing.T) {
	state := quotaSuccessState(map[string]any{"rawShapeHash": proquota.JSONShapeHash(`{"a":1,"items":[{"b":true}]}`)})
	if state["schemaVersion"] != 2 || state["parserVersion"] != accountInspectionQuotaParserVersion || state["status"] != "success" {
		t.Fatalf("quota state metadata = %+v", state)
	}
	if state["rawShapeHash"] == "" {
		t.Fatalf("rawShapeHash = %q, want populated", state["rawShapeHash"])
	}
}

func TestAntigravityQuotaURLsUseSummaryEndpoint(t *testing.T) {
	for _, url := range antigravityQuotaURLs() {
		if !strings.Contains(url, "retrieveUserQuotaSummary") {
			t.Fatalf("antigravity quota url = %q, want retrieveUserQuotaSummary", url)
		}
	}
}
