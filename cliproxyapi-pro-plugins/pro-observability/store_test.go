package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestUsageStore(t *testing.T) *usageStore {
	t.Helper()
	store, err := openUsageStore(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	return store
}

func TestUsageStorePersistsFullRPCEventAndSummary(t *testing.T) {
	store := openTestUsageStore(t)
	event, err := usageEventFromRPC(testUsageRecord(t), time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.insertEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 || result.Skipped != 0 {
		t.Fatalf("insert result = %#v", result)
	}

	var requestID, provider, executorType, model, alias, endpoint string
	var policyID, profileID, profileName, policyMode, requestedModel, effectiveModel string
	var clientIP, forwardedFor, userAgent string
	var inputTokens, outputTokens, reasoningTokens, cacheReadTokens, cacheWriteTokens, totalTokens int64
	var accountingVersion int
	var accountingQuality, tokenBreakdownJSON string
	var attemptIndex, stream, failed int64
	if err := store.db.QueryRow(`select
		request_id, provider, executor_type, model, alias, endpoint,
		api_key_policy_id, profile_id, profile_name_snapshot, policy_mode, requested_model, effective_model,
		client_ip, x_forwarded_for, user_agent,
		input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens,
		accounting_version, accounting_quality, token_breakdown_json, attempt_index, stream, failed
		from usage_events where event_hash = ?`, event.EventHash).Scan(
		&requestID, &provider, &executorType, &model, &alias, &endpoint,
		&policyID, &profileID, &profileName, &policyMode, &requestedModel, &effectiveModel,
		&clientIP, &forwardedFor, &userAgent,
		&inputTokens, &outputTokens, &reasoningTokens, &cacheReadTokens, &cacheWriteTokens, &totalTokens,
		&accountingVersion, &accountingQuality, &tokenBreakdownJSON, &attemptIndex, &stream, &failed,
	); err != nil {
		t.Fatal(err)
	}
	if requestID != "request-1" || provider != "codex" || executorType != "codex" || model != "gpt-5" || alias != "smart" || endpoint != "POST /v1/responses" {
		t.Fatalf("base fields = %q %q %q %q %q %q", requestID, provider, executorType, model, alias, endpoint)
	}
	if policyID != "policy-1" || profileID != "profile-1" || profileName != "Production" || policyMode != "profile" || requestedModel != "smart" || effectiveModel != "gpt-5" {
		t.Fatalf("policy fields = %q %q %q %q %q %q", policyID, profileID, profileName, policyMode, requestedModel, effectiveModel)
	}
	if clientIP != "192.0.2.1" || forwardedFor != "198.51.100.2" || userAgent != "plugin-contract-test" {
		t.Fatalf("client fields = %q %q %q", clientIP, forwardedFor, userAgent)
	}
	if inputTokens != 11 || outputTokens != 7 || reasoningTokens != 3 || cacheReadTokens != 2 || cacheWriteTokens != 1 || totalTokens != 18 || accountingVersion != 2 || accountingQuality != "complete" || tokenBreakdownJSON == "" {
		t.Fatalf("accounting fields = %d %d %d %d %d %d %d %q %q", inputTokens, outputTokens, reasoningTokens, cacheReadTokens, cacheWriteTokens, totalTokens, accountingVersion, accountingQuality, tokenBreakdownJSON)
	}
	if attemptIndex != 2 || stream != 1 || failed != 1 {
		t.Fatalf("request state = attempt:%d stream:%d failed:%d", attemptIndex, stream, failed)
	}
	summary, err := store.summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalRequests != 1 || summary.SuccessCount != 0 || summary.FailureCount != 1 || summary.TotalTokens != 18 || summary.LatestEventID <= 0 || summary.Generation != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestUsageStoreSkipsDuplicatesAndResetBarrier(t *testing.T) {
	ctx := context.Background()
	store := openTestUsageStore(t)
	event, err := usageEventFromRPC(testUsageRecord(t), time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.insertEvent(ctx, event); err != nil || result.Inserted != 1 {
		t.Fatalf("first insert = %#v, %v", result, err)
	}
	if result, err := store.insertEvent(ctx, event); err != nil || result.Skipped != 1 {
		t.Fatalf("duplicate insert = %#v, %v", result, err)
	}
	resetAt := event.TimestampMS + 1000
	resetSummary, err := store.reset(ctx, resetAt)
	if err != nil {
		t.Fatal(err)
	}
	if resetSummary.Generation != 2 || resetSummary.ResetAtMS != resetAt || resetSummary.TotalRequests != 0 {
		t.Fatalf("reset summary = %#v", resetSummary)
	}
	event.EventHash = hashString("stale-after-reset")
	if result, err := store.insertEvent(ctx, event); err != nil || result.Skipped != 1 {
		t.Fatalf("stale insert = %#v, %v", result, err)
	}
	event.TimestampMS = resetAt + 1
	event.Timestamp = time.UnixMilli(event.TimestampMS).UTC().Format(time.RFC3339Nano)
	event.EventHash = buildEventHash(event)
	if result, err := store.insertEvent(ctx, event); err != nil || result.Inserted != 1 {
		t.Fatalf("fresh insert = %#v, %v", result, err)
	}
	summary, err := store.summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Generation != 2 || summary.TotalRequests != 1 || summary.FailureCount != 1 || summary.TotalTokens != 18 {
		t.Fatalf("post-reset summary = %#v", summary)
	}
}

func TestUsageStoreRollsBackEventWhenSummaryUpdateFails(t *testing.T) {
	ctx := context.Background()
	store := openTestUsageStore(t)
	if _, err := store.db.ExecContext(ctx, `create trigger fail_usage_summary before update on usage_summary begin select raise(abort, 'summary failure'); end`); err != nil {
		t.Fatal(err)
	}
	event, err := usageEventFromRPC(testUsageRecord(t), time.Date(2026, 8, 21, 1, 2, 4, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.insertEvent(ctx, event); err == nil {
		t.Fatal("insert error = nil, want summary failure")
	}
	var count int64
	if err := store.db.QueryRowContext(ctx, `select count(*) from usage_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("usage event count = %d, want rollback", count)
	}
}
