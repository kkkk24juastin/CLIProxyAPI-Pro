package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct { uint32_t abi_version; void* host_ctx; cliproxy_host_call_fn call; cliproxy_host_free_fn free_buffer; } cliproxy_host_api;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct { uint32_t abi_version; cliproxy_plugin_call_fn call; cliproxy_plugin_free_fn free_buffer; cliproxy_plugin_shutdown_fn shutdown; } cliproxy_plugin_api;
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;
static void store_host_api(const cliproxy_host_api* host) { stored_host = host; }
static int host_api_available(void) { return stored_host != NULL && stored_host->call != NULL; }
static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (!host_api_available()) return 1;
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}
static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) stored_host->free_buffer(ptr, len);
}
*/
import "C"

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	usage "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/pro-observability/internal/usage"
	"gopkg.in/yaml.v3"
)

const (
	pluginVersion             = "0.1.0"
	quotaCacheContractVersion = 1
	methodQuotaCacheGet       = "quota_cache.get"
	methodQuotaCachePut       = "quota_cache.put"
	methodQuotaCacheDelete    = "quota_cache.delete"
	methodQuotaCacheObserve   = "quota_cache.observe"
	methodRuntimeStatsGet     = "runtime_state.auth_stats.get"
	methodRuntimeStatsPut     = "runtime_state.auth_stats.put"
	methodRuntimeStateDelete  = "runtime_state.auth.delete"
	methodProSettingGet       = "pro_settings.get"
	methodProSettingPut       = "pro_settings.put"
	methodHostProBackupExport = "host.pro_backup.export"
	methodHostProBackupImport = "host.pro_backup.import"
)

var state = struct {
	sync.RWMutex
	cancel         context.CancelFunc
	service        *usage.Service
	router         http.Handler
	strategy       string
	routingEnabled bool
	cursors        map[string]string
	weighted       map[string]map[string]int
	selected       map[string]int64
	stats          map[string]usage.AuthRuntimeStats
	usageContract  int
	dbPath         string
	legacyDBPath   string
}{}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	UsagePlugin       bool `json:"usage_plugin"`
	ManagementAPI     bool `json:"management_api"`
	Scheduler         bool `json:"scheduler"`
	QuotaCacheStore   bool `json:"quota_cache_store"`
	RuntimeStateStore bool `json:"runtime_state_store"`
	ProSettingsStore  bool `json:"pro_settings_store"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type quotaCacheEntry struct {
	ID                  string          `json:"id"`
	Provider            string          `json:"provider"`
	FileName            string          `json:"file_name"`
	AuthIndex           string          `json:"auth_index,omitempty"`
	IdentityFingerprint string          `json:"identity_fingerprint,omitempty"`
	Data                json.RawMessage `json:"data"`
	CachedAt            int64           `json:"cached_at"`
	AccessedAt          int64           `json:"accessed_at"`
	ObservedAt          int64           `json:"observed_at"`
	StoredAt            int64           `json:"stored_at"`
	Version             int             `json:"version"`
	Revision            int64           `json:"revision"`
}

type quotaCacheGetRequest struct {
	ContractVersion int    `json:"contract_version"`
	Provider        string `json:"provider,omitempty"`
	FileName        string `json:"file_name,omitempty"`
}

type quotaCacheGetResponse struct {
	ContractVersion int               `json:"contract_version"`
	Entries         []quotaCacheEntry `json:"entries"`
}

type quotaCachePutRequest struct {
	ContractVersion int             `json:"contract_version"`
	Entry           quotaCacheEntry `json:"entry"`
	Merge           bool            `json:"merge,omitempty"`
}

type quotaCacheDeleteRequest struct {
	ContractVersion int    `json:"contract_version"`
	Provider        string `json:"provider,omitempty"`
	FileName        string `json:"file_name,omitempty"`
}

type quotaObservationRequest struct {
	ContractVersion int `json:"contract_version"`
	Observation     struct {
		Provider   string      `json:"provider"`
		FileName   string      `json:"file_name"`
		AuthIndex  string      `json:"auth_index,omitempty"`
		Email      string      `json:"email,omitempty"`
		Label      string      `json:"label,omitempty"`
		Model      string      `json:"model,omitempty"`
		Status     int         `json:"status"`
		Headers    http.Header `json:"headers,omitempty"`
		Body       []byte      `json:"body,omitempty"`
		ObservedAt time.Time   `json:"observed_at"`
	} `json:"observation"`
}

type quotaCacheMutationResponse struct {
	ContractVersion int `json:"contract_version"`
}

type runtimeRequestBucket struct {
	BucketID int64 `json:"bucket_id"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
}

