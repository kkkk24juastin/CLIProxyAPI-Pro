package embeddedusage

import (
	"context"
	"fmt"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
)

type ProSettingsPluginBackend interface {
	GetProSetting(context.Context, pluginapi.ProSettingGetRequest) (pluginapi.ProSettingGetResponse, bool, error)
	PutProSetting(context.Context, pluginapi.ProSettingPutRequest) (pluginapi.ProSettingPutResponse, bool, error)
}

var proSettingsBackendState struct {
	sync.RWMutex
	plugin ProSettingsPluginBackend
}

func SetProSettingsPluginBackend(backend ProSettingsPluginBackend) {
	proSettingsBackendState.Lock()
	proSettingsBackendState.plugin = backend
	proSettingsBackendState.Unlock()
}

func proSettingsPluginBackend() ProSettingsPluginBackend {
	proSettingsBackendState.RLock()
	defer proSettingsBackendState.RUnlock()
	return proSettingsBackendState.plugin
}

func getPluginProSetting(ctx context.Context, namespace string) (ProSetting, bool, error) {
	backend := proSettingsPluginBackend()
	if backend == nil {
		return ProSetting{}, false, fmt.Errorf("plugin pro settings backend is not available")
	}
	resp, handled, err := backend.GetProSetting(ctx, pluginapi.ProSettingGetRequest{Namespace: namespace})
	if err != nil {
		return ProSetting{}, false, err
	}
	if !handled {
		return ProSetting{}, false, fmt.Errorf("plugin pro settings backend is not available")
	}
	return proSettingFromPlugin(resp.Setting), resp.Found, nil
}

func putPluginProSetting(ctx context.Context, item ProSetting) error {
	backend := proSettingsPluginBackend()
	if backend == nil {
		return fmt.Errorf("plugin pro settings backend is not available")
	}
	_, handled, err := backend.PutProSetting(ctx, pluginapi.ProSettingPutRequest{Setting: proSettingToPlugin(item)})
	if err != nil {
		return err
	}
	if !handled {
		return fmt.Errorf("plugin pro settings backend is not available")
	}
	return nil
}

func proSettingToPlugin(item ProSetting) pluginapi.ProSetting {
	return pluginapi.ProSetting{Namespace: item.Namespace, SchemaVersion: item.SchemaVersion, Settings: item.Settings, UpdatedAtMS: item.UpdatedAtMS}
}

func proSettingFromPlugin(item pluginapi.ProSetting) ProSetting {
	return ProSetting{Namespace: item.Namespace, SchemaVersion: item.SchemaVersion, Settings: item.Settings, UpdatedAtMS: item.UpdatedAtMS}
}
