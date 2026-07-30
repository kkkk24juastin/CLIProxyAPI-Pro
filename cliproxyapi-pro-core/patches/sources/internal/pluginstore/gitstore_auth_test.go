package pluginstore

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type gitStoreAuditDoer struct {
	requests []*http.Request
}

func (d *gitStoreAuditDoer) Do(req *http.Request) (*http.Response, error) {
	d.requests = append(d.requests, req.Clone(req.Context()))
	body := `{"tag_name":"v1.2.3","assets":[]}`
	if strings.Contains(req.URL.Path, "/releases/assets/") {
		body = "plugin-archive"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func TestGitStoreGitTokenAuthenticatesGitHubReleaseMetadata(t *testing.T) {
	t.Setenv("GITSTORE_GIT_TOKEN", " gitstore-token ")
	doer := &gitStoreAuditDoer{}
	client := Client{HTTPClient: doer}

	_, err := client.FetchLatestRelease(context.Background(), Plugin{
		Repository: "https://github.com/example/sample-plugin",
	})
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if len(doer.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(doer.requests))
	}
	if got := doer.requests[0].Header.Get("Authorization"); got != "Bearer gitstore-token" {
		t.Fatalf("Authorization = %q, want Bearer gitstore-token", got)
	}
}

func TestGitStoreGitTokenUsesAuthenticatedGitHubAssetAPI(t *testing.T) {
	t.Setenv("GITSTORE_GIT_TOKEN", "gitstore-token")
	doer := &gitStoreAuditDoer{}
	client := Client{HTTPClient: doer}
	asset := ReleaseAsset{
		APIURL:             "https://api.github.com/repos/example/sample-plugin/releases/assets/42",
		BrowserDownloadURL: "https://github.com/example/sample-plugin/releases/download/v1.2.3/sample.zip",
	}

	data, err := client.DownloadAsset(context.Background(), asset)
	if err != nil {
		t.Fatalf("DownloadAsset() error = %v", err)
	}
	if string(data) != "plugin-archive" {
		t.Fatalf("DownloadAsset() = %q, want plugin archive", data)
	}
	if len(doer.requests) != 1 || doer.requests[0].URL.String() != asset.APIURL {
		t.Fatalf("request URL = %#v, want %q", doer.requests, asset.APIURL)
	}
	if got := doer.requests[0].Header.Get("Authorization"); got != "Bearer gitstore-token" {
		t.Fatalf("Authorization = %q, want Bearer gitstore-token", got)
	}
}

func TestExplicitNoneRuleSuppressesGitStoreGitToken(t *testing.T) {
	t.Setenv("GITSTORE_GIT_TOKEN", "gitstore-token")
	doer := &gitStoreAuditDoer{}
	client := Client{
		HTTPClient: doer,
		Auth: []AuthConfig{{
			Match:   "https://api.github.com/repos/",
			ApplyTo: []string{RequestKindMetadata, RequestKindArtifact},
			Type:    AuthTypeNone,
		}},
	}

	_, err := client.FetchLatestRelease(context.Background(), Plugin{
		Repository: "https://github.com/example/sample-plugin",
	})
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if got := doer.requests[0].Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

func TestGitStoreGitTokenDoesNotMatchLookalikeHost(t *testing.T) {
	t.Setenv("GITSTORE_GIT_TOKEN", "gitstore-token")
	if _, ok := gitStoreGitHubToken("https://api.github.com.evil.example/repos/x/y/releases/latest", RequestKindMetadata); ok {
		t.Fatal("lookalike GitHub host unexpectedly received GITSTORE_GIT_TOKEN")
	}
}
