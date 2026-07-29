package pluginhost

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type authModelFilterStub struct {
	handled  bool
	excluded []string
}

func (f authModelFilterStub) FilterAuthModels(context.Context, pluginapi.AuthModelFilterRequest) (pluginapi.AuthModelFilterResponse, error) {
	return pluginapi.AuthModelFilterResponse{Handled: f.handled, ExcludedModelIDs: append([]string(nil), f.excluded...)}, nil
}

func TestFilterModelsForAuthChainsSubtractiveFilters(t *testing.T) {
	host := newHostWithRecords(
		capabilityRecord{id: "first", priority: 20, plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{AuthModelFilter: authModelFilterStub{handled: true, excluded: []string{"MODEL-B", "not-present"}}}}},
		capabilityRecord{id: "second", priority: 10, plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{AuthModelFilter: authModelFilterStub{handled: true, excluded: []string{"model-c"}}}}},
	)
	result := host.FilterModelsForAuth(context.Background(), &coreauth.Auth{ID: "auth-1", Provider: "xai"}, []*registry.ModelInfo{
		{ID: "model-a"}, {ID: "model-b"}, {ID: "model-c"},
	})
	if !result.Handled {
		t.Fatal("FilterModelsForAuth() handled = false")
	}
	if len(result.Models) != 1 || result.Models[0].ID != "model-a" {
		t.Fatalf("FilterModelsForAuth() models = %#v, want model-a", result.Models)
	}
}