type authRuntimeStats struct {
	AuthIndex           string                 `json:"auth_index"`
	AuthID              string                 `json:"auth_id"`
	FileName            string                 `json:"file_name,omitempty"`
	IdentityFingerprint string                 `json:"identity_fingerprint,omitempty"`
	SelectedCount       int64                  `json:"selected_count"`
	SuccessCount        int64                  `json:"success_count"`
	FailureCount        int64                  `json:"failure_count"`
	RecentBuckets       []runtimeRequestBucket `json:"recent_buckets"`
	UpdatedAtMS         int64                  `json:"updated_at_ms"`
}

type authRuntimeStatsGetRequest struct {
	AuthIndex string `json:"auth_index,omitempty"`
	AuthID    string `json:"auth_id,omitempty"`
}

type authRuntimeStatsGetResponse struct {
	Found bool             `json:"found"`
	Stats authRuntimeStats `json:"stats"`
}

type authRuntimeStatsPutRequest struct {
	Stats authRuntimeStats `json:"stats"`
}

type authRuntimeStateDeleteRequest struct {
	AuthID    string `json:"auth_id,omitempty"`
	AuthIndex string `json:"auth_index,omitempty"`
	FileName  string `json:"file_name,omitempty"`
}

type proSetting struct {
	Namespace     string          `json:"namespace"`
	SchemaVersion int             `json:"schemaVersion"`
	Settings      json.RawMessage `json:"settings"`
	UpdatedAtMS   int64           `json:"updatedAtMs"`
}

type proSettingGetRequest struct {
	Namespace string `json:"namespace"`
}

type proSettingGetResponse struct {
	Found   bool       `json:"found"`
	Setting proSetting `json:"setting"`
}

type proSettingPutRequest struct {
	Setting proSetting `json:"setting"`
}

type hostBackupExportRequest struct {
	Kind string `json:"kind"`
}

type hostBackupExportResponse struct {
	Found bool            `json:"found"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type hostBackupImportRequest struct {
	Kind           string                     `json:"kind"`
	Data           json.RawMessage            `json:"data,omitempty"`
	RoutingCursors []usage.RoutingCursorState `json:"routingCursors,omitempty"`
	RuntimeStats   []usage.AuthRuntimeStats   `json:"runtimeStats,omitempty"`
	ProSettings    []usage.ProSetting         `json:"proSettings,omitempty"`
}

// usageRecord mirrors the JSON RPC contract locally so this plugin remains
// buildable against hosts that predate the extended observability fields.
type usageRecord struct {
	ContractVersion     int
	Provider            string
	ExecutorType        string
	Model               string
	Alias               string
	APIKey              string
	AuthID              string
	AuthIndex           string
	AuthType            string
	Source              string
	RequestID           string
	Endpoint            string
	ClientIP            string
	XForwardedFor       string
	UserAgent           string
	ReasoningEffort     string
	ServiceTier         string
	ResponseServiceTier string
	Generate            bool
	Stream              bool
	AttemptIndex        *int64
	RequestedAt         time.Time
	Latency             time.Duration
	TTFT                time.Duration
	Failed              bool
	Failure             usageFailure
	Detail              usageDetail
	ResponseHeaders     http.Header
}

type usageFailure struct {
	StatusCode int
	Body       string
}

type usageDetail struct {
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
	TokenBreakdown      usageTokenBreakdown
}

type usageTokenBreakdown struct {
	SchemaVersion      int                       `json:"schema_version"`
	Quality            string                    `json:"quality"`
	TotalTokens        int64                     `json:"total_tokens"`
	Input              usageTokenInputBreakdown  `json:"input"`
	Output             usageTokenOutputBreakdown `json:"output"`
	UnclassifiedTokens int64                     `json:"unclassified_tokens"`
}

type usageTokenInputBreakdown struct {
	TotalTokens      int64 `json:"total_tokens"`
	UncachedTokens   int64 `json:"uncached_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type usageTokenOutputBreakdown struct {
	TotalTokens        int64 `json:"total_tokens"`
	NonReasoningTokens int64 `json:"non_reasoning_tokens"`
	ReasoningTokens    int64 `json:"reasoning_tokens"`
}

type managementRegistration struct {
	Routes    []managementRoute `json:"routes"`
	Resources []resourceRoute   `json:"resources"`
}

type managementRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type resourceRoute struct {
	Path        string `json:"path"`
	Menu        string `json:"menu"`
	Description string `json:"description"`
}

//go:embed web/index.html
var observabilityManagementPage []byte

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr, response.len = nil, 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required", http.StatusBadRequest))
		return 1
	}
	var raw []byte
	if request != nil && requestLen > 0 {
		raw = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	result, err := handleMethod(C.GoString(method), raw)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error(), http.StatusInternalServerError))
		return 1
	}
	writeResponse(response, result)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() { stopService() }

func handleMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		configureRouting(raw)
		if err := ensureService(); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodUsageHandle:
		var record usageRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("decode usage record: %w", err)
		}
		if err := ingestUsage(record); err != nil {
			return nil, err
		}
		return okEnvelope(struct{}{})
	case methodQuotaCacheGet:
		return handleQuotaCacheGet(raw)
	case methodQuotaCachePut:
		return handleQuotaCachePut(raw)
	case methodQuotaCacheDelete:
		return handleQuotaCacheDelete(raw)
	case methodQuotaCacheObserve:
		return handleQuotaCacheObserve(raw)
	case methodRuntimeStatsGet:
		return handleRuntimeStatsGet(raw)
	case methodRuntimeStatsPut:
		return handleRuntimeStatsPut(raw)
	case methodRuntimeStateDelete:
		return handleRuntimeStateDelete(raw)
	case methodProSettingGet:
		return handleProSettingGet(raw)
	case methodProSettingPut:
		return handleProSettingPut(raw)
	case pluginabi.MethodSchedulerPick:
		var request pluginapi.SchedulerPickRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("decode scheduler request: %w", err)
		}
		picked, err := pickAuth(request)
		if err != nil {
			return nil, err
		}
		return okEnvelope(picked)
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration{
			Routes: managementRoutes(),
			Resources: []resourceRoute{{
				Path:        "/ui",
				Menu:        "可观测性",
				Description: "插件统一管理 usage、运行统计、价格与备份",
			}},
		})
	case pluginabi.MethodManagementHandle:
		var request pluginapi.ManagementRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
		return okEnvelope(handleManagement(request))
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method, http.StatusNotFound), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "Pro Observability",
			Version:          pluginVersion,
			Author:           "ssfun",
			GitHubRepository: "https://github.com/ssfun/CLIProxyAPI-Pro",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name: "db-path", Type: pluginapi.ConfigFieldTypeString,
					Description: "SQLite database path owned by this plugin. Changing it requires a plugin restart.",
				},
				{
					Name: "legacy-db-path", Type: pluginapi.ConfigFieldTypeString,
					Description: "Legacy Core SQLite path used only by the automatic startup migration.",
				},
				{
					Name: "routing-strategy", Type: pluginapi.ConfigFieldTypeEnum,
					EnumValues:  []string{"round-robin", "weighted-round-robin", "fill-first"},
					Description: "Plugin-owned routing strategy.",
				},
			},
		},
		Capabilities: registrationCapabilities{UsagePlugin: true, ManagementAPI: true, Scheduler: true, QuotaCacheStore: true, RuntimeStateStore: true, ProSettingsStore: true},
	}
}

func configureRouting(raw []byte) {
	var request lifecycleRequest
	_ = json.Unmarshal(raw, &request)
	strategy := "round-robin"
	dbPath := ""
	legacyDBPath := ""
	if len(request.ConfigYAML) > 0 {
		var config struct {
			DBPath          string `yaml:"db-path"`
			LegacyDBPath    string `yaml:"legacy-db-path"`
			RoutingStrategy string `yaml:"routing-strategy"`
		}
		if yaml.Unmarshal(request.ConfigYAML, &config) == nil {
			dbPath = strings.TrimSpace(config.DBPath)
			legacyDBPath = strings.TrimSpace(config.LegacyDBPath)
			candidate := strings.ToLower(strings.TrimSpace(config.RoutingStrategy))
			if candidate == "fill-first" || candidate == "weighted-round-robin" || candidate == "round-robin" {
				strategy = candidate
			}
		}
	}
	state.Lock()
	state.strategy = strategy
	state.routingEnabled = true
	state.dbPath = dbPath
	state.legacyDBPath = legacyDBPath
	if state.cursors == nil {
		state.cursors = make(map[string]string)
		state.weighted = make(map[string]map[string]int)
		state.selected = make(map[string]int64)
		state.stats = make(map[string]usage.AuthRuntimeStats)
	}
	state.Unlock()
}

func ensureService() error {
	state.Lock()
	defer state.Unlock()
	if state.service != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := usage.LoadConfig()
	if state.dbPath != "" {
		cfg.DBPath = state.dbPath
	}
	if state.legacyDBPath != "" {
		cfg.LegacyDBPath = state.legacyDBPath
	}
	service, err := usage.StartWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		return err
	}
	if service == nil {
		cancel()
		return fmt.Errorf("usage service is disabled")
	}
	usage.SetDefaultService(service)
	installHostBackupBridge()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	service.Server().RegisterGinRoutes(router.Group("/usage"))
	state.cancel, state.service, state.router = cancel, service, router
	if state.cursors == nil {
		state.cursors = make(map[string]string)
		state.weighted = make(map[string]map[string]int)
		state.selected = make(map[string]int64)
		state.stats = make(map[string]usage.AuthRuntimeStats)
	}
	return nil
}

func stopService() {
	state.Lock()
	cancel := state.cancel
	service := state.service
	state.cancel, state.service, state.router = nil, nil, nil
	state.Unlock()
	usage.SetDefaultService(nil)
	if cancel != nil {
		cancel()
	}
	if service != nil {
		service.Wait()
	}
	C.store_host_api(nil)
}

