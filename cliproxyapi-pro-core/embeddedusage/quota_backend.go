package embeddedusage

import (
	"context"
	"fmt"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
)

const QuotaCacheBackendPlugin = "plugin"

// QuotaCachePluginBackend is implemented by pluginhost.Host without coupling the
// embedded compatibility facade to a concrete plugin ID.
type QuotaCachePluginBackend interface {
	GetQuotaCache(context.Context, pluginapi.QuotaCacheGetRequest) (pluginapi.QuotaCacheGetResponse, bool, error)
	PutQuotaCache(context.Context, pluginapi.QuotaCachePutRequest) (pluginapi.QuotaCachePutResponse, bool, error)
	DeleteQuotaCache(context.Context, pluginapi.QuotaCacheDeleteRequest) (pluginapi.QuotaCacheDeleteResponse, bool, error)
	ObserveQuota(context.Context, pluginapi.QuotaObservationRequest) (pluginapi.QuotaObservationResponse, bool, error)
}

var quotaCacheBackendState struct {
	sync.RWMutex
	plugin QuotaCachePluginBackend
}

func SetQuotaCachePluginBackend(backend QuotaCachePluginBackend) {
	quotaCacheBackendState.Lock()
	quotaCacheBackendState.plugin = backend
	quotaCacheBackendState.Unlock()
}

func QuotaCacheBackendMode() string {
	return QuotaCacheBackendPlugin
}

func quotaCachePluginBackend() QuotaCachePluginBackend {
	quotaCacheBackendState.RLock()
	defer quotaCacheBackendState.RUnlock()
	return quotaCacheBackendState.plugin
}

func quotaCachePluginUnavailable() error {
	return fmt.Errorf("plugin quota cache backend is not available")
}

func SetQuotaCache(ctx context.Context, entry QuotaCacheEntry) error {
	if legacyCompatibilityServiceAvailable() {
		return setLegacyQuotaCache(ctx, entry)
	}
	return putPluginQuotaCache(ctx, entry, false)
}

func MergeXAIQuotaCache(ctx context.Context, entry QuotaCacheEntry) error {
	if legacyCompatibilityServiceAvailable() {
		return mergeLegacyXAIQuotaCache(ctx, entry)
	}
	return putPluginQuotaCache(ctx, entry, true)
}

func GetQuotaCache(ctx context.Context, provider, fileName string) ([]QuotaCacheEntry, error) {
	if legacyCompatibilityServiceAvailable() {
		return getLegacyQuotaCache(ctx, provider, fileName)
	}
	backend := quotaCachePluginBackend()
	if backend == nil {
		return nil, quotaCachePluginUnavailable()
	}
	resp, handled, err := backend.GetQuotaCache(ctx, pluginapi.QuotaCacheGetRequest{
		ContractVersion: pluginapi.QuotaCacheContractVersion,
		Provider:        provider, FileName: fileName,
	})
	if err != nil {
		return nil, err
	}
	if !handled {
		return nil, quotaCachePluginUnavailable()
	}
	entries := make([]QuotaCacheEntry, 0, len(resp.Entries))
	for _, entry := range resp.Entries {
		entries = append(entries, quotaCacheEntryFromPlugin(entry))
	}
	return entries, nil
}

func DeleteQuotaCache(ctx context.Context, provider, fileName string) error {
	if legacyCompatibilityServiceAvailable() {
		return deleteLegacyQuotaCache(ctx, provider, fileName)
	}
	backend := quotaCachePluginBackend()
	pluginErr := quotaCachePluginUnavailable()
	if backend != nil {
		_, handled, err := backend.DeleteQuotaCache(ctx, pluginapi.QuotaCacheDeleteRequest{
			ContractVersion: pluginapi.QuotaCacheContractVersion,
			Provider:        provider, FileName: fileName,
		})
		pluginErr = err
		if err == nil && !handled {
			pluginErr = quotaCachePluginUnavailable()
		}
	}
	return pluginErr
}

func ObserveXAIQuotaResponse(ctx context.Context, observation XAIQuotaObservation) error {
	if legacyCompatibilityServiceAvailable() {
		return observeLegacyXAIQuotaResponse(ctx, observation)
	}
	return observePluginQuota(ctx, observation)
}

func putPluginQuotaCache(ctx context.Context, entry QuotaCacheEntry, merge bool) error {
	backend := quotaCachePluginBackend()
	if backend == nil {
		return quotaCachePluginUnavailable()
	}
	_, handled, err := backend.PutQuotaCache(ctx, pluginapi.QuotaCachePutRequest{
		ContractVersion: pluginapi.QuotaCacheContractVersion,
		Entry:           quotaCacheEntryToPlugin(entry), Merge: merge,
	})
	if err != nil {
		return err
	}
	if !handled {
		return quotaCachePluginUnavailable()
	}
	return nil
}

func observePluginQuota(ctx context.Context, observation XAIQuotaObservation) error {
	backend := quotaCachePluginBackend()
	if backend == nil {
		return quotaCachePluginUnavailable()
	}
	_, handled, err := backend.ObserveQuota(ctx, pluginapi.QuotaObservationRequest{
		ContractVersion: pluginapi.QuotaCacheContractVersion,
		Observation: pluginapi.QuotaObservation{
			Provider: "xai", FileName: observation.FileName, AuthIndex: observation.AuthIndex,
			Email: observation.Email, Label: observation.Label, Model: observation.Model,
			Status: observation.Status, Headers: observation.Header, Body: observation.Body,
			ObservedAt: observation.ObservedAt,
		},
	})
	if err != nil {
		return err
	}
	if !handled {
		return quotaCachePluginUnavailable()
	}
	return nil
}

func setLegacyQuotaCache(ctx context.Context, entry QuotaCacheEntry) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return globalService.store.SetQuotaCache(ctx, entry)
}

func getLegacyQuotaCache(ctx context.Context, provider, fileName string) ([]QuotaCacheEntry, error) {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return nil, fmt.Errorf("usage service is not available")
	}
	return globalService.store.GetQuotaCache(ctx, provider, fileName)
}

func deleteLegacyQuotaCache(ctx context.Context, provider, fileName string) error {
	globalStateMu.RLock()
	defer globalStateMu.RUnlock()
	if globalService == nil || globalService.store == nil {
		return fmt.Errorf("usage service is not available")
	}
	return globalService.store.DeleteQuotaCache(ctx, provider, fileName)
}

func quotaCacheEntryToPlugin(entry QuotaCacheEntry) pluginapi.QuotaCacheEntry {
	return pluginapi.QuotaCacheEntry{
		ID: entry.ID, Provider: entry.Provider, FileName: entry.FileName, AuthIndex: entry.AuthIndex,
		IdentityFingerprint: entry.IdentityFingerprint, Data: entry.Data, CachedAt: entry.CachedAt,
		AccessedAt: entry.AccessedAt, ObservedAt: entry.ObservedAt, StoredAt: entry.StoredAt,
		Version: entry.Version, Revision: entry.Revision,
	}
}

func quotaCacheEntryFromPlugin(entry pluginapi.QuotaCacheEntry) QuotaCacheEntry {
	return QuotaCacheEntry{
		ID: entry.ID, Provider: entry.Provider, FileName: entry.FileName, AuthIndex: entry.AuthIndex,
		IdentityFingerprint: entry.IdentityFingerprint, Data: entry.Data, CachedAt: entry.CachedAt,
		AccessedAt: entry.AccessedAt, ObservedAt: entry.ObservedAt, StoredAt: entry.StoredAt,
		Version: entry.Version, Revision: entry.Revision,
	}
}
