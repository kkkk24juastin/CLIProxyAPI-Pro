package inspection

import "sync"

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