func currentService() *usage.Service {
	state.RLock()
	defer state.RUnlock()
	return state.service
}

func installHostBackupBridge() {
	if C.host_api_available() == 0 {
		return
	}
	exporter := func(kind string) func() ([]byte, bool, error) {
		return func() ([]byte, bool, error) {
			raw, err := callHost(methodHostProBackupExport, hostBackupExportRequest{Kind: kind})
			if err != nil {
				return nil, false, err
			}
			var response hostBackupExportResponse
			if err = json.Unmarshal(raw, &response); err != nil {
				return nil, false, fmt.Errorf("decode host backup export response: %w", err)
			}
			return append([]byte(nil), response.Data...), response.Found, nil
		}
	}
	usage.SetAccountInspectionScheduleHandlers(exporter("account-inspection-schedule"), func(raw []byte) error {
		_, err := callHost(methodHostProBackupImport, hostBackupImportRequest{Kind: "account-inspection-schedule", Data: append(json.RawMessage(nil), raw...)})
		return err
	})
	usage.SetAccountInspectionSnapshotHandlers(exporter("account-inspection-snapshot"), func(raw []byte) error {
		_, err := callHost(methodHostProBackupImport, hostBackupImportRequest{Kind: "account-inspection-snapshot", Data: append(json.RawMessage(nil), raw...)})
		return err
	})
	usage.SetAuthRuntimeStateImportHandler(func(cursors []usage.RoutingCursorState, stats []usage.AuthRuntimeStats) error {
		_, err := callHost(methodHostProBackupImport, hostBackupImportRequest{Kind: "runtime-state", RoutingCursors: cursors, RuntimeStats: stats})
		return err
	})
	usage.SetProSettingsImportHandler(func(settings []usage.ProSetting) error {
		_, err := callHost(methodHostProBackupImport, hostBackupImportRequest{Kind: "pro-settings", ProSettings: settings})
		return err
	})
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal host callback: %w", err)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate host callback payload")
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}
	callCode := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback returned no response, code=%d", int(callCode))
	}
	var env envelope
	if err = json.Unmarshal(rawResponse, &env); err != nil {
		return nil, fmt.Errorf("decode host callback envelope: %w", err)
	}
	if !env.OK || callCode != 0 {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback failed, code=%d", int(callCode))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}

func handleProSettingGet(raw []byte) ([]byte, error) {
	var request proSettingGetRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode pro setting get: %w", err)
	}
	service := currentService()
	if service == nil {
		return nil, fmt.Errorf("usage service is not available")
	}
	item, found, err := service.ProSetting(context.Background(), request.Namespace)
	if err != nil {
		return nil, err
	}
	return okEnvelope(proSettingGetResponse{Found: found, Setting: proSettingFromUsage(item)})
}

func handleProSettingPut(raw []byte) ([]byte, error) {
	var request proSettingPutRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode pro setting put: %w", err)
	}
	service := currentService()
	if service == nil {
		return nil, fmt.Errorf("usage service is not available")
	}
	if err := service.SetProSetting(context.Background(), proSettingToUsage(request.Setting)); err != nil {
		return nil, err
	}
	return okEnvelope(struct{}{})
}

func proSettingFromUsage(item usage.ProSetting) proSetting {
	return proSetting{Namespace: item.Namespace, SchemaVersion: item.SchemaVersion, Settings: item.Settings, UpdatedAtMS: item.UpdatedAtMS}
}

func proSettingToUsage(item proSetting) usage.ProSetting {
	return usage.ProSetting{Namespace: item.Namespace, SchemaVersion: item.SchemaVersion, Settings: item.Settings, UpdatedAtMS: item.UpdatedAtMS}
}

func handleQuotaCacheGet(raw []byte) ([]byte, error) {
	var request quotaCacheGetRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode quota cache get: %w", err)
	}
	if request.ContractVersion > quotaCacheContractVersion {
		return nil, fmt.Errorf("quota cache contract version %d is not supported", request.ContractVersion)
	}
	entries, err := usage.GetQuotaCache(context.Background(), request.Provider, request.FileName)
	if err != nil {
		return nil, err
	}
	response := quotaCacheGetResponse{ContractVersion: quotaCacheContractVersion, Entries: make([]quotaCacheEntry, 0, len(entries))}
	for _, entry := range entries {
		response.Entries = append(response.Entries, quotaCacheEntryFromUsage(entry))
	}
	return okEnvelope(response)
}

func handleQuotaCachePut(raw []byte) ([]byte, error) {
	var request quotaCachePutRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode quota cache put: %w", err)
	}
	if request.ContractVersion > quotaCacheContractVersion {
		return nil, fmt.Errorf("quota cache contract version %d is not supported", request.ContractVersion)
	}
	entry := quotaCacheEntryToUsage(request.Entry)
	var err error
	if request.Merge {
		err = usage.MergeXAIQuotaCache(context.Background(), entry)
	} else {
		err = usage.SetQuotaCache(context.Background(), entry)
	}
	if err != nil {
		return nil, err
	}
	return okEnvelope(quotaCacheMutationResponse{ContractVersion: quotaCacheContractVersion})
}

