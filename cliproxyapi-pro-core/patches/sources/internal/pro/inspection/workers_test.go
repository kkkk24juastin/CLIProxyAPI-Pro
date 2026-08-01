package inspection

import (
	"sync"
	"testing"
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
