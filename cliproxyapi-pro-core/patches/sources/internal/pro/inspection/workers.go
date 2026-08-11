package inspection

import (
	"context"
	"sync"
)

// KeyedLimiter is a zero-value-ready shared concurrency gate. Each acquisition
// is constrained by both a caller-supplied global limit and a per-key limit.
// Callers may update limits between acquisitions; already running work is not
// interrupted when a limit is lowered.
type KeyedLimiter struct {
	mu        sync.Mutex
	active    int
	activeKey map[string]int
	changed   chan struct{}
}

func (l *KeyedLimiter) Acquire(ctx context.Context, workers, keyWorkers int, key string) (func(), error) {
	if l == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workers <= 0 {
		workers = 1
	}
	if keyWorkers <= 0 {
		keyWorkers = 1
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		l.mu.Lock()
		if l.activeKey == nil {
			l.activeKey = make(map[string]int)
		}
		if l.changed == nil {
			l.changed = make(chan struct{})
		}
		if l.active < workers && l.activeKey[key] < keyWorkers {
			l.active++
			l.activeKey[key]++
			l.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					l.mu.Lock()
					if l.active > 0 {
						l.active--
					}
					if l.activeKey[key] <= 1 {
						delete(l.activeKey, key)
					} else {
						l.activeKey[key]--
					}
					close(l.changed)
					l.changed = make(chan struct{})
					l.mu.Unlock()
				})
			}, nil
		}
		changed := l.changed
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// RunWorkers executes indexed work with a bounded worker count. beforeNext is
// the cooperative scheduler gate used for pause/stop handling.
func RunWorkers(total, workers int, beforeNext func() bool, run func(int) bool) {
	if total <= 0 || run == nil {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	cursor := 0
	var cursorMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < workers && i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if beforeNext != nil && !beforeNext() {
					return
				}
				cursorMu.Lock()
				index := cursor
				cursor++
				cursorMu.Unlock()
				if index >= total || !run(index) {
					return
				}
			}
		}()
	}
	wg.Wait()
}

// RunKeyedWorkers executes work with both a global worker limit and a per-key
// worker limit. Key-specific consumers acquire the global slot only when they
// have an item ready, so one saturated key cannot block unrelated keys.
func RunKeyedWorkers(total, workers, keyWorkers int, key func(int) string, beforeNext func() bool, run func(int) bool) {
	if total <= 0 || key == nil || run == nil {
		return
	}
	if workers <= 0 {
		workers = 1
	}
	if keyWorkers <= 0 {
		keyWorkers = 1
	}
	groups := make(map[string][]int)
	order := make([]string, 0)
	for index := 0; index < total; index++ {
		groupKey := key(index)
		if _, ok := groups[groupKey]; !ok {
			order = append(order, groupKey)
		}
		groups[groupKey] = append(groups[groupKey], index)
	}

	global := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, groupKey := range order {
		indices := groups[groupKey]
		cursor := 0
		var cursorMu sync.Mutex
		for worker := 0; worker < keyWorkers && worker < len(indices); worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					cursorMu.Lock()
					if cursor >= len(indices) {
						cursorMu.Unlock()
						return
					}
					index := indices[cursor]
					cursor++
					cursorMu.Unlock()

					global <- struct{}{}
					if beforeNext != nil && !beforeNext() {
						<-global
						return
					}
					keepRunning := run(index)
					<-global
					if !keepRunning {
						return
					}
				}
			}()
		}
	}
	wg.Wait()
}
