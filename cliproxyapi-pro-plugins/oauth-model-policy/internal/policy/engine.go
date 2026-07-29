package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	pluginconfig "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/oauth-model-policy/internal/config"
)

const (
	xaiBillingURL               = "https://cli-chat-proxy.grok.com/v1/billing"
	xaiSuperGrokLimitCents      = int64(15_000)
	xaiXPremiumPlusLimitCents   = int64(20_000)
	xaiSuperGrokHeavyLimitCents = int64(150_000)
)

type HTTPDo func(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error)

type Input struct {
	AuthID       string
	AuthProvider string
	AuthKind     string
	StorageJSON  []byte
	Metadata     map[string]any
	Attributes   map[string]string
	Models       []pluginapi.ModelInfo
	HTTPDo       HTTPDo
}

type Result struct {
	Handled          bool
	ExcludedModelIDs []string
	Annotations      map[string]string
}

type cacheEntry struct {
	Plan       string
	ObservedAt time.Time
}

type Engine struct {
	mu    sync.RWMutex
	cfg   pluginconfig.Config
	cache map[string]cacheEntry
}

func New() *Engine {
	cfg, _ := pluginconfig.Parse(nil)
	return &Engine{cfg: cfg, cache: make(map[string]cacheEntry)}
}

func (e *Engine) ApplyConfig(cfg pluginconfig.Config) {
	e.mu.Lock()
	e.cfg = cfg
	e.cache = make(map[string]cacheEntry)
	e.mu.Unlock()
}

func (e *Engine) Filter(ctx context.Context, input Input) Result {
	provider := normalizeKey(input.AuthProvider)
	if provider != "xai" || normalizeKey(input.AuthKind) != "oauth" {
		return Result{}
	}
	e.mu.RLock()
	cfg := e.cfg
	providerCfg, configured := cfg.Providers[provider]
	e.mu.RUnlock()
	if !configured || len(providerCfg.Plans) == 0 {
		return Result{}
	}

	plan, source, resolveErr := e.resolvePlan(ctx, cfg, input)
	rule, matchedPlan, matched := ruleForPlan(providerCfg, plan)
	if !matched {
		return Result{}
	}
	excluded := matchExcludedModels(input.Models, rule.ExcludedModels)
	annotations := map[string]string{
		"plan_key":     plan,
		"plan_source":  source,
		"matched_rule": matchedPlan,
	}
	if resolveErr != nil {
		annotations["plan_error"] = resolveErr.Error()
	}
	return Result{Handled: true, ExcludedModelIDs: excluded, Annotations: annotations}
}

func (e *Engine) resolvePlan(ctx context.Context, cfg pluginconfig.Config, input Input) (string, string, error) {
	if plan := localPlan(input); plan != "" {
		return plan, "auth", nil
	}
	now := time.Now()
	e.mu.RLock()
	cached, hasCache := e.cache[input.AuthID]
	e.mu.RUnlock()
	if hasCache && now.Sub(cached.ObservedAt) <= cfg.CacheTTL {
		return cached.Plan, "cache", nil
	}
	plan, errResolve := resolveXAIPlan(ctx, cfg.ResolveTimeout, input)
	if errResolve == nil && plan != "" {
		e.mu.Lock()
		e.cache[input.AuthID] = cacheEntry{Plan: plan, ObservedAt: now}
		e.mu.Unlock()
		return plan, "billing", nil
	}
	if hasCache && cached.Plan != "" {
		return cached.Plan, "stale-cache", errResolve
	}
	if errResolve == nil {
		errResolve = fmt.Errorf("xai plan is unavailable")
	}
	return "unknown", "unknown", errResolve
}

func localPlan(input Input) string {
	sources := []map[string]any{input.Metadata, stringMapToAny(input.Attributes)}
	storage := map[string]any{}
	if len(input.StorageJSON) > 0 && json.Unmarshal(input.StorageJSON, &storage) == nil {
		sources = append(sources, storage)
	}
	for _, source := range sources {
		if plan := planFromMap(source); plan != "" {
			return plan
		}
	}
	return ""
}

func planFromMap(source map[string]any) string {
	if source == nil {
		return ""
	}
	for _, key := range []string{"plan_type", "planType", "plan", "package"} {
		if plan := normalizePlan(stringValue(source[key])); plan != "" {
			return plan
		}
	}
	if billing, ok := source["billing"].(map[string]any); ok {
		if plan := planFromMap(billing); plan != "" {
			return plan
		}
		if limit, known := numberValue(firstValue(billing, "monthlyLimitCents", "monthly_limit_cents", "monthlyLimit", "monthly_limit")); known {
			return xaiPlanFromLimit(limit)
		}
	}
	return ""
}

