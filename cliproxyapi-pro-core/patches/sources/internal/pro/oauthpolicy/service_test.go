package oauthpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/oauthpolicy/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

type memorySettingsStore struct {
	mu    sync.Mutex
	items map[string]settings.Item
}

func (s *memorySettingsStore) Get(_ context.Context, namespace string) (settings.Item, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, found := s.items[namespace]
	return item, found, nil
}

func (s *memorySettingsStore) Put(_ context.Context, item settings.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item.Settings = append(json.RawMessage(nil), item.Settings...)
	s.items[item.Namespace] = item
	return nil
}

func (s *memorySettingsStore) Delete(_ context.Context, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, namespace)
	return nil
}

func (*memorySettingsStore) Subscribe(string, func(context.Context, settings.Item) error) func() {
	return func() {}
}

func TestNewMigratesLegacyNamespace(t *testing.T) {
	store := &memorySettingsStore{items: map[string]settings.Item{
		settings.LegacyNamespaceOAuthModelPolicy: {
			Namespace:     settings.LegacyNamespaceOAuthModelPolicy,
			SchemaVersion: 1,
			Settings:      json.RawMessage(`{"enabled":true,"providers":{"codex":{"plans":{"pro":{"excluded-models":[]}}}}}`),
		},
	}}
	service, err := New(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	service.Close()
	if _, found := store.items[settings.LegacyNamespaceOAuthModelPolicy]; found {
		t.Fatal("legacy namespace survived migration")
	}
	if _, found := store.items[settings.NamespaceOAuthPolicy]; !found {
		t.Fatal("new OAuth policy namespace was not written")
	}
}

func TestUpdateConfigDoesNotWaitForAccountRefresh(t *testing.T) {
	store := &memorySettingsStore{items: map[string]settings.Item{}}
	service, err := New(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	service.SetChangeHandler(func(ctx context.Context) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		close(finished)
	})
	cfg, err := modelconfig.Parse([]byte(`{"enabled":true,"providers":{"codex":{"plans":{"pro":{"priority":10}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan error, 1)
	go func() { returned <- service.UpdateConfig(context.Background(), cfg) }()
	select {
	case err = <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("UpdateConfig waited for account refresh")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("account refresh did not start")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("account refresh did not finish")
	}
	service.Close()
}

func TestUpdateConfigCoalescesConcurrentAccountRefreshes(t *testing.T) {
	store := &memorySettingsStore{items: map[string]settings.Item{}}
	service, err := New(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondFinished := make(chan struct{})
	var calls atomic.Int32
	service.SetChangeHandler(func(ctx context.Context) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
			}
		case 2:
			close(secondFinished)
		}
	})
	for priority := 1; priority <= 3; priority++ {
		raw := []byte(fmt.Sprintf(`{"enabled":true,"providers":{"codex":{"plans":{"pro":{"priority":%d}}}}}`, priority))
		cfg, errParse := modelconfig.Parse(raw)
		if errParse != nil {
			t.Fatal(errParse)
		}
		if err = service.UpdateConfig(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		if priority == 1 {
			select {
			case <-firstStarted:
			case <-time.After(time.Second):
				t.Fatal("first account refresh did not start")
			}
		}
	}
	close(releaseFirst)
	select {
	case <-secondFinished:
	case <-time.After(time.Second):
		t.Fatal("coalesced account refresh did not finish")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("account refresh calls = %d, want 2", got)
	}
}
