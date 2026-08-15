package host

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	oauthpolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/policy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type policyProbeRequester struct {
	prepareCalls int
	executeCalls int
}

func (r *policyProbeRequester) PrepareHttpRequest(_ context.Context, _ *coreauth.Auth, req *http.Request) error {
	r.prepareCalls++
	req.Header.Set("Authorization", "Bearer refreshed-token")
	return nil
}

func (r *policyProbeRequester) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	r.executeCalls++
	return nil, nil
}

func TestDoPolicyHTTPBypassesExecutorWithoutLosingPreparedCredential(t *testing.T) {
	const wantUserAgent = "auth-card-agent/current"
	var gotAuthorization, gotUserAgent, gotContentType, gotAccept, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuthorization = req.Header.Get("Authorization")
		gotUserAgent = req.Header.Get("User-Agent")
		gotContentType = req.Header.Get("Content-Type")
		gotAccept = req.Header.Get("Accept")
		body, _ := io.ReadAll(req.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"paidTier":{"id":"g1-pro-tier"}}`)
	}))
	t.Cleanup(server.Close)

	requester := &policyProbeRequester{}
	response, err := doPolicyHTTP(context.Background(), nil, &coreauth.Auth{ID: "antigravity-test", Provider: "antigravity"}, oauthpolicy.HTTPRequest{
		Method:         http.MethodPost,
		URL:            server.URL,
		Headers:        http.Header{"Authorization": []string{"Bearer stale-token"}, "Content-Type": []string{"application/json"}, "User-Agent": []string{wantUserAgent}},
		Body:           []byte(`{"metadata":{"ideType":"ANTIGRAVITY"}}`),
		BypassExecutor: true,
	}, requester)
	if err != nil {
		t.Fatalf("doPolicyHTTP() error = %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), "g1-pro-tier") {
		t.Fatalf("doPolicyHTTP() response = %#v", response)
	}
	if requester.prepareCalls != 1 || requester.executeCalls != 0 {
		t.Fatalf("requester calls: prepare=%d execute=%d", requester.prepareCalls, requester.executeCalls)
	}
	if gotAuthorization != "Bearer refreshed-token" || gotUserAgent != wantUserAgent || gotContentType != "application/json" || gotAccept != "" {
		t.Fatalf("request headers: authorization=%q user-agent=%q content-type=%q accept=%q", gotAuthorization, gotUserAgent, gotContentType, gotAccept)
	}
	if gotBody != `{"metadata":{"ideType":"ANTIGRAVITY"}}` {
		t.Fatalf("request body = %q", gotBody)
	}
}
