package pluginhost

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (h *Host) activeProSettingsStore() (capabilityRecord, pluginapi.ProSettingsStore, bool) {
	if h == nil {
		return capabilityRecord{}, nil, false
	}
	for _, record := range h.activeRecords() {
		store := record.plugin.Capabilities.ProSettingsStore
		if store != nil && !h.isPluginFused(record.id) {
			return record, store, true
		}
	}
	return capabilityRecord{}, nil, false
}

func (h *Host) GetProSetting(ctx context.Context, req pluginapi.ProSettingGetRequest) (resp pluginapi.ProSettingGetResponse, handled bool, err error) {
	record, store, ok := h.activeProSettingsStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "ProSettingsStore.GetProSetting", recovered)
			resp = pluginapi.ProSettingGetResponse{}
			err = fmt.Errorf("pro setting get panic: %v", recovered)
		}
	}()
	resp, err = store.GetProSetting(ctx, req)
	return resp, handled, err
}

func (h *Host) PutProSetting(ctx context.Context, req pluginapi.ProSettingPutRequest) (resp pluginapi.ProSettingPutResponse, handled bool, err error) {
	record, store, ok := h.activeProSettingsStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "ProSettingsStore.PutProSetting", recovered)
			resp = pluginapi.ProSettingPutResponse{}
			err = fmt.Errorf("pro setting put panic: %v", recovered)
		}
	}()
	resp, err = store.PutProSetting(ctx, req)
	return resp, handled, err
}
