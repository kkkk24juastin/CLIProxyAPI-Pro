package profeatures

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/oauthmodelpolicy/config"
	modelpolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/oauthmodelpolicy/policy"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/proxypool/config"
	proxyengine "github.com/router-for-me/CLIProxyAPI/v7/internal/proxypool/engine"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
)

const settingSchemaVersion = 1

type Runtime struct {
	mu             sync.RWMutex
	proxyConfig    proxyconfig.Config
	modelConfig    modelconfig.Config
	proxyEngine    *proxyengine.Engine
	modelEngine    *modelpolicy.Engine
	baseProxyURL   string
	proxyConfigErr string
	modelConfigErr string
	modelChange    func(context.Context)
	unregister     []func()
	closed         bool
}

type ModelPolicyStatus struct {
	Enabled        bool   `json:"enabled"`
	CacheTTL       string `json:"cacheTTL"`
	ResolveTimeout string `json:"resolveTimeout"`
	Providers      int    `json:"providers"`
	LastError      string `json:"lastError,omitempty"`
}

func New(ctx context.Context, configFilePath, baseProxyURL string) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	migrated, err := migrateLegacySettings(ctx, configFilePath)
	if err != nil {
		return nil, err
	}
	if migrated {
		baseProxyURL = baseProxyURLFromConfigFile(configFilePath, baseProxyURL)
	}
	proxyCfg, proxyErr := loadProxyConfig(ctx)
	modelCfg, modelErr := loadModelConfig(ctx)
	runtime := &Runtime{
		proxyConfig:  proxyCfg,
		modelConfig:  modelCfg,
		proxyEngine:  proxyengine.New(),
		modelEngine:  modelpolicy.New(),
		baseProxyURL: strings.TrimSpace(baseProxyURL),
	}
	if proxyErr != nil {
		runtime.proxyConfigErr = proxyErr.Error()
	}
	if modelErr != nil {
		runtime.modelConfigErr = modelErr.Error()
	}
	runtime.modelEngine.ApplyConfig(modelCfg)
	if proxyCfg.Enabled && proxyErr == nil {
		if err := runtime.proxyEngine.ApplyConfig(proxyCfg); err != nil {
			runtime.proxyConfigErr = err.Error()
		}
	}
	runtime.refreshProxyOverrideLocked()
	runtime.unregister = append(runtime.unregister,
		embeddedusage.RegisterProSettingConsumer(embeddedusage.ProSettingNamespaceProxyPool, runtime.applyImportedProxySetting),
		embeddedusage.RegisterProSettingConsumer(embeddedusage.ProSettingNamespaceOAuthModelPolicy, runtime.applyImportedModelSetting),
	)
	return runtime, nil
}

func loadProxyConfig(ctx context.Context) (proxyconfig.Config, error) {
	cfg := proxyconfig.Default()
	item, found, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceProxyPool)
	if err != nil || !found {
		return cfg, err
	}
	if item.SchemaVersion != settingSchemaVersion {
		return cfg, fmt.Errorf("unsupported proxy pool schema version %d", item.SchemaVersion)
	}
	return proxyconfig.Parse(item.Settings)
}

func loadModelConfig(ctx context.Context) (modelconfig.Config, error) {
	cfg, _ := modelconfig.Parse(nil)
	item, found, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceOAuthModelPolicy)
	if err != nil || !found {
		return cfg, err
	}
	if item.SchemaVersion != settingSchemaVersion {
		return cfg, fmt.Errorf("unsupported OAuth model policy schema version %d", item.SchemaVersion)
	}
	return modelconfig.Parse(item.Settings)
}

func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	unregister := append([]func(){}, r.unregister...)
	r.unregister = nil
	engine := r.proxyEngine
	r.proxyEngine = nil
	proxyutil.ClearRuntimeProxyOverride()
	r.mu.Unlock()
	for _, unregisterOne := range unregister {
		unregisterOne()
	}
	if engine != nil {
		engine.Close()
	}
}

func (r *Runtime) SetBaseProxyURL(value string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.baseProxyURL = strings.TrimSpace(value)
	r.refreshProxyOverrideLocked()
	r.mu.Unlock()
}

func (r *Runtime) BaseProxyURL() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.baseProxyURL
}

func (r *Runtime) SetModelPolicyChangeHandler(handler func(context.Context)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.modelChange = handler
	r.mu.Unlock()
}

func (r *Runtime) refreshProxyOverrideLocked() {
	if r.closed || !r.proxyConfig.Enabled || !r.proxyConfig.TakeoverEnabled || r.proxyEngine == nil || !r.proxyEngine.Status().Ready {
		proxyutil.ClearRuntimeProxyOverride()
		return
	}
	proxyutil.SetRuntimeProxyOverride(r.baseProxyURL, "socks5://"+r.proxyConfig.Listen)
}

