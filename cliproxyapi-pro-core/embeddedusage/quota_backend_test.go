package embeddedusage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/pluginapi"
)

type quotaCacheBackendStub struct {
	entry       pluginapi.QuotaCacheEntry
	putMerge    bool
	observation pluginapi.QuotaObservation
}

func (s *quotaCacheBackendStub) GetQuotaCache(context.Context, pluginapi.QuotaCacheGetRequest) (pluginapi.QuotaCacheGetResponse, bool, error) {
	return pluginapi.QuotaCacheGetResponse{ContractVersion: pluginapi.QuotaCacheContractVersion, Entries: []pluginapi.QuotaCacheEntry{s.entry}}, true, nil
}

func (s *quotaCacheBackendStub) PutQuotaCache(_ context.Context, req pluginapi.QuotaCachePutRequest) (pluginapi.QuotaCachePutResponse, bool, error) {
	s.entry, s.putMerge = req.Entry, req.Merge
	return pluginapi.QuotaCachePutResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, true, nil
}

func (s *quotaCacheBackendStub) DeleteQuotaCache(context.Context, pluginapi.QuotaCacheDeleteRequest) (pluginapi.QuotaCacheDeleteResponse, bool, error) {
	s.entry = pluginapi.QuotaCacheEntry{}
	return pluginapi.QuotaCacheDeleteResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, true, nil
}

func (s *quotaCacheBackendStub) ObserveQuota(_ context.Context, req pluginapi.QuotaObservationRequest) (pluginapi.QuotaObservationResponse, bool, error) {
	s.observation = req.Observation
	return pluginapi.QuotaObservationResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, true, nil
}

func TestPluginQuotaCacheBackendOwnsRuntimeWithoutLegacyService(t *testing.T) {
	SetDefaultService(nil)
	stub := &quotaCacheBackendStub{}
	SetQuotaCachePluginBackend(stub)
	t.Cleanup(func() { SetQuotaCachePluginBackend(nil) })
	if mode := QuotaCacheBackendMode(); mode != QuotaCacheBackendPlugin {
		t.Fatalf("QuotaCacheBackendMode() = %q, want plugin", mode)
	}

	entry := QuotaCacheEntry{Provider: "xai", FileName: "x.json", Data: json.RawMessage(`{"billing":{"plan":"free"}}`)}
	if err := MergeXAIQuotaCache(context.Background(), entry); err != nil {
		t.Fatalf("MergeXAIQuotaCache() error = %v", err)
	}
	if !stub.putMerge || stub.entry.FileName != "x.json" {
		t.Fatalf("plugin put = %#v merge=%v", stub.entry, stub.putMerge)
	}
	entries, err := GetQuotaCache(context.Background(), "xai", "x.json")
	if err != nil || len(entries) != 1 || entries[0].Provider != "xai" {
		t.Fatalf("GetQuotaCache() = %#v, %v", entries, err)
	}
	if err = ObserveXAIQuotaResponse(context.Background(), XAIQuotaObservation{FileName: "x.json", Model: "grok"}); err != nil {
		t.Fatalf("ObserveXAIQuotaResponse() error = %v", err)
	}
	if stub.observation.Provider != "xai" || stub.observation.FileName != "x.json" {
		t.Fatalf("observation = %#v", stub.observation)
	}
	if err = DeleteQuotaCache(context.Background(), "xai", "x.json"); err != nil || stub.entry.Provider != "" {
		t.Fatalf("DeleteQuotaCache() entry=%#v err=%v", stub.entry, err)
	}
}

func TestPluginQuotaCacheBackendFailsClosedWhenCapabilityMissing(t *testing.T) {
	SetDefaultService(nil)
	SetQuotaCachePluginBackend(nil)
	if err := SetQuotaCache(context.Background(), QuotaCacheEntry{Provider: "codex", FileName: "a.json", Data: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("SetQuotaCache() error = nil, want missing plugin capability")
	}
}