func handleQuotaCacheDelete(raw []byte) ([]byte, error) {
	var request quotaCacheDeleteRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode quota cache delete: %w", err)
	}
	if request.ContractVersion > quotaCacheContractVersion {
		return nil, fmt.Errorf("quota cache contract version %d is not supported", request.ContractVersion)
	}
	if err := usage.DeleteQuotaCache(context.Background(), request.Provider, request.FileName); err != nil {
		return nil, err
	}
	return okEnvelope(quotaCacheMutationResponse{ContractVersion: quotaCacheContractVersion})
}

func handleQuotaCacheObserve(raw []byte) ([]byte, error) {
	var request quotaObservationRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode quota observation: %w", err)
	}
	if request.ContractVersion > quotaCacheContractVersion {
		return nil, fmt.Errorf("quota cache contract version %d is not supported", request.ContractVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(request.Observation.Provider), "xai") {
		return okEnvelope(quotaCacheMutationResponse{ContractVersion: quotaCacheContractVersion})
	}
	if err := usage.ObserveXAIQuotaResponse(context.Background(), usage.XAIQuotaObservation{
		FileName: request.Observation.FileName, AuthIndex: request.Observation.AuthIndex,
		Email: request.Observation.Email, Label: request.Observation.Label, Model: request.Observation.Model,
		Status: request.Observation.Status, Header: request.Observation.Headers,
		Body: request.Observation.Body, ObservedAt: request.Observation.ObservedAt,
	}); err != nil {
		return nil, err
	}
	return okEnvelope(quotaCacheMutationResponse{ContractVersion: quotaCacheContractVersion})
}

func quotaCacheEntryFromUsage(entry usage.QuotaCacheEntry) quotaCacheEntry {
	return quotaCacheEntry{
		ID: entry.ID, Provider: entry.Provider, FileName: entry.FileName, AuthIndex: entry.AuthIndex,
		IdentityFingerprint: entry.IdentityFingerprint, Data: entry.Data, CachedAt: entry.CachedAt,
		AccessedAt: entry.AccessedAt, ObservedAt: entry.ObservedAt, StoredAt: entry.StoredAt,
		Version: entry.Version, Revision: entry.Revision,
	}
}

func quotaCacheEntryToUsage(entry quotaCacheEntry) usage.QuotaCacheEntry {
	return usage.QuotaCacheEntry{
		ID: entry.ID, Provider: entry.Provider, FileName: entry.FileName, AuthIndex: entry.AuthIndex,
		IdentityFingerprint: entry.IdentityFingerprint, Data: entry.Data, CachedAt: entry.CachedAt,
		AccessedAt: entry.AccessedAt, ObservedAt: entry.ObservedAt, StoredAt: entry.StoredAt,
		Version: entry.Version, Revision: entry.Revision,
	}
}

func handleRuntimeStatsGet(raw []byte) ([]byte, error) {
	var request authRuntimeStatsGetRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth runtime stats get: %w", err)
	}
	state.RLock()
	service := state.service
	state.RUnlock()
	if service == nil {
		return nil, fmt.Errorf("usage service is unavailable")
	}
	item, found, err := service.AuthRuntimeStats(context.Background(), request.AuthIndex, request.AuthID)
	if err != nil {
		return nil, err
	}
	return okEnvelope(authRuntimeStatsGetResponse{Found: found, Stats: authRuntimeStatsFromUsage(item)})
}

func handleRuntimeStatsPut(raw []byte) ([]byte, error) {
	var request authRuntimeStatsPutRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth runtime stats put: %w", err)
	}
	item := authRuntimeStatsToUsage(request.Stats)
	state.RLock()
	service := state.service
	state.RUnlock()
	if service == nil {
		return nil, fmt.Errorf("usage service is unavailable")
	}
	if err := service.SetAuthRuntimeStats(context.Background(), item); err != nil {
		return nil, err
	}
	state.Lock()
	if item.AuthIndex != "" {
		state.stats[item.AuthIndex] = item
	}
	state.Unlock()
	return okEnvelope(struct{}{})
}

func handleRuntimeStateDelete(raw []byte) ([]byte, error) {
	var request authRuntimeStateDeleteRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode auth runtime state delete: %w", err)
	}
	state.RLock()
	service := state.service
	state.RUnlock()
	if service == nil {
		return nil, fmt.Errorf("usage service is unavailable")
	}
	if err := service.DeleteAuthRuntimeState(context.Background(), request.AuthID, request.AuthIndex, request.FileName); err != nil {
		return nil, err
	}
	state.Lock()
	delete(state.stats, request.AuthIndex)
	state.Unlock()
	return okEnvelope(struct{}{})
}

