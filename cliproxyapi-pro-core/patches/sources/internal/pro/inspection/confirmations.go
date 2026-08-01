package inspection

import (
	"strings"
	"sync"
)

// ConfirmationCounter owns consecutive auto-action confirmation state.
type ConfirmationCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewConfirmationCounter() *ConfirmationCounter {
	return &ConfirmationCounter{counts: make(map[string]int)}
}

func (c *ConfirmationCounter) Confirm(key string, required int) (bool, int, int) {
	if required <= 1 {
		return true, 1, 1
	}
	key = strings.TrimSpace(key)
	if key == "" || c == nil {
		return true, 1, required
	}
	c.mu.Lock()
	if c.counts == nil {
		c.counts = make(map[string]int)
	}
	count := c.counts[key] + 1
	c.counts[key] = count
	c.mu.Unlock()
	return count >= required, count, required
}

func (c *ConfirmationCounter) ClearPrefix(prefix string) {
	if c == nil || strings.TrimSpace(prefix) == "" {
		return
	}
	c.mu.Lock()
	for key := range c.counts {
		if strings.HasPrefix(key, prefix) {
			delete(c.counts, key)
		}
	}
	c.mu.Unlock()
}
