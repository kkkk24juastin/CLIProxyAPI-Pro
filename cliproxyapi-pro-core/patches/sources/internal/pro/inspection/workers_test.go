package inspection

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunWorkersVisitsEachIndexOnce(t *testing.T) {
	visited := make(map[int]int)
	var mu sync.Mutex
	RunWorkers(25, 4, nil, func(index int) bool {
		mu.Lock()
		visited[index]++
		mu.Unlock()
		return true
	})
	if len(visited) != 25 {
		t.Fatalf("visited %d indexes", len(visited))
	}
	for index := 0; index < 25; index++ {
		if visited[index] != 1 {
			t.Fatalf("index %d visited %d times", index, visited[index])
		}
	}
}

func TestRunKeyedWorkersEnforcesBothLimitsWithoutHeadOfLineBlocking(t *testing.T) {
	keys := []string{
		"xai", "xai", "xai", "xai", "xai", "xai",
		"codex", "codex", "codex", "codex", "codex", "codex",
	}
	release := make(chan struct{})
	reachedExpectedConcurrency := make(chan struct{})
	done := make(chan struct{})
	active := 0
	maxActive := 0
	activeByKey := map[string]int{}
	maxByKey := map[string]int{}
	var once sync.Once
	var mu sync.Mutex

	go func() {
		RunKeyedWorkers(len(keys), 4, 2, func(index int) string {
			return keys[index]
		}, nil, func(index int) bool {
			mu.Lock()
			key := keys[index]
			active++
			activeByKey[key]++
			if active > maxActive {
				maxActive = active
			}
			if activeByKey[key] > maxByKey[key] {
				maxByKey[key] = activeByKey[key]
			}
			if active == 4 && activeByKey["xai"] == 2 && activeByKey["codex"] == 2 {
				once.Do(func() { close(reachedExpectedConcurrency) })
			}
			mu.Unlock()
			<-release
			mu.Lock()
			active--
			activeByKey[key]--
			mu.Unlock()
			return true
		})
		close(done)
	}()

	select {
	case <-reachedExpectedConcurrency:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("workers did not fill global capacity across provider groups")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keyed workers did not finish")
	}
	if maxActive != 4 || maxByKey["xai"] != 2 || maxByKey["codex"] != 2 {
		t.Fatalf("max concurrency = total:%d byKey:%v", maxActive, maxByKey)
	}
}

func TestKeyedLimiterSharesGlobalAndPerKeyCapacity(t *testing.T) {
	var limiter KeyedLimiter
	releaseXAI, err := limiter.Acquire(context.Background(), 2, 1, "xai")
	if err != nil {
		t.Fatalf("Acquire(xai) error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if _, err = limiter.Acquire(ctx, 2, 1, "xai"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire(blocked xai) error = %v, want deadline", err)
	}
	cancel()

	releaseCodex, err := limiter.Acquire(context.Background(), 2, 1, "codex")
	if err != nil {
		t.Fatalf("Acquire(codex) error = %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	if _, err = limiter.Acquire(ctx, 2, 1, "gemini-cli"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire(blocked global capacity) error = %v, want deadline", err)
	}
	cancel()

	releaseCodex()
	releaseSecondCodex, err := limiter.Acquire(context.Background(), 2, 1, "codex")
	if err != nil {
		t.Fatalf("Acquire(released codex) error = %v", err)
	}
	releaseSecondCodex()
	releaseXAI()
}

func TestKeyedLimiterRejectsCanceledContextWithFreeCapacity(t *testing.T) {
	var limiter KeyedLimiter
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := limiter.Acquire(ctx, 4, 2, "xai"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) error = %v, want canceled", err)
	}
}
