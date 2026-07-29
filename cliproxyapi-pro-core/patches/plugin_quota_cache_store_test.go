package pluginhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type testQuotaCacheStore struct {
	entry    pluginapi.QuotaCacheEntry
	observed pluginapi.QuotaObservation
}

func (s *testQuotaCacheStore) GetQuotaCache(context.Context, pluginapi.QuotaCacheGetRequest) (pluginapi.QuotaCacheGetResponse, error) {
	return pluginapi.QuotaCacheGetResponse{ContractVersion: pluginapi.QuotaCacheContractVersion, Entries: []pluginapi.QuotaCacheEntry{s.entry}}, nil
}

func (s *testQuotaCacheStore) PutQuotaCache(_ context.Context, req pluginapi.QuotaCachePutRequest) (pluginapi.QuotaCachePutResponse, error) {
	s.entry = req.Entry
	return pluginapi.QuotaCachePutResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, nil
}

func (s *testQuotaCacheStore) DeleteQuotaCache(context.Context, pluginapi.QuotaCacheDeleteRequest) (pluginapi.QuotaCacheDeleteResponse, error) {
	s.entry = pluginapi.QuotaCacheEntry{}
	return pluginapi.QuotaCacheDeleteResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, nil
}

func (s *testQuotaCacheStore) ObserveQuota(_ context.Context, req pluginapi.QuotaObservationRequest) (pluginapi.QuotaObservationResponse, error) {
	s.observed = req.Observation
	return pluginapi.QuotaObservationResponse{ContractVersion: pluginapi.QuotaCacheContractVersion}, nil
}

func TestQuotaCacheStoreFacadeUsesActivePlugin(t *testing.T) {
	store := &testQuotaCacheStore{}
	host := New()
	setHostSnapshotForTest(host, true, capabilityRecord{
		id: "observability", priority: 10,
		plugin: pluginapi.Plugin{Metadata: pluginapi.Metadata{Name: "observability", Version: "1", Author: "test", GitHubRepository: "https://example.com"}, Capabilities: pluginapi.Capabilities{QuotaCacheStore: store}},
	})
	entry := pluginapi.QuotaCacheEntry{Provider: "codex", FileName: "a.json", Data: json.RawMessage(`{"ok":true}`)}
	if _, handled, err := host.PutQuotaCache(context.Background(), pluginapi.QuotaCachePutRequest{Entry: entry}); err != nil || !handled {
		t.Fatalf("PutQuotaCache() handled=%v err=%v", handled, err)
	}
	got, handled, err := host.GetQuotaCache(context.Background(), pluginapi.QuotaCacheGetRequest{Provider: "codex"})
	if err != nil || !handled || len(got.Entries) != 1 || got.Entries[0].FileName != "a.json" {
		t.Fatalf("GetQuotaCache() = %#v handled=%v err=%v", got, handled, err)
	}
	if _, handled, err = host.ObserveQuota(context.Background(), pluginapi.QuotaObservationRequest{Observation: pluginapi.QuotaObservation{Provider: "xai", FileName: "x.json"}}); err != nil || !handled || store.observed.Provider != "xai" {
		t.Fatalf("ObserveQuota() handled=%v observed=%#v err=%v", handled, store.observed, err)
	}
}

type quotaCacheRPCClient struct {
	methods []string
}

func (c *quotaCacheRPCClient) Call(_ context.Context, method string, _ []byte) ([]byte, error) {
	c.methods = append(c.methods, method)
	switch method {
	case pluginabi.MethodQuotaCacheGet:
		return marshalRPCResult(pluginapi.QuotaCacheGetResponse{ContractVersion: pluginapi.QuotaCacheContractVersion, Entries: []pluginapi.QuotaCacheEntry{{Provider: "xai"}}})
	case pluginabi.MethodQuotaCachePut:
		return marshalRPCResult(pluginapi.QuotaCachePutResponse{ContractVersion: pluginapi.QuotaCacheContractVersion})
	case pluginabi.MethodQuotaCacheDelete:
		return marshalRPCResult(pluginapi.QuotaCacheDeleteResponse{ContractVersion: pluginapi.QuotaCacheContractVersion})
	case pluginabi.MethodQuotaCacheObserve:
		return marshalRPCResult(pluginapi.QuotaObservationResponse{ContractVersion: pluginapi.QuotaCacheContractVersion})
	default:
		return nil, nil
	}
}

func (c *quotaCacheRPCClient) Shutdown() {}

func TestRPCQuotaCacheStoreUsesDedicatedMethods(t *testing.T) {
	client := &quotaCacheRPCClient{}
	store := rpcQuotaCacheStore{rpcPluginAdapter: &rpcPluginAdapter{client: client}}
	if got, err := store.GetQuotaCache(context.Background(), pluginapi.QuotaCacheGetRequest{}); err != nil || len(got.Entries) != 1 {
		t.Fatalf("GetQuotaCache() = %#v, %v", got, err)
	}
	if _, err := store.PutQuotaCache(context.Background(), pluginapi.QuotaCachePutRequest{}); err != nil {
		t.Fatalf("PutQuotaCache() error = %v", err)
	}
	if _, err := store.DeleteQuotaCache(context.Background(), pluginapi.QuotaCacheDeleteRequest{}); err != nil {
		t.Fatalf("DeleteQuotaCache() error = %v", err)
	}
	if _, err := store.ObserveQuota(context.Background(), pluginapi.QuotaObservationRequest{}); err != nil {
		t.Fatalf("ObserveQuota() error = %v", err)
	}
	want := []string{pluginabi.MethodQuotaCacheGet, pluginabi.MethodQuotaCachePut, pluginabi.MethodQuotaCacheDelete, pluginabi.MethodQuotaCacheObserve}
	if stringMethods := strings.Join(client.methods, ","); stringMethods != strings.Join(want, ",") {
		t.Fatalf("methods = %v, want %v", client.methods, want)
	}
}
