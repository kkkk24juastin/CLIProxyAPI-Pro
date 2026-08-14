package management

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	proapp "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/app"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func registerAPIKeyPolicyTestCatalog(t *testing.T) {
	t.Helper()
	const clientID = "api-key-policy-management-test"
	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient(clientID, "claude", []*registry.ModelInfo{{ID: "claude-sonnet-4-6"}})
	t.Cleanup(func() { modelRegistry.UnregisterClient(clientID) })
}

func newAPIKeyPolicyManagementHarness(t *testing.T, keys []string) (*Handler, *gin.Engine) {
	t.Helper()
	registerAPIKeyPolicyTestCatalog(t)
	gin.SetMode(gin.TestMode)
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	application, err := proapp.New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithoutConfigFilePath(&config.Config{SDKConfig: config.SDKConfig{APIKeys: append([]string(nil), keys...)}}, nil)
	h.SetProApp(application)
	t.Cleanup(func() {
		h.Shutdown()
		application.Close()
		cancel()
	})
	router := gin.New()
	group := router.Group("/v0/management")
	group.Use(func(c *gin.Context) {
		c.Set(apiKeyPolicyManagementSessionContextKey, c.GetHeader("X-Test-Policy-Session"))
		c.Next()
	})
	h.RegisterAPIKeyPolicyRoutes(group)
	return h, router
}

func newAuthenticatedAPIKeyPolicyManagementHarness(t *testing.T, keys []string) (*Handler, *gin.Engine) {
	t.Helper()
	registerAPIKeyPolicyTestCatalog(t)
	const managementPassword = "management-policy-test-secret"
	t.Setenv("MANAGEMENT_PASSWORD", managementPassword)
	gin.SetMode(gin.TestMode)
	t.Setenv("USAGE_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	application, err := proapp.New(ctx, filepath.Join(t.TempDir(), "missing-config.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlerWithoutConfigFilePath(&config.Config{SDKConfig: config.SDKConfig{APIKeys: append([]string(nil), keys...)}}, nil)
	h.SetProApp(application)
	t.Cleanup(func() {
		h.Shutdown()
		application.Close()
		cancel()
	})
	router := gin.New()
	group := router.Group("/v0/management")
	group.Use(h.Middleware())
	h.RegisterAPIKeyPolicyRoutes(group)
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusNotFound) })
	return h, router
}