func authRuntimeStatsFromUsage(item usage.AuthRuntimeStats) authRuntimeStats {
	out := authRuntimeStats{
		AuthIndex: item.AuthIndex, AuthID: item.AuthID, FileName: item.FileName,
		IdentityFingerprint: item.IdentityFingerprint, SelectedCount: item.SelectedCount,
		SuccessCount: item.SuccessCount, FailureCount: item.FailureCount, UpdatedAtMS: item.UpdatedAtMS,
		RecentBuckets: make([]runtimeRequestBucket, 0, len(item.RecentBuckets)),
	}
	for _, bucket := range item.RecentBuckets {
		out.RecentBuckets = append(out.RecentBuckets, runtimeRequestBucket{BucketID: bucket.BucketID, Success: bucket.Success, Failed: bucket.Failed})
	}
	return out
}

func authRuntimeStatsToUsage(item authRuntimeStats) usage.AuthRuntimeStats {
	out := usage.AuthRuntimeStats{
		AuthIndex: item.AuthIndex, AuthID: item.AuthID, FileName: item.FileName,
		IdentityFingerprint: item.IdentityFingerprint, SelectedCount: item.SelectedCount,
		SuccessCount: item.SuccessCount, FailureCount: item.FailureCount, UpdatedAtMS: item.UpdatedAtMS,
		RecentBuckets: make([]usage.RuntimeRequestBucket, 0, len(item.RecentBuckets)),
	}
	for _, bucket := range item.RecentBuckets {
		out.RecentBuckets = append(out.RecentBuckets, usage.RuntimeRequestBucket{BucketID: bucket.BucketID, Success: bucket.Success, Failed: bucket.Failed})
	}
	return out
}

