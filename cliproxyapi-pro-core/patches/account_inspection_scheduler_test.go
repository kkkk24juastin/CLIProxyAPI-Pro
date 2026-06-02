package management

import "testing"

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

func testStatusCode(value int) *int {
	return &value
}

func TestLimitAccountInspectionResultsKeepsEachHealthBucket(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionResult("healthy-1", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("healthy-2", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("auth-1", accountInspectionActionDelete, false, nil, false, ""),
		testInspectionResult("auth-2", accountInspectionActionKeep, false, testStatusCode(401), false, ""),
		testInspectionResult("quota-1", accountInspectionActionDisable, false, nil, false, ""),
		testInspectionResult("quota-2", accountInspectionActionKeep, false, nil, true, ""),
		testInspectionResult("error-1", accountInspectionActionKeep, false, nil, false, "network error"),
		testInspectionResult("error-2", accountInspectionActionKeep, false, nil, false, "timeout"),
		testInspectionResult("recoverable-1", accountInspectionActionEnable, true, nil, false, ""),
		testInspectionResult("recoverable-2", accountInspectionActionKeep, true, nil, false, ""),
		testInspectionResult("disabled-1", accountInspectionActionKeep, true, nil, false, ""),
		testInspectionResult("disabled-2", accountInspectionActionKeep, true, nil, false, ""),
	}

	limited, wasLimited := limitAccountInspectionResults(results, 1, accountInspectionResultHealthCounts(results), 3)
	if !wasLimited {
		t.Fatal("limitAccountInspectionResults() wasLimited = false, want true")
	}

	seenBuckets := make(map[accountInspectionHealthBucket]int)
	for _, result := range limited {
		seenBuckets[accountInspectionResultHealthBucketOf(result)]++
	}

	for _, bucket := range []accountInspectionHealthBucket{
		accountInspectionHealthHealthy,
		accountInspectionHealthAuthInvalid,
		accountInspectionHealthQuotaExhausted,
		accountInspectionHealthInspectionError,
		accountInspectionHealthRecoverable,
		accountInspectionHealthDisabled,
	} {
		if seenBuckets[bucket] != 1 {
			t.Fatalf("bucket %s count = %d, want 1; limited=%v", bucket, seenBuckets[bucket], limited)
		}
	}
}

func TestLimitAccountInspectionResultsStopsAfterRepresentativeWindow(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionResult("healthy", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("auth", accountInspectionActionDelete, false, nil, false, ""),
		testInspectionResult("quota", accountInspectionActionDisable, false, nil, false, ""),
		testInspectionResult("error", accountInspectionActionKeep, false, nil, false, "network error"),
		testInspectionResult("recoverable", accountInspectionActionEnable, true, nil, false, ""),
		testInspectionResult("disabled", accountInspectionActionKeep, true, nil, false, ""),
		testInspectionResult("late-healthy", accountInspectionActionKeep, false, nil, false, ""),
	}

	limited, wasLimited := limitAccountInspectionResults(results, 1, accountInspectionResultHealthCounts(results), 3)
	if !wasLimited {
		t.Fatal("limitAccountInspectionResults() wasLimited = false, want true")
	}
	if len(limited) != 6 {
		t.Fatalf("limited len = %d, want representative six buckets; limited=%v", len(limited), limited)
	}
	for _, result := range limited {
		if result.Key == "late-healthy" {
			t.Fatalf("limitAccountInspectionResults() included late row after representative window: %v", limited)
		}
	}
}

func TestLimitAccountInspectionResultsStopsWhenMissingBucketsHaveNoTargets(t *testing.T) {
	results := []accountInspectionResult{
		testInspectionResult("healthy-1", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("healthy-2", accountInspectionActionKeep, false, nil, false, ""),
		testInspectionResult("late-healthy", accountInspectionActionKeep, false, nil, false, ""),
	}

	limited, wasLimited := limitAccountInspectionResults(results, 1, accountInspectionResultHealthCounts(results), 0)
	if !wasLimited {
		t.Fatal("limitAccountInspectionResults() wasLimited = false, want true")
	}
	if len(limited) != 1 || limited[0].Key != "healthy-1" {
		t.Fatalf("limited = %+v, want only first healthy representative", limited)
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
	if status.Results != nil || status.Logs != nil || status.HealthCounts != nil {
		t.Fatalf("streamStatusLocked(light) leaked details: results=%v logs=%v health=%v", status.Results, status.Logs, status.HealthCounts)
	}
	if status.ResultsLimited || status.LogsLimited {
		t.Fatalf("streamStatusLocked(light) limited flags = results:%v logs:%v, want false", status.ResultsLimited, status.LogsLimited)
	}
}

func TestStreamStatusLockedReturnsLimitedDetailsWithFullHealthCounts(t *testing.T) {
	scheduler := &accountInspectionScheduler{
		status: accountInspectionStatus{
			Results: []accountInspectionResult{
				testInspectionResult("healthy-1", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionResult("healthy-2", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionResult("auth-1", accountInspectionActionDelete, false, nil, false, ""),
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
		ResultLimit:    1,
		LogLimit:       2,
	})

	if status.HealthCounts == nil {
		t.Fatal("streamStatusLocked(details) HealthCounts = nil")
	}
	if status.HealthCounts.Total != 4 || status.HealthCounts.Healthy != 2 || status.HealthCounts.AuthInvalid != 2 {
		t.Fatalf("HealthCounts = %+v, want total=4 healthy=2 authInvalid=2", *status.HealthCounts)
	}
	if !status.ResultsLimited || !status.LogsLimited {
		t.Fatalf("limited flags = results:%v logs:%v, want true/true", status.ResultsLimited, status.LogsLimited)
	}
	if len(status.Results) != 2 {
		t.Fatalf("limited results len = %d, want 2 health-bucket rows", len(status.Results))
	}
	if len(status.Logs) != 2 || status.Logs[0].Time != 2 || status.Logs[1].Time != 3 {
		t.Fatalf("limited logs = %+v, want last two log entries", status.Logs)
	}
}

func TestHealthCountsLockedRebuildsStaleCache(t *testing.T) {
	scheduler := &accountInspectionScheduler{
		status: accountInspectionStatus{
			Results: []accountInspectionResult{
				testInspectionResult("healthy", accountInspectionActionKeep, false, nil, false, ""),
				testInspectionResult("auth", accountInspectionActionDelete, false, nil, false, ""),
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
