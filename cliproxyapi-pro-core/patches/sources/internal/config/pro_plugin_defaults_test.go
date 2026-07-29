package config

import "testing"

func TestEnsureProObservabilityPluginDefaultsEnablesSingleStack(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("USAGE_DATA_DIR", dataDir)
	cfg := &Config{}
	cfg.Routing.Strategy = "fill-first"
	cfg.EnsureProObservabilityPluginDefaults()
	if !cfg.Plugins.Enabled {
		t.Fatal("plugins are disabled")
	}
	item, ok := cfg.Plugins.Configs[ProObservabilityPluginID]
	if !ok || item.Enabled == nil || !*item.Enabled {
		t.Fatalf("pro-observability config = %#v", item)
	}
	var raw struct {
		Enabled         bool   `yaml:"enabled"`
		RoutingStrategy string `yaml:"routing-strategy"`
		DBPath          string `yaml:"db-path"`
		LegacyDBPath    string `yaml:"legacy-db-path"`
	}
	if err := item.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if !raw.Enabled || raw.RoutingStrategy != "fill-first" || raw.DBPath != raw.LegacyDBPath || raw.DBPath == "" {
		t.Fatalf("raw config = %#v", raw)
	}
}

func TestEnsureProObservabilityPluginDefaultsOverridesDisable(t *testing.T) {
	t.Setenv("USAGE_DB_PATH", "/data/legacy.sqlite")
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", "/data/plugin.sqlite")
	cfg, err := ParseConfigBytes([]byte(`plugins:
  enabled: false
  configs:
    pro-observability:
      enabled: false
routing:
  strategy: weighted-round-robin
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.EnsureProObservabilityPluginDefaults()
	item := cfg.Plugins.Configs[ProObservabilityPluginID]
	if !cfg.Plugins.Enabled || item.Enabled == nil || !*item.Enabled {
		t.Fatalf("required plugin was not enabled: %#v", cfg.Plugins)
	}
	var raw map[string]any
	if err = item.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["enabled"] != true || raw["routing-strategy"] != "weighted-round-robin" {
		t.Fatalf("raw config = %#v", raw)
	}
	if raw["db-path"] != "/data/plugin.sqlite" || raw["legacy-db-path"] != "/data/legacy.sqlite" {
		t.Fatalf("database config = %#v", raw)
	}
}

func TestEnsureProObservabilityPluginDefaultsDisablesPlaceholderExample(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`plugins:
  enabled: false
  configs:
    example:
      enabled: true
      mode: safe
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.EnsureProObservabilityPluginDefaults()
	example := cfg.Plugins.Configs["example"]
	if example.Enabled == nil || *example.Enabled {
		t.Fatalf("placeholder example remained enabled: %#v", example)
	}
	var raw map[string]any
	if err = example.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["enabled"] != false || raw["mode"] != "safe" {
		t.Fatalf("placeholder config = %#v", raw)
	}
}

func TestEnsureProObservabilityPluginDefaultsPreservesConfiguredPaths(t *testing.T) {
	t.Setenv("USAGE_DB_PATH", "")
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", "")
	cfg, err := ParseConfigBytes([]byte(`plugins:
  configs:
    pro-observability:
      db-path: /custom/plugin.sqlite
      legacy-db-path: /custom/legacy.sqlite
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.EnsureProObservabilityPluginDefaults()
	item := cfg.Plugins.Configs[ProObservabilityPluginID]
	var raw map[string]any
	if err = item.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["db-path"] != "/custom/plugin.sqlite" || raw["legacy-db-path"] != "/custom/legacy.sqlite" {
		t.Fatalf("database config = %#v", raw)
	}
}

func TestEnsureProObservabilityPluginDefaultsRetiresHistoricalShadowPath(t *testing.T) {
	t.Setenv("USAGE_DATA_DIR", "/production")
	t.Setenv("USAGE_DB_PATH", "")
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", "")
	cfg, err := ParseConfigBytes([]byte(`plugins:
  configs:
    pro-observability:
      db-path: /shadow/usage-plugin.sqlite
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.EnsureProObservabilityPluginDefaults()
	item := cfg.Plugins.Configs[ProObservabilityPluginID]
	var raw map[string]any
	if err = item.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["db-path"] != "/production/usage.sqlite" || raw["legacy-db-path"] != "/production/usage.sqlite" {
		t.Fatalf("database config = %#v", raw)
	}
}

func TestEnsureProObservabilityPluginDefaultsHonorsExplicitShadowNamedTarget(t *testing.T) {
	t.Setenv("USAGE_DB_PATH", "/production/usage.sqlite")
	t.Setenv("PRO_OBSERVABILITY_DB_PATH", "/explicit/usage-plugin.sqlite")
	cfg := &Config{}
	cfg.EnsureProObservabilityPluginDefaults()
	item := cfg.Plugins.Configs[ProObservabilityPluginID]
	var raw map[string]any
	if err := item.Raw.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw["db-path"] != "/explicit/usage-plugin.sqlite" {
		t.Fatalf("database config = %#v", raw)
	}
}
