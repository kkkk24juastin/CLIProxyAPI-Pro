package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

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
			if got := accountInspectionResultHealthBucketOf(result); got != accountInspectionHealthAuthInvalid {
				t.Fatalf("health bucket = %q, want %q", got, accountInspectionHealthAuthInvalid)
			}
		})

		t.Run(provider+"/quota", func(t *testing.T) {
			result := testInspectionQuotaResult(provider+"-quota", provider, accountInspectionActionDisable)
			if provider == "codex" || provider == "xai" {
				result.StatusCode = testStatusCode(http.StatusPaymentRequired)
			}
			if got := accountInspectionResultHealthBucketOf(result); got != accountInspectionHealthQuotaExhausted {
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
			if got := accountInspectionResultHealthBucketOf(result); got != accountInspectionHealthInspectionError {
				t.Fatalf("health bucket = %q, want %q", got, accountInspectionHealthInspectionError)
			}
			if isAccountInspectionAccountInvalidResult(result) {
				t.Fatal("request error was classified as account invalid")
			}
		})
	}
}

func TestAccountInspectionHealthClassificationDoesNotInferFactsFromActions(t *testing.T) {
	deleteOnly := testInspectionResult("delete-only", accountInspectionActionDelete, false, nil, false, "")
	if got := accountInspectionResultHealthBucketOf(deleteOnly); got != accountInspectionHealthHealthy {
		t.Fatalf("delete-only health bucket = %q, want %q", got, accountInspectionHealthHealthy)
	}

	disableOnly := testInspectionResult("disable-only", accountInspectionActionDisable, false, nil, false, "")
	if got := accountInspectionResultHealthBucketOf(disableOnly); got != accountInspectionHealthHealthy {
		t.Fatalf("disable-only health bucket = %q, want %q", got, accountInspectionHealthHealthy)
	}
}