func (r *Runtime) ProxyConfig() proxyconfig.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.proxyConfig
}

func (r *Runtime) ModelConfig() modelconfig.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelConfig
}

func persistSetting(ctx context.Context, namespace string, raw []byte) error {
	return embeddedusage.SetProSetting(ctx, embeddedusage.ProSetting{
		Namespace: namespace, SchemaVersion: settingSchemaVersion, Settings: raw,
	})
}

func (r *Runtime) UpdateProxyConfig(ctx context.Context, cfg proxyconfig.Config) error {
	if r == nil {
		return fmt.Errorf("Pro feature runtime is unavailable")
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		return err
	}
	raw, err := proxyconfig.Marshal(cfg)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("Pro feature runtime is closed")
	}
	oldConfig := r.proxyConfig
	oldRaw, _ := proxyconfig.Marshal(oldConfig)
	if err := persistSetting(ctx, embeddedusage.ProSettingNamespaceProxyPool, raw); err != nil {
		return err
	}
	if cfg.Enabled {
		if r.proxyEngine == nil {
			r.proxyEngine = proxyengine.New()
		}
		if err := r.proxyEngine.ApplyConfig(cfg); err != nil {
			_ = persistSetting(context.Background(), embeddedusage.ProSettingNamespaceProxyPool, oldRaw)
			r.proxyConfigErr = err.Error()
			return err
		}
	} else {
		oldEngine := r.proxyEngine
		r.proxyEngine = proxyengine.New()
		if oldEngine != nil {
			go oldEngine.Close()
		}
	}
	r.proxyConfig = cfg
	r.proxyConfigErr = ""
	r.refreshProxyOverrideLocked()
	return nil
}

func (r *Runtime) UpdateModelConfig(ctx context.Context, cfg modelconfig.Config) error {
	if r == nil {
		return fmt.Errorf("Pro feature runtime is unavailable")
	}
	raw, err := modelconfig.Marshal(cfg)
	if err != nil {
		return err
	}
	normalized, err := modelconfig.Parse(raw)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("Pro feature runtime is closed")
	}
	if err := persistSetting(ctx, embeddedusage.ProSettingNamespaceOAuthModelPolicy, raw); err != nil {
		r.mu.Unlock()
		return err
	}
	r.modelConfig = normalized
	r.modelConfigErr = ""
	r.modelEngine.ApplyConfig(normalized)
	handler := r.modelChange
	r.mu.Unlock()
	if handler != nil {
		handler(ctx)
	}
	return nil
}

func (r *Runtime) applyImportedProxySetting(ctx context.Context, item embeddedusage.ProSetting) error {
	if item.SchemaVersion != settingSchemaVersion {
		return fmt.Errorf("unsupported proxy pool schema version %d", item.SchemaVersion)
	}
	cfg, err := proxyconfig.Parse(item.Settings)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cfg.Enabled {
		if r.proxyEngine == nil {
			r.proxyEngine = proxyengine.New()
		}
		if err := r.proxyEngine.ApplyConfig(cfg); err != nil {
			return err
		}
	} else if r.proxyEngine != nil {
		oldEngine := r.proxyEngine
		r.proxyEngine = proxyengine.New()
		go oldEngine.Close()
	}
	r.proxyConfig = cfg
	r.proxyConfigErr = ""
	r.refreshProxyOverrideLocked()
	return nil
}

func (r *Runtime) applyImportedModelSetting(ctx context.Context, item embeddedusage.ProSetting) error {
	if item.SchemaVersion != settingSchemaVersion {
		return fmt.Errorf("unsupported OAuth model policy schema version %d", item.SchemaVersion)
	}
	cfg, err := modelconfig.Parse(item.Settings)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("Pro feature runtime is closed")
	}
	r.modelConfig = cfg
	r.modelConfigErr = ""
	r.modelEngine.ApplyConfig(cfg)
	handler := r.modelChange
	r.mu.Unlock()
	if handler != nil {
		handler(ctx)
	}
	return nil
}

func (r *Runtime) ProxyStatus() proxyengine.Status {
	r.mu.RLock()
	engine := r.proxyEngine
	configErr := r.proxyConfigErr
	r.mu.RUnlock()
	if engine == nil {
		return proxyengine.Status{LastError: configErr}
	}
	status := engine.Status()
	if status.LastError == "" {
		status.LastError = configErr
	}
	return status
}

func (r *Runtime) ModelStatus() ModelPolicyStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return ModelPolicyStatus{
		Enabled: r.modelConfig.Enabled, CacheTTL: r.modelConfig.CacheTTL.String(),
		ResolveTimeout: r.modelConfig.ResolveTimeout.String(), Providers: len(r.modelConfig.Providers), LastError: r.modelConfigErr,
	}
}

