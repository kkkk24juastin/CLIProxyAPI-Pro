package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProObservabilityPluginID = "pro-observability"

// EnsureProObservabilityPluginDefaults makes the plugin-owned persistence stack
// mandatory for Pro builds. Existing routing configuration remains the source
// of truth and is forwarded to the plugin.
func (cfg *Config) EnsureProObservabilityPluginDefaults() {
	if cfg == nil {
		return
	}
	cfg.NormalizePluginsConfig()
	cfg.Plugins.Enabled = true
	enabled := true
	if example, ok := cfg.Plugins.Configs["example"]; ok {
		disabled := false
		example.Enabled = &disabled
		example.Raw = *ensurePluginMappingNode(&example.Raw)
		setPluginConfigScalar(&example.Raw, "enabled", "!!bool", "false")
		cfg.Plugins.Configs["example"] = example
	}
	item := cfg.Plugins.Configs[ProObservabilityPluginID]
	item.Enabled = &enabled
	item.Raw = *ensurePluginMappingNode(&item.Raw)
	setPluginConfigScalar(&item.Raw, "enabled", "!!bool", "true")
	existingDBPath := pluginConfigScalar(&item.Raw, "db-path")
	existingLegacyDBPath := pluginConfigScalar(&item.Raw, "legacy-db-path")
	dataDir := strings.TrimSpace(os.Getenv("USAGE_DATA_DIR"))
	if dataDir == "" {
		dataDir = "/CLIProxyAPI/usage"
	}
	legacyDBPath := strings.TrimSpace(os.Getenv("USAGE_DB_PATH"))
	if legacyDBPath == "" {
		legacyDBPath = existingLegacyDBPath
	}
	if legacyDBPath == "" {
		legacyDBPath = filepath.Join(dataDir, "usage.sqlite")
	}
	targetDBPath := strings.TrimSpace(os.Getenv("PRO_OBSERVABILITY_DB_PATH"))
	if targetDBPath == "" {
		targetDBPath = existingDBPath
		if targetDBPath == "" || filepath.Base(filepath.Clean(targetDBPath)) == "usage-plugin.sqlite" {
			targetDBPath = legacyDBPath
		}
	}
	setPluginConfigScalar(&item.Raw, "db-path", "!!str", targetDBPath)
	setPluginConfigScalar(&item.Raw, "legacy-db-path", "!!str", legacyDBPath)
	strategy := strings.ToLower(strings.TrimSpace(cfg.Routing.Strategy))
	switch strategy {
	case "round-robin", "weighted-round-robin", "fill-first":
	default:
		strategy = "round-robin"
	}
	setPluginConfigScalar(&item.Raw, "routing-strategy", "!!str", strategy)
	cfg.Plugins.Configs[ProObservabilityPluginID] = item
}

func pluginConfigScalar(mapping *yaml.Node, key string) string {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key && mapping.Content[index+1].Kind == yaml.ScalarNode {
			return strings.TrimSpace(mapping.Content[index+1].Value)
		}
	}
	return ""
}

func ensurePluginMappingNode(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.MappingNode {
		return deepCopyNode(node)
	}
	return defaultPluginInstanceConfigNode()
}

func setPluginConfigScalar(mapping *yaml.Node, key, tag, value string) {
	if mapping == nil {
		return
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		mapping.Content[index+1].Kind = yaml.ScalarNode
		mapping.Content[index+1].Tag = tag
		mapping.Content[index+1].Value = value
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}