func TestAutoErrorActionsUseSemanticErrorCategory(t *testing.T) {
	settings := defaultAccountInspectionSettings()
	settings.AutoExecuteAccountInvalidAction = accountInspectionActionDelete
	settings.AutoExecuteRequestErrorAction = accountInspectionActionDisable

	authInvalid := testInspectionAuthInvalidResult("auth-invalid", "claude", accountInspectionActionKeep)
	if got := autoActionForResult(authInvalid, settings); got != accountInspectionActionDelete {
		t.Fatalf("auth-invalid auto action = %q, want %q", got, accountInspectionActionDelete)
	}

	requestError := testInspectionProviderResult("request-error", "xai", accountInspectionActionKeep, false, testStatusCode(http.StatusBadRequest), false, "temporary deep-probe failure")
	requestError.ErrorCode = "xai_deep_probe_error"
	requestError.DeepProbeStatus = string(accountInspectionDeepProbeTransientError)
	if got := autoActionForResult(requestError, settings); got != accountInspectionActionDisable {
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
	scheduler.healthCounts = accountInspectionResultHealthCounts(scheduler.status.Results)

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

func TestSyncAuthInspectionLastErrorClearsMetadata(t *testing.T) {
	auth := &Auth{
		LastError: &AuthError{Code: "token_refresh_error", Message: "old refresh failed"},
		Metadata: map[string]any{
			"last_error": map[string]any{"code": "token_refresh_error", "message": "old refresh failed"},
			"email":      "user@example.com",
		},
	}

	syncAuthInspectionLastError(auth, nil)

	if auth.LastError != nil {
		t.Fatalf("LastError = %#v, want nil", auth.LastError)
	}
	if _, ok := auth.Metadata["last_error"]; ok {
		t.Fatalf("metadata last_error = %#v, want removed", auth.Metadata["last_error"])
	}
	if auth.Metadata["email"] != "user@example.com" {
		t.Fatalf("metadata email = %#v, want preserved", auth.Metadata["email"])
	}
}

func TestAutoActionConfirmationDelaysExecution(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	result := testInspectionResult("quota", accountInspectionActionDisable, false, nil, true, "")
	settings := defaultAccountInspectionSettings()
	settings.AutoExecuteConfirmations = 2
	settings.AutoExecuteQuotaLimitDisable = true

	action := autoActionForResult(result, settings)
	if action != accountInspectionActionDisable {
		t.Fatalf("autoActionForResult() = %q, want disable", action)
	}
	confirmed, count, required := scheduler.confirmAutoAction(result, action, settings.AutoExecuteConfirmations)
	if confirmed || count != 1 || required != 2 {
		t.Fatalf("first confirmation = confirmed:%v count:%d required:%d, want false/1/2", confirmed, count, required)
	}
	confirmed, count, required = scheduler.confirmAutoAction(result, action, settings.AutoExecuteConfirmations)
	if !confirmed || count != 2 || required != 2 {
		t.Fatalf("second confirmation = confirmed:%v count:%d required:%d, want true/2/2", confirmed, count, required)
	}
	scheduler.clearAutoActionConfirmation(result)
	if len(scheduler.autoActionConfirmations) != 0 {
		t.Fatalf("autoActionConfirmations = %+v, want empty after clear", scheduler.autoActionConfirmations)
	}
}

func TestEnablePreservesNonInspectionLastError(t *testing.T) {
	auth := &Auth{
		Disabled:    true,
		Status:      AuthStatusDisabled,
		LastError:   &AuthError{Code: "upstream_refresh_error", Message: "refresh failed"},
		Metadata:    map[string]any{"last_error": map[string]any{"code": "upstream_refresh_error", "message": "refresh failed"}},
		Unavailable: true,
	}
	setAuthInspectionDisabledState(auth, false)
	if auth.Disabled || auth.Status != AuthStatusError || !auth.Unavailable {
		t.Fatalf("enabled auth state = disabled:%v status:%q unavailable:%v", auth.Disabled, auth.Status, auth.Unavailable)
	}
	if auth.LastError == nil || auth.LastError.Code != "upstream_refresh_error" {
		t.Fatalf("LastError = %#v, want preserved upstream error", auth.LastError)
	}
	if _, ok := auth.Metadata["last_error"]; !ok {
		t.Fatal("metadata last_error was removed")
	}
}

func TestQuotaSuccessStateIncludesParserMetadata(t *testing.T) {
	state := quotaSuccessState(map[string]any{"rawShapeHash": jsonShapeHash(`{"a":1,"items":[{"b":true}]}`)})
	if state["schemaVersion"] != 2 || state["parserVersion"] != accountInspectionQuotaParserVersion || state["status"] != "success" {
		t.Fatalf("quota state metadata = %+v", state)
	}
	if state["rawShapeHash"] == "" {
		t.Fatalf("rawShapeHash = %q, want populated", state["rawShapeHash"])
	}
}

func TestAntigravityQuotaURLsUseSummaryEndpoint(t *testing.T) {
	for _, url := range antigravityQuotaURLs() {
		if !strings.Contains(url, "retrieveUserQuotaSummary") {
			t.Fatalf("antigravity quota url = %q, want retrieveUserQuotaSummary", url)
		}
	}
}

func TestBuildAntigravityGroupsSupportsSummaryBuckets(t *testing.T) {
	body := `{
		"groups": [{
			"displayName": "Claude/GPT",
			"description": "premium models",
			"buckets": [
				{"bucketId": "weekly", "displayName": "Weekly", "window": "weekly", "remainingFraction": 0.75, "resetTime": "2026-01-02T03:04:05Z"},
				{"bucket_id": "five-hour", "display_name": "Five hour", "window": "5h", "remaining_fraction": 0.25, "reset_time": "2026-01-01T03:04:05Z"}
			]
		}]
	}`

	groups, err := buildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("buildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	if groups[0]["id"] != "claude-gpt" {
		t.Fatalf("group id = %#v, want claude-gpt", groups[0]["id"])
	}
	buckets, ok := groups[0]["buckets"].([]map[string]any)
	if !ok || len(buckets) != 2 {
		t.Fatalf("buckets = %#v, want two parsed buckets", groups[0]["buckets"])
	}
	if _, ok := groups[0]["remainingFraction"]; ok {
		t.Fatalf("remainingFraction is present on group, want latest bucket-only shape")
	}
	if buckets[0]["id"] != "weekly" || buckets[1]["id"] != "five-hour" {
		t.Fatalf("bucket order = %q/%q, want weekly/five-hour", buckets[0]["id"], buckets[1]["id"])
	}
	used := antigravityGroupUsedPercent(map[string]any{"buckets": buckets})
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v, want 75", used)
	}
}

func TestBuildAntigravityGroupsCanonicalizesLatestGroups(t *testing.T) {
	body := `{
		"groups": [
			{
				"buckets": [
					{"bucketId": "gemini-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": "2026-06-20T00:39:10Z", "remainingFraction": 0.9997293},
					{"bucketId": "gemini-5h", "displayName": "Five Hour Limit", "window": "5h", "resetTime": "2026-06-17T15:04:15Z", "remainingFraction": 1}
				],
				"displayName": "Gemini Models",
				"description": "Models within this group: Gemini Flash, Gemini Pro"
			},
			{
				"buckets": [
					{"bucketId": "3p-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": "2026-06-24T04:38:44Z", "remainingFraction": 0.9914995},
					{"bucketId": "3p-5h", "displayName": "Five Hour Limit", "window": "5h", "resetTime": "2026-06-17T12:12:15Z", "remainingFraction": 0.999886}
				],
				"displayName": "Claude and GPT models",
				"description": "Models within this group: Claude Opus, Claude Sonnet, GPT-OSS"
			}
		]
	}`

	groups, err := buildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("buildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups len = %d, want 2", len(groups))
	}
	if groups[0]["id"] != "gemini" || groups[1]["id"] != "claude-gpt" {
		t.Fatalf("group ids = %#v/%#v, want gemini/claude-gpt", groups[0]["id"], groups[1]["id"])
	}
	used := antigravityUsedPercent(groups, accountInspectionAntigravityQuotaModeClaudeGpt)
	if used == nil || math.Abs(*used-0.85005) > 0.0001 {
		t.Fatalf("claude-gpt used percent = %v, want about 0.85005", used)
	}
}

