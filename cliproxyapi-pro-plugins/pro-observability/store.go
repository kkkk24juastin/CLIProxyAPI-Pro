package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const usageEventsSchema = `create table if not exists usage_events (
	id integer primary key autoincrement,
	request_id text,
	event_hash text not null unique,
	timestamp_ms integer not null,
	timestamp text not null,
	provider text,
	executor_type text,
	model text not null,
	alias text,
	endpoint text,
	method text,
	path text,
	auth_type text,
	auth_index text,
	source text,
	source_hash text,
	api_key_hash text,
	api_key_policy_id text,
	profile_id text,
	profile_name_snapshot text,
	policy_mode text,
	requested_model text,
	effective_model text,
	client_ip text,
	x_forwarded_for text,
	user_agent text,
	input_tokens integer not null default 0,
	output_tokens integer not null default 0,
	reasoning_tokens integer not null default 0,
	cached_tokens integer not null default 0,
	cache_tokens integer not null default 0,
	cache_read_tokens integer not null default 0,
	cache_write_tokens integer not null default 0,
	total_tokens integer not null default 0,
	accounting_version integer not null default 0,
	accounting_quality text not null default '',
	uncached_input_tokens integer not null default 0,
	unclassified_tokens integer not null default 0,
	token_breakdown_json text,
	latency_ms integer,
	ttft_ms integer,
	status_code integer,
	error_code text,
	error_message text,
	upstream_request_id text,
	retry_after text,
	attempt_index integer,
	stream integer not null default 0,
	reasoning_effort text,
	service_tier text,
	effective_service_tier text,
	speed text,
	effective_speed text,
	estimated_cost real,
	price_rule_id integer,
	cost_breakdown_json text,
	failed integer not null default 0,
	raw_json text,
	created_at_ms integer not null
)`

const usageSummarySchema = `create table if not exists usage_summary (
	id integer primary key check (id = 1),
	latest_event_id integer not null default 0,
	total_requests integer not null default 0,
	success_count integer not null default 0,
	failure_count integer not null default 0,
	total_tokens integer not null default 0,
	generation integer not null default 1,
	reset_at_ms integer not null default 0,
	updated_at_ms integer not null
)`

var usageEventInsertColumns = []string{
	"request_id", "event_hash", "timestamp_ms", "timestamp", "provider", "executor_type", "model", "alias", "endpoint", "method", "path",
	"auth_type", "auth_index", "source", "source_hash", "api_key_hash", "api_key_policy_id", "profile_id", "profile_name_snapshot", "policy_mode", "requested_model", "effective_model", "client_ip", "x_forwarded_for", "user_agent",
	"input_tokens", "output_tokens", "reasoning_tokens", "cached_tokens", "cache_tokens", "cache_read_tokens", "cache_write_tokens", "total_tokens",
	"accounting_version", "accounting_quality", "uncached_input_tokens", "unclassified_tokens", "token_breakdown_json",
	"latency_ms", "ttft_ms", "status_code", "error_code", "error_message", "upstream_request_id", "retry_after", "attempt_index", "stream", "reasoning_effort", "service_tier", "effective_service_tier", "speed", "effective_speed",
	"estimated_cost", "price_rule_id", "cost_breakdown_json", "failed", "raw_json", "created_at_ms",
}

var additiveUsageEventColumns = map[string]string{
	"ttft_ms":                "integer",
	"status_code":            "integer",
	"error_code":             "text",
	"error_message":          "text",
	"upstream_request_id":    "text",
	"retry_after":            "text",
	"attempt_index":          "integer",
	"stream":                 "integer not null default 0",
	"reasoning_effort":       "text",
	"service_tier":           "text",
	"effective_service_tier": "text",
	"speed":                  "text",
	"effective_speed":        "text",
	"executor_type":          "text",
	"alias":                  "text",
	"cache_read_tokens":      "integer not null default 0",
	"cache_write_tokens":     "integer not null default 0",
	"accounting_version":     "integer not null default 0",
	"accounting_quality":     "text not null default ''",
	"uncached_input_tokens":  "integer not null default 0",
	"unclassified_tokens":    "integer not null default 0",
	"token_breakdown_json":   "text",
	"estimated_cost":         "real",
	"price_rule_id":          "integer",
	"cost_breakdown_json":    "text",
	"client_ip":              "text",
	"x_forwarded_for":        "text",
	"user_agent":             "text",
	"api_key_policy_id":      "text",
	"profile_id":             "text",
	"profile_name_snapshot":  "text",
	"policy_mode":            "text",
	"requested_model":        "text",
	"effective_model":        "text",
}

