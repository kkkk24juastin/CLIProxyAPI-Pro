package usage

import (
	"context"
	"reflect"
	"strings"
	"sync"
)

type attemptTrackerContextKey struct{}
type attemptIndexContextKey struct{}
type skipMonitoringContextKey struct{}

type attemptTracker struct {
	mu   sync.Mutex
	next int64
}

// WithAttemptTracking attaches one request-scoped upstream-attempt counter.
func WithAttemptTracking(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracker, ok := ctx.Value(attemptTrackerContextKey{}).(*attemptTracker); ok && tracker != nil {
		return ctx
	}
	return context.WithValue(ctx, attemptTrackerContextKey{}, &attemptTracker{})
}

// NextAttemptContext allocates the next zero-based upstream-attempt index.
func NextAttemptContext(ctx context.Context) context.Context {
	ctx = WithAttemptTracking(ctx)
	tracker, _ := ctx.Value(attemptTrackerContextKey{}).(*attemptTracker)
	tracker.mu.Lock()
	index := tracker.next
	tracker.next++
	tracker.mu.Unlock()
	return context.WithValue(ctx, attemptIndexContextKey{}, index)
}

// AttemptIndexFromContext returns the current upstream-attempt index when instrumented.
func AttemptIndexFromContext(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	index, ok := ctx.Value(attemptIndexContextKey{}).(int64)
	return index, ok
}

// WithSkipMonitoring marks an internal diagnostic request whose usage must
// not be persisted by the request-monitoring sink. Other usage consumers still
// receive the record so this flag does not change executor or auth semantics.
func WithSkipMonitoring(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, skipMonitoringContextKey{}, true)
}

// SkipMonitoringFromContext reports whether request-monitoring persistence
// should ignore usage emitted from ctx.
func SkipMonitoringFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	skip, _ := ctx.Value(skipMonitoringContextKey{}).(bool)
	return skip
}

// UnregisterNamed removes a named plugin only when the current registration
// still belongs to the supplied plugin. Passing nil removes it unconditionally.
func (m *Manager) UnregisterNamed(name string, plugin Plugin) {
	if m == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	m.pluginsMu.Lock()
	defer m.pluginsMu.Unlock()
	index, exists := m.named[name]
	if !exists || index < 0 || index >= len(m.plugins) {
		return
	}
	current := m.plugins[index]
	if plugin != nil && !samePlugin(current, plugin) {
		owners := removeNamedOwner(m.namedOwners[name], plugin)
		if len(owners) == 0 {
			delete(m.namedOwners, name)
		} else {
			m.namedOwners[name] = owners
		}
		return
	}
	if plugin == nil {
		m.plugins[index] = nil
		delete(m.named, name)
		delete(m.namedOwners, name)
		return
	}
	owners := m.namedOwners[name]
	if count := len(owners); count > 0 {
		m.plugins[index] = owners[count-1]
		owners = owners[:count-1]
		if len(owners) == 0 {
			delete(m.namedOwners, name)
		} else {
			m.namedOwners[name] = owners
		}
		return
	}
	m.plugins[index] = nil
	delete(m.named, name)
}

func removeNamedOwner(owners []Plugin, plugin Plugin) []Plugin {
	for index := len(owners) - 1; index >= 0; index-- {
		if samePlugin(owners[index], plugin) {
			return append(owners[:index], owners[index+1:]...)
		}
	}
	return owners
}

func samePlugin(left, right Plugin) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	return leftValue.Type() == rightValue.Type() &&
		leftValue.Type().Comparable() &&
		leftValue.Interface() == rightValue.Interface()
}

// UnregisterNamedPlugin removes a matching named plugin from the default manager.
func UnregisterNamedPlugin(name string, plugin Plugin) {
	DefaultManager().UnregisterNamed(name, plugin)
}