func TestBuildAntigravityGroupsSupportsWrappedBody(t *testing.T) {
	body := `{
		"body": "{\"groups\":[{\"displayName\":\"Claude/GPT\",\"buckets\":[{\"bucketId\":\"weekly\",\"displayName\":\"Weekly\",\"window\":\"weekly\",\"remainingFraction\":0.5}]}]}"
	}`

	groups, err := buildAntigravityGroups(body)
	if err != nil {
		t.Fatalf("buildAntigravityGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	buckets, ok := groups[0]["buckets"].([]map[string]any)
	if !ok || len(buckets) != 1 || buckets[0]["remainingFraction"] != 0.5 {
		t.Fatalf("wrapped buckets = %#v, want one 0.5 bucket", groups[0]["buckets"])
	}
}

func TestBuildAntigravitySubscriptionMapsPaidTierPlan(t *testing.T) {
	payload := map[string]any{
		"currentTier": map[string]any{"id": "free-tier", "name": "Free"},
		"paidTier": map[string]any{
			"id":   "g1-ultra-tier",
			"name": "Ultra",
			"availableCredits": []any{
				map[string]any{
					"creditType":                  "AI",
					"creditAmount":                float64(20),
					"minimumCreditAmountForUsage": "1",
				},
			},
		},
	}

	subscription := buildAntigravitySubscription(payload)
	if subscription == nil {
		t.Fatal("buildAntigravitySubscription() = nil, want subscription")
	}
	if subscription["plan"] != "ultra" || subscription["tierId"] != "g1-ultra-tier" || subscription["source"] != "paid" {
		t.Fatalf("subscription = %#v, want ultra paid tier", subscription)
	}
	credits, ok := subscription["availableCredits"].([]map[string]any)
	if !ok || len(credits) != 1 || credits[0]["creditType"] != "AI" {
		t.Fatalf("availableCredits = %#v, want AI credit entry", subscription["availableCredits"])
	}
}

func TestAntigravityUsedPercentFallsBackWhenClaudeGroupNameChanges(t *testing.T) {
	groups := []map[string]any{{
		"id":    "quota-group-1",
		"label": "Premium Models",
		"buckets": []map[string]any{{
			"id":                "weekly",
			"label":             "Weekly",
			"remainingFraction": 0.35,
		}},
	}}

	used := antigravityUsedPercent(groups, accountInspectionAntigravityQuotaModeClaudeGpt)
	if used == nil || *used != 65 {
		t.Fatalf("used percent = %v, want 65 fallback from buckets", used)
	}
	if model := selectAntigravityDeepProbeModel(groups, ""); model != "claude-sonnet-4-6" {
		t.Fatalf("deep probe model = %q, want default claude-sonnet-4-6", model)
	}
}

func TestBuildAntigravityGroupsRejectsLegacyModelsShape(t *testing.T) {
	body := `{
		"models": {
			"claude-sonnet-4-6": {"quotaInfo": {"remainingFraction": 0.4, "resetTime": "2026-01-02T03:04:05Z"}},
			"gpt-oss-120b-medium": {"quota_info": {"remaining_fraction": 0.8}}
		}
	}`

	if _, err := buildAntigravityGroups(body); err == nil {
		t.Fatalf("buildAntigravityGroups() error = nil, want legacy models shape rejected")
	}
}

func TestBuildCodexWindowsClassifiesTeamMonthlyWindows(t *testing.T) {
	body := `{
		"rate_limit": {
			"primary_window": {"limit_window_seconds": 18000, "used_percent": 12.5, "reset_after_seconds": 60},
			"secondary_window": {"limit_window_seconds": 2592000, "used_percent": 42.5, "reset_after_seconds": 120},
			"allowed": true
		},
		"code_review_rate_limit": {
			"primary_window": {"limit_window_seconds": 18000, "used_percent": 5},
			"secondary_window": {"limit_window_seconds": 2419200, "used_percent": 88}
		},
		"additional_rate_limits": [{
			"limit_name": "Premium Tokens",
			"rate_limit": {
				"primary_window": {"limit_window_seconds": 18000, "used_percent": 11},
				"secondary_window": {"limit_window_seconds": 2678400, "used_percent": 22}
			}
		}]
	}`

	_, windows, used := buildCodexWindows(body)
	if used == nil || *used != 88 {
		t.Fatalf("used percent = %v, want 88", used)
	}
	labelsByID := make(map[string]string)
	for _, window := range windows {
		id, _ := window["id"].(string)
		labelKey, _ := window["labelKey"].(string)
		labelsByID[id] = labelKey
	}
	if labelsByID["monthly"] != "codex_quota.team_secondary_window" {
		t.Fatalf("monthly label = %q, want team secondary", labelsByID["monthly"])
	}
	if labelsByID["code-review-monthly"] != "codex_quota.code_review_team_secondary_window" {
		t.Fatalf("code review monthly label = %q, want code review team secondary", labelsByID["code-review-monthly"])
	}
	if labelsByID["premium-tokens-monthly-0"] != "codex_quota.additional_team_secondary_window" {
		t.Fatalf("additional monthly label = %q, want additional team secondary", labelsByID["premium-tokens-monthly-0"])
	}
}

func TestCodexQuotaStateValuesIncludesSubscriptionAndResetCredits(t *testing.T) {
	auth := &Auth{
		Metadata: map[string]any{
			"id_token": map[string]any{
				"chatgpt_subscription_active_until": float64(1790000000),
			},
		},
		Attributes: map[string]string{
			"plan_type": "plus",
		},
	}
	payload := map[string]any{
		"rate_limit_reset_credits": map[string]any{
			"available_count": float64(2),
		},
	}
	windows := []map[string]any{{"id": "five-hour"}}

	values := codexQuotaStateValues(auth, payload, windows, `{"rate_limit":{}}`)
	if values["planType"] != "plus" {
		t.Fatalf("planType = %#v, want plus", values["planType"])
	}
	if values["subscriptionActiveUntil"] != float64(1790000000) {
		t.Fatalf("subscriptionActiveUntil = %#v, want id token timestamp", values["subscriptionActiveUntil"])
	}
	if values["rateLimitResetCreditsAvailableCount"] != float64(2) {
		t.Fatalf("rateLimitResetCreditsAvailableCount = %#v, want 2", values["rateLimitResetCreditsAvailableCount"])
	}
	if values["rawShapeHash"] == "" {
		t.Fatalf("rawShapeHash = %q, want populated", values["rawShapeHash"])
	}
}

func TestBuildXAIBillingSummaryParsesBillingConfig(t *testing.T) {
	body := `{
		"config": {
			"monthlyLimit": {"val": 10000},
			"used": {"val": 2500},
			"onDemandCap": {"val": 5000},
			"billingPeriodStart": "2026-06-01T00:00:00Z",
			"billingPeriodEnd": "2026-07-01T00:00:00Z"
		}
	}`

	billing, used, err := buildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("buildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 25 {
		t.Fatalf("used percent = %v, want 25", used)
	}
	if billing["monthlyLimitCents"] != 10000.0 || billing["usedCents"] != 2500.0 || billing["onDemandCapCents"] != 5000.0 {
		t.Fatalf("billing cents = %+v, want parsed cent values", billing)
	}
	if billing["billingPeriodEnd"] != "2026-07-01T00:00:00Z" {
		t.Fatalf("billing period end = %#v", billing["billingPeriodEnd"])
	}
	if billing["usedPercent"] != 25.0 {
		t.Fatalf("billing usedPercent = %#v, want 25", billing["usedPercent"])
	}
	if billing["includedUsedCents"] != 2500.0 {
		t.Fatalf("includedUsedCents = %#v, want 2500", billing["includedUsedCents"])
	}
	if billing["onDemandUsedCents"] != 0.0 {
		t.Fatalf("onDemandUsedCents = %#v, want 0", billing["onDemandUsedCents"])
	}
}

func TestBuildXAIBillingSummarySupportsSnakeCaseAndNumericValues(t *testing.T) {
	body := `{
		"config": {
			"monthly_limit": 8000,
			"used": 6000,
			"on_demand_cap": 12000,
			"billing_period_end": "2026-07-01T00:00:00Z"
		}
	}`

	billing, used, err := buildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("buildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 75 {
		t.Fatalf("used percent = %v, want 75", used)
	}
	if billing["monthlyLimitCents"] != 8000.0 || billing["usedCents"] != 6000.0 || billing["onDemandCapCents"] != 12000.0 {
		t.Fatalf("billing cents = %+v, want parsed snake_case numeric values", billing)
	}
}

func TestBuildXAIBillingSummarySupportsWeeklyCreditsShape(t *testing.T) {
	body := `{
		"config": {
			"currentPeriod": {
				"type": "weekly",
				"start": "2026-07-06T00:00:00Z",
				"end": "2026-07-13T00:00:00Z"
			},
			"creditUsagePercent": 64.5,
			"productUsage": [
				{"product": "Grok", "usagePercent": 50},
				{"product": "Think", "usage_percent": "82.25"}
			]
		}
	}`

	billing, used, err := buildXAIBillingSummary(body)
	if err != nil {
		t.Fatalf("buildXAIBillingSummary() error = %v", err)
	}
	if used == nil || *used != 64.5 {
		t.Fatalf("used percent = %v, want 64.5", used)
	}
	if billing["periodType"] != "weekly" || billing["usagePercent"] != 64.5 {
		t.Fatalf("weekly billing = %+v, want weekly usage percent", billing)
	}
	if billing["periodEnd"] != "2026-07-13T00:00:00Z" {
		t.Fatalf("periodEnd = %#v, want weekly period end", billing["periodEnd"])
	}
	usage, ok := billing["productUsage"].([]map[string]any)
	if !ok || len(usage) != 2 {
		t.Fatalf("productUsage = %#v, want 2 normalized items", billing["productUsage"])
	}
	if usage[1]["usagePercent"] != 82.25 {
		t.Fatalf("second usagePercent = %#v, want 82.25", usage[1]["usagePercent"])
	}
}

func TestXAIPlanTypeFromMonthlyBilling(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "free missing limit", body: `{"config": {}}`, want: "free"},
		{name: "free ignores on demand cap", body: `{"config": {"onDemandCap": {"val": 20000}}}`, want: "free"},
		{name: "free zero limit", body: `{"config": {"monthlyLimit": {"val": 0}}}`, want: "free"},
		{name: "supergrok", body: `{"config": {"monthlyLimit": {"val": 15000}}}`, want: "supergrok"},
		{name: "x premium plus", body: `{"config": {"monthlyLimit": {"val": 20000}}}`, want: "x-premium-plus"},
		{name: "supergrok heavy", body: `{"config": {"monthlyLimit": {"val": 150000}}}`, want: "supergrok-heavy"},
		{name: "unknown paid", body: `{"config": {"monthlyLimit": {"val": 99000}}}`, want: "paid-unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := xaiPlanTypeFromBillingBody(http.StatusOK, tt.body)
			if !known || got != tt.want {
				t.Fatalf("xaiPlanTypeFromBillingBody() = %q, %v; want %q, true", got, known, tt.want)
			}
		})
	}
	if got, known := xaiPlanTypeFromBillingBody(http.StatusUnauthorized, `{"config": {}}`); known || got != "" {
		t.Fatalf("failed billing inferred plan = %q, %v", got, known)
	}
}

