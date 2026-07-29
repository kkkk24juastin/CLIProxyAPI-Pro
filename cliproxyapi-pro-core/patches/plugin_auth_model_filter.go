package pluginhost

import (
	"context"
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

// AuthModelFilterResult is the host result after every active filter has run.
type AuthModelFilterResult struct {
	Models  []*registry.ModelInfo
	Handled bool
}

// FilterModelsForAuth runs every active auth-model filter in plugin priority order.
// Each filter can only subtract IDs from the current set, so filters compose safely.
func (h *Host) FilterModelsForAuth(ctx context.Context, auth *coreauth.Auth, models []*registry.ModelInfo) AuthModelFilterResult {
	current := cloneRegistryModels(models)
	if h == nil || auth == nil || len(current) == 0 {
		return AuthModelFilterResult{Models: current}
	}
	handled := false
	for _, record := range h.activeRecords() {
		filter := record.plugin.Capabilities.AuthModelFilter
		if filter == nil || h.isPluginFused(record.id) {
			continue
		}
		resp, errFilter := h.callAuthModelFilter(ctx, record, filter, auth, current)
		if errFilter != nil {
			log.WithField("plugin_id", record.id).WithError(errFilter).Warn("pluginhost: auth model filter failed open")
			continue
		}
		if !resp.Handled {
			continue
		}
		handled = true
		current = subtractFilteredModelIDs(current, resp.ExcludedModelIDs)
		if len(current) == 0 {
			break
		}
	}
	return AuthModelFilterResult{Models: current, Handled: handled}
}

func (h *Host) callAuthModelFilter(ctx context.Context, record capabilityRecord, filter pluginapi.AuthModelFilter, auth *coreauth.Auth, models []*registry.ModelInfo) (resp pluginapi.AuthModelFilterResponse, err error) {
	if h == nil || filter == nil || auth == nil || h.isPluginFused(record.id) || !h.recordCurrent(record) {
		return pluginapi.AuthModelFilterResponse{}, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "AuthModelFilter.FilterAuthModels", recovered)
			resp = pluginapi.AuthModelFilterResponse{}
			err = fmt.Errorf("auth model filter panic: %v", recovered)
		}
	}()
	pluginModels := make([]pluginapi.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		pluginModels = append(pluginModels, registryModelInfoToPluginModelInfo(model))
	}
	authIndex := strings.TrimSpace(auth.Index)
	if authIndex == "" {
		authIndex = strings.TrimSpace(auth.EnsureIndex())
	}
	return filter.FilterAuthModels(ctx, pluginapi.AuthModelFilterRequest{
		Plugin:       clonePluginMetadata(record.meta),
		AuthID:       auth.ID,
		AuthIndex:    authIndex,
		AuthProvider: auth.Provider,
		AuthKind:     auth.AuthKind(),
		StorageJSON:  storageJSONFromAuth(auth),
		Metadata:     cloneAnyMap(auth.Metadata),
		Attributes:   cloneStringMap(auth.Attributes),
		Models:       pluginModels,
		Host:         h.hostConfigSummary(),
		HTTPClient:   h.newHTTPClient(auth, auth.Provider),
	})
}

func subtractFilteredModelIDs(models []*registry.ModelInfo, excluded []string) []*registry.ModelInfo {
	if len(models) == 0 || len(excluded) == 0 {
		return models
	}
	blocked := make(map[string]struct{}, len(excluded))
	for _, modelID := range excluded {
		if key := strings.ToLower(strings.TrimSpace(modelID)); key != "" {
			blocked[key] = struct{}{}
		}
	}
	if len(blocked) == 0 {
		return models
	}
	filtered := make([]*registry.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		if _, found := blocked[strings.ToLower(strings.TrimSpace(model.ID))]; found {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered
}
