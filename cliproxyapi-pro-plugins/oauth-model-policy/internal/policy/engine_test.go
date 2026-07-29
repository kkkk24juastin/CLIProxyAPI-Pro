package policy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	pluginconfig "github.com/ssfun/CLIProxyAPI-Pro/cliproxyapi-pro-plugins/oauth-model-policy/internal/config"
)

func TestFilterUsesXAIPlanFromBilling(t *testing.T) {
	cfg, errParse := pluginconfig.Parse([]byte(`
providers:
  xai:
    plans:
      x-premium-plus:
        excluded-models: ["grok-4.5-*"]
      _unknown:
        excluded-models: ["grok-*"]
`))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	engine := New()
	engine.ApplyConfig(cfg)
	storage, _ := json.Marshal(map[string]any{"access_token": "token", "subject": "user"})
	result := engine.Filter(context.Background(), Input{
		AuthID: "xai-1", AuthProvider: "xai", AuthKind: "oauth", StorageJSON: storage,
		Models: []pluginapi.ModelInfo{{ID: "grok-4.5-reasoning"}, {ID: "grok-4"}},
		HTTPDo: func(_ context.Context, request pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
			if request.Method != "GET" || request.URL != xaiBillingURL {
				t.Fatalf("billing request = %#v", request)
			}
			if got := request.Headers["Authorization"]; len(got) != 1 || got[0] != "Bearer token" {
				t.Fatalf("Authorization = %#v", got)
			}
			if got := request.Headers["x-userid"]; len(got) != 1 || got[0] != "user" {
				t.Fatalf("x-userid = %#v", got)
			}
			return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"config":{"monthlyLimit":{"val":20000}}}`)}, nil
		},
	})
	if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-4.5-reasoning" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["plan_key"] != "x-premium-plus" || result.Annotations["plan_source"] != "billing" {
		t.Fatalf("annotations = %#v", result.Annotations)
	}
}

func TestFilterUsesUnknownRuleWhenBillingFails(t *testing.T) {
	cfg, _ := pluginconfig.Parse([]byte(`
providers:
  xai:
    plans:
      _unknown:
        excluded-models: ["grok-pro-*"]
`))
	engine := New()
	engine.ApplyConfig(cfg)
	result := engine.Filter(context.Background(), Input{
		AuthID: "xai-2", AuthProvider: "xai", AuthKind: "oauth",
		Models: []pluginapi.ModelInfo{{ID: "grok-pro-1"}, {ID: "grok-basic"}},
	})
	if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-pro-1" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["matched_rule"] != "_unknown" {
		t.Fatalf("matched rule = %#v", result.Annotations)
	}
}

func TestFilterFallsBackToStalePlanCache(t *testing.T) {
	cfg, _ := pluginconfig.Parse([]byte(`
cache-ttl: 1s
providers:
  xai:
    plans:
      supergrok:
        excluded-models: ["grok-imagine-video"]
      _unknown:
        excluded-models: ["grok-pro-*"]
`))
	engine := New()
	engine.ApplyConfig(cfg)
	engine.cache["xai\x00xai-3"] = cacheEntry{Plan: "supergrok", ObservedAt: time.Now().Add(-time.Hour)}
	storage, _ := json.Marshal(map[string]any{"access_token": "token"})
	result := engine.Filter(context.Background(), Input{
		AuthID: "xai-3", AuthProvider: "xai", AuthKind: "oauth", StorageJSON: storage,
		Models: []pluginapi.ModelInfo{{ID: "grok-imagine-video"}, {ID: "grok-pro-1"}},
		HTTPDo: func(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
			return pluginapi.HTTPResponse{}, errors.New("temporary billing failure")
		},
	})
	if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-imagine-video" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["plan_source"] != "stale-cache" || result.Annotations["plan_key"] != "supergrok" {
		t.Fatalf("annotations = %#v", result.Annotations)
	}
}

func TestRuleForPlanSeparatesUnknownAndDefaultFallbacks(t *testing.T) {
	provider := pluginconfig.Provider{Plans: map[string]pluginconfig.Plan{
		"_unknown": {ExcludedModels: []string{"unknown-*"}},
		"_default": {ExcludedModels: []string{"default-*"}},
	}}
	unknownRule, unknownKey, unknownMatched := ruleForPlan(provider, "unknown")
	if !unknownMatched || unknownKey != "_unknown" || unknownRule.ExcludedModels[0] != "unknown-*" {
		t.Fatalf("unknown fallback = %#v, %q, %t", unknownRule, unknownKey, unknownMatched)
	}
	knownRule, knownKey, knownMatched := ruleForPlan(provider, "supergrok")
	if !knownMatched || knownKey != "_default" || knownRule.ExcludedModels[0] != "default-*" {
		t.Fatalf("known fallback = %#v, %q, %t", knownRule, knownKey, knownMatched)
	}
	defaultOnly := pluginconfig.Provider{Plans: map[string]pluginconfig.Plan{
		"_default": {ExcludedModels: []string{"default-*"}},
	}}
	if _, key, matched := ruleForPlan(defaultOnly, "unknown"); matched || key != "" {
		t.Fatalf("unknown plan matched _default: key=%q matched=%t", key, matched)
	}
}

func TestFilterUsesLocalPlansForEveryProvider(t *testing.T) {
	cfg, errParse := pluginconfig.Parse([]byte(`
providers:
  codex:
    plans:
      plus: {excluded-models: [blocked-*]}
  claude:
    plans:
      max: {excluded-models: [blocked-*]}
  gemini-cli:
    plans:
      standard: {excluded-models: [blocked-*]}
  antigravity:
    plans:
      ultra-lite: {excluded-models: [blocked-*]}
  kimi:
    plans:
      team: {excluded-models: [blocked-*]}
`))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	claims, _ := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "plus"},
	})
	codexToken := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
	tests := []struct {
		provider string
		storage  map[string]any
		wantPlan string
	}{
		{provider: "codex", storage: map[string]any{"id_token": codexToken}, wantPlan: "plus"},
		{provider: "claude", storage: map[string]any{"account": map[string]any{"has_claude_max": true}}, wantPlan: "max"},
		{provider: "gemini-cli", storage: map[string]any{"currentTier": map[string]any{"id": "standard-tier"}}, wantPlan: "standard"},
		{provider: "antigravity", storage: map[string]any{"subscription": map[string]any{"tierId": "g1-ultra-lite-tier"}}, wantPlan: "ultra-lite"},
		{provider: "kimi", storage: map[string]any{"package": "team"}, wantPlan: "team"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			storage, _ := json.Marshal(test.storage)
			engine := New()
			engine.ApplyConfig(cfg)
			result := engine.Filter(context.Background(), Input{
				AuthID: test.provider + "-1", AuthProvider: test.provider, AuthKind: "oauth", StorageJSON: storage,
				Models: []pluginapi.ModelInfo{{ID: "blocked-model"}, {ID: "allowed-model"}},
			})
			if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "blocked-model" {
				t.Fatalf("Filter() = %#v", result)
			}
			if result.Annotations["plan_key"] != test.wantPlan || result.Annotations["plan_source"] != "auth" {
				t.Fatalf("annotations = %#v", result.Annotations)
			}
		})
	}
}

func TestFilterResolvesClaudePlanFromProfile(t *testing.T) {
	cfg, _ := pluginconfig.Parse([]byte(`
providers:
  claude:
    plans:
      pro: {excluded-models: [claude-opus-*]}
`))
	storage, _ := json.Marshal(map[string]any{"access_token": "claude-token"})
	engine := New()
	engine.ApplyConfig(cfg)
	result := engine.Filter(context.Background(), Input{
		AuthID: "claude-1", AuthProvider: "claude", AuthKind: "oauth", StorageJSON: storage,
		Models: []pluginapi.ModelInfo{{ID: "claude-opus-4"}, {ID: "claude-sonnet-4"}},
		HTTPDo: func(_ context.Context, request pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
			if request.URL != claudeProfileURL || request.Headers.Get("Authorization") != "Bearer claude-token" {
				t.Fatalf("profile request = %#v", request)
			}
			return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"account":{"has_claude_max":false,"has_claude_pro":true}}`)}, nil
		},
	})
	if !result.Handled || result.Annotations["plan_key"] != "pro" || result.Annotations["plan_source"] != "provider-api" {
		t.Fatalf("Filter() = %#v", result)
	}
}

