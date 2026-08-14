package observability

import (
	"context"
	"encoding/json"
	"strings"

	probackup "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/backup"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pro/settings"
)

// SettingsStore adapts the observability-owned shared SQLite repository to the
// module-facing settings port. The dependency points from the infrastructure
// adapter to the port, so business modules never import observability or the
// historical embeddedusage façade.
type SettingsStore struct{}

func NewSettingsStore() SettingsStore { return SettingsStore{} }

type uncoordinatedSettingsStore struct{ SettingsStore }

func (uncoordinatedSettingsStore) Put(ctx context.Context, item settings.Item) error {
	return setProSetting(ctx, ProSetting{
		Namespace: item.Namespace, SchemaVersion: item.SchemaVersion,
		Settings: append(json.RawMessage(nil), item.Settings...), UpdatedAtMS: item.UpdatedAtMS,
	})
}

func (uncoordinatedSettingsStore) Delete(ctx context.Context, namespace string) error {
	return deleteProSetting(ctx, namespace)
}

func (SettingsStore) ExecuteWrite(
	ctx context.Context,
	operation func(context.Context, settings.Store) error,
) error {
	if operation == nil {
		return nil
	}
	return probackup.Default.ExecuteWrite(ctx, func(ctx context.Context) error {
		return operation(ctx, uncoordinatedSettingsStore{})
	})
}

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

func (SettingsStore) Delete(ctx context.Context, namespace string) error {
	return DeleteProSetting(ctx, namespace)
}

func (SettingsStore) GetPlanSnapshot(ctx context.Context, provider, fileName, authIndex string) ([]byte, int64, bool, error) {
	provider = strings.TrimSpace(provider)
	fileName = strings.TrimSpace(fileName)
	authIndex = strings.TrimSpace(authIndex)
	if fileName == "" {
		fileName = authIndex
	}
	entries, err := GetQuotaCache(ctx, provider, fileName)
	if err != nil {
		return nil, 0, false, err
	}
	for _, entry := range entries {
		if entry.AuthIndex != "" && authIndex != "" && entry.AuthIndex != authIndex {
			continue
		}
		return append([]byte(nil), entry.Data...), entry.ObservedAt, true, nil
	}
	return nil, 0, false, nil
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
