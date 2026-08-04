package management

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
	prorouting "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/routing"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	routingPolicyUsagePluginName   = "pro-routing-request-protection"
	routingProtectionOwner         = prorouting.ProtectionOwner
	routingProtectionModeObserve   = prorouting.ModeObserve
	routingProtectionModeEnforce   = prorouting.ModeEnforce
	routingProtectionMaxEvents     = 100
	routingProtectionSchemaVersion = 1
)

var routingPolicyControllers sync.Map

var routingProtectionProviders = []string{
	"antigravity",
	"xai",
	"codex",
	"gemini-cli",
	"gemini",
	"gemini-interactions",
	"vertex",
	"aistudio",
	"claude",
	"kimi",
}

type routingPolicyController struct {
	h                     *Handler
	mu                    sync.Mutex
	confirmations         *prorouting.ConfirmationTracker
	events                []routingProtectionEvent
	lifecycleMu           sync.Mutex
	usageWG               sync.WaitGroup
	stopped               bool
	configMu              sync.RWMutex
	requestProtection     routingRequestProtectionConfig
	proSettingsUnregister func()
}

type routingPolicyGlobalSettings struct {
	Strategy                      string `json:"strategy"`
	SessionAffinity               bool   `json:"sessionAffinity"`
	SessionAffinityTTL            string `json:"sessionAffinityTTL"`
	RequestRetry                  int    `json:"requestRetry"`
	MaxRetryCredentials           int    `json:"maxRetryCredentials"`
	MaxRetryInterval              int    `json:"maxRetryInterval"`
	CoolingEnabled                bool   `json:"coolingEnabled"`
	SaveCooldownStatus            bool   `json:"saveCooldownStatus"`
	TransientErrorCooldownSeconds int    `json:"transientErrorCooldownSeconds"`
	QuotaSwitchProject            bool   `json:"quotaSwitchProject"`
	QuotaSwitchPreviewModel       bool   `json:"quotaSwitchPreviewModel"`
	QuotaAntigravityCredits       bool   `json:"quotaAntigravityCredits"`
	CodexIdentityConfuse          bool   `json:"codexIdentityConfuse"`
}

type routingRequestProtectionConfig = prorouting.RequestProtectionConfig

type routingProtectionProviderPolicy = prorouting.ProviderPolicy

type routingPolicyResponse struct {
	Global             routingPolicyGlobalSettings      `json:"global"`
	RequestProtection  routingRequestProtectionConfig   `json:"requestProtection"`
	AvailableProviders []string                         `json:"availableProviders"`
	Active             []routingProtectionActiveAccount `json:"active"`
	RecentEvents       []routingProtectionEvent         `json:"recentEvents"`
}

type routingProtectionActiveAccount struct {
	Provider    string `json:"provider"`
	AuthID      string `json:"authId"`
	AuthIndex   string `json:"authIndex"`
	FileName    string `json:"fileName"`
	StatusCode  int    `json:"statusCode"`
	Reason      string `json:"reason"`
	TriggeredAt int64  `json:"triggeredAt"`
	ReleaseAt   int64  `json:"releaseAt"`
}

