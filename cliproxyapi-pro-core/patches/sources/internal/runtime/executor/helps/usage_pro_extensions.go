package helps

import (
	"context"
	"fmt"

	internallogging "github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	apikeypolicy "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/apikeypolicy"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

func prepareUsageRecordForPublish(ctx context.Context, record *usage.Record) {
	if record == nil {
		return
	}
	record.ResponseHeaders = internallogging.GetResponseHeaders(ctx)
	if attemptIndex, ok := usage.AttemptIndexFromContext(ctx); ok {
		record.AttemptIndex = &attemptIndex
	}
	quotaDetail := usage.EnsureTokenBreakdownForProvider(record.Detail, record.Provider, record.ExecutorType)
	totalTokens := quotaDetail.TokenBreakdown.TotalTokens
	if totalTokens == 0 {
		totalTokens = quotaDetail.TotalTokens
	}
	attemptIndex := int64(-1)
	if record.AttemptIndex != nil {
		attemptIndex = *record.AttemptIndex
	}
	eventID := fmt.Sprintf("usage:%d:provider=%q:executor=%q:model=%q:alias=%q:attempt=%d", record.RequestedAt.UnixNano(), record.Provider, record.ExecutorType, record.Model, record.Alias, attemptIndex)
	if err := apikeypolicy.SettleQuotaUsage(ctx, eventID, apikeypolicy.QuotaUsageDelta{
		Provider: record.Provider, Model: record.Model,
		InputTokens: quotaDetail.InputTokens, OutputTokens: quotaDetail.OutputTokens,
		ReasoningTokens: quotaDetail.ReasoningTokens, CachedTokens: quotaDetail.CachedTokens,
		CacheReadTokens: quotaDetail.CacheReadTokens, CacheWriteTokens: quotaDetail.CacheCreationTokens,
		TotalTokens: totalTokens, ServiceTier: record.ServiceTier,
		EffectiveServiceTier: record.ResponseServiceTier, Speed: record.Speed,
		EffectiveSpeed: record.ResponseSpeed,
	}); err != nil {
		log.WithError(err).Error("failed to settle API key quota usage")
	}
}

func (b *StreamUsageBuffer) ObserveClaude(detail usage.Detail, ok bool) {
	if b == nil || !ok {
		return
	}
	preservedInput := b.detail.InputTokens
	preservedCacheRead := b.detail.CacheReadTokens
	preservedCacheCreation := b.detail.CacheCreationTokens
	b.Observe(detail, true)
	merged := false
	if detail.InputTokens == 0 && preservedInput != 0 {
		b.detail.InputTokens = preservedInput
		merged = true
	}
	if detail.CacheReadTokens == 0 {
		b.detail.CacheReadTokens = preservedCacheRead
		merged = merged || preservedCacheRead != 0
	}
	if detail.CacheCreationTokens == 0 {
		b.detail.CacheCreationTokens = preservedCacheCreation
		merged = merged || preservedCacheCreation != 0
	}
	if !merged {
		return
	}
	b.detail.CachedTokens = b.detail.CacheReadTokens
	if b.detail.CachedTokens == 0 {
		b.detail.CachedTokens = b.detail.CacheCreationTokens
	}
	b.detail.TotalTokens = b.detail.InputTokens + b.detail.OutputTokens + b.detail.CacheReadTokens + b.detail.CacheCreationTokens
	b.detail.TokenBreakdown = usage.NewIndependentTokenBreakdown(
		b.detail.InputTokens,
		b.detail.CacheReadTokens,
		b.detail.CacheCreationTokens,
		max(b.detail.OutputTokens-b.detail.ReasoningTokens, 0),
		b.detail.ReasoningTokens,
		b.detail.TotalTokens,
	)
}