func authenticatedPolicyRequest(t *testing.T, router *gin.Engine, method, path, password string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if password != "" {
		req.Header.Set("Authorization", "Bearer "+password)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func policyRequest(t *testing.T, router *gin.Engine, method, path, session string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if session != "" {
		req.Header.Set("X-Test-Policy-Session", session)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func bindingResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Items            []apiKeyPolicyBinding `json:"items"`
	Orphaned         []map[string]any      `json:"orphaned"`
	NextCursor       string                `json:"nextCursor"`
	ConfigGeneration uint64                `json:"configGeneration"`
} {
	t.Helper()
	var response struct {
		Items            []apiKeyPolicyBinding `json:"items"`
		Orphaned         []map[string]any      `json:"orphaned"`
		NextCursor       string                `json:"nextCursor"`
		ConfigGeneration uint64                `json:"configGeneration"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func takeoverStatusResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	TakeoverEnabled      bool   `json:"takeoverEnabled"`
	Healthy              bool   `json:"healthy"`
	PolicyGeneration     uint64 `json:"policyGeneration"`
	ConfiguredGeneration uint64 `json:"configuredGeneration"`
} {
	t.Helper()
	var response struct {
		TakeoverEnabled      bool   `json:"takeoverEnabled"`
		Healthy              bool   `json:"healthy"`
		PolicyGeneration     uint64 `json:"policyGeneration"`
		ConfiguredGeneration uint64 `json:"configuredGeneration"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func createPolicyBody(keyRef string) map[string]any {
	return map[string]any{
		"keyRef":      keyRef,
		"displayName": "Primary key",
		"initialProfile": map[string]any{
			"name": "Production", "providers": []string{"claude"}, "models": []string{"claude-sonnet-4-6"}, "mappings": []any{},
		},
	}
}

func TestAPIKeyPolicyMaskMatchesManagementDisplayContract(t *testing.T) {
	for raw, want := range map[string]string{
		"":                                  "",
		"a":                                 "a********a",
		"abc":                               "a********c",
		"abcd":                              "ab******cd",
		"sk-sensitive-canary-123456789":     "sk******89",
		"another-upstream-key-abcdefghijkl": "an******kl",
	} {
		if got := maskAPIKey(raw); got != want {
			t.Fatalf("maskAPIKey(%q)=%q, want %q", raw, got, want)
		}
	}
}

func TestAPIKeyPolicyBindingsAndKeyReferenceSecurity(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"sk-sensitive-canary-123456789"})
	listed := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	response := bindingResponse(t, listed)
	if len(response.Items) != 1 || response.Items[0].KeyRef == "" || response.Items[0].MaskedKey == "sk-sensitive-canary-123456789" {
		t.Fatalf("unsafe binding response: %s", listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "sk-sensitive-canary-123456789") {
		t.Fatalf("binding leaked a raw key: %s", listed.Body.String())
	}
	keyRef := response.Items[0].KeyRef

	crossSession := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-b", createPolicyBody(keyRef))
	if crossSession.Code != http.StatusConflict || !strings.Contains(crossSession.Body.String(), "api_key_reference_stale") {
		t.Fatalf("cross-session status=%d body=%s", crossSession.Code, crossSession.Body.String())
	}
	// References are consumed even when the first attempt uses the wrong session.
	reused := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(keyRef))
	if reused.Code != http.StatusConflict {
		t.Fatalf("reused status=%d body=%s", reused.Code, reused.Body.String())
	}

	listed = policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil)
	keyRef = bindingResponse(t, listed).Items[0].KeyRef
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"sk-sensitive-canary-123456789"}}})
	staleGeneration := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(keyRef))
	if staleGeneration.Code != http.StatusConflict || !strings.Contains(staleGeneration.Body.String(), "api_key_reference_stale") {
		t.Fatalf("generation status=%d body=%s", staleGeneration.Code, staleGeneration.Body.String())
	}

	listed = policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil)
	keyRef = bindingResponse(t, listed).Items[0].KeyRef
	h.apiKeyRefsMu.Lock()
	reference := h.apiKeyRefs[keyRef]
	reference.expiresAt = time.Now().Add(-time.Second)
	h.apiKeyRefs[keyRef] = reference
	h.apiKeyRefsMu.Unlock()
	expired := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(keyRef))
	if expired.Code != http.StatusConflict || !strings.Contains(expired.Body.String(), "api_key_reference_stale") {
		t.Fatalf("expired status=%d body=%s", expired.Code, expired.Body.String())
	}
}

func TestAPIKeyPolicySameSessionReferenceCreatesOnceAndResponsesHideSecrets(t *testing.T) {
	const rawKey = "sk-sensitive-canary-abcdefghijklmnopqrstuvwxyz"
	_, router := newAPIKeyPolicyManagementHarness(t, []string{rawKey})
	listed := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil)
	keyRef := bindingResponse(t, listed).Items[0].KeyRef
	created := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(keyRef))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	reused := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(keyRef))
	if reused.Code != http.StatusConflict || !strings.Contains(reused.Body.String(), "api_key_reference_stale") {
		t.Fatalf("reuse status=%d body=%s", reused.Code, reused.Body.String())
	}
	fingerprint := regexp.MustCompile(`(?i)[a-f0-9]{64}`)
	for name, payload := range map[string]string{"list": listed.Body.String(), "create": created.Body.String(), "reuse": reused.Body.String()} {
		if strings.Contains(payload, rawKey) || fingerprint.MatchString(payload) {
			t.Fatalf("%s response exposed a raw key or full fingerprint: %s", name, payload)
		}
	}
}

