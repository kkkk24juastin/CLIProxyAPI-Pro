package embeddedusage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage/internalusage"
	_ "modernc.org/sqlite"
)

type InsertResult struct {
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"`
}

type UsageSummary struct {
	TotalRequests int64
	SuccessCount  int64
	FailureCount  int64
	TotalTokens   int64
}

type usageSummarySnapshot struct {
	LatestID int64
	Summary  UsageSummary
}

type cachedUsageSummary struct {
	LatestID int64
	Summary  UsageSummary
}

type QuotaCacheEntry struct {
	ID         string          `json:"id"`
	Provider   string          `json:"provider"`
	FileName   string          `json:"fileName"`
	Data       json.RawMessage `json:"data"`
	CachedAt   int64           `json:"cachedAt"`
	AccessedAt int64           `json:"accessedAt"`
	Version    int             `json:"version"`
}

type QuotaCacheStats struct {
	TotalEntries int64 `json:"totalEntries"`
	UpdatedAt    int64 `json:"updatedAt"`
}

type MonitoringSettings struct {
	RetentionDays int                          `json:"retentionDays"`
	WebDAV        MonitoringWebDAVBackupConfig `json:"webdav"`
}

type MonitoringWebDAVBackupConfig struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	RetentionDays   int    `json:"retentionDays"`
	URL             string `json:"url"`
	Username        string `json:"username"`
	Password        string `json:"password"`
}

type ModelPrice struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
	Cache      float64 `json:"cache"`
}

type modelPricesExportRecord struct {
	RecordType string                `json:"record_type"`
	Version    int                   `json:"version"`
	Prices     map[string]ModelPrice `json:"prices"`
	ExportedAt int64                 `json:"exported_at_ms"`
}

type monitoringSettingsExportRecord struct {
	RecordType string             `json:"record_type"`
	Version    int                `json:"version"`
	Settings   MonitoringSettings `json:"settings"`
	ExportedAt int64              `json:"exported_at_ms"`
}

type quotaCacheExportRecord struct {
	RecordType string            `json:"record_type"`
	Version    int               `json:"version"`
	Entries    []QuotaCacheEntry `json:"entries"`
	ExportedAt int64             `json:"exported_at_ms"`
}

const modelPricesExportRecordType = "model_prices"
const quotaCacheExportRecordType = "quota_cache"
const monitoringSettingsExportRecordType = "monitoring_settings"

type Store struct {
	db           *sql.DB
	quotaCacheMu sync.Mutex
	summaryMu    sync.RWMutex
	summaryCache *cachedUsageSummary
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	db.SetMaxOpenConns(1)
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	db := s.db
	s.db = nil
	return db.Close()
}

func (s *Store) invalidateUsageSummaryCache() {
	s.summaryMu.Lock()
	s.summaryCache = nil
	s.summaryMu.Unlock()
}

