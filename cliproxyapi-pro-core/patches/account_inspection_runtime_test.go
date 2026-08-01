package management

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	proinspection "github.com/router-for-me/CLIProxyAPI/v6/internal/pro/inspection"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type accountInspectionTestStorage struct {
	meta map[string]any
}

type accountInspectionAuthStore struct {
	path string
}

type xaiInspectionRoutingExecutor struct {
	requests []*http.Request
}

func (e *xaiInspectionRoutingExecutor) Identifier() string { return "xai" }

func (e *xaiInspectionRoutingExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *xaiInspectionRoutingExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, nil
}

func (e *xaiInspectionRoutingExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *xaiInspectionRoutingExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *xaiInspectionRoutingExecutor) HttpRequest(_ context.Context, _ *coreauth.Auth, req *http.Request) (*http.Response, error) {
	e.requests = append(e.requests, req.Clone(context.Background()))
	body := `{"id":"chatcmpl-test","choices":[]}`
	if strings.Contains(req.URL.RawQuery, "format=credits") {
		body = `{"config":{"period_type":"weekly","usage_percent":10}}`
	} else if strings.HasSuffix(req.URL.Path, "/billing") {
		body = `{"config":{"monthly_limit":0,"used":0}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (s *accountInspectionAuthStore) List(context.Context) ([]*coreauth.Auth, error) {
	return nil, nil
}

func (s *accountInspectionAuthStore) Save(_ context.Context, auth *coreauth.Auth) (string, error) {
	raw, err := json.Marshal(auth.Metadata)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(s.path, append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return s.path, nil
}

func (s *accountInspectionAuthStore) Delete(context.Context, string) error {
	return nil
}

func (s *accountInspectionTestStorage) SetMetadata(meta map[string]any) {
	s.meta = meta
}

func (s *accountInspectionTestStorage) SaveTokenToFile(path string) error {
	raw, err := json.Marshal(s.meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func testInspectionResult(key string, action accountInspectionAction, disabled bool, statusCode *int, isQuota bool, err string) accountInspectionResult {
	return accountInspectionResult{
		Key:        key,
		Provider:   "test",
		FileName:   key + ".json",
		AuthIndex:  key,
		Action:     action,
		Disabled:   disabled,
		StatusCode: statusCode,
		IsQuota:    isQuota,
		Error:      err,
	}
}

func testInspectionProviderResult(key string, provider string, action accountInspectionAction, disabled bool, statusCode *int, isQuota bool, err string) accountInspectionResult {
	result := testInspectionResult(key, action, disabled, statusCode, isQuota, err)
	result.Provider = provider
	return result
}

func testStatusCode(value int) *int {
	return &value
}

func testInspectionAuthInvalidResult(key string, provider string, action accountInspectionAction) accountInspectionResult {
	result := testInspectionProviderResult(key, provider, action, false, testStatusCode(http.StatusUnauthorized), false, "")
	result.ErrorCode = "inspection_http_error"
	return result
}

func testInspectionQuotaResult(key string, provider string, action accountInspectionAction) accountInspectionResult {
	return testInspectionProviderResult(key, provider, action, false, nil, true, "")
}

func TestAccountFromAuthUsesFileNameWhenEmailUnavailable(t *testing.T) {
	auth := &coreauth.Auth{
		ID:       "codex-account",
		Provider: "codex",
		FileName: "codex-account.json",
		Metadata: map[string]any{"name": "Codex Account"},
	}

	account := accountFromAuth(auth)
	if account.DisplayName != auth.FileName {
		t.Fatalf("display name = %q, want file name %q", account.DisplayName, auth.FileName)
	}
	if account.Name != "Codex Account" {
		t.Fatalf("name = %q, want metadata name preserved", account.Name)
	}
}

func TestAccountFromAuthPrefersEmailOverFileName(t *testing.T) {
	auth := &coreauth.Auth{
		ID:       "codex-account",
		Provider: "codex",
		FileName: "codex-account.json",
		Metadata: map[string]any{"email": "owner@example.com"},
	}

	account := accountFromAuth(auth)
	if account.DisplayName != "owner@example.com" {
		t.Fatalf("display name = %q, want email", account.DisplayName)
	}
}

func TestManagementHandlerShutdownReleasesBackgroundOwners(t *testing.T) {
	t.Setenv("ACCOUNT_INSPECTION_SCHEDULE_PATH", filepath.Join(t.TempDir(), "schedule.json"))
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	scheduler := schedulerForHandler(h)
	if scheduler == nil {
		t.Fatal("scheduler was not registered")
	}
	if routingPolicyControllerForHandler(h) == nil {
		t.Fatal("routing policy controller was not registered")
	}

	h.Shutdown()
	h.Shutdown()

	if schedulerForHandler(h) != nil {
		t.Fatal("scheduler registration survived shutdown")
	}
	if routingPolicyControllerForHandler(h) != nil {
		t.Fatal("routing policy controller registration survived shutdown")
	}
	select {
	case <-h.lifecycleContext.Done():
	default:
		t.Fatal("handler lifecycle context was not canceled")
	}
	if err := scheduler.startRun(true); err == nil {
		t.Fatal("shut down scheduler accepted a new run")
	}
}

func TestPaginateAccountInspectionResultsReturnsRequestedPage(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionResult("healthy-1", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("healthy-2", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("auth-1", accountInspectionActionDelete, false, nil, false, ""),
		testInspectionResult("auth-2", accountInspectionActionKeep, false, testStatusCode(401), false, ""),
	}

	page, info := paginateAccountInspectionResults(results, 2, 2, "", false, "", "")
	if info.Page != 2 || info.PageSize != 2 || info.Total != 4 || info.TotalPages != 2 || info.HasMore {
		t.Fatalf("page info = %+v, want page=2 size=2 total=4 totalPages=2 hasMore=false", info)
	}
	if len(page) != 2 || page[0].Key != "auth-1" || page[1].Key != "auth-2" {
		t.Fatalf("page = %+v, want auth-1/auth-2", page)
	}
}

func TestPaginateAccountInspectionResultsFiltersHealthBuckets(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionResult("healthy", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionAuthInvalidResult("auth", "test", accountInspectionActionDelete),
		testInspectionQuotaResult("quota", "test", accountInspectionActionDisable),
		testInspectionResult("error", accountInspectionActionKeep, false, nil, false, "network error"),
		testInspectionResult("recoverable", accountInspectionActionEnable, true, nil, false, ""),
		testInspectionResult("disabled", accountInspectionActionKeep, true, nil, false, ""),
	}

	page, info := paginateAccountInspectionResults(results, 1, 10, "quotaExhausted", false, "", "")
	if info.Total != 1 || info.HasMore {
		t.Fatalf("quota page info = %+v, want total=1 hasMore=false", info)
	}
	if len(page) != 1 || page[0].Key != "quota" {
		t.Fatalf("quota page = %+v, want quota", page)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "pending", false, "", "")
	if info.Total != 3 {
		t.Fatalf("pending page info = %+v, want total=3", info)
	}
	if len(page) != 3 || page[0].Key != "auth" || page[1].Key != "quota" || page[2].Key != "recoverable" {
		t.Fatalf("pending page = %+v, want auth/quota/recoverable", page)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "attention", false, "", "")
	if info.Total != 5 || len(page) != 5 {
		t.Fatalf("attention page info = %+v page=%+v, want all non-healthy results", info, page)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "attention", true, "", "")
	if info.Total != 3 || len(page) != 3 || page[0].Key != "auth" || page[1].Key != "quota" || page[2].Key != "recoverable" {
		t.Fatalf("pending attention page = %+v info=%+v, want auth/quota/recoverable", page, info)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "accountIssues", false, "", "")
	if info.Total != 2 || len(page) != 2 || page[0].Key != "auth" || page[1].Key != "error" {
		t.Fatalf("account issues page = %+v info=%+v, want auth/error", page, info)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "quotaChanges", false, "", "")
	if info.Total != 2 || len(page) != 2 || page[0].Key != "quota" || page[1].Key != "recoverable" {
		t.Fatalf("quota changes page = %+v info=%+v, want quota/recoverable", page, info)
	}
}

func TestAccountInspectionHealthClassificationUsesSemanticEvidenceAcrossProviders(t *testing.T) {
	providers := []string{"antigravity", "claude", "codex", "gemini-cli", "kimi", "xai"}
	for _, provider := range providers {
		provider := provider
		t.Run(provider+"/auth-invalid", func(t *testing.T) {
			result := testInspectionAuthInvalidResult(provider+"-auth", provider, accountInspectionActionDisable)
			if provider == "codex" {
				result.Action = accountInspectionActionDelete
			}
			if got := proinspection.HealthBucketOf(result); got != accountInspectionHealthAuthInvalid {
				t.Fatalf("health bucket = %q, want %q", got, accountInspectionHealthAuthInvalid)
			}
		})

		t.Run(provider+"/quota", func(t *testing.T) {
			result := testInspectionQuotaResult(provider+"-quota", provider, accountInspectionActionDisable)
			if provider == "codex" || provider == "xai" {
				result.StatusCode = testStatusCode(http.StatusPaymentRequired)
			}
			if got := proinspection.HealthBucketOf(result); got != accountInspectionHealthQuotaExhausted {
				t.Fatalf("health bucket = %q, want %q", got, accountInspectionHealthQuotaExhausted)
			}
		})
	}

	requestErrors := []struct {
		name            string
		provider        string
		errorCode       string
		status          *int
		deepProbeStatus string
	}{
		{name: "antigravity-deep-probe", provider: "antigravity", errorCode: "antigravity_deep_probe_error", status: testStatusCode(http.StatusBadRequest), deepProbeStatus: string(accountInspectionDeepProbeTransientError)},
		{name: "claude-probe", provider: "claude", errorCode: "inspection_probe_error", status: testStatusCode(http.StatusBadGateway)},
		{name: "codex-missing-auth-index", provider: "codex", errorCode: "missing_auth_index"},
		{name: "gemini-cli-probe", provider: "gemini-cli", errorCode: "inspection_probe_error", status: testStatusCode(http.StatusServiceUnavailable)},
		{name: "kimi-token-refresh", provider: "kimi", errorCode: "token_refresh_error"},
		{name: "xai-deep-probe", provider: "xai", errorCode: "xai_deep_probe_error", status: testStatusCode(http.StatusBadRequest), deepProbeStatus: string(accountInspectionDeepProbeTransientError)},
	}
	for _, tt := range requestErrors {
		t.Run(tt.name, func(t *testing.T) {
			result := testInspectionProviderResult(tt.name, tt.provider, accountInspectionActionDelete, false, tt.status, false, "probe failed")
			result.ErrorCode = tt.errorCode
			result.DeepProbeStatus = tt.deepProbeStatus
			if got := proinspection.HealthBucketOf(result); got != accountInspectionHealthInspectionError {
				t.Fatalf("health bucket = %q, want %q", got, accountInspectionHealthInspectionError)
			}
			if proinspection.IsAccountInvalidResult(result) {
				t.Fatal("request error was classified as account invalid")
			}
		})
	}
}

func TestAccountInspectionHealthClassificationDoesNotInferFactsFromActions(t *testing.T) {
	deleteOnly := testInspectionResult("delete-only", accountInspectionActionDelete, false, nil, false, "")
	if got := proinspection.HealthBucketOf(deleteOnly); got != accountInspectionHealthHealthy {
		t.Fatalf("delete-only health bucket = %q, want %q", got, accountInspectionHealthHealthy)
	}

	disableOnly := testInspectionResult("disable-only", accountInspectionActionDisable, false, nil, false, "")
	if got := proinspection.HealthBucketOf(disableOnly); got != accountInspectionHealthHealthy {
		t.Fatalf("disable-only health bucket = %q, want %q", got, accountInspectionHealthHealthy)
	}
}

func TestAutoErrorActionsUseSemanticErrorCategory(t *testing.T) {
	settings := proinspection.DefaultSettings()
	settings.AutoExecuteAccountInvalidAction = accountInspectionActionDelete
	settings.AutoExecuteRequestErrorAction = accountInspectionActionDisable

	authInvalid := testInspectionAuthInvalidResult("auth-invalid", "claude", accountInspectionActionKeep)
	if got := proinspection.AutoActionForResult(authInvalid, settings); got != accountInspectionActionDelete {
		t.Fatalf("auth-invalid auto action = %q, want %q", got, accountInspectionActionDelete)
	}

	requestError := testInspectionProviderResult("request-error", "xai", accountInspectionActionKeep, false, testStatusCode(http.StatusBadRequest), false, "temporary deep-probe failure")
	requestError.ErrorCode = "xai_deep_probe_error"
	requestError.DeepProbeStatus = string(accountInspectionDeepProbeTransientError)
	if got := proinspection.AutoActionForResult(requestError, settings); got != accountInspectionActionDisable {
		t.Fatalf("request-error auto action = %q, want %q", got, accountInspectionActionDisable)
	}
}

func TestPaginateAccountInspectionResultsFiltersProvider(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionProviderResult("codex-healthy", "codex", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionProviderResult("claude-healthy", "claude", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionAuthInvalidResult("codex-auth", "codex", accountInspectionActionDelete),
		testInspectionAuthInvalidResult("claude-auth", "claude", accountInspectionActionDelete),
	}

	page, info := paginateAccountInspectionResults(results, 1, 10, "pending", false, "codex", "")
	if info.Total != 1 || info.TotalPages != 1 || info.HasMore {
		t.Fatalf("codex pending page info = %+v, want total=1 totalPages=1 hasMore=false", info)
	}
	if len(page) != 1 || page[0].Key != "codex-auth" {
		t.Fatalf("codex pending page = %+v, want codex-auth", page)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "healthy", false, "claude", "")
	if info.Total != 1 || len(page) != 1 || page[0].Key != "claude-healthy" {
		t.Fatalf("claude healthy page = %+v info=%+v, want claude-healthy", page, info)
	}
}

func TestPaginateAccountInspectionResultsSearchesAccountIdentity(t *testing.T) {
	first := testInspectionProviderResult("first", "codex", accountInspectionActionKeep, false, nil, false, "")
	first.FileName = "codex-alice.json"
	first.DisplayName = "Alice Primary"
	first.Email = "alice@example.com"
	first.Name = "Alice"
	first.AuthIndex = "auth-alice"
	second := testInspectionProviderResult("second", "claude", accountInspectionActionKeep, false, nil, false, "")
	second.FileName = "claude-bob.json"
	second.Email = "bob@example.com"
	results := []accountInspectionResult{first, second}

	page, info := paginateAccountInspectionResults(results, 1, 10, "healthy", false, "codex", "ALICE@EXAMPLE")
	if info.Total != 1 || len(page) != 1 || page[0].Key != "first" {
		t.Fatalf("account search page = %+v info=%+v, want first result only", page, info)
	}

	page, info = paginateAccountInspectionResults(results, 1, 10, "healthy", false, "", "claude-bob")
	if info.Total != 1 || len(page) != 1 || page[0].Key != "second" {
		t.Fatalf("file-name search page = %+v info=%+v, want second result only", page, info)
	}
}

func TestStreamStatusLockedOmitsDetailsForLightSnapshots(t *testing.T) {
	scheduler := &accountInspectionScheduler{
		status: accountInspectionStatus{
			Results: []accountInspectionResult{
				testInspectionResult("healthy", accountInspectionActionKeep, false, nil, false, ""),
			},
			Logs: []accountInspectionLogEntry{{Time: 1, Level: "info", Message: "hello"}},
		},
	}

	status := scheduler.streamStatusLocked(accountInspectionSnapshotOptions{})
	if status.Results != nil || status.Logs != nil || status.HealthCounts != nil || status.ProviderHealthCounts != nil {
		t.Fatalf("streamStatusLocked(light) leaked details: results=%v logs=%v health=%v providerHealth=%v", status.Results, status.Logs, status.HealthCounts, status.ProviderHealthCounts)
	}
	if status.ResultsLimited || status.LogsLimited {
		t.Fatalf("streamStatusLocked(light) limited flags = results:%v logs:%v, want false", status.ResultsLimited, status.LogsLimited)
	}
	if status.ResultsPage != nil || status.LogsPage != nil {
		t.Fatalf("streamStatusLocked(light) leaked page info: results=%v logs=%v", status.ResultsPage, status.LogsPage)
	}
}

func TestStreamStatusLockedReturnsPagedDetailsWithFullHealthCounts(t *testing.T) {
	scheduler := &accountInspectionScheduler{
		status: accountInspectionStatus{
			Results: []accountInspectionResult{
				testInspectionResult("healthy-1", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionResult("healthy-2", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionAuthInvalidResult("auth-1", "test", accountInspectionActionDelete),
				testInspectionResult("auth-2", accountInspectionActionKeep, false, testStatusCode(401), false, ""),
			},
			Logs: []accountInspectionLogEntry{
				{Time: 1, Level: "info", Message: "one"},
				{Time: 2, Level: "info", Message: "two"},
				{Time: 3, Level: "info", Message: "three"},
			},
		},
	}

	status := scheduler.streamStatusLocked(accountInspectionSnapshotOptions{
		IncludeDetails: true,
		ResultPage:     2,
		ResultPageSize: 2,
		LogPage:        1,
		LogPageSize:    2,
	})

	if status.HealthCounts == nil {
		t.Fatal("streamStatusLocked(details) HealthCounts = nil")
	}
	if status.HealthCounts.Total != 4 || status.HealthCounts.Healthy != 2 || status.HealthCounts.AuthInvalid != 2 {
		t.Fatalf("HealthCounts = %+v, want total=4 healthy=2 authInvalid=2", *status.HealthCounts)
	}
	providerCounts := status.ProviderHealthCounts["test"]
	if providerCounts.Total != 4 || providerCounts.Healthy != 2 || providerCounts.AuthInvalid != 2 {
		t.Fatalf("ProviderHealthCounts[test] = %+v, want total=4 healthy=2 authInvalid=2", providerCounts)
	}
	if status.ResultsPage == nil || status.ResultsPage.Total != 4 || status.ResultsPage.Page != 2 || status.ResultsPage.PageSize != 2 {
		t.Fatalf("ResultsPage = %+v, want page=2 size=2 total=4", status.ResultsPage)
	}
	if status.LogsPage == nil || status.LogsPage.Total != 3 || status.LogsPage.Page != 1 || status.LogsPage.PageSize != 2 || !status.LogsPage.HasMore {
		t.Fatalf("LogsPage = %+v, want page=1 size=2 total=3 hasMore=true", status.LogsPage)
	}
	if len(status.Results) != 2 {
		t.Fatalf("paged results len = %d, want 2", len(status.Results))
	}
	if status.Results[0].Key != "auth-1" || status.Results[1].Key != "auth-2" {
		t.Fatalf("paged results = %+v, want auth rows", status.Results)
	}
	if len(status.Logs) != 2 || status.Logs[0].Time != 2 || status.Logs[1].Time != 3 {
		t.Fatalf("paged logs = %+v, want last two log entries", status.Logs)
	}
}

func TestPaginateAccountInspectionPageSizeCapsAtServerMax(t *testing.T) {
	results := make([]accountInspectionResult, accountInspectionMaxResultPageSize+5)
	for index := range results {
		results[index] = testInspectionResult("result", accountInspectionActionKeep, false, nil, false, "")
	}
	page, info := paginateAccountInspectionResults(results, 1, accountInspectionMaxResultPageSize+100, "", false, "", "")
	if info.PageSize != accountInspectionMaxResultPageSize {
		t.Fatalf("result page size = %d, want capped %d", info.PageSize, accountInspectionMaxResultPageSize)
	}
	if len(page) != accountInspectionMaxResultPageSize {
		t.Fatalf("result page len = %d, want %d", len(page), accountInspectionMaxResultPageSize)
	}

	logs := make([]accountInspectionLogEntry, accountInspectionMaxLogPageSize+5)
	for index := range logs {
		logs[index] = accountInspectionLogEntry{Time: int64(index + 1), Level: "info", Message: "log"}
	}
	logPage, logInfo := paginateAccountInspectionLogs(logs, 1, accountInspectionMaxLogPageSize+100, "")
	if logInfo.PageSize != accountInspectionMaxLogPageSize {
		t.Fatalf("log page size = %d, want capped %d", logInfo.PageSize, accountInspectionMaxLogPageSize)
	}
	if len(logPage) != accountInspectionMaxLogPageSize {
		t.Fatalf("log page len = %d, want %d", len(logPage), accountInspectionMaxLogPageSize)
	}
}

func TestHealthCountsLockedRebuildsStaleCache(t *testing.T) {
	scheduler := &accountInspectionScheduler{
		status: accountInspectionStatus{
			Results: []accountInspectionResult{
				testInspectionResult("healthy", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionAuthInvalidResult("auth", "test", accountInspectionActionDelete),
			},
		},
	}

	counts := scheduler.healthCountsLocked()
	if counts.Total != 2 || counts.Healthy != 1 || counts.AuthInvalid != 1 {
		t.Fatalf("healthCountsLocked() = %+v, want total=2 healthy=1 authInvalid=1", counts)
	}
	if scheduler.healthCounts != counts {
		t.Fatalf("scheduler healthCounts cache = %+v, want %+v", scheduler.healthCounts, counts)
	}
}

func TestAccountInspectionResultSnapshotPersistsAndRestoresReadOnly(t *testing.T) {
	snapshotPath := filepath.Join(t.TempDir(), "account-inspection-snapshot.json")
	settings := proinspection.DefaultSettings()
	settings.TargetType = "xai"
	result := testInspectionProviderResult("xai-1", "xai", accountInspectionActionKeep, false, testStatusCode(502), false, "upstream error")
	result.ErrorDetail = `{"error":{"message":"raw upstream response"}}`

	source := &accountInspectionScheduler{
		snapshotPath:    snapshotPath,
		lastRunSettings: settings,
		status: accountInspectionStatus{
			State:          accountInspectionStateCompleted,
			LastStartedAt:  1000,
			LastFinishedAt: 2000,
			Summary:        accountInspectionSummary{TotalFiles: 1, ProbeSetCount: 1, SampledCount: 1, ErrorCount: 1},
			Results:        []accountInspectionResult{result},
		},
	}
	if err := os.WriteFile(snapshotPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(existing snapshot) error = %v", err)
	}
	if err := source.saveResultSnapshotLocked(); err != nil {
		t.Fatalf("saveResultSnapshotLocked() error = %v", err)
	}
	info, err := os.Stat(snapshotPath)
	if err != nil {
		t.Fatalf("os.Stat(snapshot) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("snapshot mode = %o, want 600", got)
	}

	restored := &accountInspectionScheduler{snapshotPath: snapshotPath}
	if err := restored.loadResultSnapshot(); err != nil {
		t.Fatalf("loadResultSnapshot() error = %v", err)
	}
	if !restored.status.RestoredSnapshot {
		t.Fatal("restored snapshot is not marked read-only")
	}
	if restored.lastRunSettings.TargetType != "xai" {
		t.Fatalf("restored target type = %q, want xai", restored.lastRunSettings.TargetType)
	}
	if len(restored.status.Results) != 1 || restored.status.Results[0].ErrorDetail != result.ErrorDetail {
		t.Fatalf("restored results = %+v, want original error detail", restored.status.Results)
	}
	if len(restored.status.Logs) != 0 {
		t.Fatalf("restored logs = %+v, want none", restored.status.Logs)
	}

	if _, err := restored.inspectOne(context.Background(), accountInspectionActionItem{}); !errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) {
		t.Fatalf("inspectOne() error = %v, want read-only error", err)
	}
	if _, err := restored.refreshTokenNow(context.Background(), accountInspectionActionItem{}); !errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) {
		t.Fatalf("refreshTokenNow() error = %v, want read-only error", err)
	}
	if _, err := restored.executeManualActions(context.Background(), nil); !errors.Is(err, errAccountInspectionRestoredSnapshotReadOnly) {
		t.Fatalf("executeManualActions() error = %v, want read-only error", err)
	}
}

func TestRefreshTokenNowRespectsBackupLifecyclePause(t *testing.T) {
	lifecycle := &proinspection.Lifecycle{}
	if err := lifecycle.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	scheduler := &accountInspectionScheduler{lifecycle: lifecycle}
	if _, err := scheduler.refreshTokenNow(context.Background(), accountInspectionActionItem{}); !errors.Is(err, proinspection.ErrPaused) {
		t.Fatalf("refreshTokenNow() error = %v, want paused", err)
	}
}

func TestDecodeAccountInspectionSnapshotRebuildsHealthCountsWithSemanticClassification(t *testing.T) {
	result := testInspectionProviderResult(
		"xai-transient",
		"xai",
		accountInspectionActionDelete,
		false,
		testStatusCode(http.StatusBadRequest),
		false,
		"temporary deep-probe failure",
	)
	result.ErrorCode = "xai_deep_probe_error"
	result.DeepProbeStatus = string(accountInspectionDeepProbeTransientError)
	raw, err := json.Marshal(accountInspectionResultSnapshot{
		Version:        accountInspectionResultSnapshotVersion,
		State:          accountInspectionStateCompleted,
		LastStartedAt:  1000,
		LastFinishedAt: 2000,
		HealthCounts: accountInspectionHealthCounts{
			Total:       1,
			AuthInvalid: 1,
		},
		Results: []accountInspectionResult{result},
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil {
		t.Fatalf("decodeAccountInspectionResultSnapshot() error = %v", err)
	}
	if snapshot.HealthCounts.Total != 1 || snapshot.HealthCounts.AuthInvalid != 0 || snapshot.HealthCounts.InspectionError != 1 {
		t.Fatalf("rebuilt HealthCounts = %+v, want total=1 inspectionError=1", snapshot.HealthCounts)
	}
}

func TestDecodeAccountInspectionSnapshotMigratesLegacyXAIQuotaEvidence(t *testing.T) {
	result := testInspectionProviderResult(
		"xai-legacy-quota",
		"xai",
		accountInspectionActionKeep,
		true,
		testStatusCode(http.StatusForbidden),
		false,
		"",
	)
	result.ErrorCode = "inspection_http_error"
	result.DeepProbeStatus = string(accountInspectionDeepProbeAuthError)
	result.DeepProbeError = "You have run out of credits or need a Grok subscription."
	raw, err := json.Marshal(accountInspectionResultSnapshot{
		Version:        accountInspectionResultSnapshotVersion,
		State:          accountInspectionStateCompleted,
		LastStartedAt:  1000,
		LastFinishedAt: 2000,
		HealthCounts: accountInspectionHealthCounts{
			Total:       1,
			AuthInvalid: 1,
		},
		Results: []accountInspectionResult{result},
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}

	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil {
		t.Fatalf("decodeAccountInspectionResultSnapshot() error = %v", err)
	}
	if !snapshot.Results[0].IsQuota || snapshot.Results[0].ErrorCode != "" {
		t.Fatalf("migrated result = %+v, want quota result without auth error code", snapshot.Results[0])
	}
	if snapshot.HealthCounts.QuotaExhausted != 1 || snapshot.HealthCounts.AuthInvalid != 0 {
		t.Fatalf("migrated HealthCounts = %+v, want quotaExhausted=1", snapshot.HealthCounts)
	}
}

func TestAccountInspectionSnapshotExportKeepsLastFinishedSnapshotDuringRun(t *testing.T) {
	snapshotPath := filepath.Join(t.TempDir(), "account-inspection-snapshot.json")
	scheduler := &accountInspectionScheduler{
		snapshotPath:    snapshotPath,
		lastRunSettings: proinspection.DefaultSettings(),
		status: accountInspectionStatus{
			State:          accountInspectionStateCompleted,
			LastStartedAt:  1000,
			LastFinishedAt: 2000,
			Results:        []accountInspectionResult{testInspectionResult("finished", accountInspectionActionKeep, false, nil, false, "")},
		},
	}
	if err := scheduler.saveResultSnapshotLocked(); err != nil {
		t.Fatalf("saveResultSnapshotLocked() error = %v", err)
	}
	scheduler.status = accountInspectionStatus{State: accountInspectionStateRunning, LastStartedAt: 3000}

	raw, ok, err := scheduler.exportResultSnapshot()
	if err != nil {
		t.Fatalf("exportResultSnapshot() error = %v", err)
	}
	if !ok {
		t.Fatal("exportResultSnapshot() ok = false, want previous finished snapshot")
	}
	snapshot, err := decodeAccountInspectionResultSnapshot(raw)
	if err != nil {
		t.Fatalf("decodeAccountInspectionResultSnapshot() error = %v", err)
	}
	if snapshot.LastFinishedAt != 2000 || len(snapshot.Results) != 1 || snapshot.Results[0].Key != "finished" {
		t.Fatalf("exported snapshot = %+v, want previous finished run", snapshot)
	}
}

func TestHealthCountsCacheTracksResultUpdates(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	healthy := testInspectionResult("account", accountInspectionActionKeep, false, nil, false, "")
	if !scheduler.updateInspectionResultLocked(healthy, true, func(current accountInspectionResult) (accountInspectionResult, bool) {
		return current, true
	}) {
		t.Fatal("updateInspectionResultLocked() append healthy = false, want true")
	}
	if scheduler.healthCounts.Total != 1 || scheduler.healthCounts.Healthy != 1 {
		t.Fatalf("after append healthCounts = %+v, want total=1 healthy=1", scheduler.healthCounts)
	}

	authInvalid := healthy
	authInvalid.Action = accountInspectionActionDelete
	authInvalid.StatusCode = testStatusCode(http.StatusUnauthorized)
	authInvalid.ErrorCode = "inspection_http_error"
	if !scheduler.updateInspectionResultLocked(authInvalid, true, func(current accountInspectionResult) (accountInspectionResult, bool) {
		return authInvalid, true
	}) {
		t.Fatal("updateInspectionResultLocked() replace auth invalid = false, want true")
	}
	if scheduler.healthCounts.Total != 1 || scheduler.healthCounts.Healthy != 0 || scheduler.healthCounts.AuthInvalid != 1 {
		t.Fatalf("after replace healthCounts = %+v, want total=1 healthy=0 authInvalid=1", scheduler.healthCounts)
	}

	if !scheduler.removeInspectionResultLocked(authInvalid) {
		t.Fatal("removeInspectionResultLocked() = false, want true")
	}
	if scheduler.healthCounts.Total != 0 || scheduler.healthCounts.AuthInvalid != 0 {
		t.Fatalf("after remove healthCounts = %+v, want empty", scheduler.healthCounts)
	}
}

func TestMergeTokenRefreshResultUpdatesErrorCodeAndHealthCounts(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	healthy := testInspectionResult("account", accountInspectionActionKeep, false, nil, false, "")
	healthy.Provider = "codex"
	scheduler.status.Results = []accountInspectionResult{healthy}
	scheduler.status.Summary = summarizeAccountInspection(1, 1, nil, scheduler.status.Results)
	scheduler.healthCounts = proinspection.ResultHealthCounts(scheduler.status.Results)

	failed := healthy
	failed.TokenRefreshTriggered = true
	failed.TokenRefreshStatus = "failed"
	failed.TokenRefreshError = "refresh failed"
	failed.Error = "refresh failed"
	failed.ErrorCode = "token_refresh_error"
	failed.ActionReason = "刷新令牌失败，保留账号"
	scheduler.mergeTokenRefreshResultLocked(failed)

	got := scheduler.status.Results[0]
	if got.ErrorCode != "token_refresh_error" || got.Error != "refresh failed" || got.TokenRefreshStatus != "failed" {
		t.Fatalf("merged failed refresh = %+v, want token_refresh_error", got)
	}
	if scheduler.healthCounts.InspectionError != 1 || scheduler.healthCounts.Healthy != 0 || scheduler.status.Summary.ErrorCount != 1 {
		t.Fatalf("after failed refresh health=%+v summary=%+v, want inspection error and summary error", scheduler.healthCounts, scheduler.status.Summary)
	}

	success := got
	success.TokenRefreshStatus = "success"
	success.TokenRefreshError = ""
	success.Error = ""
	success.ErrorCode = ""
	scheduler.mergeTokenRefreshResultLocked(success)

	got = scheduler.status.Results[0]
	if got.ErrorCode != "" || got.Error != "" || got.TokenRefreshStatus != "success" {
		t.Fatalf("merged successful refresh = %+v, want cleared token refresh error", got)
	}
	if scheduler.healthCounts.Healthy != 1 || scheduler.healthCounts.InspectionError != 0 || scheduler.status.Summary.ErrorCount != 0 {
		t.Fatalf("after successful refresh health=%+v summary=%+v, want healthy and no summary error", scheduler.healthCounts, scheduler.status.Summary)
	}
}
