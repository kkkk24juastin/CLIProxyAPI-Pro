package config

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultCacheTTL       = 30 * time.Minute
	DefaultResolveTimeout = 15 * time.Second
)

type Config struct {
	Enabled        bool
	CacheTTL       time.Duration
	ResolveTimeout time.Duration
	Providers      map[string]Provider
}

type Provider struct {
	Plans map[string]Plan `yaml:"plans" json:"plans"`
}

type Plan struct {
	ExcludedModels []string `yaml:"excluded-models" json:"excluded-models"`
	Prefix         *string  `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Priority       *int     `yaml:"priority,omitempty" json:"priority,omitempty"`
	Weight         *int64   `yaml:"weight,omitempty" json:"weight,omitempty"`
}

type rawConfig struct {
	Enabled        *bool               `yaml:"enabled" json:"enabled"`
	CacheTTL       string              `yaml:"cache-ttl" json:"cache-ttl"`
	ResolveTimeout string              `yaml:"resolve-timeout" json:"resolve-timeout"`
	Providers      map[string]Provider `yaml:"providers" json:"providers"`
}

func Parse(raw []byte) (Config, error) {
	decoded := rawConfig{}
	if len(raw) > 0 {
		if errUnmarshal := yaml.Unmarshal(raw, &decoded); errUnmarshal != nil {
			return Config{}, fmt.Errorf("parse oauth account policy config: %w", errUnmarshal)
		}
	}
	cfg := Config{Enabled: len(decoded.Providers) > 0, CacheTTL: DefaultCacheTTL, ResolveTimeout: DefaultResolveTimeout, Providers: map[string]Provider{}}
	if decoded.Enabled != nil {
		cfg.Enabled = *decoded.Enabled
	}
	var err error
	if strings.TrimSpace(decoded.CacheTTL) != "" {
		cfg.CacheTTL, err = time.ParseDuration(strings.TrimSpace(decoded.CacheTTL))
		if err != nil || cfg.CacheTTL <= 0 {
			return Config{}, fmt.Errorf("cache-ttl must be a positive duration")
		}
	}
	if strings.TrimSpace(decoded.ResolveTimeout) != "" {
		cfg.ResolveTimeout, err = time.ParseDuration(strings.TrimSpace(decoded.ResolveTimeout))
		if err != nil || cfg.ResolveTimeout <= 0 {
			return Config{}, fmt.Errorf("resolve-timeout must be a positive duration")
		}
	}
	seenProviders := make(map[string]string, len(decoded.Providers))
	for rawProvider, provider := range decoded.Providers {
		providerKey := normalizeKey(rawProvider)
		if providerKey == "" {
			continue
		}
		if previous, exists := seenProviders[providerKey]; exists {
			return Config{}, fmt.Errorf("providers contains duplicate normalized key %q from %q and %q", providerKey, previous, rawProvider)
		}
		seenProviders[providerKey] = rawProvider
		clean := Provider{Plans: map[string]Plan{}}
		seenPlans := make(map[string]string, len(provider.Plans))
		for rawPlan, plan := range provider.Plans {
			planKey := normalizePlanKey(providerKey, rawPlan)
			if planKey == "" {
				continue
			}
			if previous, exists := seenPlans[planKey]; exists {
				return Config{}, fmt.Errorf("providers.%s.plans contains duplicate normalized key %q from %q and %q", providerKey, planKey, previous, rawPlan)
			}
			seenPlans[planKey] = rawPlan
			patterns := make([]string, 0, len(plan.ExcludedModels))
			seen := map[string]struct{}{}
			for _, pattern := range plan.ExcludedModels {
				pattern = strings.ToLower(strings.TrimSpace(pattern))
				if pattern == "" {
					continue
				}
				if _, errMatch := path.Match(pattern, ""); errMatch != nil {
					return Config{}, fmt.Errorf("providers.%s.plans.%s.excluded-models contains invalid pattern %q: %w", providerKey, planKey, pattern, errMatch)
				}
				if _, exists := seen[pattern]; exists {
					continue
				}
				seen[pattern] = struct{}{}
				patterns = append(patterns, pattern)
			}
			cleanPlan := Plan{ExcludedModels: patterns, Priority: plan.Priority, Weight: plan.Weight}
			if plan.Prefix != nil {
				prefix := strings.Trim(strings.TrimSpace(*plan.Prefix), "/")
				if prefix != "" && strings.Contains(prefix, "/") {
					return Config{}, fmt.Errorf("providers.%s.plans.%s.prefix must be one path segment", providerKey, planKey)
				}
				cleanPlan.Prefix = &prefix
			}
			if plan.Weight != nil {
				weight := *plan.Weight
				if weight <= 0 {
					weight = 0
				}
				if weight > 1_000_000 {
					return Config{}, fmt.Errorf("providers.%s.plans.%s.weight must not exceed 1000000", providerKey, planKey)
				}
				cleanPlan.Weight = &weight
			}
			clean.Plans[planKey] = cleanPlan
		}
		cfg.Providers[providerKey] = clean
	}
	return cfg, nil
}

func Marshal(cfg Config) ([]byte, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	enabled := normalized.Enabled
	return json.Marshal(rawConfig{
		Enabled:        &enabled,
		CacheTTL:       normalized.CacheTTL.String(),
		ResolveTimeout: normalized.ResolveTimeout.String(),
		Providers:      normalized.Providers,
	})
}

func normalizeConfig(cfg Config) (Config, error) {
	enabled := cfg.Enabled
	raw, err := yaml.Marshal(rawConfig{
		Enabled:        &enabled,
		CacheTTL:       cfg.CacheTTL.String(),
		ResolveTimeout: cfg.ResolveTimeout.String(),
		Providers:      cfg.Providers,
	})
	if err != nil {
		return Config{}, err
	}
	return Parse(raw)
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "_") {
		return "_" + strings.ReplaceAll(strings.TrimPrefix(value, "_"), "_", "-")
	}
	return strings.ReplaceAll(value, "_", "-")
}

func normalizePlanKey(provider, value string) string {
	key := normalizeKey(value)
	if strings.HasPrefix(key, "plan-") {
		key = strings.TrimPrefix(key, "plan-")
	}
	switch provider {
	case "xai":
		switch key {
		case "super-grok":
			return "supergrok"
		case "super-grok-heavy":
			return "supergrok-heavy"
		}
	case "codex":
		if key == "prolite" {
			return "pro-lite"
		}
	case "gemini-cli":
		switch key {
		case "free-tier":
			return "free"
		case "legacy-tier":
			return "legacy"
		case "standard-tier":
			return "standard"
		case "g1-pro-tier", "pro-tier":
			return "pro"
		case "g1-ultra-tier", "ultra-tier":
			return "ultra"
		}
	case "antigravity":
		switch key {
		case "free-tier":
			return "free"
		case "g1-pro-tier":
			return "pro"
		case "g1-ultra-tier":
			return "ultra"
		case "g1-ultra-lite-tier":
			return "ultra-lite"
		}
	}
	return key
}
