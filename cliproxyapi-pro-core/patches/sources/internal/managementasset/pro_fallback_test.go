package managementasset

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type proManagementAuditRoundTripper func(*http.Request) (*http.Response, error)

func (fn proManagementAuditRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestGitStoreGitTokenCoversManagementMetadataAndAssetAPI(t *testing.T) {
	t.Setenv("GITSTORE_GIT_TOKEN", " management-token ")
	requests := make([]*http.Request, 0, 2)
	client := &http.Client{Transport: proManagementAuditRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Clone(req.Context()))
		body := `{"assets":[{"url":"https://api.github.com/repos/ssfun/CLIProxyAPI-Pro/releases/assets/42","name":"management.html","browser_download_url":"https://github.com/ssfun/CLIProxyAPI-Pro/releases/download/v1/management.html"}]}`
		if strings.Contains(req.URL.Path, "/releases/assets/") {
			body = "pro-management"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	asset, _, err := fetchLatestAsset(context.Background(), client, defaultManagementReleaseURL)
	if err != nil {
		t.Fatalf("fetchLatestAsset() error = %v", err)
	}
	data, _, err := downloadReleaseAsset(context.Background(), client, asset)
	if err != nil {
		t.Fatalf("downloadReleaseAsset() error = %v", err)
	}
	if string(data) != "pro-management" {
		t.Fatalf("downloadReleaseAsset() = %q, want Pro management", data)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	for index, req := range requests {
		if got := req.Header.Get("Authorization"); got != "Bearer management-token" {
			t.Fatalf("request %d Authorization = %q, want Bearer management-token", index, got)
		}
	}
	if requests[1].URL.String() != asset.APIURL {
		t.Fatalf("asset request URL = %q, want %q", requests[1].URL, asset.APIURL)
	}
}

func TestEmbeddedProManagementFallbackIsWrittenLocally(t *testing.T) {
	if len(proManagementFallbackHTML) == 0 {
		t.Fatal("embedded Pro management fallback is empty")
	}
	path := t.TempDir() + "/management.html"
	if !ensureFallbackManagementHTML(context.Background(), http.DefaultClient, path) {
		t.Fatal("ensureFallbackManagementHTML() = false, want true")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(proManagementFallbackHTML) {
		t.Fatal("written fallback differs from embedded Pro management")
	}
}

func TestManagementGitStoreTokenDoesNotMatchLookalikeHost(t *testing.T) {
	if isGitHubAPIURL("https://api.github.com.evil.example/repos/x/y/releases/latest") {
		t.Fatal("lookalike GitHub host unexpectedly matched")
	}
}
