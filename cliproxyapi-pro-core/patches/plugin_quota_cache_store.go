package pluginhost

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (h *Host) activeQuotaCacheStore() (capabilityRecord, pluginapi.QuotaCacheStore, bool) {
	if h == nil {
		return capabilityRecord{}, nil, false
	}
	for _, record := range h.activeRecords() {
		store := record.plugin.Capabilities.QuotaCacheStore
		if store != nil && !h.isPluginFused(record.id) {
			return record, store, true
		}
	}
	return capabilityRecord{}, nil, false
}

func (h *Host) GetQuotaCache(ctx context.Context, req pluginapi.QuotaCacheGetRequest) (resp pluginapi.QuotaCacheGetResponse, handled bool, err error) {
	record, store, ok := h.activeQuotaCacheStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "QuotaCacheStore.GetQuotaCache", recovered)
			resp = pluginapi.QuotaCacheGetResponse{}
			err = fmt.Errorf("quota cache get panic: %v", recovered)
		}
	}()
	resp, err = store.GetQuotaCache(ctx, req)
	return resp, handled, err
}

func (h *Host) PutQuotaCache(ctx context.Context, req pluginapi.QuotaCachePutRequest) (resp pluginapi.QuotaCachePutResponse, handled bool, err error) {
	record, store, ok := h.activeQuotaCacheStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "QuotaCacheStore.PutQuotaCache", recovered)
			resp = pluginapi.QuotaCachePutResponse{}
			err = fmt.Errorf("quota cache put panic: %v", recovered)
		}
	}()
	resp, err = store.PutQuotaCache(ctx, req)
	return resp, handled, err
}

func (h *Host) DeleteQuotaCache(ctx context.Context, req pluginapi.QuotaCacheDeleteRequest) (resp pluginapi.QuotaCacheDeleteResponse, handled bool, err error) {
	record, store, ok := h.activeQuotaCacheStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "QuotaCacheStore.DeleteQuotaCache", recovered)
			resp = pluginapi.QuotaCacheDeleteResponse{}
			err = fmt.Errorf("quota cache delete panic: %v", recovered)
		}
	}()
	resp, err = store.DeleteQuotaCache(ctx, req)
	return resp, handled, err
}

func (h *Host) ObserveQuota(ctx context.Context, req pluginapi.QuotaObservationRequest) (resp pluginapi.QuotaObservationResponse, handled bool, err error) {
	record, store, ok := h.activeQuotaCacheStore()
	if !ok {
		return resp, false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "QuotaCacheStore.ObserveQuota", recovered)
			resp = pluginapi.QuotaObservationResponse{}
			err = fmt.Errorf("quota observation panic: %v", recovered)
		}
	}()
	resp, err = store.ObserveQuota(ctx, req)
	return resp, handled, err
}