func TestAPIKeyPolicyRoutesRequireRealManagementMiddlewareAndBindSession(t *testing.T) {
	_, router := newAuthenticatedAPIKeyPolicyManagementHarness(t, []string{"middleware-key-123456789"})
	unauthenticated := authenticatedPolicyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
	listed := authenticatedPolicyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "management-policy-test-secret", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("authenticated status=%d body=%s", listed.Code, listed.Body.String())
	}
	capabilities := authenticatedPolicyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-capabilities", "management-policy-test-secret", nil)
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"apiVersion":2`) || !strings.Contains(capabilities.Body.String(), `"atomic_workspace_save"`) || !strings.Contains(capabilities.Body.String(), `"orphaned_purge_guard"`) || !strings.Contains(capabilities.Body.String(), `"takeover_control"`) {
		t.Fatalf("capabilities status=%d body=%s", capabilities.Code, capabilities.Body.String())
	}
	status := authenticatedPolicyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-status", "management-policy-test-secret", nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"takeoverEnabled":false`) {
		t.Fatalf("initial takeover status=%d body=%s", status.Code, status.Body.String())
	}
	initialStatus := takeoverStatusResponse(t, status)
	toggled := authenticatedPolicyRequest(t, router, http.MethodPut, "/v0/management/api-key-policy-takeover", "management-policy-test-secret", map[string]any{
		"enabled": true, "policyGeneration": initialStatus.PolicyGeneration, "configuredGeneration": initialStatus.ConfiguredGeneration,
	})
	if toggled.Code != http.StatusOK || !strings.Contains(toggled.Body.String(), `"takeoverEnabled":true`) {
		t.Fatalf("takeover update status=%d body=%s", toggled.Code, toggled.Body.String())
	}
	keyRef := bindingResponse(t, listed).Items[0].KeyRef
	created := authenticatedPolicyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "management-policy-test-secret", createPolicyBody(keyRef))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
}

func TestAPIKeyPolicyTakeoverSupportsEmergencyStopAndRejectsStaleActivation(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"takeover-handler-key"})
	statusRecorder := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-status", "session-a", nil)
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	status := takeoverStatusResponse(t, statusRecorder)
	missingGeneration := policyRequest(t, router, http.MethodPut, "/v0/management/api-key-policy-takeover", "session-a", map[string]any{"enabled": true})
	if missingGeneration.Code != http.StatusBadRequest {
		t.Fatalf("missing generation status=%d body=%s", missingGeneration.Code, missingGeneration.Body.String())
	}
	staleGeneration := policyRequest(t, router, http.MethodPut, "/v0/management/api-key-policy-takeover", "session-a", map[string]any{
		"enabled": true, "policyGeneration": status.PolicyGeneration + 1, "configuredGeneration": status.ConfiguredGeneration,
	})
	if staleGeneration.Code != http.StatusConflict || !strings.Contains(staleGeneration.Body.String(), "api_key_policy_state_changed") {
		t.Fatalf("stale generation status=%d body=%s", staleGeneration.Code, staleGeneration.Body.String())
	}
	enabled := policyRequest(t, router, http.MethodPut, "/v0/management/api-key-policy-takeover", "session-a", map[string]any{
		"enabled": true, "policyGeneration": status.PolicyGeneration, "configuredGeneration": status.ConfiguredGeneration,
	})
	if enabled.Code != http.StatusOK || !takeoverStatusResponse(t, enabled).TakeoverEnabled {
		t.Fatalf("enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	h.apiKeyPolicyService().MarkUnavailable()
	capabilities := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-capabilities", "session-a", nil)
	if capabilities.Code != http.StatusOK {
		t.Fatalf("unhealthy capabilities=%d body=%s", capabilities.Code, capabilities.Body.String())
	}
	unhealthyRecorder := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-status", "session-a", nil)
	unhealthy := takeoverStatusResponse(t, unhealthyRecorder)
	if unhealthyRecorder.Code != http.StatusOK || unhealthy.Healthy || !unhealthy.TakeoverEnabled {
		t.Fatalf("unhealthy status=%d body=%s", unhealthyRecorder.Code, unhealthyRecorder.Body.String())
	}
	disabled := policyRequest(t, router, http.MethodPut, "/v0/management/api-key-policy-takeover", "session-a", map[string]any{"enabled": false})
	disabledStatus := takeoverStatusResponse(t, disabled)
	if disabled.Code != http.StatusOK || disabledStatus.Healthy || disabledStatus.TakeoverEnabled {
		t.Fatalf("emergency stop status=%d body=%s", disabled.Code, disabled.Body.String())
	}
}

func TestAPIKeyPolicyWorkspacePatchIsAtomic(t *testing.T) {
	_, router := newAPIKeyPolicyManagementHarness(t, []string{"workspace-http-key-123456789"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	createdRecorder := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(listed.Items[0].KeyRef))
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdRecorder.Code, createdRecorder.Body.String())
	}
	var created struct {
		ID              string `json:"id"`
		DisplayName     string `json:"displayName"`
		ActiveProfileID string `json:"activeProfileId"`
		Version         int64  `json:"version"`
	}
	if err := json.Unmarshal(createdRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{"name": "Production v2", "providers": []string{"claude"}, "models": []string{"claude-sonnet-4-6"}, "mappings": []any{}}
	updatedRecorder := policyRequest(t, router, http.MethodPatch, "/v0/management/api-key-policies/"+created.ID, "session-a", map[string]any{
		"displayName": "After", "version": created.Version, "profileId": created.ActiveProfileID, "profile": profile,
	})
	if updatedRecorder.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updatedRecorder.Code, updatedRecorder.Body.String())
	}
	var updated struct {
		DisplayName string `json:"displayName"`
		Version     int64  `json:"version"`
	}
	if err := json.Unmarshal(updatedRecorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "After" || updated.Version != created.Version+1 {
		t.Fatalf("updated response=%s", updatedRecorder.Body.String())
	}

	failed := policyRequest(t, router, http.MethodPatch, "/v0/management/api-key-policies/"+created.ID, "session-a", map[string]any{
		"displayName": "Must roll back", "version": updated.Version, "profileId": "missing-profile", "profile": profile,
	})
	if failed.Code != http.StatusNotFound || !strings.Contains(failed.Body.String(), "api_key_profile_not_found") {
		t.Fatalf("failed update status=%d body=%s", failed.Code, failed.Body.String())
	}
	stored := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policies/"+created.ID, "session-a", nil)
	if stored.Code != http.StatusOK || !strings.Contains(stored.Body.String(), `"displayName":"After"`) || strings.Contains(stored.Body.String(), "Must roll back") {
		t.Fatalf("partial workspace commit: %s", stored.Body.String())
	}
}