func resolveXAIPlan(ctx context.Context, timeout time.Duration, input Input) (string, error) {
	if input.HTTPDo == nil {
		return "", fmt.Errorf("host http client is unavailable")
	}
	storage := map[string]any{}
	if len(input.StorageJSON) > 0 {
		if errUnmarshal := json.Unmarshal(input.StorageJSON, &storage); errUnmarshal != nil {
			return "", fmt.Errorf("decode xai auth storage: %w", errUnmarshal)
		}
	}
	sources := []map[string]any{storage, input.Metadata, stringMapToAny(input.Attributes)}
	token := firstString(sources, "access_token", "accessToken")
	if token == "" {
		return "", fmt.Errorf("xai access token is unavailable")
	}
	userID := firstString(sources, "x_user_id", "xUserId", "user_id", "userId", "subject", "sub", "id")
	requestCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	headers := http.Header{
		"Authorization":         []string{"Bearer " + token},
		"x-xai-token-auth":      []string{"xai-grok-cli"},
		"x-grok-client-version": []string{"0.2.91"},
		"Accept":                []string{"*/*"},
		"User-Agent":            []string{"grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)"},
	}
	if userID != "" {
		headers["x-userid"] = []string{userID}
	}
	resp, errDo := input.HTTPDo(requestCtx, pluginapi.HTTPRequest{Method: http.MethodGet, URL: xaiBillingURL, Headers: headers})
	if errDo != nil {
		return "", fmt.Errorf("fetch xai billing: %w", errDo)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch xai billing returned HTTP %d", resp.StatusCode)
	}
	payload := map[string]any{}
	if errUnmarshal := json.Unmarshal(resp.Body, &payload); errUnmarshal != nil {
		return "", fmt.Errorf("decode xai billing: %w", errUnmarshal)
	}
	config, _ := payload["config"].(map[string]any)
	if config == nil {
		return "", fmt.Errorf("xai billing config is missing")
	}
	limit, known := numberValue(firstValue(config, "monthlyLimit", "monthly_limit"))
	if !known {
		return "free", nil
	}
	return xaiPlanFromLimit(limit), nil
}

func xaiPlanFromLimit(limit float64) string {
	rounded := int64(math.Round(limit))
	if rounded == 0 {
		return "free"
	}
	switch rounded {
	case xaiSuperGrokLimitCents:
		return "supergrok"
	case xaiXPremiumPlusLimitCents:
		return "x-premium-plus"
	case xaiSuperGrokHeavyLimitCents:
		return "supergrok-heavy"
	default:
		return "paid-unknown"
	}
}

func ruleForPlan(provider pluginconfig.Provider, plan string) (pluginconfig.Plan, string, bool) {
	plan = normalizePlan(plan)
	for _, key := range []string{plan, "_unknown", "_default"} {
		key = normalizePlan(key)
		if rule, ok := provider.Plans[key]; ok {
			return rule, key, true
		}
	}
	return pluginconfig.Plan{}, "", false
}

func matchExcludedModels(models []pluginapi.ModelInfo, patterns []string) []string {
	if len(models) == 0 || len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0)
	for _, model := range models {
		modelID := strings.ToLower(strings.TrimSpace(model.ID))
		if modelID == "" {
			continue
		}
		for _, pattern := range patterns {
			matched, errMatch := path.Match(pattern, modelID)
			if errMatch == nil && matched {
				out = append(out, model.ID)
				break
			}
		}
	}
	return out
}

func normalizePlan(value string) string {
	value = normalizeKey(value)
	switch value {
	case "premium-plus", "x-premium+", "x-premium-plus":
		return "x-premium-plus"
	case "super-grok":
		return "supergrok"
	case "super-grok-heavy":
		return "supergrok-heavy"
	}
	return value
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "_") {
		return "_" + strings.ReplaceAll(strings.TrimPrefix(value, "_"), "_", "-")
	}
	return strings.ReplaceAll(value, "_", "-")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return numberValue(typed["val"])
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, errParse := typed.Float64()
		return parsed, errParse == nil
	case string:
		parsed, errParse := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, errParse == nil
	default:
		return 0, false
	}
}

func firstValue(source map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			return value
		}
	}
	return nil
}

func firstString(sources []map[string]any, keys ...string) string {
	for _, source := range sources {
		for _, key := range keys {
			if value := stringValue(source[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func stringMapToAny(source map[string]string) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
