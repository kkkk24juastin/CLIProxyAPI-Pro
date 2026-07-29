package config

import (
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
	CacheTTL       time.Duration
	ResolveTimeout time.Duration
	Providers      map[string]Provider
}

type Provider struct {
	Plans map[string]Plan `yaml:"plans"`
}

type Plan struct {
	ExcludedModels []string `yaml:"excluded-models"`
}

type rawConfig struct {
	CacheTTL       string              `yaml:"cache-ttl"`
	ResolveTimeout string              `yaml:"resolve-timeout"`
	Providers      map[string]Provider `yaml:"providers"`
}

func Parse(raw []byte) (Config, error) {
	decoded := rawConfig{}
	if len(raw) > 0 {
		if errUnmarshal := yaml.Unmarshal(raw, &decoded); errUnmarshal != nil {
			return Config{}, fmt.Errorf("parse oauth model policy config: %w", errUnmarshal)
		}
	}
	cfg := Config{CacheTTL: DefaultCacheTTL, ResolveTimeout: DefaultResolveTimeout, Providers: map[string]Provider{}}
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
	for rawProvider, provider := range decoded.Providers {
		providerKey := normalizeKey(rawProvider)
		if providerKey == "" {
			continue
		}
		clean := Provider{Plans: map[string]Plan{}}
		for rawPlan, plan := range provider.Plans {
			planKey := normalizeKey(rawPlan)
			if planKey == "" {
				continue
			}
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
			clean.Plans[planKey] = Plan{ExcludedModels: patterns}
		}
		cfg.Providers[providerKey] = clean
	}
	return cfg, nil
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "_") {
		return "_" + strings.ReplaceAll(strings.TrimPrefix(value, "_"), "_", "-")
	}
	return strings.ReplaceAll(value, "_", "-")
}
