package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	methodPluginRegister     = "plugin.register"
	methodPluginReconfigure  = "plugin.reconfigure"
	methodPluginShutdown     = "plugin.shutdown"
	methodUsageHandle        = "usage.handle"
	methodManagementRegister = "management.register"
	methodManagementHandle   = "management.handle"
)

type pluginConfig struct {
	WriteEnabled bool
	DatabasePath string
}

type yamlConfigDocument struct {
	Plugins struct {
		Configs map[string]yamlPluginConfig `yaml:"configs"`
	} `yaml:"plugins"`
}

type yamlPluginConfig struct {
	WriteEnabled bool   `yaml:"write-enabled"`
	DatabasePath string `yaml:"database-path"`
}

type runtimeStatus struct {
	PluginID      string        `json:"pluginId"`
	Version       string        `json:"version"`
	WriteEnabled  bool          `json:"writeEnabled"`
	DatabasePath  string        `json:"databasePath"`
	StoreOpen     bool          `json:"storeOpen"`
	MigrationMode string        `json:"migrationMode"`
	LastError     string        `json:"lastError,omitempty"`
	Summary       *usageSummary `json:"summary,omitempty"`
}

type pluginRuntime struct {
	mu        sync.RWMutex
	config    pluginConfig
	store     *usageStore
	lastErrMu sync.RWMutex
	lastErr   string
}

var activeRuntime pluginRuntime

func dispatchMethod(method string, rawRequest []byte) ([]byte, error) {
	switch method {
	case methodPluginRegister, methodPluginReconfigure:
		var request lifecycleRequest
		if len(rawRequest) > 0 {
			if err := json.Unmarshal(rawRequest, &request); err != nil {
				return nil, fmt.Errorf("decode lifecycle request: %w", err)
			}
		}
		if err := activeRuntime.configure(request.ConfigYAML); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case methodUsageHandle:
		if err := activeRuntime.handleUsage(context.Background(), rawRequest); err != nil {
			return nil, err
		}
		return okEnvelope(struct{}{})
	case methodManagementRegister:
		return okEnvelope(managementRegistration{Routes: []managementRoute{{
			Method:      http.MethodGet,
			Path:        managementStatusPath,
			Description: "Reports the opt-in observability writer migration state.",
		}}})
	case methodManagementHandle:
		return handleManagement(rawRequest)
	case methodPluginShutdown:
		shutdownRuntime()
		return okEnvelope(struct{}{})
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginSchemaVersion,
		Metadata: registrationMetadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           pluginAuthor,
			GitHubRepository: pluginRepository,
		},
		Capabilities: registrationCapabilities{
			UsagePlugin:   true,
			ManagementAPI: true,
		},
	}
}

func parsePluginConfig(raw []byte) (pluginConfig, error) {
	databasePath := strings.TrimSpace(os.Getenv("USAGE_DB_PATH"))
	if databasePath == "" {
		databasePath = defaultDatabasePath
	}
	config := pluginConfig{DatabasePath: databasePath}
	if len(raw) == 0 {
		return config, nil
	}
	var scoped yamlPluginConfig
	if err := yaml.Unmarshal(raw, &scoped); err != nil {
		return pluginConfig{}, fmt.Errorf("decode plugin config: %w", err)
	}
	plugin := scoped
	var document yamlConfigDocument
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return pluginConfig{}, fmt.Errorf("decode plugin config document: %w", err)
	}
	if nested, ok := document.Plugins.Configs[pluginID]; ok {
		plugin = nested
	}
	config.WriteEnabled = plugin.WriteEnabled
	if path := strings.TrimSpace(plugin.DatabasePath); path != "" {
		config.DatabasePath = path
	}
	return config, nil
}

func (runtime *pluginRuntime) configure(raw []byte) error {
	config, err := parsePluginConfig(raw)
	if err != nil {
		return err
	}
	var nextStore *usageStore
	if config.WriteEnabled {
		nextStore, err = openUsageStore(config.DatabasePath)
		if err != nil {
			return fmt.Errorf("enable observability writer: %w", err)
		}
	}
	runtime.mu.Lock()
	previousStore := runtime.store
	runtime.config = config
	runtime.store = nextStore
	runtime.mu.Unlock()
	runtime.setLastError(nil)
	if previousStore != nil {
		if err := previousStore.close(); err != nil {
			return fmt.Errorf("close previous observability store: %w", err)
		}
	}
	return nil
}

func (runtime *pluginRuntime) handleUsage(ctx context.Context, raw []byte) error {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if !runtime.config.WriteEnabled || runtime.store == nil {
		return nil
	}
	event, err := usageEventFromRPC(raw, time.Now())
	if err == nil {
		_, err = runtime.store.insertEvent(ctx, event)
	}
	if err != nil {
		runtime.setLastError(err)
		return err
	}
	return nil
}

func (runtime *pluginRuntime) setLastError(err error) {
	runtime.lastErrMu.Lock()
	defer runtime.lastErrMu.Unlock()
	runtime.lastErr = ""
	if err != nil {
		runtime.lastErr = err.Error()
	}
}

func (runtime *pluginRuntime) status(ctx context.Context) runtimeStatus {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	status := runtimeStatus{
		PluginID:      pluginID,
		Version:       pluginVersion,
		WriteEnabled:  runtime.config.WriteEnabled,
		DatabasePath:  runtime.config.DatabasePath,
		StoreOpen:     runtime.store != nil,
		MigrationMode: "shadow-writer-disabled",
		LastError:     runtime.lastError(),
	}
	if runtime.config.WriteEnabled {
		status.MigrationMode = "opt-in-writer"
	}
	if runtime.store != nil {
		if summary, err := runtime.store.summary(ctx); err == nil {
			status.Summary = &summary
		} else if status.LastError == "" {
			status.LastError = err.Error()
		}
	}
	return status
}

func (runtime *pluginRuntime) lastError() string {
	runtime.lastErrMu.RLock()
	defer runtime.lastErrMu.RUnlock()
	return runtime.lastErr
}

func handleManagement(raw []byte) ([]byte, error) {
	var request managementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	path := strings.TrimRight(strings.TrimSpace(request.Path), "/")
	if method == http.MethodGet && (path == managementStatusPath || strings.HasSuffix(path, managementStatusPath)) {
		body, err := json.Marshal(activeRuntime.status(context.Background()))
		if err != nil {
			return nil, err
		}
		return okEnvelope(managementResponse{
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
		})
	}
	body, _ := json.Marshal(map[string]string{"error": "not_found"})
	return okEnvelope(managementResponse{
		StatusCode: http.StatusNotFound,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       body,
	})
}

func shutdownRuntime() {
	activeRuntime.mu.Lock()
	store := activeRuntime.store
	activeRuntime.store = nil
	activeRuntime.config = pluginConfig{}
	activeRuntime.mu.Unlock()
	activeRuntime.setLastError(nil)
	if store != nil {
		_ = store.close()
	}
}

func okEnvelope(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: result})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}
