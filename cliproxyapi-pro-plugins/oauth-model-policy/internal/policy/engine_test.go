package policy

import (
	"context"
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
	engine.cache["xai-3"] = cacheEntry{Plan: "supergrok", ObservedAt: time.Now().Add(-time.Hour)}
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
}