type usageStore struct {
	db *sql.DB
	mu sync.Mutex
}

func openUsageStore(databasePath string) (*usageStore, error) {
	databasePath = strings.TrimSpace(databasePath)
	if databasePath == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if databasePath != ":memory:" && !strings.HasPrefix(databasePath, "file:") {
		parent := filepath.Dir(databasePath)
		if parent != "." {
			if err := os.MkdirAll(parent, 0o750); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open usage database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &usageStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *usageStore) init(ctx context.Context) error {
	statements := []string{
		`pragma journal_mode = WAL`,
		`pragma synchronous = FULL`,
		`pragma busy_timeout = 5000`,
		usageEventsSchema,
		usageSummarySchema,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize usage database: %w", err)
		}
	}
	if err := ensureColumns(ctx, store.db, "usage_events", additiveUsageEventColumns); err != nil {
		return err
	}
	if err := ensureColumns(ctx, store.db, "usage_summary", map[string]string{
		"generation":  "integer not null default 1",
		"reset_at_ms": "integer not null default 0",
	}); err != nil {
		return err
	}
	indexes := []string{
		`create index if not exists idx_usage_events_timestamp on usage_events(timestamp_ms)`,
		`create index if not exists idx_usage_events_recent on usage_events(timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_request_id on usage_events(request_id)`,
		`create index if not exists idx_usage_events_model on usage_events(model)`,
		`create index if not exists idx_usage_events_provider_recent on usage_events(provider, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_model_recent on usage_events(model, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_failed_recent on usage_events(failed, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_auth_index on usage_events(auth_index)`,
		`create index if not exists idx_usage_events_auth_index_timestamp on usage_events(auth_index, timestamp_ms, id)`,
		`create index if not exists idx_usage_events_api_key_timestamp on usage_events(api_key_hash, timestamp_ms)`,
		`create index if not exists idx_usage_events_api_key_recent on usage_events(api_key_hash, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_policy_recent on usage_events(api_key_policy_id, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_profile_recent on usage_events(profile_id, timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_policy_mode_recent on usage_events(policy_mode, timestamp_ms desc, id desc)`,
	}
	for _, statement := range indexes {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize usage index: %w", err)
		}
	}
	_, err := store.db.ExecContext(ctx, `insert or ignore into usage_summary(
		id, latest_event_id, total_requests, success_count, failure_count, total_tokens, generation, reset_at_ms, updated_at_ms
	) values (1, 0, 0, 0, 0, 0, 1, 0, ?)`, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("initialize usage summary: %w", err)
	}
	return nil
}

func ensureColumns(ctx context.Context, db *sql.DB, table string, definitions map[string]string) error {
	rows, err := db.QueryContext(ctx, `pragma table_info(`+table+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan %s columns: %w", table, err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close %s column inspection: %w", table, err)
	}
	for name, definition := range definitions {
		if existing[name] {
			continue
		}
		if _, err := db.ExecContext(ctx, `alter table `+table+` add column `+name+` `+definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, name, err)
		}
	}
	return nil
}