func (r *Runtime) ProbeProxy(ctx context.Context, nodeID, proxyURL, testURL string) proxyengine.ProbeResult {
	r.mu.RLock()
	engine := r.proxyEngine
	r.mu.RUnlock()
	if engine == nil {
		return proxyengine.ProbeResult{NodeID: nodeID, Error: "proxy pool is not initialized", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	if strings.TrimSpace(proxyURL) != "" {
		return engine.ProbeDraft(ctx, nodeID, proxyURL, testURL)
	}
	return engine.Probe(ctx, nodeID, testURL)
}

func (r *Runtime) ProbeAllProxies(ctx context.Context, concurrency int) []proxyengine.ProbeResult {
	r.mu.RLock()
	engine := r.proxyEngine
	r.mu.RUnlock()
	if engine == nil {
		return []proxyengine.ProbeResult{}
	}
	return engine.ProbeAll(ctx, concurrency)
}

func (r *Runtime) RecoverProxy(nodeID string) error {
	r.mu.RLock()
	engine := r.proxyEngine
	r.mu.RUnlock()
	if engine == nil {
		return fmt.Errorf("proxy pool is not initialized")
	}
	return engine.Recover(nodeID)
}

func (r *Runtime) ResetProxyStats() {
	r.mu.RLock()
	engine := r.proxyEngine
	r.mu.RUnlock()
	if engine != nil {
		engine.ResetStats()
	}
}

func (r *Runtime) FilterModels(ctx context.Context, hostCfg *internalconfig.Config, auth *coreauth.Auth, models []*registry.ModelInfo) []*registry.ModelInfo {
	if r == nil || auth == nil || len(models) == 0 {
		return models
	}
	r.mu.RLock()
	engine := r.modelEngine
	r.mu.RUnlock()
	if engine == nil {
		return models
	}
	inputModels := make([]modelpolicy.ModelInfo, 0, len(models))
	for _, model := range models {
		if model != nil {
			inputModels = append(inputModels, modelpolicy.ModelInfo{ID: model.ID})
		}
	}
	result := engine.Filter(ctx, modelpolicy.Input{
		AuthID: auth.ID, AuthProvider: auth.Provider, AuthKind: auth.AuthKind(),
		StorageJSON: storageJSONFromAuth(auth), Metadata: auth.Metadata, Attributes: auth.Attributes, Models: inputModels,
		HTTPDo: func(callCtx context.Context, req modelpolicy.HTTPRequest) (modelpolicy.HTTPResponse, error) {
			return doPolicyHTTP(callCtx, hostCfg, auth, req)
		},
	})
	if len(result.ExcludedModelIDs) == 0 {
		return models
	}
	blocked := make(map[string]struct{}, len(result.ExcludedModelIDs))
	for _, id := range result.ExcludedModelIDs {
		blocked[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	filtered := make([]*registry.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		if _, found := blocked[strings.ToLower(strings.TrimSpace(model.ID))]; !found {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func storageJSONFromAuth(auth *coreauth.Auth) []byte {
	if auth == nil {
		return nil
	}
	if rawProvider, ok := auth.Storage.(interface{ RawJSON() []byte }); ok {
		return bytes.Clone(rawProvider.RawJSON())
	}
	raw, _ := json.Marshal(auth.Metadata)
	return raw
}

func doPolicyHTTP(ctx context.Context, cfg *internalconfig.Config, auth *coreauth.Auth, req modelpolicy.HTTPRequest) (modelpolicy.HTTPResponse, error) {
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return modelpolicy.HTTPResponse{}, err
	}
	for key, values := range req.Headers {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	client := helps.NewProxyAwareHTTPClient(ctx, cfg, auth, 0)
	resp, err := client.Do(httpReq)
	if err != nil {
		helpersRecordPolicyError(ctx, cfg, err)
		return modelpolicy.HTTPResponse{}, err
	}
	defer resp.Body.Close()
	helpersRecordPolicyMetadata(ctx, cfg, resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		helpersRecordPolicyError(ctx, cfg, err)
		return modelpolicy.HTTPResponse{}, err
	}
	if len(body) > 0 {
		helps.AppendAPIResponseChunk(ctx, cfg, body)
	}
	headers := make(map[string][]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = append([]string(nil), values...)
	}
	return modelpolicy.HTTPResponse{StatusCode: resp.StatusCode, Headers: headers, Body: body}, nil
}

func helpersRecordPolicyMetadata(ctx context.Context, cfg *internalconfig.Config, resp *http.Response) {
	if resp != nil {
		helps.RecordAPIResponseMetadata(ctx, cfg, resp.StatusCode, resp.Header.Clone())
	}
}

func helpersRecordPolicyError(ctx context.Context, cfg *internalconfig.Config, err error) {
	if err != nil {
		helps.RecordAPIResponseError(ctx, cfg, err)
	}
}