func TestXAISummaryUsedPercentUsesFreeQuotaOnlyForFreePlan(t *testing.T) {
	freeQuota := map[string]any{"usedTokens": 75.0, "limitTokens": 100.0, "exhausted": false}
	free := emptyXAIBillingSummary()
	free["planType"] = "free"
	free["freeQuota"] = freeQuota
	if got := xaiSummaryUsedPercent(free); got == nil || *got != 75 {
		t.Fatalf("free used percent = %v, want 75", got)
	}
	paid := emptyXAIBillingSummary()
	paid["planType"] = "x-premium-plus"
	paid["freeQuota"] = map[string]any{"exhausted": true}
	if got := xaiSummaryUsedPercent(paid); got != nil {
		t.Fatalf("paid free-model exhaustion used percent = %v, want nil", got)
	}
}

func TestAccountInspectionDeepProbesUnknownXAIQuota(t *testing.T) {
	decision := accountInspectionDecision{Action: accountInspectionActionKeep}
	if !accountInspectionShouldDeepProbe(decision) {
		t.Fatal("unknown xAI quota should allow an explicitly enabled deep probe")
	}
}

func TestMergeXAIBillingSummariesCombinesWeeklyAndMonthly(t *testing.T) {
	weekly, _, err := buildXAIBillingSummary(`{
		"config": {
			"current_period": {"type": "weekly", "end": "2026-07-13T00:00:00Z"},
			"credit_usage_percent": 10,
			"product_usage": [{"product": "Grok", "usage_percent": 10}]
		}
	}`)
	if err != nil {
		t.Fatalf("weekly build error = %v", err)
	}
	monthly, _, err := buildXAIBillingSummary(`{
		"config": {
			"monthly_limit": 150000,
			"used": 160000,
			"on_demand_cap": 20000,
			"billing_period_end": "2026-08-01T00:00:00Z"
		}
	}`)
	if err != nil {
		t.Fatalf("monthly build error = %v", err)
	}

	merged := mergeXAIBillingSummaries(weekly, monthly)
	if merged["periodType"] != "weekly" || merged["usagePercent"] != 10.0 {
		t.Fatalf("merged weekly fields = %+v", merged)
	}
	if merged["monthlyLimitCents"] != 150000.0 || merged["includedUsedCents"] != 150000.0 || merged["onDemandUsedCents"] != 10000.0 {
		t.Fatalf("merged monthly fields = %+v", merged)
	}
	if merged["usedPercent"] != 100.0 || merged["onDemandUsedPercent"] != 50.0 {
		t.Fatalf("merged percentages = %+v", merged)
	}
	usage, ok := merged["productUsage"].([]map[string]any)
	if !ok || len(usage) != 1 || usage[0]["product"] != "Grok" {
		t.Fatalf("merged productUsage = %#v, want weekly product usage", merged["productUsage"])
	}
}