func TestAPIKeyPolicyOrphanedBindingsAreCursorPaginated(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"key-a", "key-b", "key-c"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	for _, item := range listed.Items {
		created := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(item.KeyRef))
		if created.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
		}
	}
	h.SetConfig(&config.Config{})
	firstRecorder := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings?orphaned_limit=2", "session-a", nil)
	first := bindingResponse(t, firstRecorder)
	if len(first.Orphaned) != 2 || first.NextCursor == "" {
		t.Fatalf("first page=%s", firstRecorder.Body.String())
	}
	secondRecorder := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings?orphaned_limit=2&orphaned_cursor="+first.NextCursor, "session-a", nil)
	second := bindingResponse(t, secondRecorder)
	if len(second.Orphaned) != 1 || second.NextCursor != "" {
		t.Fatalf("second page=%s", secondRecorder.Body.String())
	}
	invalid := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings?orphaned_cursor=bad", "session-a", nil)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_api_key_policy_cursor") {
		t.Fatalf("invalid cursor status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestAPIKeyPolicyBindingsDeduplicateKeysAndReuseSessionReference(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{" duplicate-key ", "duplicate-key"})
	first := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	second := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	if len(first.Items) != 1 || len(second.Items) != 1 {
		t.Fatalf("duplicate configured keys produced bindings: first=%d second=%d", len(first.Items), len(second.Items))
	}
	if first.Items[0].KeyRef == "" || first.Items[0].KeyRef != second.Items[0].KeyRef {
		t.Fatalf("same session/generation did not reuse keyRef: %q %q", first.Items[0].KeyRef, second.Items[0].KeyRef)
	}
	h.apiKeyRefsMu.Lock()
	referenceCount := len(h.apiKeyRefs)
	h.apiKeyRefsMu.Unlock()
	if referenceCount != 1 {
		t.Fatalf("keyRef count=%d, want 1", referenceCount)
	}
}

func TestAPIKeyPolicyReferenceCapacityAndGenerationCleanup(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"key-a"})
	identity, err := apikeypolicy.NewAuthenticatedAPIKeyIdentity("capacity-key")
	if err != nil {
		t.Fatal(err)
	}
	_, generation := h.apiKeyConfigSnapshot()
	h.apiKeyRefsMu.Lock()
	for index := 0; index < apiKeyReferencePerSessionLimit; index++ {
		h.apiKeyRefs[strconv.Itoa(index)] = apiKeyReference{
			identity: identity, generation: generation, session: "full-session", expiresAt: time.Now().Add(time.Minute),
		}
	}
	h.apiKeyRefsMu.Unlock()
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set(apiKeyPolicyManagementSessionContextKey, "full-session")
	otherIdentity, err := apikeypolicy.NewAuthenticatedAPIKeyIdentity("other-capacity-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.issueAPIKeyReference(context, otherIdentity, generation); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("capacity error=%v", err)
	}
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"key-b"}}})
	listed := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("new generation list status=%d body=%s", listed.Code, listed.Body.String())
	}
	h.apiKeyRefsMu.Lock()
	remaining := len(h.apiKeyRefs)
	h.apiKeyRefsMu.Unlock()
	if remaining != 1 {
		t.Fatalf("old-generation references retained: %d", remaining)
	}
}

