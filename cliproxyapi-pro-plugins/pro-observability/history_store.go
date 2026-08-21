package main

import (
	"context"
	"strings"
)

func appendUsageEventQueryFilters(options usageEventQueryOptions, includeCursor bool) ([]string, []any) {
	wheres := make([]string, 0, 10)
	args := make([]any, 0, 12)
	if options.SnapshotMaxID > 0 {
		wheres = append(wheres, `id <= ?`)
		args = append(args, options.SnapshotMaxID)
	}
	if includeCursor && options.BeforeTimestamp > 0 && options.BeforeID > 0 {
		wheres = append(wheres, `(timestamp_ms < ? or (timestamp_ms = ? and id < ?))`)
		args = append(args, options.BeforeTimestamp, options.BeforeTimestamp, options.BeforeID)
	}
	if options.FromMS > 0 {
		wheres = append(wheres, `timestamp_ms >= ?`)
		args = append(args, options.FromMS)
	}
	if options.ToMS > 0 {
		wheres = append(wheres, `timestamp_ms <= ?`)
		args = append(args, options.ToMS)
	}
	if value := strings.TrimSpace(options.Provider); value != "" {
		wheres = append(wheres, `provider = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(options.Model); value != "" {
		wheres = append(wheres, `model = ?`)
		args = append(args, value)
	}
	if values := splitUsageEventFilterValues(options.AuthIndex, 100); len(values) > 0 {
		wheres = append(wheres, `auth_index in (`+strings.TrimRight(strings.Repeat("?,", len(values)), ",")+`)`)
		for _, value := range values {
			args = append(args, value)
		}
	}
	if value := strings.TrimSpace(options.APIKeyHash); value != "" {
		wheres = append(wheres, `api_key_hash = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(options.APIKeyPolicyID); value != "" {
		wheres = append(wheres, `api_key_policy_id = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(options.ProfileID); value != "" {
		wheres = append(wheres, `profile_id = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(options.PolicyMode); value != "" {
		if strings.EqualFold(value, "unknown") {
			wheres = append(wheres, `coalesce(policy_mode, '') = ''`)
		} else {
			wheres = append(wheres, `policy_mode = ?`)
			args = append(args, value)
		}
	}
	if options.Failed != nil {
		failed := 0
		if *options.Failed {
			failed = 1
		}
		wheres = append(wheres, `failed = ?`)
		args = append(args, failed)
	}
	searchPredicates := make([]string, 0, 2)
	searchArgs := make([]any, 0, 101)
	if value := strings.ToLower(strings.TrimSpace(options.Search)); value != "" {
		searchRunes := []rune(value)
		if len(searchRunes) > 200 {
			value = string(searchRunes[:200])
		}
		searchPredicates = append(searchPredicates, `instr(lower(
			coalesce(request_id, '') || char(10) || coalesce(provider, '') || char(10) ||
			coalesce(executor_type, '') || char(10) || coalesce(model, '') || char(10) ||
			coalesce(alias, '') || char(10) || coalesce(endpoint, '') || char(10) ||
			coalesce(method, '') || char(10) || coalesce(path, '') || char(10) ||
			coalesce(auth_type, '') || char(10) || coalesce(auth_index, '') || char(10) ||
			coalesce(source, '') || char(10) || coalesce(source_hash, '') || char(10) ||
			coalesce(api_key_hash, '') || char(10) || coalesce(api_key_policy_id, '') || char(10) ||
			coalesce(profile_id, '') || char(10) || coalesce(profile_name_snapshot, '') || char(10) ||
			coalesce(policy_mode, '') || char(10) || coalesce(requested_model, '') || char(10) ||
			coalesce(effective_model, '') || char(10) || coalesce(client_ip, '') || char(10) ||
			coalesce(x_forwarded_for, '') || char(10) || coalesce(user_agent, '') || char(10) ||
			coalesce(error_code, '') || char(10) ||
			coalesce(error_message, '') || char(10) || coalesce(upstream_request_id, '')
		), ?) > 0`)
		searchArgs = append(searchArgs, value)
	}
	if values := splitUsageEventFilterValues(options.SearchAuthIndexes, 100); len(values) > 0 {
		searchPredicates = append(searchPredicates, `auth_index in (`+strings.TrimRight(strings.Repeat("?,", len(values)), ",")+`)`)
		for _, value := range values {
			searchArgs = append(searchArgs, value)
		}
	}
	if len(searchPredicates) > 0 {
		wheres = append(wheres, `(`+strings.Join(searchPredicates, ` or `)+`)`)
		args = append(args, searchArgs...)
	}
	return wheres, args
}

func splitUsageEventFilterValues(value string, limit int) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func usageEventQueryWhere(wheres []string) string {
	if len(wheres) == 0 {
		return ""
	}
	return ` where ` + strings.Join(wheres, ` and `)
}

func (store *usageStore) queryHistoryEvents(ctx context.Context, options usageEventQueryOptions) (usageEventQueryPage, error) {
	limit := boundedUsageEventLimit(options.Limit)
	matchedTotal := options.MatchedTotal
	if !options.SkipCount {
		countWheres, countArgs := appendUsageEventQueryFilters(options, false)
		if err := store.db.QueryRowContext(ctx, `select count(*) from usage_events`+usageEventQueryWhere(countWheres), countArgs...).Scan(&matchedTotal); err != nil {
			return usageEventQueryPage{}, err
		}
	}

	queryWheres, queryArgs := appendUsageEventQueryFilters(options, true)
	queryArgs = append(queryArgs, limit+1)
	events, err := store.queryEvents(ctx, usageEventQueryWhere(queryWheres)+` order by timestamp_ms desc, id desc limit ?`, queryArgs...)
	if err != nil {
		return usageEventQueryPage{}, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return usageEventQueryPage{Events: events, MatchedTotal: matchedTotal, HasMore: hasMore}, nil
}