func TestXAIRequestHeadersIncludeGrokClientAndUserID(t *testing.T) {
	auth := &Auth{
		Provider: "xai",
		Metadata: map[string]any{"sub": "user-123"},
	}
	headers := xaiRequestHeaders(auth)
	if headers["x-xai-token-auth"] != "xai-grok-cli" {
		t.Fatalf("x-xai-token-auth = %q", headers["x-xai-token-auth"])
	}
	if headers["x-grok-client-version"] != "0.2.91" {
		t.Fatalf("x-grok-client-version = %q", headers["x-grok-client-version"])
	}
	if headers["x-userid"] != "user-123" {
		t.Fatalf("x-userid = %q, want user-123", headers["x-userid"])
	}
}

func TestXAIBillingURLMatchesUpstreamQuotaConfig(t *testing.T) {
	if got := xaiBillingURL(); got != "https://cli-chat-proxy.grok.com/v1/billing" {
		t.Fatalf("xaiBillingURL() = %q, want upstream billing endpoint", got)
	}
	if got := xaiBillingWeeklyURL(); got != "https://cli-chat-proxy.grok.com/v1/billing?format=credits" {
		t.Fatalf("xaiBillingWeeklyURL() = %q, want upstream weekly billing endpoint", got)
	}
}

func TestXAIDeepProbeDefaultsAndNormalization(t *testing.T) {
	defaults := defaultAccountInspectionSettings()
	if defaults.XAIDeepProbeEnabled {
		t.Fatal("xAI deep probe should be disabled by default")
	}
	if defaults.XAIDeepProbeModel != "grok-4.5" {
		t.Fatalf("default xAI deep probe model = %q, want grok-4.5", defaults.XAIDeepProbeModel)
	}

	normalized := normalizeAccountInspectionSchedule(accountInspectionSchedule{Settings: accountInspectionSettings{
		XAIDeepProbeEnabled: true,
		XAIDeepProbeModel:   "   ",
	}})
	if !normalized.Settings.XAIDeepProbeEnabled || normalized.Settings.XAIDeepProbeModel != "grok-4.5" {
		t.Fatalf("normalized xAI deep probe settings = enabled:%v model:%q", normalized.Settings.XAIDeepProbeEnabled, normalized.Settings.XAIDeepProbeModel)
	}
}