func TestFilterResolvesGoogleProviderPlans(t *testing.T) {
	cfg, _ := pluginconfig.Parse([]byte(`
providers:
  gemini-cli:
    plans:
      ultra: {excluded-models: [gemini-pro-*]}
  antigravity:
    plans:
      pro: {excluded-models: [claude-*]}
`))
	tests := []struct {
		provider, url, response, model, wantPlan string
		storage                                  map[string]any
	}{
		{
			provider: "gemini-cli", url: geminiCodeAssistURL,
			storage:  map[string]any{"token": map[string]any{"access_token": "google-token"}, "project_id": "project-1"},
			response: `{"paidTier":{"id":"g1-ultra-tier"}}`, model: "gemini-pro-1", wantPlan: "ultra",
		},
		{
			provider: "antigravity", url: antigravityCodeAssistURL,
			storage:  map[string]any{"access_token": "google-token"},
			response: `{"currentTier":{"id":"g1-pro-tier"}}`, model: "claude-sonnet", wantPlan: "pro",
		},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			storage, _ := json.Marshal(test.storage)
			engine := New()
			engine.ApplyConfig(cfg)
			result := engine.Filter(context.Background(), Input{
				AuthID: test.provider + "-1", AuthProvider: test.provider, AuthKind: "oauth", StorageJSON: storage,
				Models: []pluginapi.ModelInfo{{ID: test.model}},
				HTTPDo: func(_ context.Context, request pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
					if request.URL != test.url || request.Headers.Get("Authorization") != "Bearer google-token" {
						t.Fatalf("plan request = %#v", request)
					}
					return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(test.response)}, nil
				},
			})
			if !result.Handled || result.Annotations["plan_key"] != test.wantPlan || result.Annotations["plan_source"] != "provider-api" {
				t.Fatalf("Filter() = %#v", result)
			}
		})
	}
}
