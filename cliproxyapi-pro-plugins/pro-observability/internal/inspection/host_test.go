package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type gatewayStub struct {
	auths       []HostAuthEntry
	patch       HostHealthPatch
	delete      string
	listErr     error
	httpRequest HostHTTPRequest
}

func (g *gatewayStub) List(context.Context) ([]HostAuthEntry, error) { return g.auths, g.listErr }
func (g *gatewayStub) Refresh(context.Context, string, bool) (HostAuthRefreshResponse, error) {
	return HostAuthRefreshResponse{}, nil
}
func (g *gatewayStub) HTTPDo(_ context.Context, request HostHTTPRequest) (HostHTTPResponse, error) {
	g.httpRequest = request
	return HostHTTPResponse{StatusCode: http.StatusOK}, nil
}

func TestCompatManagerPreservesListFailure(t *testing.T) {
	want := errors.New("host list unavailable")
	manager := &compatAuthManager{gateway: &gatewayStub{listErr: want}}
	if _, err := manager.ListContext(context.Background()); !errors.Is(err, want) {
		t.Fatalf("ListContext() error = %v, want %v", err, want)
	}
}

func TestCompatManagerPropagatesRequestDeadline(t *testing.T) {
	gateway := &gatewayStub{}
	manager := &compatAuthManager{gateway: gateway}
	auth := &Auth{Index: "codex:auth-1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/probe", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, err = manager.HttpRequest(ctx, auth, request); err != nil {
		t.Fatalf("HttpRequest() error = %v", err)
	}
	if gateway.httpRequest.TimeoutMS < 1 || gateway.httpRequest.TimeoutMS > 2500 {
		t.Fatalf("forwarded timeout = %dms", gateway.httpRequest.TimeoutMS)
	}
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
		InspectionMetadata:  map[string]any{"base_url": "https://api.x.ai"},
		InspectionAttrs:     map[string]string{"api_key_configured": "true"},
		InspectionUserAgent: "provider-agent/1.0",
		InspectionError:     &HostHealthError{Code: "inspection_http_error", Message: "HTTP 401", HTTPStatus: 401},
	})
	if auth.Metadata["account_id"] != "account-1" || auth.Metadata["base_url"] != "https://api.x.ai" || auth.Attributes["api_key_configured"] != "true" {
		t.Fatalf("auth projection = %#v", auth)
	}
	if auth.Attributes["inspection_user_agent"] != "provider-agent/1.0" {
		t.Fatalf("inspection user agent = %#v", auth.Attributes)
	}
	if auth.LastError == nil || auth.LastError.Code != "inspection_http_error" || auth.LastError.HTTPStatus != 401 {
		t.Fatalf("inspection error projection = %#v", auth.LastError)
	}
}

func TestSuccessfulInspectionClearsProjectedInspectionError(t *testing.T) {
	gateway := &gatewayStub{auths: []HostAuthEntry{{
		ID: "auth-1", AuthIndex: "codex:auth-1", Name: "auth.json", Provider: "codex", Revision: 42,
		InspectionError: &HostHealthError{Code: "inspection_http_error", Message: "HTTP 401", HTTPStatus: 401},
	}}}
	scheduler := &accountInspectionScheduler{h: newCompatHandler(context.Background(), gateway)}
	scheduler.clearInspectionAuthError(context.Background(), accountInspectionAccount{AuthIndex: "codex:auth-1"})
	if !gateway.patch.ClearError || gateway.patch.Error != nil || gateway.patch.ExpectedRevision != 42 {
		t.Fatalf("clear patch = %#v", gateway.patch)
	}
}

func TestInspectionProbeHeadersUseHostDefaults(t *testing.T) {
	auth := &Auth{Attributes: map[string]string{"inspection_user_agent": "host-agent/1.0"}}
	scheduler := &accountInspectionScheduler{}
	if got := scheduler.codexUserAgent(auth); got != "host-agent/1.0" {
		t.Fatalf("codex user agent = %q", got)
	}
	if got := scheduler.claudeHeaders(auth)["User-Agent"]; got != "host-agent/1.0" {
		t.Fatalf("claude user agent = %q", got)
	}
}

func TestActionRejectsStaleRuntimeIdentity(t *testing.T) {
	gateway := &gatewayStub{auths: []HostAuthEntry{
		{ID: "auth-a", AuthIndex: "codex:auth-a", Name: "a.json", Provider: "codex", Revision: 1},
		{ID: "auth-b", AuthIndex: "codex:auth-b", Name: "b.json", Provider: "codex", Revision: 1},
	}}
	scheduler := &accountInspectionScheduler{h: newCompatHandler(context.Background(), gateway)}
	result := accountFromAuth(authFromHostEntry(gateway.auths[0])).baseResult()
	result.AuthIndex = gateway.auths[1].AuthIndex
	if err := scheduler.executeAction(context.Background(), result, accountInspectionActionDisable); !errors.Is(err, errAccountInspectionResultStale) {
		t.Fatalf("executeAction() error = %v, want stale result", err)
	}
	if gateway.patch.AuthIndex != "" {
		t.Fatalf("stale action reached host patch: %#v", gateway.patch)
	}
}