func ingestUsage(record usageRecord) error {
	state.Lock()
	service := state.service
	if record.ContractVersion > state.usageContract {
		state.usageContract = record.ContractVersion
	}
	state.Unlock()
	if service == nil {
		return fmt.Errorf("usage service is unavailable")
	}
	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	payload := map[string]any{
		"timestamp": timestamp.UTC().Format(time.RFC3339Nano), "provider": record.Provider,
		"executor_type": record.ExecutorType, "model": record.Model, "alias": record.Alias,
		"api_key": record.APIKey, "auth_index": record.AuthIndex, "auth_type": record.AuthType,
		"source": record.Source, "request_id": record.RequestID, "endpoint": record.Endpoint,
		"client_ip": record.ClientIP, "x_forwarded_for": record.XForwardedFor, "user_agent": record.UserAgent,
		"latency_ms": record.Latency.Milliseconds(), "ttft_ms": record.TTFT.Milliseconds(),
		"attempt_index": record.AttemptIndex, "stream": record.Stream,
		"reasoning_effort": record.ReasoningEffort, "service_tier": record.ServiceTier,
		"response_service_tier": record.ResponseServiceTier, "generate": record.Generate,
		"failed": record.Failed, "response_headers": record.ResponseHeaders,
		"fail": map[string]any{"status_code": record.Failure.StatusCode, "body": record.Failure.Body},
		"tokens": map[string]any{
			"input_tokens": record.Detail.InputTokens, "output_tokens": record.Detail.OutputTokens,
			"reasoning_tokens": record.Detail.ReasoningTokens, "cached_tokens": record.Detail.CachedTokens,
			"cache_read_tokens": record.Detail.CacheReadTokens, "cache_creation_tokens": record.Detail.CacheCreationTokens,
			"total_tokens": record.Detail.TotalTokens,
		},
		"accounting_version": record.Detail.TokenBreakdown.SchemaVersion,
		"token_breakdown":    record.Detail.TokenBreakdown,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := service.IngestRaw(context.Background(), raw); err != nil {
		return err
	}
	return nil
}

func pickAuth(request pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
	state.RLock()
	routingEnabled := state.routingEnabled
	state.RUnlock()
	if !routingEnabled {
		return pluginapi.SchedulerPickResponse{}, nil
	}
	candidates := append([]pluginapi.SchedulerAuthCandidate(nil), request.Candidates...)
	if len(candidates) == 0 {
		return pluginapi.SchedulerPickResponse{}, nil
	}
	bestPriority := candidates[0].Priority
	for _, candidate := range candidates[1:] {
		if candidate.Priority > bestPriority {
			bestPriority = candidate.Priority
		}
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Priority == bestPriority {
			filtered = append(filtered, candidate)
		}
	}
	if request.Stream && hasWebsocketCandidate(filtered) {
		websocket := filtered[:0]
		for _, candidate := range filtered {
			if candidateWebsocket(candidate) {
				websocket = append(websocket, candidate)
			}
		}
		filtered = websocket
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	providers := append([]string(nil), request.Providers...)
	sort.Strings(providers)
	providerKey := strings.TrimSpace(request.Provider)
	if providerKey == "" {
		providerKey = strings.Join(providers, ",")
	}
	key := fmt.Sprintf("plugin|%s|%s|%d|%t", strings.ToLower(providerKey), strings.ToLower(strings.TrimSpace(request.Model)), bestPriority, request.Stream)

	state.Lock()
	defer state.Unlock()
	strategy := state.strategy
	if strategy == "" {
		strategy = "round-robin"
	}
	index := 0
	previousCursor := state.cursors[key]
	var previousWeights map[string]int
	persistedCursor := ""
	if strategy == "fill-first" {
		index = 0
	} else if strategy == "weighted-round-robin" {
		if err := restoreWeightedCursorLocked(key, filtered); err != nil {
			return pluginapi.SchedulerPickResponse{}, err
		}
		previousWeights = cloneWeights(state.weighted[key])
		index = smoothWeightedPickLocked(key, filtered)
		encoded, err := json.Marshal(struct {
			Weights map[string]int `json:"weights"`
		}{Weights: state.weighted[key]})
		if err != nil {
			state.weighted[key] = previousWeights
			return pluginapi.SchedulerPickResponse{}, fmt.Errorf("encode weighted routing cursor: %w", err)
		}
		persistedCursor = string(encoded)
	} else {
		lastID, loaded := state.cursors[key]
		if !loaded && state.service != nil {
			cursor, ok, err := state.service.RoutingCursor(context.Background(), key)
			if err != nil {
				return pluginapi.SchedulerPickResponse{}, fmt.Errorf("restore routing cursor: %w", err)
			}
			if ok {
				lastID = cursor.LastAuthID
			}
			state.cursors[key] = lastID
		}
		previousCursor = lastID
		for candidateIndex, candidate := range filtered {
			if candidate.ID == lastID {
				index = (candidateIndex + 1) % len(filtered)
				break
			}
			if lastID != "" && candidate.ID > lastID {
				index = candidateIndex
				break
			}
		}
	}
	picked := filtered[index]
	state.cursors[key] = picked.ID
	if persistedCursor == "" {
		persistedCursor = picked.ID
	}
	if state.service != nil {
		if err := state.service.SetRoutingCursor(context.Background(), usage.RoutingCursorState{CursorKey: key, LastAuthID: persistedCursor, UpdatedAtMS: time.Now().UnixMilli()}); err != nil {
			state.cursors[key] = previousCursor
			if previousWeights != nil {
				state.weighted[key] = previousWeights
			}
			return pluginapi.SchedulerPickResponse{}, fmt.Errorf("persist routing cursor: %w", err)
		}
	}
	return pluginapi.SchedulerPickResponse{Handled: true, AuthID: picked.ID}, nil
}

func restoreWeightedCursorLocked(key string, candidates []pluginapi.SchedulerAuthCandidate) error {
	if state.weighted[key] != nil {
		return nil
	}
	weights := make(map[string]int, len(candidates))
	if state.service != nil {
		cursor, ok, err := state.service.RoutingCursor(context.Background(), key)
		if err != nil {
			return fmt.Errorf("restore weighted routing cursor: %w", err)
		}
		if ok {
			var payload struct {
				Weights map[string]int `json:"weights"`
			}
			if json.Unmarshal([]byte(cursor.LastAuthID), &payload) == nil {
				for _, candidate := range candidates {
					if weight, exists := payload.Weights[candidate.ID]; exists {
						weights[candidate.ID] = weight
					}
				}
			}
		}
	}
	state.weighted[key] = weights
	return nil
}

func cloneWeights(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	cloned := make(map[string]int, len(source))
	for id, weight := range source {
		cloned[id] = weight
	}
	return cloned
}

func candidateWebsocket(candidate pluginapi.SchedulerAuthCandidate) bool {
	value := candidate.Attributes["websockets"]
	if value == "" && candidate.Metadata != nil {
		value = fmt.Sprint(candidate.Metadata["websockets"])
	}
	parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
	return parsed
}

func hasWebsocketCandidate(candidates []pluginapi.SchedulerAuthCandidate) bool {
	for _, candidate := range candidates {
		if candidateWebsocket(candidate) {
			return true
		}
	}
	return false
}

func candidateWeight(candidate pluginapi.SchedulerAuthCandidate) int {
	value := candidate.Attributes["weight"]
	if value == "" && candidate.Metadata != nil {
		value = fmt.Sprint(candidate.Metadata["weight"])
	}
	weight, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || weight <= 0 {
		return 1
	}
	return weight
}

func smoothWeightedPickLocked(key string, candidates []pluginapi.SchedulerAuthCandidate) int {
	weights := state.weighted[key]
	if weights == nil {
		weights = make(map[string]int)
		state.weighted[key] = weights
	}
	total, selectedIndex, selectedWeight := 0, 0, 0
	for index, candidate := range candidates {
		weight := candidateWeight(candidate)
		total += weight
		weights[candidate.ID] += weight
		if index == 0 || weights[candidate.ID] > selectedWeight {
			selectedIndex, selectedWeight = index, weights[candidate.ID]
		}
	}
	for id := range weights {
		found := false
		for _, candidate := range candidates {
			if candidate.ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(weights, id)
		}
	}
	weights[candidates[selectedIndex].ID] -= total
	return selectedIndex
}

func managementRoutes() []managementRoute {
	methods := map[string][]string{
		http.MethodGet:    {"", "/export", "/status", "/events", "/aggregates", "/account", "/quota-cache", "/model-prices", "/model-price-rules", "/model-prices/sync-status", "/settings"},
		http.MethodPost:   {"/import", "/reset", "/model-prices/sync", "/model-prices/recalculate"},
		http.MethodPut:    {"/quota-cache", "/model-prices", "/model-price-rules", "/settings"},
		http.MethodDelete: {"/quota-cache", "/model-price-rules"},
	}
	routes := make([]managementRoute, 0, 21)
	for method, paths := range methods {
		for _, path := range paths {
			routes = append(routes, managementRoute{Method: method, Path: "/usage" + path, Description: "Plugin-owned observability API"})
		}
	}
	routes = append(routes, managementRoute{Method: http.MethodGet, Path: "/pro/observability/runtime", Description: "Return the plugin routing and persistence runtime."})
	routes = append(routes, managementRoute{Method: http.MethodGet, Path: "/pro/observability/migration/status", Description: "Return the automatic storage migration status."})
	return routes
}

func handleManagement(request pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	path := strings.TrimSpace(request.Path)
	if request.Method == http.MethodGet && path == "/v0/resource/plugins/pro-observability/ui" {
		return pluginapi.ManagementResponse{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Content-Type":            []string{"text/html; charset=utf-8"},
				"Content-Security-Policy": []string{"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-ancestors *"},
				"Cache-Control":           []string{"no-store"},
			},
			Body: append([]byte(nil), observabilityManagementPage...),
		}
	}
	if request.Method == http.MethodGet && path == "/v0/management/pro/observability/runtime" {
		return jsonManagementResponse(http.StatusOK, routingRuntimeSnapshot())
	}
	if path == "/v0/management/pro/observability/migration/status" && request.Method == http.MethodGet {
		state.RLock()
		service := state.service
		state.RUnlock()
		if service == nil {
			return jsonManagementResponse(http.StatusServiceUnavailable, map[string]any{"error": "usage_service_unavailable"})
		}
		return jsonManagementResponse(http.StatusOK, service.StorageMigration())
	}
	state.RLock()
	router := state.router
	state.RUnlock()
	if router == nil {
		return jsonManagementResponse(http.StatusServiceUnavailable, map[string]any{"error": "usage_service_unavailable"})
	}
	managementPrefix := "/v0/management/usage"
	if path != managementPrefix && !strings.HasPrefix(path, managementPrefix+"/") {
		return jsonManagementResponse(http.StatusNotFound, map[string]any{"error": "not_found"})
	}
	path = "/usage" + strings.TrimPrefix(path, managementPrefix)
	requestURL := path
	if encoded := request.Query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	httpRequest, err := http.NewRequest(request.Method, requestURL, bytes.NewReader(request.Body))
	if err != nil {
		return jsonManagementResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	httpRequest.Header = request.Headers.Clone()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httpRequest)
	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return jsonManagementResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	return pluginapi.ManagementResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: body}
}

