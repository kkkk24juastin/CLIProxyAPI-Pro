// Package observability owns lifecycle boundaries shared by usage ingestion,
// retention, pricing synchronization, streaming and backup jobs.
package observability

import (
	"context"
	"sync"
	"time"
)

type Module struct {
	mu      sync.Mutex
	paused  bool
	active  int
	resumed chan struct{}
}

func New() *Module { return &Module{} }

func (m *Module) Begin(ctx context.Context) (func(), error) {
	if m == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		m.mu.Lock()
		if !m.paused {
			m.active++
			m.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					m.mu.Lock()
					if m.active > 0 {
						m.active--
					}
					m.mu.Unlock()
				})
			}, nil
		}
		resumed := m.resumed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-resumed:
		}
	}
}

func (m *Module) Run(ctx context.Context, operation func() error) error {
	release, err := m.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	if operation == nil {
		return nil
	}
	return operation()
}

func (m *Module) Pause(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	startedPause := false
	if !m.paused {
		m.paused = true
		m.resumed = make(chan struct{})
		startedPause = true
	}
	m.mu.Unlock()
	for {
		m.mu.Lock()
		active := m.active
		m.mu.Unlock()
		if active == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			if startedPause {
				_ = m.Resume(context.Background())
			}
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (m *Module) Resume(context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.paused {
		m.paused = false
		close(m.resumed)
		m.resumed = nil
	}
	m.mu.Unlock()
	return nil
}
