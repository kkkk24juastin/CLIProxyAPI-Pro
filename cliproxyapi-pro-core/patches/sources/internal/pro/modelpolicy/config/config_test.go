package config

import "testing"

func TestParseNormalizesXAIPlans(t *testing.T) {
	cfg, errParse := Parse([]byte(`
cache-ttl: 10m
providers:
  XAI:
    plans:
      X_PREMIUM_PLUS:
        excluded-models: [" GROK-4-* ", "grok-4-*"]
      _unknown:
        excluded-models: ["grok-pro-*"]
`))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	plan := cfg.Providers["xai"].Plans["x-premium-plus"]
	if len(plan.ExcludedModels) != 1 || plan.ExcludedModels[0] != "grok-4-*" {
		t.Fatalf("excluded models = %#v", plan.ExcludedModels)
	}
	if _, ok := cfg.Providers["xai"].Plans["_unknown"]; !ok {
		t.Fatal("_unknown fallback plan was not preserved")
	}
}

func TestParseRejectsInvalidModelPattern(t *testing.T) {
	_, errParse := Parse([]byte(`
providers:
  xai:
    plans:
      free:
        excluded-models: ["grok-["]
`))
	if errParse == nil {
		t.Fatal("Parse() error = nil, want invalid pattern error")
	}
}

func TestParseCanonicalizesProviderPlanAliases(t *testing.T) {
	cfg, errParse := Parse([]byte(`
providers:
  claude:
    plans:
      plan_max: {excluded-models: [claude-opus-*]}
  gemini-cli:
    plans:
      g1-ultra-tier: {excluded-models: [gemini-pro-*]}
  antigravity:
    plans:
      g1-ultra-lite-tier: {excluded-models: [claude-*]}
`))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	for provider, plan := range map[string]string{
		"claude": "max", "gemini-cli": "ultra", "antigravity": "ultra-lite",
	} {
		if _, ok := cfg.Providers[provider].Plans[plan]; !ok {
			t.Fatalf("providers.%s.plans.%s was not canonicalized", provider, plan)
		}
	}
}
