package policy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/config"
)

func TestFilterUsesXAIPlanFromBilling(t *testing.T) {
	cfg, errParse := modelconfig.Parse([]byte(`
providers:
  xai:
    plans:
      paid-unknown:
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
		Models: []ModelInfo{{ID: "grok-4.5-reasoning"}, {ID: "grok-4"}},
		HTTPDo: func(_ context.Context, request HTTPRequest) (HTTPResponse, error) {
			if request.Method != "GET" || request.URL != xaiBillingURL {
				t.Fatalf("billing request = %#v", request)
			}
			if got := request.Headers["Authorization"]; len(got) != 1 || got[0] != "Bearer token" {
				t.Fatalf("Authorization = %#v", got)
			}
			if got := request.Headers["x-userid"]; len(got) != 1 || got[0] != "user" {
				t.Fatalf("x-userid = %#v", got)
			}
			clientVersion := request.Headers.Get("x-grok-client-version")
			if clientVersion == "" {
				t.Fatal("x-grok-client-version is empty")
			}
			if got := request.Headers.Get("User-Agent"); got != "xai-grok-workspace/"+clientVersion {
				t.Fatalf("User-Agent = %q", got)
			}
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"config":{"monthlyLimit":{"val":20000}}}`)}, nil
		},
	})
	if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-4.5-reasoning" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["plan_key"] != "paid-unknown" || result.Annotations["plan_source"] != "billing" {
		t.Fatalf("annotations = %#v", result.Annotations)
	}
}

func TestFilterReturnsAccountRoutingOverrides(t *testing.T) {
	cfg, err := modelconfig.Parse([]byte(`
providers:
  codex:
    plans:
      pro:
        excluded-models: [blocked-*]
        prefix: codex-pro
        priority: 80
        weight: 5
`))
	if err != nil {
		t.Fatal(err)
	}
	engine := New()
	engine.ApplyConfig(cfg)
	result := engine.Filter(context.Background(), Input{
		AuthID: "codex-1", AuthProvider: "codex", AuthKind: "oauth",
		Attributes: map[string]string{"plan_type": "pro"},
		Models:     []ModelInfo{{ID: "gpt-5"}, {ID: "blocked-model"}},
	})
	if !result.Handled || result.Prefix == nil || *result.Prefix != "codex-pro" || result.Priority == nil || *result.Priority != 80 || result.Weight == nil || *result.Weight != 5 {
		t.Fatalf("account policy result = %#v", result)
	}
	if len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "blocked-model" {
		t.Fatalf("excluded models = %#v", result.ExcludedModelIDs)
	}
}

func TestFilterDoesNotQueryCLIBillingForXAIOfficialAPI(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
providers:
  xai:
    plans:
      paid-unknown:
        excluded-models: ["grok-cli-only"]
`))
	engine := New()
	engine.ApplyConfig(cfg)
	called := false
	result := engine.Filter(context.Background(), Input{
		AuthID: "xai-official", AuthProvider: "xai", AuthKind: "oauth",
		Attributes: map[string]string{"using_api": "true"},
		Models:     []ModelInfo{{ID: "grok-cli-only"}},
		HTTPDo: func(context.Context, HTTPRequest) (HTTPResponse, error) {
			called = true
			return HTTPResponse{}, nil
		},
	})
	if called {
		t.Fatal("official API account queried CLI billing")
	}
	if !result.Handled || result.Annotations["plan_key"] != "paid-unknown" {
		t.Fatalf("Filter() = %#v", result)
	}
}

func TestFilterUsesUnknownRuleWhenBillingFails(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
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
		Models: []ModelInfo{{ID: "grok-pro-1"}, {ID: "grok-basic"}},
	})
	if !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-pro-1" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["matched_rule"] != "_unknown" {
		t.Fatalf("matched rule = %#v", result.Annotations)
	}
}

func TestFilterDoesNotUseXAIPlanCacheBeforeBilling(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
cache-ttl: 1s
providers:
  xai:
    plans:
      supergrok:
        excluded-models: ["grok-imagine-video"]
`))
	engine := New()
	engine.ApplyConfig(cfg)
	engine.cache["xai\x00xai-3"] = cacheEntry{Plan: "supergrok-heavy", ObservedAt: time.Now()}
	storage, _ := json.Marshal(map[string]any{"access_token": "token", "plan_type": "supergrok-heavy"})
	called := false
	result := engine.Filter(context.Background(), Input{
		AuthID: "xai-3", AuthProvider: "xai", AuthKind: "oauth", StorageJSON: storage,
		Models: []ModelInfo{{ID: "grok-imagine-video"}},
		HTTPDo: func(context.Context, HTTPRequest) (HTTPResponse, error) {
			called = true
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"config":{"monthlyLimit":{"val":15000}}}`)}, nil
		},
	})
	if !called || !result.Handled || len(result.ExcludedModelIDs) != 1 || result.ExcludedModelIDs[0] != "grok-imagine-video" {
		t.Fatalf("Filter() = %#v", result)
	}
	if result.Annotations["plan_source"] != "billing" || result.Annotations["plan_key"] != "supergrok" {
		t.Fatalf("annotations = %#v", result.Annotations)
	}
}

func TestRuleForPlanSeparatesUnknownAndDefaultFallbacks(t *testing.T) {
	provider := modelconfig.Provider{Plans: map[string]modelconfig.Plan{
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
	defaultOnly := modelconfig.Provider{Plans: map[string]modelconfig.Plan{
		"_default": {ExcludedModels: []string{"default-*"}},
	}}
	if _, key, matched := ruleForPlan(defaultOnly, "unknown"); matched || key != "" {
		t.Fatalf("unknown plan matched _default: key=%q matched=%t", key, matched)
	}
}

func TestFilterUsesLocalPlansForEveryProvider(t *testing.T) {
	cfg, errParse := modelconfig.Parse([]byte(`
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
				Models: []ModelInfo{{ID: "blocked-model"}, {ID: "allowed-model"}},
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
	cfg, _ := modelconfig.Parse([]byte(`
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
		Models: []ModelInfo{{ID: "claude-opus-4"}, {ID: "claude-sonnet-4"}},
		HTTPDo: func(_ context.Context, request HTTPRequest) (HTTPResponse, error) {
			if request.URL != claudeProfileURL || request.Headers.Get("Authorization") != "Bearer claude-token" {
				t.Fatalf("profile request = %#v", request)
			}
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"account":{"has_claude_max":false,"has_claude_pro":true}}`)}, nil
		},
	})
	if !result.Handled || result.Annotations["plan_key"] != "pro" || result.Annotations["plan_source"] != "provider-api" {
		t.Fatalf("Filter() = %#v", result)
	}
}

