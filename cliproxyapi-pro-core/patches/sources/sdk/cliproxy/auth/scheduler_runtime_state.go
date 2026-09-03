package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

func loadRoutingCursorStates() map[string]string {
	persisted := make(map[string]string)
	states, err := embeddedusage.ListRoutingCursorStates(context.Background())
	if err != nil {
		return persisted
	}
	for _, state := range states {
		if state.CursorKey != "" && state.LastAuthID != "" {
			persisted[state.CursorKey] = state.LastAuthID
		}
	}
	return persisted
}

func (s *authScheduler) applyImportedRuntimeState(states []embeddedusage.RoutingCursorState, auths []*Auth) {
	if s == nil {
		return
	}
	persistedCursors := make(map[string]string, len(states))
	for _, state := range states {
		if state.CursorKey != "" && state.LastAuthID != "" {
			persistedCursors[state.CursorKey] = state.LastAuthID
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistedCursors = persistedCursors
	s.providers = make(map[string]*providerScheduler)
	s.authProviders = make(map[string]string)
	s.mixedCursors = make(map[string]int)
	s.mixedWeightedStates = make(map[string]*smoothWeightedState)
	s.mixedRestored = make(map[string]bool)
	now := time.Now()
	for _, auth := range auths {
		s.upsertAuthLocked(auth, now)
	}
}

func (s *authScheduler) restoreMixedCursor(
	cursorKey string,
	bestPriority int,
	startSlot int,
	candidateShards []*modelScheduler,
	weights []int,
	segmentStarts []int,
) (int, string) {
	persistedCursorKey := fmt.Sprintf("mixed|%s|%d", cursorKey, bestPriority)
	if s == nil || s.mixedRestored[cursorKey] {
		return startSlot, persistedCursorKey
	}
	s.mixedRestored[cursorKey] = true
	lastAuthID := s.persistedCursors[persistedCursorKey]
	if lastAuthID == "" {
		return startSlot, persistedCursorKey
	}
	for providerIndex, shard := range candidateShards {
		if shard == nil || weights[providerIndex] <= 0 {
			continue
		}
		if _, ok := shard.entries[lastAuthID]; ok {
			return segmentStarts[providerIndex], persistedCursorKey
		}
	}
	return startSlot, persistedCursorKey
}

func (s *authScheduler) persistMixedCursor(cursorKey, authID string) {
	if s == nil || cursorKey == "" || authID == "" {
		return
	}
	s.persistedCursors[cursorKey] = authID
	queueRoutingCursorState(cursorKey, authID)
}

func (m *modelScheduler) configurePersistedReadyBucket(bucket *readyBucket, priority int) {
	if m == nil || bucket == nil {
		return
	}
	prefix := fmt.Sprintf("single|%s|%s|%d", m.providerKey, m.modelKey, priority)
	configurePersistedReadyView(&bucket.all, prefix+"|all", m.persistedCursors)
	configurePersistedReadyView(&bucket.ws, prefix+"|ws", m.persistedCursors)
}

func configurePersistedReadyView(view *readyView, cursorKey string, persisted map[string]string) {
	if view == nil {
		return
	}
	view.cursorKey = cursorKey
	view.persisted = persisted
	view.restoreAfterAuthID(persisted[cursorKey])
}

func (v *readyView) restoreAfterAuthID(lastAuthID string) {
	lastAuthID = strings.TrimSpace(lastAuthID)
	if v == nil || lastAuthID == "" {
		return
	}
	v.lastPicked = lastAuthID
}

func (v *readyView) persistSelection(entry *scheduledAuth) {
	if v == nil || entry == nil || entry.auth == nil || v.cursorKey == "" {
		return
	}
	authID := entry.auth.ID
	if v.persisted != nil {
		v.persisted[v.cursorKey] = authID
	}
	queueRoutingCursorState(v.cursorKey, authID)
}

func queueRoutingCursorState(cursorKey, authID string) {
	embeddedusage.QueueRoutingCursorState(embeddedusage.RoutingCursorState{
		CursorKey:   cursorKey,
		LastAuthID:  authID,
		UpdatedAtMS: time.Now().UnixMilli(),
	})
}
