package state

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Persister is the storage boundary required by Writer. Keeping this contract
// in the state module lets routing publish runtime mutations without depending
// on the embedded usage compatibility package.
type Persister interface {
	SetRoutingCursorState(context.Context, RoutingCursor) error
	SetAuthRuntimeStats(context.Context, AuthRuntimeStats) error
	DeleteAuthRuntimeState(context.Context, string, string, string) error
}

type mutation struct {
	cursor *RoutingCursor
	stats  *AuthRuntimeStats
	delete *deletion
	flush  chan error
}

type deletion struct {
	authID    string
	authIndex string
	fileName  string
	updatedAt int64
	done      chan error
}

// Writer serializes and coalesces high-frequency routing state writes. Queue
// overflow is merged by identity and timestamp, so selection paths never block
// on SQLite while backup imports can still establish a strict flush barrier.
type Writer struct {
	store Persister

	lifecycleMu sync.RWMutex
	cancel      context.CancelFunc
	done        chan struct{}
	queue       chan mutation

	overflowMu      sync.Mutex
	overflowCursors map[string]RoutingCursor
	overflowStats   map[string]AuthRuntimeStats
}

func NewWriter(store Persister) *Writer {
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Writer{
		store:           store,
		cancel:          cancel,
		done:            make(chan struct{}),
		queue:           make(chan mutation, 1024),
		overflowCursors: make(map[string]RoutingCursor),
		overflowStats:   make(map[string]AuthRuntimeStats),
	}
	go w.run(ctx)
	return w
}

func (w *Writer) QueueRoutingCursor(item RoutingCursor) {
	if w == nil {
		return
	}
	item.CursorKey = strings.TrimSpace(item.CursorKey)
	item.LastAuthID = strings.TrimSpace(item.LastAuthID)
	if item.CursorKey == "" || item.LastAuthID == "" {
		return
	}
	if item.UpdatedAtMS <= 0 {
		item.UpdatedAtMS = time.Now().UnixMilli()
	}
	w.enqueue(mutation{cursor: &item})
}

func (w *Writer) QueueAuthRuntimeStats(item AuthRuntimeStats) {
	if w == nil || item.AuthIndex == "" || item.AuthID == "" {
		return
	}
	if item.UpdatedAtMS <= 0 {
		item.UpdatedAtMS = time.Now().UnixMilli()
	}
	w.enqueue(mutation{stats: &item})
}