func TestAPIKeyPolicyOrphanPaginationRejectsConfigGenerationChange(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"key-a", "key-b"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	for _, item := range listed.Items {
		created := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(item.KeyRef))
		if created.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
		}
	}
	h.SetConfig(&config.Config{})
	first := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings?orphaned_limit=1", "session-a", nil))
	if first.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"key-a"}}})
	stale := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings?orphaned_limit=1&orphaned_cursor="+first.NextCursor, "session-a", nil)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "api_key_policy_config_changed") {
		t.Fatalf("stale generation status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestAPIKeyPolicyConfigAssociationMatrixAndNoYAMLWrites(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"key-a", "key-b"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	create := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(listed.Items[0].KeyRef))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var policy struct {
		ID              string `json:"id"`
		Version         int64  `json:"version"`
		ActiveProfileID string `json:"activeProfileId"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &policy); err != nil {
		t.Fatal(err)
	}
	if h.configFilePath != "" {
		t.Fatalf("test unexpectedly has config path %q", h.configFilePath)
	}

	// Reordering does not change association.
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"key-b", "key-a"}}})
	reordered := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	configured := 0
	for _, item := range reordered.Items {
		if item.Policy != nil && item.Policy.ID == policy.ID {
			configured++
		}
	}
	if configured != 1 || len(reordered.Orphaned) != 0 {
		t.Fatalf("reorder response=%+v", reordered)
	}

	// Replacing the key or deleting it retains an orphaned policy and blocks writes.
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"key-b", "key-c"}}})
	orphaned := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	if len(orphaned.Orphaned) != 1 {
		t.Fatalf("orphaned=%s", policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil).Body.String())
	}
	update := policyRequest(t, router, http.MethodPatch, "/v0/management/api-key-policies/"+policy.ID, "session-a", map[string]any{"displayName": "blocked", "version": policy.Version})
	if update.Code != http.StatusConflict || !strings.Contains(update.Body.String(), "api_key_policy_orphaned") {
		t.Fatalf("orphan update status=%d body=%s", update.Code, update.Body.String())
	}

	// Re-adding the exact same value restores the fingerprint association.
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"key-a", "key-b", "key-c"}}})
	restored := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	configured = 0
	for _, item := range restored.Items {
		if item.Policy != nil && item.Policy.ID == policy.ID {
			configured++
		}
	}
	if configured != 1 || len(restored.Orphaned) != 0 {
		t.Fatalf("restore response=%+v", restored)
	}

	deletedProfile := policyRequest(t, router, http.MethodDelete, "/v0/management/api-key-policies/"+policy.ID+"/profiles/"+policy.ActiveProfileID, "session-a", map[string]any{"version": policy.Version})
	if deletedProfile.Code != http.StatusConflict || !strings.Contains(deletedProfile.Body.String(), "active_profile_delete_forbidden") {
		t.Fatalf("active delete status=%d body=%s", deletedProfile.Code, deletedProfile.Body.String())
	}
}

func TestAPIKeyPolicyDeletePreviewIsServerDerivedAndRequiresConfiguredKey(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"preview-key"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	body := createPolicyBody(listed.Items[0].KeyRef)
	body["displayName"] = "Preview"
	createdRecorder := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", body)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdRecorder.Code, createdRecorder.Body.String())
	}
	var created struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(createdRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	preview := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policies/"+created.ID+"/delete-preview", "session-a", nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var response struct {
		PolicyID             string `json:"policyId"`
		Version              int64  `json:"version"`
		Change               string `json:"change"`
		TargetPolicyMode     string `json:"targetPolicyMode"`
		RequiresConfirmation string `json:"requiresConfirmation"`
		ActiveProfile        struct {
			Providers []string `json:"providers"`
			Models    []string `json:"models"`
		} `json:"activeProfile"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.PolicyID != created.ID || response.Version != created.Version || response.Change != "restricted_profile_to_unrestricted_passthrough" || response.TargetPolicyMode != apikeypolicy.ModePassthrough || response.RequiresConfirmation != apikeypolicy.PassthroughConfirmation || len(response.ActiveProfile.Providers) != 1 || len(response.ActiveProfile.Models) != 1 {
		t.Fatalf("preview response=%s", preview.Body.String())
	}
	h.SetConfig(&config.Config{})
	orphaned := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policies/"+created.ID+"/delete-preview", "session-a", nil)
	if orphaned.Code != http.StatusConflict || !strings.Contains(orphaned.Body.String(), "api_key_policy_orphaned") {
		t.Fatalf("orphan preview status=%d body=%s", orphaned.Code, orphaned.Body.String())
	}
}

