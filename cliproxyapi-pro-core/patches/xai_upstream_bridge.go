package executor

import (
	"net/http"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// XAIUsingAPI exposes the upstream xAI routing decision to Pro features so
// account inspection and policy discovery cannot drift from request execution.
func XAIUsingAPI(auth *cliproxyauth.Auth) bool {
	return xaiUsingAPI(auth)
}

// XAIChatBaseURL exposes the upstream non-media HTTP route resolver.
func XAIChatBaseURL(auth *cliproxyauth.Auth) string {
	return xaiChatBaseURL(auth)
}

// XAIChatRequestHeaders builds headers through the upstream request path,
// including its current Grok CLI identity and custom-header handling.
func XAIChatRequestHeaders(auth *cliproxyauth.Auth, token string, stream bool) http.Header {
	req := &http.Request{Header: make(http.Header)}
	applyXAIChatHeaders(req, auth, token, stream, "")
	return req.Header.Clone()
}

// applyProXAIHTTPRequestIdentity normalizes executor-backed management calls
// to the same CLI identity used by normal upstream xAI chat execution.
func applyProXAIHTTPRequestIdentity(req *http.Request, auth *cliproxyauth.Auth) {
	if req == nil || req.URL == nil || xaiUsingAPI(auth) {
		return
	}
	if req.URL.Scheme != "https" || req.URL.Hostname() != "cli-chat-proxy.grok.com" {
		return
	}
	req.Header.Set(xaiTokenAuthHeader, xaiTokenAuthValue)
	req.Header.Set(xaiClientVersionHeader, xaiClientVersionValue)
	req.Header.Set("User-Agent", "xai-grok-workspace/"+xaiClientVersionValue)
}