func TestXAIResponsesURLUsesConfiguredBaseURL(t *testing.T) {
	if got := xaiResponsesURL(nil); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(nil) = %q", got)
	}
	oauth := &Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1", "auth_kind": "oauth"}}
	if got := xaiResponsesURL(oauth); got != "https://cli-chat-proxy.grok.com/v1/responses" {
		t.Fatalf("xaiResponsesURL(oauth) = %q", got)
	}
	api := &Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1", "using_api": "true"}}
	if got := xaiResponsesURL(api); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(api) = %q", got)
	}
	if got := xaiOfficialChatURL(api); got != "https://api.x.ai/v1/chat/completions" {
		t.Fatalf("xaiOfficialChatURL(api) = %q", got)
	}
	metadataOAuth := &Auth{Metadata: map[string]any{"base_url": "https://api.x.ai/v1", "using_api": false}}
	if got := xaiResponsesURL(metadataOAuth); got != "https://cli-chat-proxy.grok.com/v1/responses" {
		t.Fatalf("xaiResponsesURL(metadataOAuth) = %q", got)
	}
	defaultAPI := &Auth{Attributes: map[string]string{"base_url": "https://api.x.ai/v1"}}
	if got := xaiResponsesURL(defaultAPI); got != "https://api.x.ai/v1/responses" {
		t.Fatalf("xaiResponsesURL(defaultAPI) = %q", got)
	}
	auth := &Auth{Attributes: map[string]string{"base_url": "https://xai.example/v1/"}}
	if got := xaiResponsesURL(auth); got != "https://xai.example/v1/responses" {
		t.Fatalf("xaiResponsesURL(custom) = %q", got)
	}
	headers := xaiDeepProbeHeaders(oauth)
	if headers["x-xai-token-auth"] != "xai-grok-cli" || headers["Accept"] != "text/event-stream" {
		t.Fatalf("xaiDeepProbeHeaders(oauth) = %#v", headers)
	}
	apiHeaders := xaiDeepProbeHeaders(api)
	if apiHeaders["x-xai-token-auth"] != "" || apiHeaders["Authorization"] != "Bearer $TOKEN$" {
		t.Fatalf("xaiDeepProbeHeaders(api) = %#v", apiHeaders)
	}
	officialHeaders := xaiOfficialAPIHeaders()
	if officialHeaders["x-xai-token-auth"] != "" || officialHeaders["Accept"] != "application/json" {
		t.Fatalf("xaiOfficialAPIHeaders() = %#v", officialHeaders)
	}
	customOAuth := &Auth{Attributes: map[string]string{"base_url": "https://xai.example/v1", "using_api": "false"}}
	customHeaders := xaiDeepProbeHeaders(customOAuth)
	if customHeaders["x-xai-token-auth"] != "" {
		t.Fatalf("xaiDeepProbeHeaders(customOAuth) = %#v", customHeaders)
	}
}

func TestBuildXAIOfficialHealthRequestAndSummary(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(buildXAIOfficialHealthBody(" grok-4.5 ")), &payload); err != nil {
		t.Fatalf("buildXAIOfficialHealthBody() JSON error = %v", err)
	}
	messages, _ := payload["messages"].([]any)
	if payload["model"] != "grok-4.5" || len(messages) != 1 || payload["stream"] != false || payload["max_tokens"] != float64(1) {
		t.Fatalf("official health payload = %#v", payload)
	}
	summary := xaiPaidHealthSummary()
	if summary["mode"] != "paid-health" || summary["planType"] != "paid" || summary["healthStatus"] != "chat-ok" {
		t.Fatalf("paid health summary = %#v", summary)
	}
	if _, exists := summary["freeQuota"]; exists {
		t.Fatalf("paid health summary contains free quota: %#v", summary)
	}
}