func (w *Writer) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.RLock()
	defer w.lifecycleMu.RUnlock()
	if w.queue == nil {
		return nil
	}
	done := make(chan error, 1)
	select {
	case w.queue <- mutation{flush: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Writer) Delete(ctx context.Context, authID, authIndex, fileName string) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.RLock()
	defer w.lifecycleMu.RUnlock()
	if w.queue == nil {
		return nil
	}
	request := &deletion{
		authID: authID, authIndex: authIndex, fileName: fileName,
		updatedAt: time.Now().UnixMilli(), done: make(chan error, 1),
	}
	select {
	case w.queue <- mutation{delete: request}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Writer) Close() {
	if w == nil {
		return
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if w.queue == nil {
		return
	}
	w.cancel()
	<-w.done
	w.queue = nil
	w.cancel = nil
	w.done = nil
	w.resetOverflow()
}

func (w *Writer) enqueue(item mutation) {
	w.lifecycleMu.RLock()
	defer w.lifecycleMu.RUnlock()
	if w.queue == nil {
		return
	}
	select {
	case w.queue <- item:
	default:
		w.mergeOverflow(item)
	}
}

func (w *Writer) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	cursors := make(map[string]RoutingCursor)
	stats := make(map[string]AuthRuntimeStats)
	deletedAt := make(map[string]int64)

	merge := func(item mutation) {
		if item.cursor != nil {
			current, ok := cursors[item.cursor.CursorKey]
			if !ok || item.cursor.UpdatedAtMS >= current.UpdatedAtMS {
				cursors[item.cursor.CursorKey] = *item.cursor
			}
		}
		if item.stats != nil {
			if deleted := deletedAt[item.stats.AuthIndex]; deleted > 0 {
				if item.stats.UpdatedAtMS <= deleted {
					return
				}
				delete(deletedAt, item.stats.AuthIndex)
			}
			current, ok := stats[item.stats.AuthIndex]
			if !ok || item.stats.UpdatedAtMS >= current.UpdatedAtMS {
				stats[item.stats.AuthIndex] = *item.stats
			}
		}
	}
	drainOverflow := func() {
		for _, item := range w.takeOverflow() {
			merge(item)
		}
	}
	flush := func() error {
		var firstErr error
		for key, item := range cursors {
			if err := w.store.SetRoutingCursorState(context.Background(), item); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			delete(cursors, key)
		}
		for key, item := range stats {
			if err := w.store.SetAuthRuntimeStats(context.Background(), item); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			delete(stats, key)
		}
		return firstErr
	}
	process := func(item mutation) {
		if item.flush != nil {
			drainOverflow()
			item.flush <- flush()
			close(item.flush)
			return
		}
		if item.delete == nil {
			merge(item)
			return
		}
		drainOverflow()
		flushErr := flush()
		deleteErr := w.store.DeleteAuthRuntimeState(context.Background(), item.delete.authID, item.delete.authIndex, item.delete.fileName)
		if deleteErr == nil {
			for key, value := range stats {
				if matchesDelete(value, item.delete) {
					delete(stats, key)
				}
			}
			if item.delete.authIndex != "" {
				deletedAt[item.delete.authIndex] = item.delete.updatedAt
			}
		}
		err := deleteErr
		if err == nil {
			err = flushErr
		}
		item.delete.done <- err
		close(item.delete.done)
	}
	flushBeforeStop := func() {
		for attempt := 0; attempt < 5; attempt++ {
			if err := flush(); err == nil {
				return
			}
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case item := <-w.queue:
					process(item)
				default:
					drainOverflow()
					flushBeforeStop()
					return
				}
			}
		case item := <-w.queue:
			process(item)
		case <-ticker.C:
			drainOverflow()
			_ = flush()
		}
	}
}

func (w *Writer) mergeOverflow(item mutation) {
	w.overflowMu.Lock()
	defer w.overflowMu.Unlock()
	if item.cursor != nil {
		current, ok := w.overflowCursors[item.cursor.CursorKey]
		if !ok || item.cursor.UpdatedAtMS >= current.UpdatedAtMS {
			w.overflowCursors[item.cursor.CursorKey] = *item.cursor
		}
	}
	if item.stats != nil {
		current, ok := w.overflowStats[item.stats.AuthIndex]
		if !ok || item.stats.UpdatedAtMS >= current.UpdatedAtMS {
			w.overflowStats[item.stats.AuthIndex] = *item.stats
		}
	}
}

func (w *Writer) takeOverflow() []mutation {
	w.overflowMu.Lock()
	defer w.overflowMu.Unlock()
	items := make([]mutation, 0, len(w.overflowCursors)+len(w.overflowStats))
	for _, cursor := range w.overflowCursors {
		cursor := cursor
		items = append(items, mutation{cursor: &cursor})
	}
	for _, stats := range w.overflowStats {
		stats := stats
		items = append(items, mutation{stats: &stats})
	}
	w.overflowCursors = make(map[string]RoutingCursor)
	w.overflowStats = make(map[string]AuthRuntimeStats)
	return items
}

func (w *Writer) resetOverflow() {
	w.overflowMu.Lock()
	w.overflowCursors = make(map[string]RoutingCursor)
	w.overflowStats = make(map[string]AuthRuntimeStats)
	w.overflowMu.Unlock()
}

func matchesDelete(item AuthRuntimeStats, request *deletion) bool {
	if request == nil {
		return false
	}
	return (request.authIndex != "" && item.AuthIndex == request.authIndex) ||
		(request.authID != "" && item.AuthID == request.authID) ||
		(request.fileName != "" && item.FileName == request.fileName)
}