type routingProtectionEvent struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	AuthID      string `json:"authId"`
	AuthIndex   string `json:"authIndex"`
	FileName    string `json:"fileName"`
	StatusCode  int    `json:"statusCode"`
	Mode        string `json:"mode"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	Count       int    `json:"count"`
	Required    int    `json:"required"`
	TriggeredAt int64  `json:"triggeredAt"`
	ReleaseAt   int64  `json:"releaseAt"`
}

type routingPolicyReleaseRequest struct {
	AuthIndex string `json:"authIndex"`
}

func startRoutingPolicyController(h *Handler) {
	if h == nil {
		return
	}
	requestProtection, err := loadRoutingRequestProtectionConfig(h)
	if err != nil {
		log.WithError(err).Warn("failed to load Pro routing request protection settings")
		requestProtection = defaultRoutingRequestProtectionConfig()
	}
	controller := &routingPolicyController{
		h:                 h,
		confirmations:     prorouting.NewConfirmationTracker(),
		requestProtection: requestProtection,
	}
	actual, loaded := routingPolicyControllers.LoadOrStore(h, controller)
	if loaded {
		controller, _ = actual.(*routingPolicyController)
	}
	if controller == nil {
		return
	}
	if !loaded {
		controller.proSettingsUnregister = embeddedusage.RegisterProSettingConsumer(
			embeddedusage.ProSettingNamespaceRoutingRequestProtection,
			controller.applyImportedProSetting,
		)
	}
	coreusage.RegisterNamedPlugin(routingPolicyUsagePluginName, controller)
	if !loaded {
		if h.lifecycleContext == nil {
			return
		}
		h.lifecycleWG.Add(1)
		go func() {
			defer h.lifecycleWG.Done()
			controller.reconcileLoop(h.lifecycleContext)
		}()
	}
}

func stopRoutingPolicyController(h *Handler) {
	if h == nil {
		return
	}
	value, loaded := routingPolicyControllers.LoadAndDelete(h)
	if !loaded {
		return
	}
	controller, _ := value.(*routingPolicyController)
	if controller != nil {
		controller.stop()
	}
	if controller != nil && controller.proSettingsUnregister != nil {
		controller.proSettingsUnregister()
	}
}

func (c *routingPolicyController) beginUsage() bool {
	if c == nil {
		return false
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.stopped {
		return false
	}
	c.usageWG.Add(1)
	return true
}

func (c *routingPolicyController) stop() {
	if c == nil {
		return
	}
	c.lifecycleMu.Lock()
	c.stopped = true
	c.lifecycleMu.Unlock()
	coreusage.UnregisterNamedPlugin(routingPolicyUsagePluginName, c)
	c.usageWG.Wait()
}

func routingPolicyControllerForHandler(h *Handler) *routingPolicyController {
	value, ok := routingPolicyControllers.Load(h)
	if !ok {
		return nil
	}
	controller, _ := value.(*routingPolicyController)
	return controller
}

func (c *routingPolicyController) HandleUsage(ctx context.Context, record coreusage.Record) {
	if !c.beginUsage() {
		return
	}
	defer c.usageWG.Done()
	if c.h == nil {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(record.Provider))
	policyConfig := c.requestProtectionConfig()
	policy, ok := policyConfig.Providers[provider]
	if !policyConfig.Enabled || !ok || !policy.Enabled {
		return
	}
	auth := c.authForRecord(record)
	if auth == nil {
		return
	}
	if !record.Failed {
		c.clearConfirmations(auth.ID, provider)
		return
	}
	statusCode := record.Fail.StatusCode
	if statusCode <= 0 || !routingProtectionStatusMatches(policy.StatusCodes, statusCode) {
		return
	}
	if statusCode == http.StatusTooManyRequests && policy.RequireQuotaEvidence && !routingProtectionHasQuotaEvidence(record) {
		return
	}
	if auth.Disabled && !routingProtectionOwned(auth) {
		return
	}

	now := time.Now()
	confirmed, count, required := c.confirm(auth.ID, provider, statusCode, policy, now)
	releaseAt := routingProtectionReleaseAt(record, policy, now)
	mode := normalizeRoutingProtectionMode(policyConfig.Mode)
	event := routingProtectionEvent{
		ID:          fmt.Sprintf("%d-%s-%s", now.UnixNano(), provider, auth.Index),
		Provider:    provider,
		AuthID:      auth.ID,
		AuthIndex:   auth.Index,
		FileName:    routingProtectionAuthFileName(auth),
		StatusCode:  statusCode,
		Mode:        mode,
		Action:      "observe",
		Reason:      routingProtectionReason(record),
		Count:       count,
		Required:    required,
		TriggeredAt: now.UnixMilli(),
	}
	if !releaseAt.IsZero() {
		event.ReleaseAt = releaseAt.UnixMilli()
	}
	if !confirmed {
		event.Action = "pending"
		c.appendEvent(event)
		return
	}
	if mode != routingProtectionModeEnforce {
		c.appendEvent(event)
		return
	}
	if err := c.disableAuth(ctx, auth, event); err != nil {
		event.Action = "error"
		event.Reason = err.Error()
		c.appendEvent(event)
		log.WithError(err).WithFields(log.Fields{"provider": provider, "auth_index": auth.Index, "status": statusCode}).Warn("routing request protection failed to disable auth")
		return
	}
	event.Action = "disabled"
	c.clearConfirmations(auth.ID, provider)
	c.appendEvent(event)
	log.WithFields(log.Fields{"provider": provider, "auth_index": auth.Index, "status": statusCode, "release_at": event.ReleaseAt}).Info("routing request protection disabled auth")
}

func (c *routingPolicyController) requestProtectionConfig() routingRequestProtectionConfig {
	if c == nil {
		return defaultRoutingRequestProtectionConfig()
	}
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return normalizeRoutingRequestProtectionConfig(c.requestProtection)
}

func (c *routingPolicyController) setRequestProtectionConfig(value routingRequestProtectionConfig) {
	if c == nil {
		return
	}
	c.configMu.Lock()
	c.requestProtection = normalizeRoutingRequestProtectionConfig(value)
	c.configMu.Unlock()
}

func (c *routingPolicyController) applyImportedProSetting(_ context.Context, item embeddedusage.ProSetting) error {
	value, err := decodeRoutingRequestProtectionSetting(item)
	if err != nil {
		return err
	}
	c.setRequestProtectionConfig(value)
	return nil
}

func (c *routingPolicyController) authForRecord(record coreusage.Record) *coreauth.Auth {
	if c == nil || c.h == nil || c.h.authManager == nil {
		return nil
	}
	if authID := strings.TrimSpace(record.AuthID); authID != "" {
		if auth, ok := c.h.authManager.GetByID(authID); ok {
			auth.EnsureIndex()
			return auth
		}
	}
	if authIndex := strings.TrimSpace(record.AuthIndex); authIndex != "" {
		if auth := c.h.authByIndex(authIndex); auth != nil {
			auth.EnsureIndex()
			return auth
		}
	}
	return nil
}

func (c *routingPolicyController) confirm(authID, provider string, statusCode int, policy routingProtectionProviderPolicy, now time.Time) (bool, int, int) {
	if c == nil {
		return false, 0, policy.Confirmations
	}
	return c.confirmations.Confirm(authID, provider, statusCode, policy, now)
}

func (c *routingPolicyController) clearConfirmations(authID, provider string) {
	if c != nil {
		c.confirmations.Clear(authID, provider)
	}
}

func (c *routingPolicyController) disableAuth(ctx context.Context, auth *coreauth.Auth, event routingProtectionEvent) error {
	if auth == nil {
		return fmt.Errorf("auth not found")
	}
	return c.h.updateProAuth(ctx, auth.Index, func(updated *coreauth.Auth) {
		if updated == nil || (updated.Disabled && !routingProtectionOwned(updated)) {
			return
		}
		setProAuthDisabledState(updated, true)
		updated.StatusMessage = fmt.Sprintf("disabled by routing policy after HTTP %d", event.StatusCode)
		if updated.Metadata == nil {
			updated.Metadata = make(map[string]any)
		}
		updated.Metadata[routingProtectionMetadataKey] = map[string]any{
			"owner":        routingProtectionOwner,
			"provider":     event.Provider,
			"status_code":  event.StatusCode,
			"reason":       event.Reason,
			"triggered_at": event.TriggeredAt,
			"release_at":   event.ReleaseAt,
		}
	})
}

func (c *routingPolicyController) releaseAuth(ctx context.Context, auth *coreauth.Auth) error {
	if auth == nil {
		return fmt.Errorf("auth not found")
	}
	return c.h.updateProAuth(ctx, auth.Index, func(updated *coreauth.Auth) {
		if updated == nil || !routingProtectionOwned(updated) {
			return
		}
		setProAuthDisabledState(updated, false)
		clearRoutingProtectionOwnership(updated)
	})
}

func (c *routingPolicyController) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.reconcile(now)
		}
	}
}

func (c *routingPolicyController) reconcile(now time.Time) {
	if c == nil || c.h == nil || c.h.authManager == nil {
		return
	}
	for _, auth := range c.h.authManager.List() {
		if auth == nil || !routingProtectionOwned(auth) {
			continue
		}
		auth.EnsureIndex()
		metadata := routingProtectionMetadata(auth)
		releaseAt := routingProtectionMetadataInt64(metadata, "release_at")
		if !auth.Disabled {
			_ = c.h.updateProAuth(context.Background(), auth.Index, func(updated *coreauth.Auth) {
				if updated != nil && updated.Metadata != nil {
					delete(updated.Metadata, routingProtectionMetadataKey)
				}
			})
			continue
		}
		if releaseAt <= 0 || now.UnixMilli() < releaseAt {
			continue
		}
		if err := c.releaseAuth(context.Background(), auth); err != nil {
			log.WithError(err).WithField("auth_index", auth.Index).Warn("routing request protection failed to auto-enable auth")
			continue
		}
		c.appendEvent(routingProtectionEvent{
			ID:          fmt.Sprintf("%d-release-%s", now.UnixNano(), auth.Index),
			Provider:    strings.ToLower(strings.TrimSpace(auth.Provider)),
			AuthID:      auth.ID,
			AuthIndex:   auth.Index,
			FileName:    routingProtectionAuthFileName(auth),
			Mode:        routingProtectionModeEnforce,
			Action:      "released",
			Reason:      "automatic release time reached",
			TriggeredAt: now.UnixMilli(),
		})
	}
}

func (c *routingPolicyController) appendEvent(event routingProtectionEvent) {
	c.mu.Lock()
	c.events = append([]routingProtectionEvent{event}, c.events...)
	if len(c.events) > routingProtectionMaxEvents {
		c.events = c.events[:routingProtectionMaxEvents]
	}
	c.mu.Unlock()
}

func (c *routingPolicyController) recentEvents() []routingProtectionEvent {
	if c == nil {
		return []routingProtectionEvent{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]routingProtectionEvent{}, c.events...)
}

func routingProtectionStatusMatches(values []int, status int) bool {
	return prorouting.StatusMatches(values, status)
}

func routingProtectionHasQuotaEvidence(record coreusage.Record) bool {
	return prorouting.HasQuotaEvidence(routingProtectionFailure(record))
}

func routingProtectionReleaseAt(record coreusage.Record, policy routingProtectionProviderPolicy, now time.Time) time.Time {
	return prorouting.ReleaseAt(routingProtectionFailure(record), policy, now)
}

func routingProtectionReason(record coreusage.Record) string {
	return prorouting.Reason(routingProtectionFailure(record))
}

func routingProtectionFailure(record coreusage.Record) prorouting.UsageFailure {
	return prorouting.UsageFailure{
		StatusCode: record.Fail.StatusCode,
		Headers:    record.ResponseHeaders,
		Body:       record.Fail.Body,
	}
}

func routingProtectionMetadata(auth *coreauth.Auth) map[string]any {
	if auth == nil {
		return nil
	}
	return prorouting.ProtectionMetadata(auth.Metadata)
}

func routingProtectionOwned(auth *coreauth.Auth) bool {
	return auth != nil && prorouting.ProtectionOwned(auth.Metadata)
}

func routingProtectionMetadataInt64(metadata map[string]any, key string) int64 {
	return prorouting.MetadataInt64(metadata, key)
}

func defaultRoutingRequestProtectionConfig() routingRequestProtectionConfig {
	return normalizeRoutingRequestProtectionConfig(routingRequestProtectionConfig{})
}

func decodeRoutingRequestProtectionSetting(item embeddedusage.ProSetting) (routingRequestProtectionConfig, error) {
	if item.Namespace != embeddedusage.ProSettingNamespaceRoutingRequestProtection {
		return routingRequestProtectionConfig{}, fmt.Errorf("unexpected Pro setting namespace %q", item.Namespace)
	}
	if item.SchemaVersion != routingProtectionSchemaVersion {
		return routingRequestProtectionConfig{}, fmt.Errorf("unsupported routing request protection schema version %d", item.SchemaVersion)
	}
	var value routingRequestProtectionConfig
	if err := json.Unmarshal(item.Settings, &value); err != nil {
		return routingRequestProtectionConfig{}, err
	}
	return normalizeRoutingRequestProtectionConfig(value), nil
}

func loadRoutingRequestProtectionConfig(h *Handler) (routingRequestProtectionConfig, error) {
	ctx := context.Background()
	item, found, err := embeddedusage.GetProSetting(ctx, embeddedusage.ProSettingNamespaceRoutingRequestProtection)
	if err != nil {
		return routingRequestProtectionConfig{}, err
	}
	if found {
		value, err := decodeRoutingRequestProtectionSetting(item)
		if err != nil {
			return routingRequestProtectionConfig{}, err
		}
		if removed, removeErr := removeLegacyRoutingRequestProtectionConfig(h.configFilePath); removeErr != nil {
			log.WithError(removeErr).Warn("failed to remove migrated routing request protection from config.yaml")
		} else if removed {
			log.Info("removed legacy routing request protection from config.yaml")
		}
		return value, nil
	}

	legacy, found, err := readLegacyRoutingRequestProtectionConfig(h.configFilePath)
	if err != nil {
		return routingRequestProtectionConfig{}, err
	}
	if !found {
		return defaultRoutingRequestProtectionConfig(), nil
	}
	legacy = normalizeRoutingRequestProtectionConfig(legacy)
	raw, err := json.Marshal(legacy)
	if err != nil {
		return routingRequestProtectionConfig{}, err
	}
	if err := embeddedusage.SetProSetting(ctx, embeddedusage.ProSetting{
		Namespace:     embeddedusage.ProSettingNamespaceRoutingRequestProtection,
		SchemaVersion: routingProtectionSchemaVersion,
		Settings:      raw,
	}); err != nil {
		return routingRequestProtectionConfig{}, err
	}
	if _, err := removeLegacyRoutingRequestProtectionConfig(h.configFilePath); err != nil {
		log.WithError(err).Warn("migrated routing request protection but failed to remove legacy config.yaml node")
	}
	return legacy, nil
}

func readLegacyRoutingRequestProtectionConfig(configFile string) (routingRequestProtectionConfig, bool, error) {
	if strings.TrimSpace(configFile) == "" {
		return routingRequestProtectionConfig{}, false, nil
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return routingRequestProtectionConfig{}, false, nil
		}
		return routingRequestProtectionConfig{}, false, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return routingRequestProtectionConfig{}, false, err
	}
	node := routingRequestProtectionYAMLNode(&root)
	if node == nil {
		return routingRequestProtectionConfig{}, false, nil
	}
	var value routingRequestProtectionConfig
	if err := node.Decode(&value); err != nil {
		return routingRequestProtectionConfig{}, false, err
	}
	return value, true, nil
}

func removeLegacyRoutingRequestProtectionConfig(configFile string) (bool, error) {
	if strings.TrimSpace(configFile) == "" {
		return false, nil
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return false, fmt.Errorf("invalid yaml document structure")
	}
	top := root.Content[0]
	routingIndex := yamlMappingKeyIndex(top, "routing")
	if routingIndex < 0 || routingIndex+1 >= len(top.Content) {
		return false, nil
	}
	routing := top.Content[routingIndex+1]
	requestProtectionIndex := yamlMappingKeyIndex(routing, "request-protection")
	if requestProtectionIndex < 0 {
		return false, nil
	}
	routing.Content = append(routing.Content[:requestProtectionIndex], routing.Content[requestProtectionIndex+2:]...)
	if len(routing.Content) == 0 {
		top.Content = append(top.Content[:routingIndex], top.Content[routingIndex+2:]...)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return false, err
	}
	if err := enc.Close(); err != nil {
		return false, err
	}
	return true, os.WriteFile(configFile, config.NormalizeCommentIndentation(buf.Bytes()), 0o600)
}

func routingRequestProtectionYAMLNode(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	top := root.Content[0]
	routingIndex := yamlMappingKeyIndex(top, "routing")
	if routingIndex < 0 || routingIndex+1 >= len(top.Content) {
		return nil
	}
	routing := top.Content[routingIndex+1]
	requestProtectionIndex := yamlMappingKeyIndex(routing, "request-protection")
	if requestProtectionIndex < 0 || requestProtectionIndex+1 >= len(routing.Content) {
		return nil
	}
	return routing.Content[requestProtectionIndex+1]
}

func yamlMappingKeyIndex(node *yaml.Node, key string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return -1
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index] != nil && node.Content[index].Value == key {
			return index
		}
	}
	return -1
}

func normalizeRoutingRequestProtectionConfig(input routingRequestProtectionConfig) routingRequestProtectionConfig {
	return prorouting.NormalizeConfig(input, routingProtectionProviders)
}

func normalizeRoutingProtectionMode(value string) string {
	return prorouting.NormalizeMode(value)
}

func (h *Handler) RegisterRoutingPolicyRoutes(group *gin.RouterGroup) {
	group.GET("/routing-policy", h.GetRoutingPolicy)
	group.PATCH("/routing-policy/upstream", h.PatchRoutingPolicyUpstream)
	group.PUT("/routing-policy/request-protection", h.PutRoutingRequestProtection)
	// Keep the combined endpoint for older management clients. New clients use the split writes above.
	group.PUT("/routing-policy", h.PutRoutingPolicy)
	group.PATCH("/routing-policy", h.PutRoutingPolicy)
	group.POST("/routing-policy/release", h.ReleaseRoutingProtectedAuth)
}

func (h *Handler) GetRoutingPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, h.routingPolicyResponse())
}

func (h *Handler) PutRoutingPolicy(c *gin.Context) {
	var request struct {
		Global            routingPolicyGlobalSettings    `json:"global"`
		RequestProtection routingRequestProtectionConfig `json:"requestProtection"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	global, err := normalizeRoutingPolicyGlobalSettings(request.Global)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.persistRoutingPolicyGlobal(c, global) {
		return
	}
	if !h.persistRoutingRequestProtection(c, request.RequestProtection) {
		return
	}
	c.JSON(http.StatusOK, h.routingPolicyResponse())
}

