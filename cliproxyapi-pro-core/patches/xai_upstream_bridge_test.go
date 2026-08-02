package executor

import (
	"net/http"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestProXAIChatRequestHeadersReuseUpstreamIdentity(t *testing.T) {
	auth := &cliproxyauth.Auth{
		Provider:   "xai",
		Attributes: map[string]string{"using_api": "false"},
	}
	headers := XAIChatRequestHeaders(auth, "token", false)
	if got := headers.Get(xaiClientVersionHeader); got != xaiClientVersionValue {
		t.Fatalf("%s = %q, want %q", xaiClientVersionHeader, got, xaiClientVersionValue)
	}
	if got := headers.Get("User-Agent"); got != "xai-grok-workspace/"+xaiClientVersionValue {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestProXAIHTTPRequestIdentityOverridesStaleManagementHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://cli-chat-proxy.grok.com/v1/billing", nil)
	req.Header.Set(xaiClientVersionHeader, "0.2.91")
	req.Header.Set("User-Agent", "grok-pager/0.2.91")
	auth := &cliproxyauth.Auth{Provider: "xai", Attributes: map[string]string{"using_api": "false"}}
	applyProXAIHTTPRequestIdentity(req, auth)
	if got := req.Header.Get(xaiClientVersionHeader); got != xaiClientVersionValue {
		t.Fatalf("%s = %q, want %q", xaiClientVersionHeader, got, xaiClientVersionValue)
	}
	if got := req.Header.Get("User-Agent"); got != "xai-grok-workspace/"+xaiClientVersionValue {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestShouldObserveXAIQuotaOnlyForCLIChatProxy(t *testing.T) {
	tests := []struct {
		name string
		auth *cliproxyauth.Auth
		want bool
	}{
		{
			name: "explicit cli mode",
			auth: &cliproxyauth.Auth{Provider: "xai", Attributes: map[string]string{"using_api": "false"}},
			want: true,
		},
		{
			name: "oauth default",
			auth: &cliproxyauth.Auth{Provider: "xai", Attributes: map[string]string{"auth_kind": "oauth"}},
			want: true,
		},
		{
			name: "official api",
			auth: &cliproxyauth.Auth{Provider: "xai", Attributes: map[string]string{"using_api": "true"}},
		},
		{
			name: "custom gateway",
			auth: &cliproxyauth.Auth{Provider: "xai", Attributes: map[string]string{"using_api": "false", "base_url": "https://gateway.example/v1"}},
		},
		{name: "other provider", auth: &cliproxyauth.Auth{Provider: "codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldObserveXAIQuota(tt.auth); got != tt.want {
				t.Fatalf("shouldObserveXAIQuota() = %v, want %v", got, tt.want)
			}
		})
	}
}