func TestAPIKeyPolicyOrphanedPurgeRequiresMatchingVersionAndConfigGeneration(t *testing.T) {
	h, router := newAPIKeyPolicyManagementHarness(t, []string{"purge-key"})
	listed := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	createdRecorder := policyRequest(t, router, http.MethodPost, "/v0/management/api-key-policies", "session-a", createPolicyBody(listed.Items[0].KeyRef))
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdRecorder.Code, createdRecorder.Body.String())
	}
	var created struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err := json.Unmarshal(createdRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	h.SetConfig(&config.Config{})
	orphaned := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	if len(orphaned.Orphaned) != 1 {
		t.Fatalf("orphaned bindings=%+v", orphaned)
	}
	path := "/v0/management/orphaned-api-key-policies/" + created.ID

	missingGuard := policyRequest(t, router, http.MethodDelete, path, "session-a", nil)
	if missingGuard.Code != http.StatusBadRequest {
		t.Fatalf("missing guard status=%d body=%s", missingGuard.Code, missingGuard.Body.String())
	}
	staleVersion := policyRequest(t, router, http.MethodDelete, path, "session-a", map[string]any{
		"version": created.Version + 1, "configGeneration": orphaned.ConfigGeneration,
	})
	if staleVersion.Code != http.StatusConflict || !strings.Contains(staleVersion.Body.String(), "config_version_conflict") {
		t.Fatalf("stale version status=%d body=%s", staleVersion.Code, staleVersion.Body.String())
	}

	// Runtime config publishes the restored-key guard before access auth accepts
	// the key and before the Management snapshot catches up. Even in that narrow
	// ordering window, purge must fail without deleting the policy.
	h.apiKeyPolicyService().SetConfiguredAPIKeys([]string{"purge-key"})
	runtimeRestored := policyRequest(t, router, http.MethodDelete, path, "session-a", map[string]any{
		"version": created.Version, "configGeneration": orphaned.ConfigGeneration,
	})
	if runtimeRestored.Code != http.StatusConflict || !strings.Contains(runtimeRestored.Body.String(), "api_key_policy_not_orphaned") {
		t.Fatalf("runtime restored status=%d body=%s", runtimeRestored.Code, runtimeRestored.Body.String())
	}
	h.apiKeyPolicyService().SetConfiguredAPIKeys(nil)

	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"purge-key"}}})
	staleGeneration := policyRequest(t, router, http.MethodDelete, path, "session-a", map[string]any{
		"version": created.Version, "configGeneration": orphaned.ConfigGeneration,
	})
	if staleGeneration.Code != http.StatusConflict || !strings.Contains(staleGeneration.Body.String(), "api_key_policy_config_changed") {
		t.Fatalf("stale generation status=%d body=%s", staleGeneration.Code, staleGeneration.Body.String())
	}
	stored := policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policies/"+created.ID, "session-a", nil)
	if stored.Code != http.StatusOK || !strings.Contains(stored.Body.String(), `"state":"configured"`) {
		t.Fatalf("restored policy was deleted or orphaned: status=%d body=%s", stored.Code, stored.Body.String())
	}

	h.SetConfig(&config.Config{})
	fresh := bindingResponse(t, policyRequest(t, router, http.MethodGet, "/v0/management/api-key-policy-bindings", "session-a", nil))
	purged := policyRequest(t, router, http.MethodDelete, path, "session-a", map[string]any{
		"version": created.Version, "configGeneration": fresh.ConfigGeneration,
	})
	if purged.Code != http.StatusNoContent {
		t.Fatalf("purge status=%d body=%s", purged.Code, purged.Body.String())
	}
}