func (h *Handler) PatchRoutingPolicyUpstream(c *gin.Context) {
	var request struct {
		Global routingPolicyGlobalSettings `json:"global"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	global, err := normalizeRoutingPolicyGlobalSettings(request.Global)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.persistRoutingPolicyGlobal(c, global) {
		return
	}
	c.JSON(http.StatusOK, h.routingPolicyResponse())
}

func (h *Handler) PutRoutingRequestProtection(c *gin.Context) {
	var request struct {
		RequestProtection routingRequestProtectionConfig `json:"requestProtection"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if !h.persistRoutingRequestProtection(c, request.RequestProtection) {
		return
	}
	c.JSON(http.StatusOK, h.routingPolicyResponse())
}

func normalizeRoutingPolicyGlobalSettings(input routingPolicyGlobalSettings) (routingPolicyGlobalSettings, error) {
	input.Strategy = strings.ToLower(strings.TrimSpace(input.Strategy))
	if input.Strategy != "fill-first" {
		input.Strategy = "round-robin"
	}
	input.SessionAffinityTTL = strings.TrimSpace(input.SessionAffinityTTL)
	if input.SessionAffinityTTL == "" {
		input.SessionAffinityTTL = "1h"
	}
	if _, err := time.ParseDuration(input.SessionAffinityTTL); err != nil {
		return routingPolicyGlobalSettings{}, fmt.Errorf("invalid session affinity TTL")
	}
	input.RequestRetry = clampRoutingPolicyInt(input.RequestRetry, 0, 10)
	input.MaxRetryCredentials = clampRoutingPolicyInt(input.MaxRetryCredentials, 0, 100)
	input.MaxRetryInterval = clampRoutingPolicyInt(input.MaxRetryInterval, 0, 3600)
	input.TransientErrorCooldownSeconds = clampRoutingPolicyInt(input.TransientErrorCooldownSeconds, -1, 86400)
	return input, nil
}

func routingPolicyGlobalSettingsFromConfig(cfg *config.Config) routingPolicyGlobalSettings {
	if cfg == nil {
		return routingPolicyGlobalSettings{}
	}
	result := routingPolicyGlobalSettings{
		Strategy:                      cfg.Routing.Strategy,
		SessionAffinity:               cfg.Routing.SessionAffinity,
		SessionAffinityTTL:            cfg.Routing.SessionAffinityTTL,
		RequestRetry:                  cfg.RequestRetry,
		MaxRetryCredentials:           cfg.MaxRetryCredentials,
		MaxRetryInterval:              cfg.MaxRetryInterval,
		CoolingEnabled:                !cfg.DisableCooling,
		SaveCooldownStatus:            cfg.SaveCooldownStatus,
		TransientErrorCooldownSeconds: cfg.TransientErrorCooldownSeconds,
		QuotaSwitchProject:            cfg.QuotaExceeded.SwitchProject,
		QuotaSwitchPreviewModel:       cfg.QuotaExceeded.SwitchPreviewModel,
		QuotaAntigravityCredits:       cfg.QuotaExceeded.AntigravityCredits,
		CodexIdentityConfuse:          cfg.Codex.IdentityConfuse,
	}
	if strings.TrimSpace(result.Strategy) == "" {
		result.Strategy = "round-robin"
	}
	if strings.TrimSpace(result.SessionAffinityTTL) == "" {
		result.SessionAffinityTTL = "1h"
	}
	return result
}

func routingPolicyExistingScalarUpdates(cfg *config.Config, desired routingPolicyGlobalSettings) []config.ExistingScalarUpdate {
	current := routingPolicyGlobalSettingsFromConfig(cfg)
	updates := make([]config.ExistingScalarUpdate, 0, 13)
	appendUpdate := func(changed bool, path []string, value any) {
		if changed {
			updates = append(updates, config.ExistingScalarUpdate{Path: path, Value: value})
		}
	}
	appendUpdate(current.Strategy != desired.Strategy, []string{"routing", "strategy"}, desired.Strategy)
	appendUpdate(current.SessionAffinity != desired.SessionAffinity, []string{"routing", "session-affinity"}, desired.SessionAffinity)
	appendUpdate(current.SessionAffinityTTL != desired.SessionAffinityTTL, []string{"routing", "session-affinity-ttl"}, desired.SessionAffinityTTL)
	appendUpdate(current.RequestRetry != desired.RequestRetry, []string{"request-retry"}, desired.RequestRetry)
	appendUpdate(current.MaxRetryCredentials != desired.MaxRetryCredentials, []string{"max-retry-credentials"}, desired.MaxRetryCredentials)
	appendUpdate(current.MaxRetryInterval != desired.MaxRetryInterval, []string{"max-retry-interval"}, desired.MaxRetryInterval)
	appendUpdate(current.CoolingEnabled != desired.CoolingEnabled, []string{"disable-cooling"}, !desired.CoolingEnabled)
	appendUpdate(current.SaveCooldownStatus != desired.SaveCooldownStatus, []string{"save-cooldown-status"}, desired.SaveCooldownStatus)
	appendUpdate(current.TransientErrorCooldownSeconds != desired.TransientErrorCooldownSeconds, []string{"transient-error-cooldown-seconds"}, desired.TransientErrorCooldownSeconds)
	appendUpdate(current.QuotaSwitchProject != desired.QuotaSwitchProject, []string{"quota-exceeded", "switch-project"}, desired.QuotaSwitchProject)
	appendUpdate(current.QuotaSwitchPreviewModel != desired.QuotaSwitchPreviewModel, []string{"quota-exceeded", "switch-preview-model"}, desired.QuotaSwitchPreviewModel)
	appendUpdate(current.QuotaAntigravityCredits != desired.QuotaAntigravityCredits, []string{"quota-exceeded", "antigravity-credits"}, desired.QuotaAntigravityCredits)
	appendUpdate(current.CodexIdentityConfuse != desired.CodexIdentityConfuse, []string{"codex", "identity-confuse"}, desired.CodexIdentityConfuse)
	return updates
}

func applyRoutingPolicyGlobalSettings(cfg *config.Config, desired routingPolicyGlobalSettings) {
	cfg.Routing.Strategy = desired.Strategy
	cfg.Routing.SessionAffinity = desired.SessionAffinity
	cfg.Routing.SessionAffinityTTL = desired.SessionAffinityTTL
	cfg.RequestRetry = desired.RequestRetry
	cfg.MaxRetryCredentials = desired.MaxRetryCredentials
	cfg.MaxRetryInterval = desired.MaxRetryInterval
	cfg.DisableCooling = !desired.CoolingEnabled
	cfg.SaveCooldownStatus = desired.SaveCooldownStatus
	cfg.TransientErrorCooldownSeconds = desired.TransientErrorCooldownSeconds
	cfg.QuotaExceeded.SwitchProject = desired.QuotaSwitchProject
	cfg.QuotaExceeded.SwitchPreviewModel = desired.QuotaSwitchPreviewModel
	cfg.QuotaExceeded.AntigravityCredits = desired.QuotaAntigravityCredits
	cfg.Codex.IdentityConfuse = desired.CodexIdentityConfuse
}

func (h *Handler) persistRoutingPolicyGlobal(c *gin.Context, desired routingPolicyGlobalSettings) bool {
	h.mu.Lock()
	if h.cfg == nil {
		h.mu.Unlock()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config unavailable"})
		return false
	}
	updates := routingPolicyExistingScalarUpdates(h.cfg, desired)
	if len(updates) == 0 {
		h.mu.Unlock()
		return true
	}
	missing, err := config.SaveConfigPreserveCommentsUpdateExistingScalars(h.configFilePath, updates)
	if err != nil {
		h.mu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update config: %v", err)})
		return false
	}
	if len(missing) > 0 {
		h.mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{
			"error":   "config_key_missing",
			"message": fmt.Sprintf("configuration keys are not explicitly present and cannot be added by Pro: %s", strings.Join(missing, ", ")),
			"paths":   missing,
		})
		return false
	}
	applyRoutingPolicyGlobalSettings(h.cfg, desired)
	snapshot := h.reloadSnapshotConfigLocked()
	h.mu.Unlock()
	h.reloadConfigAfterManagementSave(c.Request.Context(), snapshot)
	return true
}

