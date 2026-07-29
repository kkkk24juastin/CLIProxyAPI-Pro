package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	internalusage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage/internalusage"
)

func TestPluginRegistrationDeclaresUsageAndManagement(t *testing.T) {
	registration := pluginRegistration()
	if !registration.Capabilities.UsagePlugin || !registration.Capabilities.ManagementAPI || !registration.Capabilities.Scheduler || !registration.Capabilities.QuotaCacheStore || !registration.Capabilities.RuntimeStateStore || !registration.Capabilities.ProSettingsStore {
		t.Fatalf("capabilities = %#v", registration.Capabilities)
	}
	routes := managementRoutes()
	if len(routes) < 20 {
		t.Fatalf("routes = %d, want complete usage API", len(routes))
	}
	found := false
	for _, route := range routes {
		if route.Method == http.MethodGet && route.Path == "/usage/model-prices" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("model-prices route not registered")
	}
}

func TestProSettingsCapabilityReadsAndWrites(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopService)
	want := proSetting{Namespace: "routing.test", SchemaVersion: 1, Settings: json.RawMessage(`{"enabled":true}`), UpdatedAtMS: 10}
	putRaw, _ := json.Marshal(proSettingPutRequest{Setting: want})
	if _, err := handleMethod(methodProSettingPut, putRaw); err != nil {
		t.Fatalf("pro setting put error = %v", err)
	}
	getRaw, _ := json.Marshal(proSettingGetRequest{Namespace: want.Namespace})
	raw, err := handleMethod(methodProSettingGet, getRaw)
	if err != nil {
		t.Fatalf("pro setting get error = %v", err)
	}
	var wrapped envelope
	var got proSettingGetResponse
	if json.Unmarshal(raw, &wrapped) != nil || json.Unmarshal(wrapped.Result, &got) != nil || !got.Found || got.Setting.Namespace != want.Namespace || string(got.Setting.Settings) != string(want.Settings) {
		t.Fatalf("pro setting = %#v raw=%s", got, raw)
	}
}

func TestRuntimeStateCapabilityRestoresExactCoreSnapshot(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	putRaw, _ := json.Marshal(authRuntimeStatsPutRequest{Stats: authRuntimeStats{
		AuthIndex: "idx", AuthID: "auth", SelectedCount: 3, SuccessCount: 7, FailureCount: 2,
		RecentBuckets: []runtimeRequestBucket{{BucketID: 10, Success: 4, Failed: 1}}, UpdatedAtMS: 100,
	}})
	if _, err := handleMethod(methodRuntimeStatsPut, putRaw); err != nil {
		t.Fatalf("runtime stats put error = %v", err)
	}
	getRaw, _ := json.Marshal(authRuntimeStatsGetRequest{AuthIndex: "idx", AuthID: "auth"})
	raw, err := handleMethod(methodRuntimeStatsGet, getRaw)
	if err != nil {
		t.Fatalf("runtime stats get error = %v", err)
	}
	var wrapped envelope
	var got authRuntimeStatsGetResponse
	if json.Unmarshal(raw, &wrapped) != nil || json.Unmarshal(wrapped.Result, &got) != nil || !got.Found || got.Stats.SuccessCount != 7 || got.Stats.SelectedCount != 3 {
		t.Fatalf("runtime stats = %#v raw=%s", got, raw)
	}
	deleteRaw, _ := json.Marshal(authRuntimeStateDeleteRequest{AuthID: "auth", AuthIndex: "idx"})
	if _, err = handleMethod(methodRuntimeStateDelete, deleteRaw); err != nil {
		t.Fatalf("runtime state delete error = %v", err)
	}
	raw, err = handleMethod(methodRuntimeStatsGet, getRaw)
	if err != nil || json.Unmarshal(raw, &wrapped) != nil || json.Unmarshal(wrapped.Result, &got) != nil || got.Found {
		t.Fatalf("runtime stats after delete = %#v raw=%s err=%v", got, raw, err)
	}
}

func TestQuotaCacheCapabilityOwnsPutGetMergeAndDelete(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)

	putRaw, _ := json.Marshal(quotaCachePutRequest{
		ContractVersion: quotaCacheContractVersion,
		Entry:           quotaCacheEntry{Provider: "xai", FileName: "x.json", Data: json.RawMessage(`{"billing":{"planType":"free"}}`), ObservedAt: 100},
		Merge:           true,
	})
	if _, err := handleMethod(methodQuotaCachePut, putRaw); err != nil {
		t.Fatalf("quota cache put error = %v", err)
	}
	getRaw, _ := json.Marshal(quotaCacheGetRequest{ContractVersion: quotaCacheContractVersion, Provider: "xai", FileName: "x.json"})
	raw, err := handleMethod(methodQuotaCacheGet, getRaw)
	if err != nil {
		t.Fatalf("quota cache get error = %v", err)
	}
	var wrapped envelope
	if err = json.Unmarshal(raw, &wrapped); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var got quotaCacheGetResponse
	if err = json.Unmarshal(wrapped.Result, &got); err != nil || len(got.Entries) != 1 || got.Entries[0].FileName != "x.json" {
		t.Fatalf("quota cache get = %#v err=%v", got, err)
	}
	deleteRaw, _ := json.Marshal(quotaCacheDeleteRequest{ContractVersion: quotaCacheContractVersion, Provider: "xai", FileName: "x.json"})
	if _, err = handleMethod(methodQuotaCacheDelete, deleteRaw); err != nil {
		t.Fatalf("quota cache delete error = %v", err)
	}
	raw, err = handleMethod(methodQuotaCacheGet, getRaw)
	if err != nil || json.Unmarshal(raw, &wrapped) != nil || json.Unmarshal(wrapped.Result, &got) != nil || len(got.Entries) != 0 {
		t.Fatalf("quota cache after delete = %#v raw=%s err=%v", got, raw, err)
	}
}

