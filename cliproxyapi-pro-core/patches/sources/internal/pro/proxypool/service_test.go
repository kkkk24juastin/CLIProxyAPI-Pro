package proxypool

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	proxyconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/proxypool/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

type serviceTestStore struct {
	mu    sync.Mutex
	items map[string]settings.Item
}

func (s *serviceTestStore) Get(_ context.Context, namespace string) (settings.Item, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, found := s.items[namespace]
	return item, found, nil
}

func (s *serviceTestStore) Put(_ context.Context, item settings.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item.Settings = append(json.RawMessage(nil), item.Settings...)
	s.items[item.Namespace] = item
	return nil
}

func (s *serviceTestStore) Delete(_ context.Context, namespace string) error {
	s.mu.Lock()
	delete(s.items, namespace)
	s.mu.Unlock()
	return nil
}

func (*serviceTestStore) Subscribe(string, func(context.Context, settings.Item) error) func() {
	return func() {}
}

type coordinatedServiceTestStore struct {
	*serviceTestStore
	writeGate sync.RWMutex
	entered   chan struct{}
	once      sync.Once
}

func (s *coordinatedServiceTestStore) ExecuteWrite(
	ctx context.Context,
	operation func(context.Context, settings.Store) error,
) error {
	s.writeGate.RLock()
	defer s.writeGate.RUnlock()
	s.once.Do(func() { close(s.entered) })
	return operation(ctx, s.serviceTestStore)
}

func TestUpdateConfigAcquiresWriteCoordinatorBeforeServiceLock(t *testing.T) {
	store := &coordinatedServiceTestStore{
		serviceTestStore: &serviceTestStore{items: map[string]settings.Item{}},
		entered:          make(chan struct{}),
	}
	service, err := New(context.Background(), store, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	service.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- service.UpdateConfig(context.Background(), proxyconfig.Default()) }()
	select {
	case <-store.entered:
		// The backup barrier is owned before UpdateConfig waits for service.mu.
	case <-time.After(time.Second):
		service.mu.Unlock()
		t.Fatal("UpdateConfig tried to acquire service.mu before the write coordinator")
	}
	service.mu.Unlock()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDisabledServiceStatusUsesConfiguredEndpoint(t *testing.T) {
	store := &serviceTestStore{items: map[string]settings.Item{}}
	service, err := New(context.Background(), store, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	status := service.Status()
	if status.Ready || status.Listen != proxyconfig.DefaultListenAddress ||
		status.ProxyURL != "socks5://"+proxyconfig.DefaultListenAddress || status.Strategy != "round-robin" {
		t.Fatalf("Status() = %+v", status)
	}
}