func TestXAIOfficialAPIQuotaDecision(t *testing.T) {
	active := xaiOfficialAPIQuotaDecision(accountInspectionAccount{}, `{"error":"credits exhausted"}`)
	if active.Action != accountInspectionActionDisable || !active.IsQuota || !strings.Contains(active.ErrorDetail, "credits exhausted") {
		t.Fatalf("active official quota decision = %#v", active)
	}
	disabled := xaiOfficialAPIQuotaDecision(accountInspectionAccount{Disabled: true}, `{"error":"credits exhausted"}`)
	if disabled.Action != accountInspectionActionKeep || !disabled.IsQuota {
		t.Fatalf("disabled official quota decision = %#v", disabled)
	}
}

func TestBuildXAIDeepProbeBodyUsesMinimalResponsesRequest(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(buildXAIDeepProbeBody(" grok-4.3 ")), &payload); err != nil {
		t.Fatalf("buildXAIDeepProbeBody() JSON error = %v", err)
	}
	input, _ := payload["input"].([]any)
	if payload["model"] != "grok-4.3" || len(input) != 1 || payload["stream"] != true || payload["store"] != false || payload["max_output_tokens"] != float64(1) {
		t.Fatalf("deep probe payload = %#v", payload)
	}
}

func TestClassifyXAIDeepProbeResponse(t *testing.T) {
	tests := []struct {
		name string
		resp accountInspectionHTTPResult
		want accountInspectionDeepProbeStatus
	}{
		{
			name: "completed sse",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"},
			want: accountInspectionDeepProbeSuccess,
		},
		{
			name: "output capped after successful execution",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: `data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n"},
			want: accountInspectionDeepProbeSuccess,
		},
		{
			name: "free usage exhausted",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusTooManyRequests, Body: `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for now."}`},
			want: accountInspectionDeepProbeQuota,
		},
		{
			name: "credits exhausted returned as forbidden",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusForbidden, Body: `{"error":{"message":"You have run out of credits or need a Grok subscription. Add credits or upgrade to SuperGrok."}}`},
			want: accountInspectionDeepProbeQuota,
		},
		{
			name: "unauthorized",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusUnauthorized, Body: `{"error":{"message":"invalid token"}}`},
			want: accountInspectionDeepProbeAuthError,
		},
		{
			name: "content filter incomplete response",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}` + "\n\n"},
			want: accountInspectionDeepProbeTransientError,
		},
		{
			name: "missing terminal response",
			resp: accountInspectionHTTPResult{StatusCode: http.StatusOK, Body: "data: {\"type\":\"response.created\"}\n\n"},
			want: accountInspectionDeepProbeTransientError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := classifyXAIDeepProbeResponse(tt.resp)
			if got != tt.want {
				t.Fatalf("classifyXAIDeepProbeResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyAntigravityDeepProbePrefersQuotaEvidenceOverAuthStatus(t *testing.T) {
	resp := accountInspectionHTTPResult{
		StatusCode: http.StatusForbidden,
		Body:       `{"error":{"status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}}`,
	}
	status, _ := classifyAntigravityDeepProbeResponse(resp)
	if status != accountInspectionDeepProbeQuota {
		t.Fatalf("classifyAntigravityDeepProbeResponse() = %q, want %q", status, accountInspectionDeepProbeQuota)
	}
}

func TestCodexDecisionPrefersQuotaEvidenceOverUnauthorizedStatus(t *testing.T) {
	decision := codexDecision(accountInspectionAccount{}, http.StatusUnauthorized, nil, true, 95)
	if !decision.IsQuota || decision.Action != accountInspectionActionDisable {
		t.Fatalf("codexDecision() = %#v, want quota disable decision", decision)
	}
	if got := accountInspectionDecisionErrorCode("codex", decision, testStatusCode(http.StatusUnauthorized)); got != "" {
		t.Fatalf("quota decision error code = %q, want empty", got)
	}
}

func TestRunXAIDeepProbeWithRetryRecoversFromEmptyResponse(t *testing.T) {
	attempts := 0
	resp, status, message, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		if attempts == 1 {
			return accountInspectionHTTPResult{StatusCode: http.StatusOK}, nil
		}
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeSuccess || message != "" {
		t.Fatalf("runXAIDeepProbeWithRetry() = resp:%+v status:%q message:%q err:%v, want success", resp, status, message, err)
	}
	if attempts != 2 {
		t.Fatalf("runXAIDeepProbeWithRetry() attempts = %d, want 2", attempts)
	}
}