func TestManagementRegistrationIncludesCanaryResource(t *testing.T) {
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

func TestManagementResourceUsesAuthenticatedBridge(t *testing.T) {
	response := handleManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/pro-observability/ui",
	})
	body := string(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Headers.Get("Content-Type"), "text/html") {
		t.Fatalf("response = %#v", response)
	}
	for _, marker := range []string{"cliproxy-plugin-resource", "/usage/status", "/pro/observability/runtime", "/pro/observability/migration/status"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("resource page missing marker %q", marker)
		}
	}
}

func TestRoutingRuntimeEndpointReportsPluginOwnership(t *testing.T) {
	configureRouting(routingConfig(t, "fill-first", false))
	response := handleManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/pro/observability/runtime",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.StatusCode, response.Body)
	}
	var payload struct {
		RoutingEnabled  bool   `json:"routingEnabled"`
		RoutingStrategy string `json:"routingStrategy"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	if !payload.RoutingEnabled || payload.RoutingStrategy != "fill-first" {
		t.Fatalf("runtime = %#v", payload)
	}
}

func TestAutomaticMigrationStatusManagementAPI(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	status := handleManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet, Path: "/v0/management/pro/observability/migration/status",
	})
	if status.StatusCode != http.StatusOK || !strings.Contains(string(status.Body), `"owner":"pro-observability"`) {
		t.Fatalf("status response=%d body=%s", status.StatusCode, status.Body)
	}
}

func TestPluginUsageRoutesAreAlwaysRegistered(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	configureRouting(pluginConfigPayload(t, ""))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	found := false
	for _, route := range managementRoutes() {
		if route.Method == http.MethodGet && route.Path == "/usage/aggregates" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("legacy /usage/aggregates route was not registered")
	}
	response := handleManagement(pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/usage/status"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, response.Body)
	}
}

func pluginConfigPayload(t *testing.T, config string) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(config)})
	if err != nil {
		t.Fatalf("marshal plugin config: %v", err)
	}
	return raw
}

func TestSchedulerPersistsRoundRobinCursor(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	configureRouting(routingConfig(t, "round-robin", true))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	request := pluginapi.SchedulerPickRequest{
		Provider: "codex", Model: "gpt-test",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-b", Provider: "codex", Priority: 10},
			{ID: "auth-a", Provider: "codex", Priority: 10},
			{ID: "auth-low", Provider: "codex", Priority: 0},
		},
	}
	if got := pickAuthForTest(t, request).AuthID; got != "auth-a" {
		t.Fatalf("first pick = %q, want auth-a", got)
	}
	if got := pickAuthForTest(t, request).AuthID; got != "auth-b" {
		t.Fatalf("second pick = %q, want auth-b", got)
	}
	state.Lock()
	state.cursors = make(map[string]string)
	state.Unlock()
	if got := pickAuthForTest(t, request).AuthID; got != "auth-a" {
		t.Fatalf("restored pick = %q, want auth-a", got)
	}
}

func TestSchedulerUsesSmoothWeights(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	configureRouting(routingConfig(t, "weighted-round-robin", true))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	request := pluginapi.SchedulerPickRequest{Provider: "gemini", Candidates: []pluginapi.SchedulerAuthCandidate{
		{ID: "heavy", Priority: 0, Attributes: map[string]string{"weight": "3"}},
		{ID: "light", Priority: 0, Attributes: map[string]string{"weight": "1"}},
	}}
	counts := map[string]int{}
	for range 8 {
		counts[pickAuthForTest(t, request).AuthID]++
	}
	if counts["heavy"] != 6 || counts["light"] != 2 {
		t.Fatalf("weighted picks = %#v", counts)
	}
}

func TestSchedulerRestoresSmoothWeightedCursor(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	configureRouting(routingConfig(t, "weighted-round-robin", true))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	request := pluginapi.SchedulerPickRequest{Provider: "gemini", Candidates: []pluginapi.SchedulerAuthCandidate{
		{ID: "heavy", Attributes: map[string]string{"weight": "3"}},
		{ID: "light", Attributes: map[string]string{"weight": "1"}},
	}}
	if got := pickAuthForTest(t, request).AuthID; got != "heavy" {
		t.Fatalf("first pick = %q, want heavy", got)
	}
	if got := pickAuthForTest(t, request).AuthID; got != "heavy" {
		t.Fatalf("second pick = %q, want heavy", got)
	}
	state.Lock()
	state.weighted = make(map[string]map[string]int)
	state.Unlock()
	if got := pickAuthForTest(t, request).AuthID; got != "light" {
		t.Fatalf("restored third pick = %q, want light", got)
	}
}

func TestSchedulerCannotDisablePluginOwnership(t *testing.T) {
	configureRouting(routingConfig(t, "round-robin", false))
	response := pickAuthForTest(t, pluginapi.SchedulerPickRequest{Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "auth-a"}}})
	if !response.Handled || response.AuthID != "auth-a" {
		t.Fatalf("response = %#v, want plugin-owned selection", response)
	}
}

func pickAuthForTest(t *testing.T, request pluginapi.SchedulerPickRequest) pluginapi.SchedulerPickResponse {
	t.Helper()
	response, err := pickAuth(request)
	if err != nil {
		t.Fatalf("pickAuth() error = %v", err)
	}
	return response
}

func routingConfig(t *testing.T, strategy string, routingEnabled bool) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte(
		"routing-enabled: " + strconv.FormatBool(routingEnabled) + "\nrouting-strategy: " + strategy + "\n",
	)})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return raw
}

func TestUsageRecordReachesPluginAPI(t *testing.T) {
	stopService()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)

	err := ingestUsage(usageRecord{
		Provider: "codex", Model: "gpt-test", AuthIndex: "auth-1", APIKey: "sk-test",
		RequestedAt: time.Now(), Latency: 250 * time.Millisecond,
		Detail: usageDetail{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	})
	if err != nil {
		t.Fatalf("ingestUsage() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := handleManagement(pluginapi.ManagementRequest{
			Method: http.MethodGet, Path: "/v0/management/usage",
		})
		if response.StatusCode == http.StatusOK {
			var payload struct {
				TotalRequests int64 `json:"total_requests"`
			}
			if json.Unmarshal(response.Body, &payload) == nil && payload.TotalRequests == 1 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	response := handleManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet, Path: "/v0/management/usage/status",
	})
	t.Fatalf("usage record was not persisted; status=%d body=%s", response.StatusCode, response.Body)
}

func TestExtendedUsageRecordReachesPluginHistory(t *testing.T) {
	stopService()
	state.Lock()
	state.usageContract = 0
	state.Unlock()
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", filepath.Join(t.TempDir(), "usage.sqlite"))
	if err := ensureService(); err != nil {
		t.Fatalf("ensureService() error = %v", err)
	}
	t.Cleanup(stopService)
	attempt := int64(1)
	err := ingestUsage(usageRecord{
		ContractVersion: 2, Provider: "codex", ExecutorType: "codex", Model: "gpt-test",
		AuthIndex: "auth-1", RequestID: "request-42", Endpoint: "POST /v1/responses",
		ClientIP: "192.0.2.10", XForwardedFor: "198.51.100.2", UserAgent: "canary-client/1.0",
		RequestedAt: time.Now(), AttemptIndex: &attempt, Stream: true, ServiceTier: "priority",
		Detail: usageDetail{
			InputTokens: 30, OutputTokens: 10, CacheReadTokens: 5, TotalTokens: 40,
			TokenBreakdown: usageTokenBreakdown{
				SchemaVersion: 2, Quality: "complete", TotalTokens: 40,
				Input:  usageTokenInputBreakdown{TotalTokens: 30, UncachedTokens: 25, CacheReadTokens: 5},
				Output: usageTokenOutputBreakdown{TotalTokens: 10, NonReasoningTokens: 10},
			},
		},
	})
	if err != nil {
		t.Fatalf("ingestUsage() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := handleManagement(pluginapi.ManagementRequest{
			Method: http.MethodGet, Path: "/v0/management/usage/events",
		})
		var payload internalusage.Payload
		if response.StatusCode == http.StatusOK && json.Unmarshal(response.Body, &payload) == nil {
			api := payload.APIs["POST /v1/responses"]
			if api != nil && api.Models["gpt-test"] != nil && len(api.Models["gpt-test"].Details) == 1 {
				detail := api.Models["gpt-test"].Details[0]
				if detail.RequestID != "request-42" || detail.ClientIP != "192.0.2.10" ||
					detail.XForwardedFor != "198.51.100.2" || detail.UserAgent != "canary-client/1.0" ||
					!detail.Stream || detail.AttemptIndex == nil || *detail.AttemptIndex != 1 ||
					detail.ServiceTier != "priority" || detail.TokenBreakdown.SchemaVersion != 2 {
					t.Fatalf("detail = %#v", detail)
				}
				runtime := routingRuntimeSnapshot()
				if runtime["usageRecordContract"] != 2 || runtime["usageRecordParity"] != true {
					t.Fatalf("runtime = %#v", runtime)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("extended usage record was not persisted")
}

func TestUnknownPluginPathIsRejected(t *testing.T) {
	response := handleManagement(pluginapi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/usage-unknown"})
	if response.StatusCode != http.StatusServiceUnavailable && response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
