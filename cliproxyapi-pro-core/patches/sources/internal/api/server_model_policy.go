package api

import (
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/client/grokbuild"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
)

func (s *Server) filterHomeModelEntries(c *gin.Context, entries []homeModelEntry) ([]homeModelEntry, bool) {
	ids := make([]string, 0, len(entries))
	byID := make(map[string]homeModelEntry, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.id)
		byID[entry.id] = entry
	}
	visible, policyErr := handlers.FilterModelsForRequest(c.Request.Context(), ids, func(string) []string { return []string{"home"} })
	if policyErr != nil {
		s.handlers.WriteErrorResponse(c, policyErr)
		return nil, false
	}
	out := make([]homeModelEntry, 0, len(visible))
	for _, model := range visible {
		entry, exists := byID[model.EffectiveID]
		if !exists {
			continue
		}
		entry.id = model.ID
		out = append(out, entry)
	}
	return out, true
}

func (s *Server) filterRegistryGrokModels(c *gin.Context, infos []*registry.ModelInfo) ([]grokbuild.ModelInfo, bool) {
	ids := make([]string, 0, len(infos))
	byID := make(map[string]*registry.ModelInfo, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		ids = append(ids, info.ID)
		byID[info.ID] = info
	}
	visible, policyErr := handlers.FilterModelsForRequest(c.Request.Context(), ids, registry.GetGlobalRegistry().GetModelProviders)
	if policyErr != nil {
		s.handlers.WriteErrorResponse(c, policyErr)
		return nil, false
	}
	out := make([]grokbuild.ModelInfo, 0, len(visible))
	for _, model := range visible {
		info := byID[model.EffectiveID]
		if info == nil {
			continue
		}
		entry := grokbuild.ModelInfo{ID: model.ID, DisplayName: info.DisplayName, ContextLength: info.ContextLength}
		if info.Thinking != nil {
			entry.ReasoningLevels = append([]string(nil), info.Thinking.Levels...)
		}
		out = append(out, entry)
	}
	return out, true
}
