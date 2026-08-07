package oauthpolicy

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

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
