package inspection

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type gatewayStub struct {
	auths  []HostAuthEntry
	patch  HostHealthPatch
	delete string
}

func (g *gatewayStub) List(context.Context) ([]HostAuthEntry, error) { return g.auths, nil }
func (g *gatewayStub) Refresh(context.Context, string, bool) (HostAuthRefreshResponse, error) {
	return HostAuthRefreshResponse{}, nil
}
func (g *gatewayStub) HTTPDo(context.Context, HostHTTPRequest) (HostHTTPResponse, error) {
	return HostHTTPResponse{StatusCode: http.StatusOK}, nil
}
func (g *gatewayStub) PatchHealth(_ context.Context, patch HostHealthPatch) (HostAuthEntry, error) {
	g.patch = patch
	return g.auths[0], nil
}
func (g *gatewayStub) Delete(context.Context, string, int64) (string, error) { return g.delete, nil }
func (g *gatewayStub) FetchQuota(context.Context, string) (HostQuotaResponse, error) {
	return HostQuotaResponse{Handled: true, Snapshot: json.RawMessage(`{"schema_version":1,"items":[]}`)}, nil
}

func TestCompatManagerWritesOnlyInspectionHealthPatch(t *testing.T) {
	gateway := &gatewayStub{auths: []HostAuthEntry{{
		ID: "auth-1", AuthIndex: "codex:auth-1", Name: "auth.json", Provider: "codex", Revision: 42,
	}}}
	manager := &compatAuthManager{gateway: gateway}
	auth := manager.List()[0]
	auth.Disabled = true
	auth.LastError = &AuthError{Code: "inspection_http_error", Message: "HTTP 401", HTTPStatus: 401}
	if _, err := manager.Update(context.Background(), auth); err != nil {
		t.Fatalf("update auth: %v", err)
	}
	if gateway.patch.AuthIndex != auth.Index || gateway.patch.ExpectedRevision != 42 || gateway.patch.Disabled == nil || !*gateway.patch.Disabled {
		t.Fatalf("patch = %#v", gateway.patch)
	}
	if gateway.patch.Error == nil || gateway.patch.Error.HTTPStatus != 401 {
		t.Fatalf("health error = %#v", gateway.patch.Error)
	}
}

func TestHostEntryKeepsOnlyExplicitInspectionProjection(t *testing.T) {
	auth := authFromHostEntry(HostAuthEntry{
		ID: "xai-1", AuthIndex: "xai:xai-1", Name: "xai.json", Provider: "xai", AccountID: "account-1",
		InspectionMetadata: map[string]any{"base_url": "https://api.x.ai"},
		InspectionAttrs:    map[string]string{"api_key_configured": "true"},
	})
	if auth.Metadata["account_id"] != "account-1" || auth.Metadata["base_url"] != "https://api.x.ai" || auth.Attributes["api_key_configured"] != "true" {
		t.Fatalf("auth projection = %#v", auth)
	}
}
