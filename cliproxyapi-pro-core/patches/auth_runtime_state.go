package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/embeddedusage"
	prorouting "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/routing"
)

func authRuntimeIdentityFingerprint(auth *Auth) string {
	if auth == nil {
		return ""
	}
	parts := []string{
		strings.ToLower(strings.TrimSpace(auth.Provider)),
		strings.ToLower(filepath.Base(strings.TrimSpace(auth.FileName))),
	}
	for _, key := range []string{"email", "account_id", "accountId", "subject", "sub", "user_id", "userId"} {
		if auth.Metadata == nil {
			break
		}
		if value, ok := auth.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			parts = append(parts, strings.ToLower(strings.TrimSpace(value)))
		}
	}
	if len(parts) == 2 {
		parts = append(parts, strings.TrimSpace(auth.Index))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func restoreAuthRuntimeStats(auth *Auth) {
	if auth == nil {
		return
	}
	auth.EnsureIndex()
	stored, ok, err := embeddedusage.GetAuthRuntimeStats(context.Background(), auth.Index, auth.ID)
	if err != nil || !ok {
		return
	}
	applyStoredAuthRuntimeStats(auth, stored)
}

func cleanupLegacyQuotaCacheOnRegister(auth *Auth) {
	if auth == nil || IsPluginVirtualAuth(auth) || auth.Metadata == nil {
		return
	}
	delete(auth.Metadata, "quota_cache")
}

func applyAuthRuntimeStats(auth *Auth, stored embeddedusage.AuthRuntimeStats) bool {
	if auth == nil {
		return false
	}
	auth.Selected = stored.SelectedCount
	auth.Success = stored.SuccessCount
	auth.Failed = stored.FailureCount
	auth.recentRequests = recentRequestRing{}
	for _, item := range stored.RecentBuckets {
		if item.BucketID <= 0 {
			continue
		}
		index := recentRequestBucketIndex(item.BucketID)
		auth.recentRequests.buckets[index] = recentRequestBucket{
			bucketID: item.BucketID,
			success:  item.Success,
			failed:   item.Failed,
		}
	}
	return true
}

func applyStoredAuthRuntimeStats(auth *Auth, stored embeddedusage.AuthRuntimeStats) bool {
	if auth == nil {
		return false
	}
	fingerprint := authRuntimeIdentityFingerprint(auth)
	if stored.IdentityFingerprint != "" && fingerprint != "" && stored.IdentityFingerprint != fingerprint {
		return false
	}
	return applyAuthRuntimeStats(auth, stored)
}

func authRuntimeStatsSnapshot(auth *Auth, now time.Time) embeddedusage.AuthRuntimeStats {
	fileName := auth.FileName
	if auth.Attributes != nil {
		if source := strings.TrimSpace(auth.Attributes[AttributeVirtualSource]); source != "" {
			fileName = filepath.Base(source)
		}
	}
	stats := embeddedusage.AuthRuntimeStats{
		AuthIndex:           auth.Index,
		AuthID:              auth.ID,
		FileName:            fileName,
		IdentityFingerprint: authRuntimeIdentityFingerprint(auth),
		SelectedCount:       auth.Selected,
		SuccessCount:        auth.Success,
		FailureCount:        auth.Failed,
		UpdatedAtMS:         now.UnixMilli(),
	}
	for _, bucket := range auth.recentRequests.buckets {
		if bucket.bucketID <= 0 {
			continue
		}
		stats.RecentBuckets = append(stats.RecentBuckets, embeddedusage.RuntimeRequestBucket{
			BucketID: bucket.bucketID,
			Success:  bucket.success,
			Failed:   bucket.failed,
		})
	}
	return stats
}

func queueAuthRuntimeStats(auth *Auth, observedAt time.Time) {
	if auth == nil {
		return
	}
	embeddedusage.QueueAuthRuntimeStats(authRuntimeStatsSnapshot(auth, observedAt))
}

const legacyRoundRobinCursorPrefix = prorouting.LegacyRoundRobinCursorPrefix

func legacyRoundRobinCursorKey(provider, model string) string {
	return prorouting.LegacyRoundRobinCursorKey(provider, canonicalModelKey(model))
}

// restoreRoutingCursorLocked restores the legacy built-in round-robin selector.
// The caller must hold s.mu.
func (s *RoundRobinSelector) restoreRoutingCursorLocked(provider, model, selectorKey string) {
	if s == nil {
		return
	}
	if s.routingCursorRestored == nil {
		s.routingCursorRestored = make(map[string]bool)
	}
	if s.routingCursorRestored[selectorKey] {
		return
	}
	if s.persistedRoutingCursors == nil {
		s.persistedRoutingCursors = make(map[string]string)
	}
	stateKey := legacyRoundRobinCursorKey(provider, model)
	lastAuthID := s.persistedRoutingCursors[stateKey]
	if lastAuthID == "" {
		if state, ok, err := embeddedusage.GetRoutingCursorState(context.Background(), stateKey); err == nil && ok {
			lastAuthID = state.LastAuthID
			s.persistedRoutingCursors[stateKey] = lastAuthID
		}
	}
	s.lastPicked[selectorKey] = lastAuthID
	s.routingCursorRestored[selectorKey] = true
}

// persistRoutingCursorLocked records the legacy built-in round-robin selection.
// The caller must hold s.mu.
func (s *RoundRobinSelector) persistRoutingCursorLocked(provider, model string, picked *Auth) {
	if s == nil || picked == nil || strings.TrimSpace(picked.ID) == "" {
		return
	}
	if s.persistedRoutingCursors == nil {
		s.persistedRoutingCursors = make(map[string]string)
	}
	stateKey := legacyRoundRobinCursorKey(provider, model)
	s.persistedRoutingCursors[stateKey] = picked.ID
	embeddedusage.QueueRoutingCursorState(embeddedusage.RoutingCursorState{
		CursorKey: stateKey, LastAuthID: picked.ID, UpdatedAtMS: time.Now().UnixMilli(),
	})
}

func (s *RoundRobinSelector) applyImportedRoutingCursors(cursors []embeddedusage.RoutingCursorState) {
	if s == nil {
		return
	}
	persisted := make(map[string]string)
	for _, state := range cursors {
		if strings.HasPrefix(state.CursorKey, legacyRoundRobinCursorPrefix) && strings.TrimSpace(state.LastAuthID) != "" {
			persisted[state.CursorKey] = state.LastAuthID
		}
	}
	s.mu.Lock()
	s.lastPicked = make(map[string]string)
	s.routingCursorRestored = make(map[string]bool)
	s.persistedRoutingCursors = persisted
	s.mu.Unlock()
}

func (m *Manager) recordAuthSelected(authID string) {
	if m == nil || strings.TrimSpace(authID) == "" {
		return
	}
	var snapshot *Auth
	var observedAt time.Time
	m.mu.Lock()
	if auth := m.auths[authID]; auth != nil {
		auth.Selected++
		snapshot = auth.Clone()
		observedAt = time.Now()
	}
	m.mu.Unlock()
	queueAuthRuntimeStats(snapshot, observedAt)
}

// ApplyImportedRuntimeState applies authoritative imported cursor/stat state to the running manager.
// This makes usage JSONL restore visible immediately without requiring another process restart.
func (m *Manager) ApplyImportedRuntimeState(cursors []embeddedusage.RoutingCursorState, stats []embeddedusage.AuthRuntimeStats) error {
	if m == nil {
		return nil
	}
	statsByIndex := make(map[string]embeddedusage.AuthRuntimeStats, len(stats))
	statsByID := make(map[string]embeddedusage.AuthRuntimeStats, len(stats))
	for _, item := range stats {
		if index := strings.TrimSpace(item.AuthIndex); index != "" {
			statsByIndex[index] = item
		}
		if authID := strings.TrimSpace(item.AuthID); authID != "" {
			statsByID[authID] = item
		}
	}

	snapshots := make([]*Auth, 0)
	var selector Selector
	m.mu.Lock()
	for _, auth := range m.auths {
		if auth == nil {
			continue
		}
		item, matchedByIndex := statsByIndex[strings.TrimSpace(auth.Index)]
		if matchedByIndex {
			// An explicit backup import is authoritative when it targets the same
			// stable auth index. Provider identity metadata can legitimately gain,
			// lose, or rotate fields across a restart, so requiring the historical
			// fingerprint here would silently leave the live account statistics at
			// zero even though the imported database row was accepted.
			applyAuthRuntimeStats(auth, item)
		} else {
			var ok bool
			item, ok = statsByID[strings.TrimSpace(auth.ID)]
			if ok {
				// ID-only fallback is less stable and keeps the fingerprint guard to
				// prevent statistics from being attached to a replaced credential.
				applyStoredAuthRuntimeStats(auth, item)
			} else {
				// Runtime-state application is authoritative. Accounts omitted from
				// the supplied snapshot must not retain counters from the previous
				// dataset; reset totals and recent buckets while preserving identity,
				// availability, and routing cursor state.
				applyAuthRuntimeStats(auth, embeddedusage.AuthRuntimeStats{})
			}
		}
		snapshots = append(snapshots, auth.Clone())
	}
	selector = m.selector
	m.mu.Unlock()

	if roundRobin, ok := selector.(*RoundRobinSelector); ok {
		roundRobin.applyImportedRoutingCursors(cursors)
	}
	if m.scheduler != nil {
		m.scheduler.applyImportedRuntimeState(cursors, snapshots)
	}
	return nil
}