func (h *Handler) persistRoutingRequestProtection(c *gin.Context, value routingRequestProtectionConfig) bool {
	value = normalizeRoutingRequestProtectionConfig(value)
	raw, err := json.Marshal(value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if err := embeddedusage.SetProSetting(c.Request.Context(), embeddedusage.ProSetting{
		Namespace:     embeddedusage.ProSettingNamespaceRoutingRequestProtection,
		SchemaVersion: routingProtectionSchemaVersion,
		Settings:      raw,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	controller := routingPolicyControllerForHandler(h)
	if controller == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing policy controller unavailable"})
		return false
	}
	controller.setRequestProtectionConfig(value)
	return true
}

func (h *Handler) ReleaseRoutingProtectedAuth(c *gin.Context) {
	controller := routingPolicyControllerForHandler(h)
	if controller == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing policy controller unavailable"})
		return
	}
	var request routingPolicyReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.AuthIndex) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "authIndex is required"})
		return
	}
	auth := h.authByIndex(strings.TrimSpace(request.AuthIndex))
	if auth == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth not found"})
		return
	}
	auth.EnsureIndex()
	if !routingProtectionOwned(auth) {
		c.JSON(http.StatusConflict, gin.H{"error": "auth is not managed by routing request protection"})
		return
	}
	if err := controller.releaseAuth(c.Request.Context(), auth); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.routingPolicyResponse())
}