func TestRunXAIDeepProbeWithRetryRecoversFromTransportError(t *testing.T) {
	attempts := 0
	_, status, _, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		if attempts == 1 {
			return accountInspectionHTTPResult{}, errors.New("temporary transport failure")
		}
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeSuccess || attempts != 2 {
		t.Fatalf("runXAIDeepProbeWithRetry() = status:%q attempts:%d err:%v, want success after 2 attempts", status, attempts, err)
	}
}

func TestRunXAIDeepProbeWithRetryDoesNotRetryContentFilter(t *testing.T) {
	attempts := 0
	_, status, message, err := runXAIDeepProbeWithRetry(context.Background(), 0, 0, func() (accountInspectionHTTPResult, error) {
		attempts++
		return accountInspectionHTTPResult{
			StatusCode: http.StatusOK,
			Body:       `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}` + "\n\n",
		}, nil
	})
	if err != nil || status != accountInspectionDeepProbeTransientError || !strings.Contains(message, "content_filter") {
		t.Fatalf("runXAIDeepProbeWithRetry() = status:%q message:%q err:%v, want content_filter transient error", status, message, err)
	}
	if attempts != 1 {
		t.Fatalf("runXAIDeepProbeWithRetry() attempts = %d, want 1", attempts)
	}
}

func TestAcquireXAIDeepProbeSerializesAndHonorsCancellation(t *testing.T) {
	scheduler := &accountInspectionScheduler{}
	releaseFirst, err := scheduler.acquireXAIDeepProbe(context.Background())
	if err != nil {
		t.Fatalf("first acquireXAIDeepProbe() error = %v", err)
	}

	secondAcquired := make(chan func(), 1)
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		release, acquireErr := scheduler.acquireXAIDeepProbe(context.Background())
		if acquireErr == nil {
			secondAcquired <- release
		}
	}()
	<-secondStarted
	select {
	case release := <-secondAcquired:
		release()
		releaseFirst()
		t.Fatal("second xAI deep probe acquired before the first probe released")
	case <-time.After(50 * time.Millisecond):
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	canceledResult := make(chan error, 1)
	go func() {
		_, acquireErr := scheduler.acquireXAIDeepProbe(canceledCtx)
		canceledResult <- acquireErr
	}()
	cancel()
	select {
	case acquireErr := <-canceledResult:
		if !errors.Is(acquireErr, context.Canceled) {
			releaseFirst()
			t.Fatalf("canceled acquireXAIDeepProbe() error = %v, want context.Canceled", acquireErr)
		}
	case <-time.After(time.Second):
		releaseFirst()
		t.Fatal("canceled acquireXAIDeepProbe() did not return")
	}

	releaseFirst()
	select {
	case release := <-secondAcquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("second xAI deep probe did not acquire after release")
	}
}

func TestSummarizeInspectionHTTPBodyExtractsCompleteNestedMessage(t *testing.T) {
	want := strings.TrimSpace(strings.Repeat("capacity unavailable ", 20))
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    http.StatusServiceUnavailable,
			"message": want,
			"status":  "UNAVAILABLE",
		},
	})
	if err != nil {
		t.Fatalf("marshal error payload: %v", err)
	}
	if got := summarizeInspectionHTTPBody(string(body)); got != want {
		t.Fatalf("summarizeInspectionHTTPBody() = %q, want complete nested message %q", got, want)
	}
	if got := inspectionHTTPErrorDetail("  " + string(body) + "\n"); got != string(body) {
		t.Fatalf("inspectionHTTPErrorDetail() = %q, want complete body %q", got, string(body))
	}
}

func TestWithInspectionHTTPErrorDetailPreservesCompleteResponse(t *testing.T) {
	body := `{"error":{"code":"invalid_token","message":"credential rejected"},"request_id":"req-123"}`
	decision := withInspectionHTTPErrorDetail(
		authErrorDecision(accountInspectionAccount{}, http.StatusUnauthorized),
		"  "+body+"\n",
	)
	if decision.ErrorDetail != body {
		t.Fatalf("ErrorDetail = %q, want complete response %q", decision.ErrorDetail, body)
	}
	if decision.Action != accountInspectionActionDisable {
		t.Fatalf("Action = %q, want %q", decision.Action, accountInspectionActionDisable)
	}
}

func TestTransientDeepProbeErrorCodeTakesPriorityOverHTTPStatus(t *testing.T) {
	decision := accountInspectionDecision{DeepProbeStatus: accountInspectionDeepProbeTransientError}
	status := testStatusCode(http.StatusBadRequest)
	tests := []struct {
		provider string
		want     string
	}{
		{provider: "antigravity", want: "antigravity_deep_probe_error"},
		{provider: "xai", want: "xai_deep_probe_error"},
	}
	for _, tt := range tests {
		if got := accountInspectionDecisionErrorCode(tt.provider, decision, status); got != tt.want {
			t.Fatalf("%s deep probe error code = %q, want %q", tt.provider, got, tt.want)
		}
		if !isInspectionAuthErrorCode(tt.want) {
			t.Fatalf("%s deep probe error code should be clearable after recovery", tt.provider)
		}
	}
}