func routingRuntimeSnapshot() map[string]any {
	state.RLock()
	defer state.RUnlock()
	storageMigration := usage.PluginStorageMigration{}
	if state.service != nil {
		storageMigration = state.service.StorageMigration()
	}
	return map[string]any{
		"routingEnabled":         state.routingEnabled,
		"routingStrategy":        state.strategy,
		"persistedCursors":       len(state.cursors),
		"runtimeStats":           len(state.stats),
		"pendingSelections":      len(state.selected),
		"usageServiceReady":      state.service != nil,
		"managementAPIReady":     state.router != nil,
		"storageOwner":           "plugin",
		"storageMigration":       storageMigration,
		"usageRecordContract":    state.usageContract,
		"usageRecordParity":      state.usageContract >= 2,
		"quotaCacheCapability":   true,
		"quotaCacheBackend":      "plugin",
		"runtimeStateCapability": true,
		"runtimeStateBackend":    "plugin",
		"streamTransportReady":   true,
	}
}

func jsonManagementResponse(status int, value any) pluginapi.ManagementResponse {
	body, _ := json.Marshal(value)
	return pluginapi.ManagementResponse{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: body}
}

func okEnvelope(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: result})
}

func errorEnvelope(code, message string, status int) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message, HTTPStatus: status}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr, response.len = ptr, C.size_t(len(raw))
}