func (h *Handler) routingPolicyResponse() routingPolicyResponse {
	response := routingPolicyResponse{
		RequestProtection:  defaultRoutingRequestProtectionConfig(),
		AvailableProviders: []string{},
		Active:             []routingProtectionActiveAccount{},
		RecentEvents:       []routingProtectionEvent{},
	}
	h.mu.Lock()
	if h.cfg != nil {
		response.Global = routingPolicyGlobalSettingsFromConfig(h.cfg)
	}
	h.mu.Unlock()
	response.AvailableProviders = h.routingProtectionAvailableProviders()
	response.Active = h.routingProtectionActiveAccounts()
	if controller := routingPolicyControllerForHandler(h); controller != nil {
		response.RequestProtection = controller.requestProtectionConfig()
		response.RecentEvents = controller.recentEvents()
	}
	return response
}

func (h *Handler) routingProtectionAvailableProviders() []string {
	if h == nil {
		return []string{}
	}
	h.mu.Lock()
	available := routingProtectionConfiguredProviderSet(h.cfg)
	manager := h.authManager
	h.mu.Unlock()
	var auths []*coreauth.Auth
	if manager != nil {
		auths = manager.List()
	}
	return orderedRoutingProtectionAvailableProviders(available, auths)
}

func routingProtectionConfiguredProviderSet(cfg *config.Config) map[string]struct{} {
	available := make(map[string]struct{}, len(routingProtectionProviders))
	if cfg == nil {
		return available
	}
	configured := map[string]bool{
		"codex":               len(cfg.CodexKey) > 0,
		"gemini":              len(cfg.GeminiKey) > 0,
		"vertex":              len(cfg.VertexCompatAPIKey) > 0,
		"claude":              len(cfg.ClaudeKey) > 0,
	}
	for provider, ok := range configured {
		if ok {
			available[provider] = struct{}{}
		}
	}
	return available
}

