package observability

import (
	"context"
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

// SettingsStore adapts the observability-owned shared SQLite repository to the
// module-facing settings port. The dependency points from the infrastructure
// adapter to the port, so business modules never import observability or the
// historical embeddedusage façade.
type SettingsStore struct{}

func NewSettingsStore() SettingsStore { return SettingsStore{} }

func (SettingsStore) Get(ctx context.Context, namespace string) (settings.Item, bool, error) {
	stored, found, err := GetProSetting(ctx, namespace)
	if err != nil || !found {
		return settings.Item{}, found, err
	}
	return settingItem(stored), true, nil
}

func (SettingsStore) Put(ctx context.Context, item settings.Item) error {
	return SetProSetting(ctx, ProSetting{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	})
}

func (SettingsStore) Subscribe(namespace string, apply func(context.Context, settings.Item) error) func() {
	if apply == nil {
		return func() {}
	}
	return RegisterProSettingConsumer(namespace, func(ctx context.Context, item ProSetting) error {
		return apply(ctx, settingItem(item))
	})
}

func settingItem(item ProSetting) settings.Item {
	return settings.Item{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	}
}