func (store *usageStore) insertEvent(ctx context.Context, event usageEvent) (insertResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return insertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var resetAtMS int64
	if err := tx.QueryRowContext(ctx, `select reset_at_ms from usage_summary where id = 1`).Scan(&resetAtMS); err != nil {
		return insertResult{}, fmt.Errorf("read usage reset barrier: %w", err)
	}
	if resetAtMS > 0 && event.TimestampMS <= resetAtMS {
		return insertResult{Skipped: 1}, nil
	}
	values := usageEventValues(event)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	query := `insert or ignore into usage_events (` + strings.Join(usageEventInsertColumns, ",") + `) values (` + placeholders + `)`
	result, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return insertResult{}, fmt.Errorf("insert usage event: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return insertResult{}, fmt.Errorf("read inserted usage rows: %w", err)
	}
	if rowsAffected == 0 {
		if err := tx.Commit(); err != nil {
			return insertResult{}, err
		}
		return insertResult{Skipped: 1}, nil
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return insertResult{}, fmt.Errorf("read usage event id: %w", err)
	}
	successCount, failureCount := int64(1), int64(0)
	if event.Failed {
		successCount, failureCount = 0, 1
	}
	_, err = tx.ExecContext(ctx, `insert into usage_summary(
		id, latest_event_id, total_requests, success_count, failure_count, total_tokens, generation, reset_at_ms, updated_at_ms
	) values (1, ?, 1, ?, ?, ?, 1, 0, ?)
	 on conflict(id) do update set
		latest_event_id = max(usage_summary.latest_event_id, excluded.latest_event_id),
		total_requests = usage_summary.total_requests + excluded.total_requests,
		success_count = usage_summary.success_count + excluded.success_count,
		failure_count = usage_summary.failure_count + excluded.failure_count,
		total_tokens = usage_summary.total_tokens + excluded.total_tokens,
		updated_at_ms = excluded.updated_at_ms`,
		eventID, successCount, failureCount, event.TotalTokens, time.Now().UnixMilli())
	if err != nil {
		return insertResult{}, fmt.Errorf("update usage summary: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return insertResult{}, err
	}
	return insertResult{Inserted: 1}, nil
}

func usageEventValues(event usageEvent) []any {
	return []any{
		nullableString(event.RequestID), event.EventHash, event.TimestampMS, event.Timestamp,
		nullableString(event.Provider), nullableString(event.ExecutorType), event.Model, nullableString(event.Alias), nullableString(event.Endpoint), nullableString(event.Method), nullableString(event.Path),
		nullableString(event.AuthType), nullableString(event.AuthIndex), nullableString(event.Source), nullableString(event.SourceHash), nullableString(event.APIKeyHash), nullableString(event.APIKeyPolicyID), nullableString(event.ProfileID), nullableString(event.ProfileNameSnapshot), nullableString(event.PolicyMode), nullableString(event.RequestedModel), nullableString(event.EffectiveModel), nullableString(event.ClientIP), nullableString(event.XForwardedFor), nullableString(event.UserAgent),
		event.InputTokens, event.OutputTokens, event.ReasoningTokens, event.CachedTokens, event.CacheTokens, event.CacheReadTokens, event.CacheWriteTokens, event.TotalTokens,
		event.AccountingVersion, event.AccountingQuality, event.UncachedInputTokens, event.UnclassifiedTokens, nullableString(event.TokenBreakdownJSON),
		nullableInt64(event.LatencyMS), nullableInt64(event.TTFTMS), nullableInt(event.StatusCode), nullableString(event.ErrorCode), nullableString(event.ErrorMessage), nullableString(event.UpstreamRequestID), nullableString(event.RetryAfter), nullableInt64(event.AttemptIndex), boolInt(event.Stream), nullableString(event.ReasoningEffort), nullableString(event.ServiceTier), nullableString(event.EffectiveServiceTier), nullableString(event.Speed), nullableString(event.EffectiveSpeed),
		nil, nil, nil, boolInt(event.Failed), nullableString(event.RawJSON), event.CreatedAtMS,
	}
}

func (store *usageStore) summary(ctx context.Context) (usageSummary, error) {
	var summary usageSummary
	err := store.db.QueryRowContext(ctx, `select latest_event_id, total_requests, success_count, failure_count, total_tokens, generation, reset_at_ms, updated_at_ms from usage_summary where id = 1`).Scan(
		&summary.LatestEventID, &summary.TotalRequests, &summary.SuccessCount, &summary.FailureCount,
		&summary.TotalTokens, &summary.Generation, &summary.ResetAtMS, &summary.UpdatedAtMS,
	)
	return summary, err
}

func (store *usageStore) reset(ctx context.Context, resetAtMS int64) (usageSummary, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if resetAtMS <= 0 {
		resetAtMS = time.Now().UnixMilli()
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return usageSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `delete from usage_events`); err != nil {
		return usageSummary{}, err
	}
	if _, err := tx.ExecContext(ctx, `update usage_summary set
		latest_event_id = 0, total_requests = 0, success_count = 0, failure_count = 0, total_tokens = 0,
		generation = generation + 1, reset_at_ms = max(reset_at_ms, ?), updated_at_ms = ? where id = 1`, resetAtMS, time.Now().UnixMilli()); err != nil {
		return usageSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return usageSummary{}, err
	}
	return store.summary(ctx)
}

func (store *usageStore) close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