func orderedRoutingProtectionAvailableProviders(available map[string]struct{}, auths []*coreauth.Auth) []string {
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(auth.Provider))
		if provider == "anthropic" {
			provider = "claude"
		}
		available[provider] = struct{}{}
	}
	providers := make([]string, 0, len(routingProtectionProviders))
	for _, provider := range routingProtectionProviders {
		if _, ok := available[provider]; ok {
			providers = append(providers, provider)
		}
	}
	return providers
}

func (h *Handler) routingProtectionActiveAccounts() []routingProtectionActiveAccount {
	if h == nil || h.authManager == nil {
		return []routingProtectionActiveAccount{}
	}
	active := make([]routingProtectionActiveAccount, 0)
	for _, auth := range h.authManager.List() {
		if auth == nil || !routingProtectionOwned(auth) {
			continue
		}
		auth.EnsureIndex()
		metadata := routingProtectionMetadata(auth)
		active = append(active, routingProtectionActiveAccount{
			Provider:    strings.ToLower(strings.TrimSpace(auth.Provider)),
			AuthID:      auth.ID,
			AuthIndex:   auth.Index,
			FileName:    routingProtectionAuthFileName(auth),
			StatusCode:  int(routingProtectionMetadataInt64(metadata, "status_code")),
			Reason:      stringFromAny(metadata["reason"]),
			TriggeredAt: routingProtectionMetadataInt64(metadata, "triggered_at"),
			ReleaseAt:   routingProtectionMetadataInt64(metadata, "release_at"),
		})
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].ReleaseAt == active[j].ReleaseAt {
			return active[i].AuthIndex < active[j].AuthIndex
		}
		if active[i].ReleaseAt == 0 {
			return false
		}
		if active[j].ReleaseAt == 0 {
			return true
		}
		return active[i].ReleaseAt < active[j].ReleaseAt
	})
	return active
}

func routingProtectionAuthFileName(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	for _, candidate := range []string{
		auth.FileName,
		authAttribute(auth, coreauth.AttributeVirtualSource),
		authAttribute(auth, "path"),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		name := filepath.Base(filepath.Clean(candidate))
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return ""
}

func clampRoutingPolicyInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
