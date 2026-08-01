package host

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	modelpolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/modelpolicy/policy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// ModelFilter is the business capability consumed by the upstream adapter.
type ModelFilter interface {
	Filter(context.Context, modelpolicy.Input) modelpolicy.Result
}

// FilterModels converts volatile upstream auth/model types at the boundary,
// leaving the model-policy module independent from host internals.
func FilterModels(ctx context.Context, hostCfg *internalconfig.Config, auth *coreauth.Auth, models []*registry.ModelInfo, filter ModelFilter) []*registry.ModelInfo {
	if auth == nil || len(models) == 0 || filter == nil {
		return models
	}
	inputModels := make([]modelpolicy.ModelInfo, 0, len(models))
	for _, model := range models {
		if model != nil {
			inputModels = append(inputModels, modelpolicy.ModelInfo{ID: model.ID})
		}
	}
	result := filter.Filter(ctx, modelpolicy.Input{
		AuthID: auth.ID, AuthProvider: auth.Provider, AuthKind: auth.AuthKind(),
		StorageJSON: storageJSONFromAuth(auth), Metadata: auth.Metadata, Attributes: auth.Attributes, Models: inputModels,
		HTTPDo: func(callCtx context.Context, req modelpolicy.HTTPRequest) (modelpolicy.HTTPResponse, error) {
			return doPolicyHTTP(callCtx, hostCfg, auth, req)
		},
	})
	if len(result.ExcludedModelIDs) == 0 {
		return models
	}
	blocked := make(map[string]struct{}, len(result.ExcludedModelIDs))
	for _, id := range result.ExcludedModelIDs {
		blocked[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	filtered := make([]*registry.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		if _, found := blocked[strings.ToLower(strings.TrimSpace(model.ID))]; !found {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func storageJSONFromAuth(auth *coreauth.Auth) []byte {
	if rawProvider, ok := auth.Storage.(interface{ RawJSON() []byte }); ok {
		return bytes.Clone(rawProvider.RawJSON())
	}
	raw, _ := json.Marshal(auth.Metadata)
	return raw
}

func doPolicyHTTP(ctx context.Context, cfg *internalconfig.Config, auth *coreauth.Auth, req modelpolicy.HTTPRequest) (modelpolicy.HTTPResponse, error) {
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return modelpolicy.HTTPResponse{}, err
	}
	for key, values := range req.Headers {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	client := helps.NewProxyAwareHTTPClient(ctx, cfg, auth, 0)
	resp, err := client.Do(httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, cfg, err)
		return modelpolicy.HTTPResponse{}, err
	}
	defer resp.Body.Close()
	helps.RecordAPIResponseMetadata(ctx, cfg, resp.StatusCode, resp.Header.Clone())
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		helps.RecordAPIResponseError(ctx, cfg, err)
		return modelpolicy.HTTPResponse{}, err
	}
	if len(body) > 0 {
		helps.AppendAPIResponseChunk(ctx, cfg, body)
	}
	headers := make(map[string][]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = append([]string(nil), values...)
	}
	return modelpolicy.HTTPResponse{StatusCode: resp.StatusCode, Headers: headers, Body: body}, nil
}