func (s *Store) init() error {
	statements := []string{
		`pragma journal_mode = WAL`,
		`pragma synchronous = FULL`,
		`pragma busy_timeout = 5000`,
		`create table if not exists usage_events (
			id integer primary key autoincrement,
			request_id text,
			event_hash text not null unique,
			timestamp_ms integer not null,
			timestamp text not null,
			provider text,
			model text not null,
			endpoint text,
			method text,
			path text,
			auth_type text,
			auth_index text,
			source text,
			source_hash text,
			api_key_hash text,
			input_tokens integer not null default 0,
			output_tokens integer not null default 0,
			reasoning_tokens integer not null default 0,
			cached_tokens integer not null default 0,
			cache_tokens integer not null default 0,
			total_tokens integer not null default 0,
			latency_ms integer,
			failed integer not null default 0,
			raw_json text,
			created_at_ms integer not null
		)`,
		`create index if not exists idx_usage_events_timestamp on usage_events(timestamp_ms)`,
		`create index if not exists idx_usage_events_recent on usage_events(timestamp_ms desc, id desc)`,
		`create index if not exists idx_usage_events_request_id on usage_events(request_id)`,
		`create index if not exists idx_usage_events_model on usage_events(model)`,
		`create index if not exists idx_usage_events_auth_index on usage_events(auth_index)`,
		`create table if not exists usage_summary (
			id integer primary key check (id = 1),
			latest_event_id integer not null default 0,
			total_requests integer not null default 0,
			success_count integer not null default 0,
			failure_count integer not null default 0,
			total_tokens integer not null default 0,
			updated_at_ms integer not null
		)`,
		`create table if not exists dead_letter_events (
			id integer primary key autoincrement,
			payload text not null,
			error text not null,
			created_at_ms integer not null
		)`,
		`create table if not exists quota_cache (
			id text primary key,
			provider text not null,
			file_name text not null,
			data_json text not null,
			cached_at_ms integer not null,
			accessed_at_ms integer not null,
			version integer not null default 1
		)`,
		`create index if not exists idx_quota_cache_provider on quota_cache(provider)`,
		`create index if not exists idx_quota_cache_accessed_at on quota_cache(accessed_at_ms)`,
		`create table if not exists model_prices (
				model text primary key,
				prompt_price real not null,
				completion_price real not null,
				cache_price real not null,
				updated_at_ms integer not null
			)`,
		`create index if not exists idx_model_prices_updated_at on model_prices(updated_at_ms)`,
		`create table if not exists monitoring_settings (
			id integer primary key check (id = 1),
			settings_json text not null,
			updated_at_ms integer not null
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return s.ensureUsageSummary()
}

func (s *Store) InsertEvents(ctx context.Context, events []internalusage.Event) (InsertResult, error) {
	if len(events) == 0 {
		return InsertResult{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `insert or ignore into usage_events (
		request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return InsertResult{}, err
	}
	defer stmt.Close()

	result := InsertResult{}
	summaryDelta := UsageSummary{}
	latestInsertedID := int64(0)
	for _, event := range events {
		failed := 0
		if event.Failed {
			failed = 1
		}
		res, err := stmt.ExecContext(ctx,
			nullString(event.RequestID), event.EventHash, event.TimestampMS, event.Timestamp,
			nullString(event.Provider), event.Model, nullString(event.Endpoint), nullString(event.Method), nullString(event.Path),
			nullString(event.AuthType), nullString(event.AuthIndex), nullString(event.Source), nullString(event.SourceHash), nullString(event.APIKeyHash),
			event.InputTokens, event.OutputTokens, event.ReasoningTokens, event.CachedTokens, event.CacheTokens, event.TotalTokens,
			nullInt(event.LatencyMS), failed, nullString(event.RawJSON), event.CreatedAtMS,
		)
		if err != nil {
			return InsertResult{}, err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			result.Inserted++
			summaryDelta.TotalRequests++
			if event.Failed {
				summaryDelta.FailureCount++
			} else {
				summaryDelta.SuccessCount++
			}
			summaryDelta.TotalTokens += event.TotalTokens
			if insertedID, err := res.LastInsertId(); err == nil && insertedID > latestInsertedID {
				latestInsertedID = insertedID
			}
		} else {
			result.Skipped++
		}
	}
	if result.Inserted > 0 {
		if latestInsertedID <= 0 {
			if err := tx.QueryRowContext(ctx, `select coalesce(max(id), 0) from usage_events`).Scan(&latestInsertedID); err != nil {
				return InsertResult{}, err
			}
		}
		if err := s.applyUsageSummaryDelta(ctx, tx, latestInsertedID, summaryDelta); err != nil {
			return InsertResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return InsertResult{}, err
	}
	if result.Inserted > 0 {
		s.invalidateUsageSummaryCache()
	}
	return result, nil
}

func (s *Store) ensureUsageSummary() error {
	snapshot, err := s.readUsageSummarySnapshot(context.Background())
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	latestID, _, cursorErr := s.LatestCursor(context.Background())
	if cursorErr != nil {
		return cursorErr
	}
	if err == sql.ErrNoRows || snapshot.LatestID != latestID {
		return s.rebuildUsageSummary(context.Background())
	}
	return nil
}

func (s *Store) readUsageSummarySnapshot(ctx context.Context) (usageSummarySnapshot, error) {
	var snapshot usageSummarySnapshot
	err := s.db.QueryRowContext(ctx, `select
		latest_event_id,
		total_requests,
		success_count,
		failure_count,
		total_tokens
		from usage_summary
		where id = 1`).Scan(
		&snapshot.LatestID,
		&snapshot.Summary.TotalRequests,
		&snapshot.Summary.SuccessCount,
		&snapshot.Summary.FailureCount,
		&snapshot.Summary.TotalTokens,
	)
	return snapshot, err
}

func (s *Store) applyUsageSummaryDelta(ctx context.Context, tx *sql.Tx, latestID int64, delta UsageSummary) error {
	_, err := tx.ExecContext(ctx, `insert into usage_summary(
		id,
		latest_event_id,
		total_requests,
		success_count,
		failure_count,
		total_tokens,
		updated_at_ms
	) values(1, ?, ?, ?, ?, ?, ?)
	on conflict(id) do update set
		latest_event_id = max(usage_summary.latest_event_id, excluded.latest_event_id),
		total_requests = usage_summary.total_requests + excluded.total_requests,
		success_count = usage_summary.success_count + excluded.success_count,
		failure_count = usage_summary.failure_count + excluded.failure_count,
		total_tokens = usage_summary.total_tokens + excluded.total_tokens,
		updated_at_ms = excluded.updated_at_ms`,
		latestID,
		delta.TotalRequests,
		delta.SuccessCount,
		delta.FailureCount,
		delta.TotalTokens,
		time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) rebuildUsageSummary(ctx context.Context) error {
	var latestID sql.NullInt64
	var totalRequests, successCount, failureCount, totalTokens sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `select
		coalesce(max(id), 0),
		count(*),
		coalesce(sum(case when failed = 0 then 1 else 0 end), 0),
		coalesce(sum(case when failed != 0 then 1 else 0 end), 0),
		coalesce(sum(total_tokens), 0)
		from usage_events`).Scan(&latestID, &totalRequests, &successCount, &failureCount, &totalTokens); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `insert into usage_summary(
		id,
		latest_event_id,
		total_requests,
		success_count,
		failure_count,
		total_tokens,
		updated_at_ms
	) values(1, ?, ?, ?, ?, ?, ?)
	on conflict(id) do update set
		latest_event_id = excluded.latest_event_id,
		total_requests = excluded.total_requests,
		success_count = excluded.success_count,
		failure_count = excluded.failure_count,
		total_tokens = excluded.total_tokens,
		updated_at_ms = excluded.updated_at_ms`,
		latestID.Int64,
		totalRequests.Int64,
		successCount.Int64,
		failureCount.Int64,
		totalTokens.Int64,
		time.Now().UnixMilli(),
	)
	if err == nil {
		s.invalidateUsageSummaryCache()
	}
	return err
}

func (s *Store) AddDeadLetter(ctx context.Context, payload string, parseErr error) error {
	_, err := s.db.ExecContext(ctx,
		`insert into dead_letter_events(payload, error, created_at_ms) values(?, ?, ?)`,
		payload, parseErr.Error(), time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) scanEvents(rows *sql.Rows) ([]internalusage.Event, error) {
	events := make([]internalusage.Event, 0)
	for rows.Next() {
		var event internalusage.Event
		var requestID, provider, endpoint, method, path, authType, authIndex, source, sourceHash, apiKeyHash, rawJSON sql.NullString
		var latency sql.NullInt64
		var failed int
		if err := rows.Scan(
			&event.ID, &requestID, &event.EventHash, &event.TimestampMS, &event.Timestamp, &provider, &event.Model,
			&endpoint, &method, &path, &authType, &authIndex, &source, &sourceHash, &apiKeyHash,
			&event.InputTokens, &event.OutputTokens, &event.ReasoningTokens, &event.CachedTokens, &event.CacheTokens, &event.TotalTokens,
			&latency, &failed, &rawJSON, &event.CreatedAtMS,
		); err != nil {
			return nil, err
		}
		event.RequestID = requestID.String
		event.Provider = provider.String
		event.Endpoint = endpoint.String
		event.Method = method.String
		event.Path = path.String
		event.AuthType = authType.String
		event.AuthIndex = authIndex.String
		event.Source = source.String
		event.SourceHash = sourceHash.String
		event.APIKeyHash = apiKeyHash.String
		event.RawJSON = rawJSON.String
		event.Failed = failed != 0
		if latency.Valid {
			value := latency.Int64
			event.LatencyMS = &value
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) RecentEvents(ctx context.Context, limit int) ([]internalusage.Event, error) {
	if limit <= 0 {
		limit = 50000
	}
	rows, err := s.db.QueryContext(ctx, `select
		id, request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms
		from usage_events indexed by idx_usage_events_recent
		order by timestamp_ms desc, id desc
		limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanEvents(rows)
}

func (s *Store) EventsAfter(ctx context.Context, afterID int64, limit int) ([]internalusage.Event, error) {
	if limit <= 0 || limit > usageEventsSentinelLimit {
		limit = usageEventsSentinelLimit
	}
	rows, err := s.db.QueryContext(ctx, `select
		id, request_id, event_hash, timestamp_ms, timestamp, provider, model, endpoint, method, path,
		auth_type, auth_index, source, source_hash, api_key_hash,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_tokens, total_tokens,
		latency_ms, failed, raw_json, created_at_ms
		from usage_events
		where id > ?
		order by id asc
		limit ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanEvents(rows)
}

func (s *Store) LatestCursor(ctx context.Context) (int64, int64, error) {
	var id sql.NullInt64
	var timestamp sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `select id, timestamp_ms from usage_events order by id desc limit 1`).Scan(&id, &timestamp); err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	latestID := int64(0)
	latestTimestamp := int64(0)
	if id.Valid {
		latestID = id.Int64
	}
	if timestamp.Valid {
		latestTimestamp = timestamp.Int64
	}
	return latestID, latestTimestamp, nil
}

func (s *Store) UsageSummary(ctx context.Context, maxID int64) (UsageSummary, error) {
	if maxID <= 0 {
		return UsageSummary{}, nil
	}
	latestID, _, err := s.LatestCursor(ctx)
	if err != nil {
		return UsageSummary{}, err
	}
	if maxID == latestID {
		s.summaryMu.RLock()
		if s.summaryCache != nil && s.summaryCache.LatestID == maxID {
			summary := s.summaryCache.Summary
			s.summaryMu.RUnlock()
			return summary, nil
		}
		s.summaryMu.RUnlock()

		snapshot, err := s.readUsageSummarySnapshot(ctx)
		if err != nil {
			if err != sql.ErrNoRows {
				return UsageSummary{}, err
			}
		} else if snapshot.LatestID == maxID {
			s.summaryMu.Lock()
			s.summaryCache = &cachedUsageSummary{LatestID: maxID, Summary: snapshot.Summary}
			s.summaryMu.Unlock()
			return snapshot.Summary, nil
		}
		if err := s.rebuildUsageSummary(ctx); err != nil {
			return UsageSummary{}, err
		}
		snapshot, err = s.readUsageSummarySnapshot(ctx)
		if err != nil {
			return UsageSummary{}, err
		}
		if snapshot.LatestID == maxID {
			s.summaryMu.Lock()
			s.summaryCache = &cachedUsageSummary{LatestID: maxID, Summary: snapshot.Summary}
			s.summaryMu.Unlock()
			return snapshot.Summary, nil
		}
	}

	var summary UsageSummary
	var totalRequests, successCount, failureCount, totalTokens sql.NullInt64
	query := `select
		count(*),
		coalesce(sum(case when failed = 0 then 1 else 0 end), 0),
		coalesce(sum(case when failed != 0 then 1 else 0 end), 0),
		coalesce(sum(total_tokens), 0)
		from usage_events`
	args := []any{}
	query += ` where id <= ?`
	args = append(args, maxID)
	err = s.db.QueryRowContext(ctx, query, args...).Scan(&totalRequests, &successCount, &failureCount, &totalTokens)
	if err != nil {
		return summary, err
	}
	if totalRequests.Valid {
		summary.TotalRequests = totalRequests.Int64
	}
	if successCount.Valid {
		summary.SuccessCount = successCount.Int64
	}
	if failureCount.Valid {
		summary.FailureCount = failureCount.Int64
	}
	if totalTokens.Valid {
		summary.TotalTokens = totalTokens.Int64
	}
	if maxID == latestID {
		s.summaryMu.Lock()
		s.summaryCache = &cachedUsageSummary{LatestID: maxID, Summary: summary}
		s.summaryMu.Unlock()
	}
	return summary, nil
}

func (s *Store) Counts(ctx context.Context) (events int64, deadLetters int64, err error) {
	if err = s.db.QueryRowContext(ctx, `select count(*) from usage_events`).Scan(&events); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRowContext(ctx, `select count(*) from dead_letter_events`).Scan(&deadLetters); err != nil {
		return 0, 0, err
	}
	return events, deadLetters, nil
}

func (s *Store) ExportJSONL(ctx context.Context) ([]byte, error) {
	events, err := s.RecentEvents(ctx, 0)
	if err != nil {
		return nil, err
	}
	prices, err := s.GetModelPrices(ctx)
	if err != nil {
		return nil, err
	}
	quotaEntries, err := s.GetQuotaCache(ctx, "", "")
	if err != nil {
		return nil, err
	}
	settings, err := s.GetMonitoringSettings(ctx)
	if err != nil {
		return nil, err
	}

	output := make([]byte, 0)
	line, err := json.Marshal(monitoringSettingsExportRecord{
		RecordType: monitoringSettingsExportRecordType,
		Version:    1,
		Settings:   settings,
		ExportedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	output = append(output, line...)
	output = append(output, '\n')
	if len(prices) > 0 {
		line, err := json.Marshal(modelPricesExportRecord{
			RecordType: modelPricesExportRecordType,
			Version:    1,
			Prices:     prices,
			ExportedAt: time.Now().UnixMilli(),
		})
		if err != nil {
			return nil, err
		}
		output = append(output, line...)
		output = append(output, '\n')
	}
	if len(quotaEntries) > 0 {
		line, err := json.Marshal(quotaCacheExportRecord{
			RecordType: quotaCacheExportRecordType,
			Version:    1,
			Entries:    quotaEntries,
			ExportedAt: time.Now().UnixMilli(),
		})
		if err != nil {
			return nil, err
		}
		output = append(output, line...)
		output = append(output, '\n')
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		event.RawJSON = ""
		line, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		output = append(output, line...)
		output = append(output, '\n')
	}
	return output, nil
}

func defaultMonitoringSettings() MonitoringSettings {
	return MonitoringSettings{
		RetentionDays: 0,
		WebDAV: MonitoringWebDAVBackupConfig{
			Enabled:         false,
			IntervalMinutes: 1440,
		},
	}
}

func normalizeMonitoringSettings(settings MonitoringSettings) MonitoringSettings {
	if settings.RetentionDays < 0 {
		settings.RetentionDays = 0
	}
	if settings.WebDAV.IntervalMinutes <= 0 {
		settings.WebDAV.IntervalMinutes = 1440
	}
	if settings.WebDAV.RetentionDays < 0 {
		settings.WebDAV.RetentionDays = 0
	}
	settings.WebDAV.URL = strings.TrimSpace(settings.WebDAV.URL)
	settings.WebDAV.Username = strings.TrimSpace(settings.WebDAV.Username)
	return settings
}

func (s *Store) GetMonitoringSettings(ctx context.Context) (MonitoringSettings, error) {
	settings := defaultMonitoringSettings()
	var raw string
	if err := s.db.QueryRowContext(ctx, `select settings_json from monitoring_settings where id = 1`).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return defaultMonitoringSettings(), err
	}
	return normalizeMonitoringSettings(settings), nil
}

func (s *Store) SetMonitoringSettings(ctx context.Context, settings MonitoringSettings) error {
	settings = normalizeMonitoringSettings(settings)
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `insert into monitoring_settings(id, settings_json, updated_at_ms) values(1, ?, ?)
		on conflict(id) do update set settings_json = excluded.settings_json, updated_at_ms = excluded.updated_at_ms`, string(raw), time.Now().UnixMilli())
	return err
}

func (s *Store) DeleteEventsBefore(ctx context.Context, beforeMs int64) (int64, error) {
	if beforeMs <= 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var deleted UsageSummary
	if err := tx.QueryRowContext(ctx, `select
		count(*),
		coalesce(sum(case when failed = 0 then 1 else 0 end), 0),
		coalesce(sum(case when failed != 0 then 1 else 0 end), 0),
		coalesce(sum(total_tokens), 0)
		from usage_events
		where timestamp_ms < ?`, beforeMs).Scan(&deleted.TotalRequests, &deleted.SuccessCount, &deleted.FailureCount, &deleted.TotalTokens); err != nil {
		return 0, err
	}
	if deleted.TotalRequests == 0 {
		return 0, nil
	}

	result, err := tx.ExecContext(ctx, `delete from usage_events where timestamp_ms < ?`, beforeMs)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected > 0 {
		var latestID int64
		if err := tx.QueryRowContext(ctx, `select coalesce(max(id), 0) from usage_events`).Scan(&latestID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `update usage_summary set
			latest_event_id = ?,
			total_requests = max(total_requests - ?, 0),
			success_count = max(success_count - ?, 0),
			failure_count = max(failure_count - ?, 0),
			total_tokens = max(total_tokens - ?, 0),
			updated_at_ms = ?
			where id = 1`,
			latestID,
			deleted.TotalRequests,
			deleted.SuccessCount,
			deleted.FailureCount,
			deleted.TotalTokens,
			time.Now().UnixMilli(),
		); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		s.invalidateUsageSummaryCache()
	}
	return affected, nil
}

func (s *Store) ApplyRetention(ctx context.Context, now time.Time) (int64, error) {
	settings, err := s.GetMonitoringSettings(ctx)
	if err != nil {
		return 0, err
	}
	if settings.RetentionDays <= 0 {
		return 0, nil
	}
	return s.DeleteEventsBefore(ctx, now.AddDate(0, 0, -settings.RetentionDays).UnixMilli())
}

func (s *Store) GetQuotaCache(ctx context.Context, provider string, fileName string) ([]QuotaCacheEntry, error) {
	query := `select id, provider, file_name, data_json, cached_at_ms, accessed_at_ms, version from quota_cache`
	args := []any{}
	switch {
	case provider != "" && fileName != "":
		query += ` where provider = ? and file_name = ?`
		args = append(args, provider, fileName)
	case provider != "":
		query += ` where provider = ?`
		args = append(args, provider)
	case fileName != "":
		query += ` where file_name = ?`
		args = append(args, fileName)
	}
	query += " order by cached_at_ms desc"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]QuotaCacheEntry, 0)
	for rows.Next() {
		var entry QuotaCacheEntry
		var raw string
		if err := rows.Scan(&entry.ID, &entry.Provider, &entry.FileName, &raw, &entry.CachedAt, &entry.AccessedAt, &entry.Version); err != nil {
			return nil, err
		}
		entry.Data = json.RawMessage(raw)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if provider != "" || fileName != "" {
		_ = s.touchQuotaCache(ctx, provider, fileName, time.Now().UnixMilli())
	}
	return entries, nil
}

func (s *Store) touchQuotaCache(ctx context.Context, provider string, fileName string, accessedAt int64) error {
	switch {
	case provider != "" && fileName != "":
		_, err := s.db.ExecContext(ctx, `update quota_cache set accessed_at_ms = ? where provider = ? and file_name = ?`, accessedAt, provider, fileName)
		return err
	case provider != "":
		_, err := s.db.ExecContext(ctx, `update quota_cache set accessed_at_ms = ? where provider = ?`, accessedAt, provider)
		return err
	case fileName != "":
		_, err := s.db.ExecContext(ctx, `update quota_cache set accessed_at_ms = ? where file_name = ?`, accessedAt, fileName)
		return err
	default:
		return nil
	}
}

func (s *Store) SetQuotaCache(ctx context.Context, entry QuotaCacheEntry) error {
	if entry.Provider == "" || entry.FileName == "" || len(entry.Data) == 0 {
		return sql.ErrNoRows
	}
	if entry.ID == "" {
		entry.ID = entry.Provider + ":" + entry.FileName
	}
	now := time.Now().UnixMilli()
	if entry.CachedAt <= 0 {
		entry.CachedAt = now
	}
	if entry.AccessedAt <= 0 {
		entry.AccessedAt = now
	}
	if entry.Version <= 0 {
		entry.Version = 1
	}
	s.quotaCacheMu.Lock()
	defer s.quotaCacheMu.Unlock()

	return retrySQLiteBusy(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `insert into quota_cache(id, provider, file_name, data_json, cached_at_ms, accessed_at_ms, version)
			values(?, ?, ?, ?, ?, ?, ?)
			on conflict(id) do update set
				provider = excluded.provider,
				file_name = excluded.file_name,
				data_json = excluded.data_json,
				cached_at_ms = excluded.cached_at_ms,
				accessed_at_ms = excluded.accessed_at_ms,
				version = excluded.version`, entry.ID, entry.Provider, entry.FileName, string(entry.Data), entry.CachedAt, entry.AccessedAt, entry.Version)
		return err
	})
}

func retrySQLiteBusy(ctx context.Context, operation func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = operation()
		if !isSQLiteBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
	return err
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func (s *Store) DeleteQuotaCache(ctx context.Context, provider string, fileName string) error {
	switch {
	case provider == "" && fileName == "":
		_, err := s.db.ExecContext(ctx, `delete from quota_cache`)
		return err
	case provider != "" && fileName != "":
		_, err := s.db.ExecContext(ctx, `delete from quota_cache where provider = ? and file_name = ?`, provider, fileName)
		return err
	case provider != "":
		_, err := s.db.ExecContext(ctx, `delete from quota_cache where provider = ?`, provider)
		return err
	default:
		_, err := s.db.ExecContext(ctx, `delete from quota_cache where file_name = ?`, fileName)
		return err
	}
}

func (s *Store) QuotaCacheStats(ctx context.Context) (QuotaCacheStats, error) {
	var stats QuotaCacheStats
	err := s.db.QueryRowContext(ctx, `select count(*), coalesce(max(cached_at_ms), 0) from quota_cache`).Scan(&stats.TotalEntries, &stats.UpdatedAt)
	return stats, err
}

func (s *Store) GetModelPrices(ctx context.Context) (map[string]ModelPrice, error) {
	rows, err := s.db.QueryContext(ctx, `select model, prompt_price, completion_price, cache_price from model_prices order by model asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := make(map[string]ModelPrice)
	for rows.Next() {
		var model string
		var price ModelPrice
		if err := rows.Scan(&model, &price.Prompt, &price.Completion, &price.Cache); err != nil {
			return nil, err
		}
		prices[model] = price
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prices, nil
}

func (s *Store) SetModelPrices(ctx context.Context, prices map[string]ModelPrice) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `delete from model_prices`); err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	stmt, err := tx.PrepareContext(ctx, `insert into model_prices(model, prompt_price, completion_price, cache_price, updated_at_ms) values(?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for model, price := range prices {
		if model == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, model, price.Prompt, price.Completion, price.Cache, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