func TestFilterResolvesProviderPlanWhenLocalValueIsUnknown(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
providers:
  claude:
    plans:
      pro: {priority: 99}
      _unknown: {priority: 1}
`))
	engine := New()
	engine.ApplyConfig(cfg)
	called := false
	result := engine.Filter(context.Background(), Input{
		AuthID: "claude-unknown", AuthProvider: "claude", AuthKind: "oauth",
		Attributes: map[string]string{"plan_type": "unknown"},
		Metadata:   map[string]any{"access_token": "claude-token"},
		HTTPDo: func(context.Context, HTTPRequest) (HTTPResponse, error) {
			called = true
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"account":{"has_claude_pro":true}}`)}, nil
		},
	})
	if !called || !result.Handled || result.Annotations["plan_key"] != "pro" || result.Priority == nil || *result.Priority != 99 {
		t.Fatalf("Filter() = %#v, provider called = %t", result, called)
	}
}

func TestForgetAuthDropsCacheAndRejectsInFlightCacheWrite(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
providers:
  claude:
    plans:
      pro: {priority: 99}
      max: {priority: 55}
`))
	engine := New()
	engine.ApplyConfig(cfg)
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan Result, 1)
	var calls atomic.Int32
	input := Input{
		AuthID: "removed", AuthProvider: "claude", AuthKind: "oauth",
		Metadata: map[string]any{"access_token": "claude-token"},
		HTTPDo: func(context.Context, HTTPRequest) (HTTPResponse, error) {
			if calls.Add(1) == 1 {
				close(started)
				<-release
				return HTTPResponse{StatusCode: 200, Body: []byte(`{"account":{"has_claude_pro":true}}`)}, nil
			}
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"account":{"has_claude_max":true}}`)}, nil
		},
	}
	go func() { finished <- engine.Filter(context.Background(), input) }()
	<-started
	engine.ForgetAuth(input.AuthID)
	close(release)
	if result := <-finished; !result.Handled || result.Annotations["plan_key"] != "pro" {
		t.Fatalf("in-flight Filter() = %#v", result)
	}
	result := engine.Filter(context.Background(), input)
	if !result.Handled || result.Annotations["plan_key"] != "max" {
		t.Fatalf("Filter() after ForgetAuth = %#v", result)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider lookup calls = %d, want 2", got)
	}
}

func TestFilterResolvesGoogleProviderPlans(t *testing.T) {
	cfg, _ := modelconfig.Parse([]byte(`
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
				Models: []ModelInfo{{ID: test.model}},
				HTTPDo: func(_ context.Context, request HTTPRequest) (HTTPResponse, error) {
					if request.URL != test.url || request.Headers.Get("Authorization") != "Bearer google-token" {
						t.Fatalf("plan request = %#v", request)
					}
					return HTTPResponse{StatusCode: 200, Body: []byte(test.response)}, nil
				},
			})
			if !result.Handled || result.Annotations["plan_key"] != test.wantPlan || result.Annotations["plan_source"] != "provider-api" {
				t.Fatalf("Filter() = %#v", result)
			}
		})
	}
}
