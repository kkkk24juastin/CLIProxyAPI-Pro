package pluginhost

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (h *Host) activeRuntimeStateStore() (capabilityRecord, pluginapi.RuntimeStateStore, bool) {
	if h == nil {
		return capabilityRecord{}, nil, false
	}
	for _, record := range h.activeRecords() {
		store := record.plugin.Capabilities.RuntimeStateStore
		if store != nil && !h.isPluginFused(record.id) {
			return record, store, true
		}
	}
	return capabilityRecord{}, nil, false
}

func (h *Host) GetAuthRuntimeStats(ctx context.Context, req pluginapi.AuthRuntimeStatsGetRequest) (resp pluginapi.AuthRuntimeStatsGetResponse, handled bool, err error) {
	record, store, ok := h.activeRuntimeStateStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "RuntimeStateStore.GetAuthRuntimeStats", recovered)
			resp = pluginapi.AuthRuntimeStatsGetResponse{}
			err = fmt.Errorf("auth runtime stats get panic: %v", recovered)
		}
	}()
	resp, err = store.GetAuthRuntimeStats(ctx, req)
	return resp, handled, err
}

func (h *Host) PutAuthRuntimeStats(ctx context.Context, req pluginapi.AuthRuntimeStatsPutRequest) (resp pluginapi.AuthRuntimeStatsPutResponse, handled bool, err error) {
	record, store, ok := h.activeRuntimeStateStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "RuntimeStateStore.PutAuthRuntimeStats", recovered)
			resp = pluginapi.AuthRuntimeStatsPutResponse{}
			err = fmt.Errorf("auth runtime stats put panic: %v", recovered)
		}
	}()
	resp, err = store.PutAuthRuntimeStats(ctx, req)
	return resp, handled, err
}

func (h *Host) DeleteAuthRuntimeState(ctx context.Context, req pluginapi.AuthRuntimeStateDeleteRequest) (resp pluginapi.AuthRuntimeStateDeleteResponse, handled bool, err error) {
	record, store, ok := h.activeRuntimeStateStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "RuntimeStateStore.DeleteAuthRuntimeState", recovered)
			resp = pluginapi.AuthRuntimeStateDeleteResponse{}
			err = fmt.Errorf("auth runtime state delete panic: %v", recovered)
		}
	}()
	resp, err = store.DeleteAuthRuntimeState(ctx, req)
	return resp, handled, err
}
