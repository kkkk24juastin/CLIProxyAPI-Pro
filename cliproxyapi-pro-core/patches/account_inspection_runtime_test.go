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
