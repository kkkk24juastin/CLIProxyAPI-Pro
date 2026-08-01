package settings

import (
	"context"
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/embeddedusage"
)

const (
	SchemaVersionOne = 1

	NamespaceRoutingRequestProtection = embeddedusage.ProSettingNamespaceRoutingRequestProtection
	NamespaceProxyPool               = embeddedusage.ProSettingNamespaceProxyPool
	NamespaceOAuthModelPolicy        = embeddedusage.ProSettingNamespaceOAuthModelPolicy
)

// Item is the module-facing representation of one versioned Pro setting.
// Keeping it outside embeddedusage prevents business modules from depending
// on the current SQLite implementation.
type Item struct {
	Namespace     string
	SchemaVersion int
	Settings      json.RawMessage
	UpdatedAtMS   int64
}

// Store is the persistence port consumed by static Pro business modules.
type Store interface {
	Get(context.Context, string) (Item, bool, error)
	Put(context.Context, Item) error
	Subscribe(string, func(context.Context, Item) error) func()
}

// EmbeddedStore adapts the existing embeddedusage implementation while the
// shared SQLite repositories are moved behind module-owned ports.
type EmbeddedStore struct{}

func NewEmbeddedStore() EmbeddedStore { return EmbeddedStore{} }

func (EmbeddedStore) Get(ctx context.Context, namespace string) (Item, bool, error) {
	stored, found, err := embeddedusage.GetProSetting(ctx, namespace)
	if err != nil || !found {
		return Item{}, found, err
	}
	return fromEmbedded(stored), true, nil
}

func (EmbeddedStore) Put(ctx context.Context, item Item) error {
	return embeddedusage.SetProSetting(ctx, embeddedusage.ProSetting{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	})
}

func (EmbeddedStore) Subscribe(namespace string, apply func(context.Context, Item) error) func() {
	if apply == nil {
		return func() {}
	}
	return embeddedusage.RegisterProSettingConsumer(namespace, func(ctx context.Context, item embeddedusage.ProSetting) error {
		return apply(ctx, fromEmbedded(item))
	})
}

func fromEmbedded(item embeddedusage.ProSetting) Item {
	return Item{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	}
}
