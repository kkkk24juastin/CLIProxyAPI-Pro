package state

import (
	"context"
	"sync"
	"testing"
)

type writerTestStore struct {
	mu      sync.Mutex
	cursors map[string]RoutingCursor
	stats   map[string]AuthRuntimeStats
	deletes int
}

func newWriterTestStore() *writerTestStore {
	return &writerTestStore{
		cursors: make(map[string]RoutingCursor),
		stats:   make(map[string]AuthRuntimeStats),
	}
}

func (s *writerTestStore) SetRoutingCursorState(_ context.Context, item RoutingCursor) error {
	s.mu.Lock()
	s.cursors[item.CursorKey] = item
	s.mu.Unlock()
	return nil
}

func (s *writerTestStore) SetAuthRuntimeStats(_ context.Context, item AuthRuntimeStats) error {
	s.mu.Lock()
	s.stats[item.AuthIndex] = item
	s.mu.Unlock()
	return nil
}

func (s *writerTestStore) DeleteAuthRuntimeState(_ context.Context, authID, authIndex, fileName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, item := range s.stats {
		if (authIndex != "" && item.AuthIndex == authIndex) ||
			(authID != "" && item.AuthID == authID) ||
			(fileName != "" && item.FileName == fileName) {
			delete(s.stats, key)
		}
	}
	s.deletes++
	return nil
}

func TestWriterCoalescesLatestStateAtFlushBarrier(t *testing.T) {
	store := newWriterTestStore()
	writer := NewWriter(store)
	defer writer.Close()

	writer.QueueRoutingCursor(RoutingCursor{CursorKey: "provider:model", LastAuthID: "old", UpdatedAtMS: 10})
	writer.QueueRoutingCursor(RoutingCursor{CursorKey: "provider:model", LastAuthID: "new", UpdatedAtMS: 20})
	writer.QueueAuthRuntimeStats(AuthRuntimeStats{AuthIndex: "auth-1", AuthID: "id-1", SuccessCount: 1, UpdatedAtMS: 10})
	writer.QueueAuthRuntimeStats(AuthRuntimeStats{AuthIndex: "auth-1", AuthID: "id-1", SuccessCount: 2, UpdatedAtMS: 20})
	if err := writer.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if got := store.cursors["provider:model"].LastAuthID; got != "new" {
		t.Fatalf("cursor LastAuthID = %q, want new", got)
	}
	if got := store.stats["auth-1"].SuccessCount; got != 2 {
		t.Fatalf("stats SuccessCount = %d, want 2", got)
	}
}

func TestWriterDeleteFlushesPendingStateBeforeDelete(t *testing.T) {
	store := newWriterTestStore()
	writer := NewWriter(store)
	defer writer.Close()

	writer.QueueAuthRuntimeStats(AuthRuntimeStats{AuthIndex: "auth-1", AuthID: "id-1", FileName: "one.json", UpdatedAtMS: 10})
	if err := writer.Delete(context.Background(), "", "auth-1", ""); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.stats["auth-1"]; ok {
		t.Fatal("deleted stats were reinserted")
	}
	if store.deletes != 1 {
		t.Fatalf("delete calls = %d, want 1", store.deletes)
	}
}

func TestWriterCloseDrainsPendingState(t *testing.T) {
	store := newWriterTestStore()
	writer := NewWriter(store)
	writer.QueueRoutingCursor(RoutingCursor{CursorKey: "provider:model", LastAuthID: "auth-1", UpdatedAtMS: 10})
	writer.Close()

	store.mu.Lock()
	defer store.mu.Unlock()
	if got := store.cursors["provider:model"].LastAuthID; got != "auth-1" {
		t.Fatalf("cursor LastAuthID = %q, want auth-1", got)
	}
}
